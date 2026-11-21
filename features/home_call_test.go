//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_call"] = func(sc *godog.ScenarioContext, _ fixtures) { homeCallSteps(sc) }
}

type homeCallWorld struct {
	now      time.Time
	state    *home.State
	disk     homeCallDisk
	receipts homeCallReceipts
	members  map[string]string
	keys     map[string]identity.Key
	taskID   string
	callID   string
	callTS   int64
	replyID  string
	lastBy   identity.Key
	lastEnv  proto.Envelope
	answers  []home.Result
	items    []home.Item
	nonce    int
}

type homeCallDisk map[string]proto.MsgRow

func (d homeCallDisk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d[id]; return m, ok }
func (d homeCallDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d homeCallDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d homeCallDisk) Pending() []proto.MsgRow                   { return nil }
func (d homeCallDisk) Queued() []store.Item                      { return nil }

type homeCallReceipts map[string]store.Outcome

func (r homeCallReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	held, ok := r[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return held, nil
}

func (r homeCallReceipts) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	r[signer+" "+nonce] = result
	return nil
}

func homeCallSteps(sc *godog.ScenarioContext) {
	w := &homeCallWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^a group home for calls with an active task and three AIs$`, w.start)
	sc.Step(`^"([^"]*)" calls "([^"]*)" in the task thread at depth (\d+) asking "([^"]*)"$`, w.callInThread)
	sc.Step(`^"([^"]*)" has called "([^"]*)" in the main line$`, w.calledInMain)
	sc.Step(`^"([^"]*)" sends the same (?:call|reply) command again$`, w.repeatLast)
	sc.Step(`^"([^"]*)" replies to the call (\d+) ms later with "([^"]*)"$`, w.replyAfter)
	sc.Step(`^(the owner posts a call to "ai-two"|"ai-one" posts a call to "ai-one"|"ai-one" posts a call to "owner"|"ai-three" replies to the main-line call|"ai-two" replies to the call in the thread)$`, w.invalidAttempt)
	sc.Step(`^the call home answers ok$`, func() error { return w.answered(1, "") })
	sc.Step(`^the call home answers ok to both$`, func() error { return w.answered(2, "") })
	sc.Step(`^the call home answers "([^"]*)"$`, func(code string) error { return w.answered(1, code) })
	sc.Step(`^the call is "waiting" in the task thread and queued "([^"]*)"$`, w.waitingAndQueued)
	sc.Step(`^the call is "answered" after (\d+) ms with the reply in the task thread$`, w.answeredAfter)
	sc.Step(`^the call home has queued "([^"]*)" for the reply$`, w.queuedForReply)
	sc.Step(`^the call home has queued nothing since the last check$`, func() error { return w.queuedExact(nil) })
}

func (w *homeCallWorld) start() error {
	browser, node, laptop := exampleKey("call browser"), exampleKey("call node"), exampleKey("call laptop")
	w.keys = map[string]identity.Key{"owner": browser, "laptop": laptop}
	w.members = map[string]string{}
	for _, name := range []string{"owner", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("call " + name))
	}
	grp := string(exampleID("call group"))
	w.state = home.NewState(grp, node.ID(), browser.ID())
	log, err := homeLog([]identity.Key{browser, node, laptop})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: grp}, Home: node.ID(), Secretary: w.members["secretary"],
		Forge: "none", Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3, ReviewRounds: 5, AskDepth: 4, AskWaitS: 45},
	}
	owner := w.members["owner"]
	w.state.People[owner] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: grp, Kind: "human", User: browser.ID(), Name: "owner"},
		Role:      "owner",
	}
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		mid := w.members[name]
		w.state.AIs[mid] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: mid}, Group: grp, Kind: "ai", User: browser.ID(), Name: name},
			Host:      laptop.ID(),
		}
	}
	w.taskID = string(exampleID("call task"))
	w.state.Tasks[w.taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID, Seq: 6}, Group: grp, Num: 1,
		Title: "Report path changes", Slug: proto.MakeSlug("Report path changes"), Origin: "brief",
		State: "active", Step: "local_check", Assignee: w.members["ai-one"], Reviewer: w.members["ai-two"],
		Pass: 90, Depends: []string{}, Created: w.now.UnixMilli() - 60_000, Stats: proto.TaskStats{Started: w.now.UnixMilli() - 60_000},
	}
	w.state.Seq, w.state.Num = 6, 1
	w.disk, w.receipts, w.answers, w.items = homeCallDisk{}, homeCallReceipts{}, nil, nil
	return nil
}

func (w *homeCallWorld) nextID() proto.ID {
	w.nonce++
	return exampleID("call nonce " + strconv.Itoa(w.nonce))
}

func (w *homeCallWorld) callInThread(from, to string, depth int, text string) error {
	thread := proto.ID(w.taskID)
	return w.postCall(w.keys["laptop"], w.members[from], w.members[to], &thread, depth, text)
}

func (w *homeCallWorld) calledInMain(from, to string) error {
	if err := w.postCall(w.keys["laptop"], w.members[from], w.members[to], nil, 1, "Main question?"); err != nil {
		return err
	}
	w.items = nil
	return nil
}

func (w *homeCallWorld) postCall(by identity.Key, as, to string, thread *proto.ID, depth int, text string) error {
	id := w.nextID()
	ts := w.now.UnixMilli() - 50_000
	post := proto.Post[proto.CallBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: id, Timestamp: ts, Group: proto.ID(w.state.ID), As: proto.ID(as)},
		Kind:        "call", Thread: thread, Text: text, Mentions: []proto.ID{},
		Body: proto.CallBody{To: to, Depth: depth, Wait: true},
	}
	raw, err := json.Marshal(post)
	if err != nil {
		return err
	}
	if err := w.send(by, by.Sign(string(raw))); err != nil {
		return err
	}
	if w.answers[len(w.answers)-1].Answer.OK {
		w.callID, w.callTS = string(id), ts
	}
	return nil
}

func (w *homeCallWorld) replyAfter(as string, deltaMS int64, text string) error {
	thread := proto.ID(w.taskID)
	return w.postReply(w.keys["laptop"], w.members[as], &thread, w.callTS+deltaMS, text)
}

func (w *homeCallWorld) postReply(by identity.Key, as string, thread *proto.ID, ts int64, text string) error {
	id := w.nextID()
	re := proto.ID(w.callID)
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: id, Timestamp: ts, Group: proto.ID(w.state.ID), As: proto.ID(as)},
		Kind:        "reply", Thread: thread, Text: text, Mentions: []proto.ID{}, Re: &re, Body: proto.EmptyBody{},
	}
	raw, err := json.Marshal(post)
	if err != nil {
		return err
	}
	if err := w.send(by, by.Sign(string(raw))); err != nil {
		return err
	}
	if w.answers[len(w.answers)-1].Answer.OK {
		w.replyID = string(id)
	}
	return nil
}

func (w *homeCallWorld) repeatLast(_ string) error {
	return w.send(w.lastBy, w.lastEnv)
}

func (w *homeCallWorld) invalidAttempt(attempt string) error {
	thread := proto.ID(w.taskID)
	switch attempt {
	case `the owner posts a call to "ai-two"`:
		return w.postCall(w.keys["owner"], "", w.members["ai-two"], nil, 1, "Ask?")
	case `"ai-one" posts a call to "ai-one"`:
		return w.postCall(w.keys["laptop"], w.members["ai-one"], w.members["ai-one"], nil, 1, "Self?")
	case `"ai-one" posts a call to "owner"`:
		return w.postCall(w.keys["laptop"], w.members["ai-one"], w.members["owner"], nil, 1, "Human?")
	case `"ai-three" replies to the main-line call`:
		return w.postReply(w.keys["laptop"], w.members["ai-three"], nil, w.callTS+500, "Wrong AI.")
	default:
		return w.postReply(w.keys["laptop"], w.members["ai-two"], &thread, w.callTS+500, "Wrong place.")
	}
}

func (w *homeCallWorld) send(by identity.Key, env proto.Envelope) error {
	res, err := home.Commands.Apply(w.state, w.disk, w.receipts, by.ID(), env, w.now)
	if err != nil {
		return err
	}
	for _, row := range res.Rows {
		if msg, ok := row.Value().(proto.MsgRow); ok {
			w.disk[msg.ID] = msg
		}
	}
	w.state.Commit(res)
	w.lastBy, w.lastEnv = by, env
	w.answers = append(w.answers, res)
	w.items = append(w.items, res.Items...)
	return nil
}

func (w *homeCallWorld) answered(n int, code string) error {
	if len(w.answers) < n {
		return fmt.Errorf("have %d answers, want at least %d", len(w.answers), n)
	}
	for _, res := range w.answers[len(w.answers)-n:] {
		if res.Answer.Code != code || res.Answer.OK != (code == "") {
			return fmt.Errorf("answered %+v (%s), want code %q", res.Answer, res.Msg, code)
		}
	}
	return nil
}

func (w *homeCallWorld) waitingAndQueued(wantItem string) error {
	call := w.disk[w.callID]
	wantState := &proto.CardState{Status: "waiting"}
	if !reflect.DeepEqual(call.State, wantState) || !reflect.DeepEqual(call.Thread, &w.taskID) {
		return fmt.Errorf("unexpected call row: %+v", call)
	}
	if err := w.lastThreadKind("call"); err != nil {
		return err
	}
	return w.queuedExact([]string{wantItem + " " + w.callID})
}

func (w *homeCallWorld) answeredAfter(wantMS int64) error {
	call := w.disk[w.callID]
	wantState := &proto.CardState{Status: "answered", Reply: &w.replyID, MS: &wantMS}
	if !reflect.DeepEqual(call.State, wantState) {
		return fmt.Errorf("unexpected call state: %+v", call.State)
	}
	reply := w.disk[w.replyID]
	if reply.Kind != "reply" || reply.State != nil || !reflect.DeepEqual(reply.Thread, &w.taskID) {
		return fmt.Errorf("unexpected reply row: %+v", reply)
	}
	return w.lastThreadKind("reply")
}

func (w *homeCallWorld) lastThreadKind(kind string) error {
	last := w.state.Tasks[w.taskID].Thread.Last
	if last == nil || last.Kind != kind {
		return fmt.Errorf("task thread last is %+v, want kind %s", last, kind)
	}
	return nil
}

func (w *homeCallWorld) queuedForReply(want string) error {
	return w.queuedExact([]string{want + " " + w.replyID})
}

func (w *homeCallWorld) queuedExact(want []string) error {
	var got []string
	for _, it := range w.items {
		got = append(got, fmt.Sprintf("%s %s %s", it.Why, w.memberName(it.AI), it.Msg))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %q, want %q", got, want)
	}
	w.items = nil
	return nil
}

func (w *homeCallWorld) memberName(id string) string {
	for name, mid := range w.members {
		if mid == id {
			return name
		}
	}
	return id
}
