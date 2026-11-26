//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// homeTaskNoMerge is what the steps that MODEL-1126 took away say when they fail.
const homeTaskNoMerge = "a task has no branch and no step merge: the scenarios on its own branch and at step merge go (MODEL-1126 plan 3.2)"

func init() {
	stepSets["home_task"] = func(sc *godog.ScenarioContext, _ fixtures) { homeTaskSteps(sc) }
}

type homeTaskStore struct {
	msgs     map[string]proto.MsgRow
	pending  []proto.MsgRow
	queued   []store.Item
	outcomes map[string]store.Outcome
}

func (d *homeTaskStore) Msg(id string) (proto.MsgRow, bool) {
	m, ok := d.msgs[id]
	return m, ok
}

func (d *homeTaskStore) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d *homeTaskStore) TypedNotes(string) []proto.NoteRow         { return nil }
func (d *homeTaskStore) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d *homeTaskStore) Pending() []proto.MsgRow                   { return d.pending }
func (d *homeTaskStore) Queued() []store.Item                      { return d.queued }
func (d *homeTaskStore) OtherMember(string) bool                   { return false }
func (d *homeTaskStore) DeviceLog(string) []proto.Envelope         { return nil }

func (d *homeTaskStore) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := d.outcomes[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (d *homeTaskStore) PutReceipt(signer, nonce string, _ int64, out store.Outcome) error {
	d.outcomes[signer+" "+nonce] = out
	return nil
}

type homeTaskWorld struct {
	now     time.Time
	state   *home.State
	store   *homeTaskStore
	members map[string]string
	keys    map[string]identity.Key
	request string
	brief   string
	lastEnv proto.Envelope
	lastBy  identity.Key
	results []home.Result
	items   []home.Item
	retired []int64
	seq     int
}

func homeTaskSteps(sc *godog.ScenarioContext) {
	w := &homeTaskWorld{now: time.UnixMilli(1763600000000)}
	homeTaskGivenSteps(sc, w)
	homeTaskActionSteps(sc, w)
	homeTaskCheckSteps(sc, w)
}

func homeTaskGivenSteps(sc *godog.ScenarioContext, w *homeTaskWorld) {
	sc.Step(`^a group home for task commands$`, w.setup)
	sc.Step(`^task 1 is active for "([^"]*)" from a confirmed brief$`, w.seedActiveTask)
	sc.Step(`^task 1 is active at step "merge" with a pending approval, a queued merge item and queued dependent task 2$`, rewritten("AC-3", homeTaskNoMerge))
}

func homeTaskActionSteps(sc *godog.ScenarioContext, w *homeTaskWorld) {
	sc.Step(`^"([^"]*)" starts a direct task titled "([^"]*)" for the owner's request$`, w.startDirect)
	sc.Step(`^the host of "([^"]*)" reports branch "([^"]*)" for task (\d+)$`, rewritten("AC-3", homeTaskNoMerge))
	sc.Step(`^the host of "([^"]*)" reports a "([^"]*)" failure on task (\d+)$`, w.reportFail)
	sc.Step(`^the host of "([^"]*)" sends the same failure command again$`, w.repeatLast)
	sc.Step(`^the (secretary|owner) (?:tries to resume|resumes) task (\d+)$`, w.resumeBy)
	sc.Step(`^the host of "([^"]*)" resumes task (\d+)$`, w.resumeByHost)
	sc.Step(`^the (secretary|owner) (?:tries to assign|assigns) task (\d+) to "([^"]*)"$`, w.assignBy)
	sc.Step(`^the secretary tries to pause task (\d+)$`, w.pauseBySecretary)
	sc.Step(`^the (secretary|owner) (?:tries to cancel|cancels) task (\d+)$`, w.cancelBy)
	sc.Step(`^the host of "([^"]*)" reports task (\d+) pre-empted by the owner's request$`, w.preemptByHost)
}

func homeTaskCheckSteps(sc *godog.ScenarioContext, w *homeTaskWorld) {
	sc.Step(`^the home answers ok with task number (\d+)$`, w.checkStartedNum)
	sc.Step(`^the home answers ok to the task command$`, func() error { return w.checkLastCode("") })
	sc.Step(`^the home refuses the task command with "([^"]*)"$`, w.checkLastCode)
	sc.Step(`^both failure commands are answered ok and task (\d+) has (\d+) failure$`, w.checkRepeatedFail)
	sc.Step(`^task (\d+) is "([^"]*)" at step "([^"]*)" with no wait$`, func(num int, state, step string) error {
		return w.checkTask(num, state, step, "")
	})
	sc.Step(`^task (\d+) is "([^"]*)" at step "([^"]*)" waiting for "([^"]*)"$`, w.checkTask)
	sc.Step(`^task (\d+) has a task card in the main line$`, w.checkTaskCard)
	sc.Step(`^the home queued "([^"]*)"$`, w.checkQueued)
	sc.Step(`^task (\d+) has (\d+) failures and the home queued "([^"]*)"$`, w.checkForgiven)
	sc.Step(`^the thread of task (\d+) holds a "([^"]*)" system message (mentioning|for) the owner$`, w.checkSystemMsg)
	sc.Step(`^the pending approval of task (\d+) is "([^"]*)" and its merge item is retired$`, rewritten("AC-3", homeTaskNoMerge))
}

func (w *homeTaskWorld) nextID() proto.ID {
	w.seq++
	return exampleID("home task " + strconv.Itoa(w.seq))
}

func (w *homeTaskWorld) setup() error {
	browser, node, laptop := exampleKey("ht browser"), exampleKey("ht node"), exampleKey("ht laptop")
	w.keys = map[string]identity.Key{"owner": browser, "ai-one": laptop, "ai-two": node, "ai-three": node, "secretary": laptop}
	w.members = map[string]string{}
	for _, name := range []string{"owner", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("ht member " + name))
	}
	grp := string(exampleID("ht group"))
	w.state = home.NewState(grp, node.ID(), browser.ID())
	log, err := homeLog([]identity.Key{browser, node, laptop})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: grp}, Home: node.ID(), Secretary: w.members["secretary"],
		Pass: 90, PassFoundation: 70, Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	owner := w.members["owner"]
	w.state.People[owner] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: grp, Kind: "human", User: browser.ID()},
		Role:      "owner",
	}
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		id := w.members[name]
		host := w.keys[name]
		w.state.AIs[id] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: grp, Kind: "ai", User: browser.ID(), Name: name},
			Host:      host.ID(), Workspace: "/work/" + name,
		}
	}
	w.state.Seq = 5
	w.request = string(w.nextID())
	w.brief = string(w.nextID())
	w.store = &homeTaskStore{
		msgs: map[string]proto.MsgRow{
			w.request: {RowHeader: proto.RowHeader{Type: "msg", ID: w.request}, By: owner, Kind: "text"},
			w.brief:   {RowHeader: proto.RowHeader{Type: "msg", ID: w.brief}, Kind: "brief", Body: json.RawMessage(`{"requester":"` + owner + `"}`)},
		},
		outcomes: map[string]store.Outcome{},
	}
	w.results, w.items, w.retired = nil, nil, nil
	return nil
}

func (w *homeTaskWorld) seedActiveTask(aiName string) error {
	taskID := string(w.nextID())
	w.state.Num = 1
	w.state.Tasks[taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 5}, Group: w.state.ID, Num: 1,
		Title: "Report path changes", Origin: "brief", Brief: w.brief,
		State: "active", Step: "work", Assignee: w.members[aiName], Reviewer: w.members["ai-three"],
		Pass: 90, Depends: []string{}, Stats: proto.TaskStats{Started: w.now.UnixMilli() - 2000}, Created: w.now.UnixMilli() - 2000,
	}
	return nil
}

func (w *homeTaskWorld) dispatch(by identity.Key, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return w.sendEnv(by, by.Sign(string(raw)))
}

func (w *homeTaskWorld) sendEnv(by identity.Key, env proto.Envelope) error {
	res, err := home.Commands.Apply(w.state, w.store, w.store, env, w.now)
	if err != nil {
		return err
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			w.store.msgs[msg.ID] = msg
			w.updatePending(msg)
		}
	}
	w.state.Commit(res)
	w.lastEnv, w.lastBy = env, by
	w.results = append(w.results, res)
	w.items = append(w.items, res.Items...)
	w.retired = append(w.retired, res.Retire...)
	return nil
}

func (w *homeTaskWorld) updatePending(msg proto.MsgRow) {
	for i, p := range w.store.pending {
		if p.ID == msg.ID {
			w.store.pending[i] = msg
		}
	}
}

func (w *homeTaskWorld) taskByNum(num int) proto.TaskRow {
	for _, t := range w.state.Tasks {
		if t.Num == num {
			return t
		}
	}
	return proto.TaskRow{}
}

func (w *homeTaskWorld) actor(who string) (identity.Key, proto.ID) {
	if who == "secretary" {
		return w.keys["secretary"], proto.ID(w.members["secretary"])
	}
	return w.keys["owner"], ""
}

func (w *homeTaskWorld) startDirect(aiName, title string) error {
	re := proto.ID(w.request)
	cmd := proto.TaskStart{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID), As: proto.ID(w.members[aiName]),
			},
			Op: "start", ID: w.nextID(),
		},
		Title: title, Plan: "Update the notice.", Re: &re, Origin: "direct",
	}
	return w.dispatch(w.keys[aiName], cmd)
}

func (w *homeTaskWorld) reportFail(aiName, reason string, num int) error {
	w.items = nil
	cmd := proto.TaskFail{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "fail", ID: proto.ID(w.taskByNum(num).ID),
		},
		Reason: reason, Detail: reason + " detail",
	}
	return w.dispatch(w.keys[aiName], cmd)
}

func (w *homeTaskWorld) repeatLast() error { return w.sendEnv(w.lastBy, w.lastEnv) }

func (w *homeTaskWorld) resumeBy(who string, num int) error {
	w.items = nil
	by, as := w.actor(who)
	cmd := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID), As: as},
		Op:          "resume", ID: proto.ID(w.taskByNum(num).ID),
	}
	return w.dispatch(by, cmd)
}

func (w *homeTaskWorld) resumeByHost(aiName string, num int) error {
	w.items = nil
	cmd := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
		Op:          "resume", ID: proto.ID(w.taskByNum(num).ID),
	}
	return w.dispatch(w.keys[aiName], cmd)
}

func (w *homeTaskWorld) assignBy(who string, num int, target string) error {
	w.items = nil
	by, as := w.actor(who)
	cmd := proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID), As: as},
			Op:          "assign", ID: proto.ID(w.taskByNum(num).ID),
		},
		Assignee: proto.ID(w.members[target]),
	}
	return w.dispatch(by, cmd)
}

func (w *homeTaskWorld) pauseBySecretary(num int) error {
	by, as := w.actor("secretary")
	cmd := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID), As: as},
		Op:          "pause", ID: proto.ID(w.taskByNum(num).ID),
	}
	return w.dispatch(by, cmd)
}

func (w *homeTaskWorld) cancelBy(who string, num int) error {
	w.items, w.retired = nil, nil
	by, as := w.actor(who)
	cmd := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID), As: as},
		Op:          "cancel", ID: proto.ID(w.taskByNum(num).ID),
	}
	return w.dispatch(by, cmd)
}

func (w *homeTaskWorld) preemptByHost(aiName string, num int) error {
	w.items = nil
	cmd := proto.TaskPreempted{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "preempted", ID: proto.ID(w.taskByNum(num).ID),
		},
		Msg: proto.ID(w.request),
	}
	return w.dispatch(w.keys[aiName], cmd)
}

func (w *homeTaskWorld) checkStartedNum(want int) error {
	last := w.results[len(w.results)-1]
	if !last.Answer.OK || last.Answer.Num != want {
		return fmt.Errorf("got answer %+v, want ok with num %d", last.Answer, want)
	}
	return nil
}

func (w *homeTaskWorld) checkLastCode(code string) error {
	last := w.results[len(w.results)-1]
	if last.Answer.Code != code || last.Answer.OK != (code == "") {
		return fmt.Errorf("got answer %+v (%s), want code %q", last.Answer, last.Msg, code)
	}
	return nil
}

func (w *homeTaskWorld) checkRepeatedFail(num, failures int) error {
	n := len(w.results)
	if !w.results[n-2].Answer.OK || !w.results[n-1].Answer.OK {
		return fmt.Errorf("answers were %+v and %+v", w.results[n-2].Answer, w.results[n-1].Answer)
	}
	if got := w.taskByNum(num).Failures; got != failures {
		return fmt.Errorf("task %d has %d failures, want %d", num, got, failures)
	}
	return nil
}

func (w *homeTaskWorld) checkTask(num int, state, step, wait string) error {
	t := w.taskByNum(num)
	gotWait := ""
	if t.Wait != nil {
		gotWait = t.Wait.Kind
	}
	if t.State != state || t.Step != step || gotWait != wait {
		return fmt.Errorf("task %d is %s/%s waiting %q, want %s/%s waiting %q", num, t.State, t.Step, gotWait, state, step, wait)
	}
	return nil
}

func (w *homeTaskWorld) checkTaskCard(num int) error {
	t := w.taskByNum(num)
	msg, ok := w.store.msgs[t.ID]
	if !ok || msg.Kind != "task" || msg.Thread != nil {
		return fmt.Errorf("missing main-line task card for %s: %+v", t.ID, msg)
	}
	return nil
}

func (w *homeTaskWorld) memberName(id string) string {
	for name, mid := range w.members {
		if mid == id {
			return name
		}
	}
	return id
}

func (w *homeTaskWorld) formatItem(it home.Item) string {
	parts := []string{it.Why, w.memberName(it.AI)}
	if it.Why == "assign" && it.Msg == w.request {
		parts = append(parts, "with request")
	}
	if it.Code != "" {
		parts = append(parts, it.Code)
	}
	return strings.Join(parts, " ")
}

func (w *homeTaskWorld) checkQueued(want string) error {
	if want == "nothing" {
		if len(w.items) != 0 {
			return fmt.Errorf("expected no items, got %+v", w.items)
		}
		return nil
	}
	var got []string
	for _, it := range w.items {
		got = append(got, w.formatItem(it))
	}
	if strings.Join(got, ", ") != want {
		return fmt.Errorf("queued %q, want %q", strings.Join(got, ", "), want)
	}
	w.items = nil
	return nil
}

func (w *homeTaskWorld) checkForgiven(num, failures int, wantItems string) error {
	if got := w.taskByNum(num).Failures; got != failures {
		return fmt.Errorf("task %d has %d failures, want %d", num, got, failures)
	}
	return w.checkQueued(wantItems)
}

func (w *homeTaskWorld) checkSystemMsg(num int, code, mode string) error {
	t := w.taskByNum(num)
	owner := w.members["owner"]
	for _, m := range w.store.msgs {
		if homeTaskMatchesSys(m, t.ID, code, mode, owner) {
			return nil
		}
	}
	return fmt.Errorf("no %s system message (%s owner) in thread of task %d", code, mode, num)
}

func homeTaskMatchesSys(m proto.MsgRow, taskID, code, mode, owner string) bool {
	if m.Kind != "system" || m.Thread == nil || *m.Thread != taskID {
		return false
	}
	var b struct {
		Code string `json:"code"`
		By   string `json:"by"`
	}
	json.Unmarshal(m.Body, &b)
	if b.Code != code {
		return false
	}
	if mode == "mentioning" {
		return slices.Equal(m.Mentions, []string{owner})
	}
	return b.By == owner
}
