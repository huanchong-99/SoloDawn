//go:build acceptance

package features

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
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

func init() {
	stepSets["home_card"] = func(sc *godog.ScenarioContext, _ fixtures) { homeCardSteps(sc) }
}

type homeCardWorld struct {
	now      time.Time
	state    *home.State
	disk     homeCardDisk
	receipts homeCardReceipts
	members  map[string]string
	devices  map[string]identity.Key
	taskID   string
	approval string
	decision string
	lastAct  proto.Envelope
	answers  []home.Result
	queued   []home.Item
	nonce    int
}

type homeCardDisk map[string]proto.MsgRow

func (d homeCardDisk) Msg(id string) (proto.MsgRow, bool) {
	m, ok := d[id]
	return m, ok
}

func (d homeCardDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d homeCardDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (d homeCardDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d homeCardDisk) Queued() []store.Item                      { return nil }
func (d homeCardDisk) OtherMember(string) bool                   { return false }

func (d homeCardDisk) Pending() []proto.MsgRow {
	var list []proto.MsgRow
	for _, m := range d {
		if m.State != nil && m.State.Status == "pending" {
			list = append(list, m)
		}
	}
	slices.SortFunc(list, func(a, b proto.MsgRow) int { return cmp.Compare(a.Pos, b.Pos) })
	return list
}

type homeCardReceipts map[[2]string]store.Outcome

func (r homeCardReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := r[[2]string{signer, nonce}]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (r homeCardReceipts) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	r[[2]string{signer, nonce}] = result
	return nil
}

func homeCardSteps(sc *godog.ScenarioContext) {
	w := &homeCardWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^a card home with an active task assigned to "([^"]*)"$`, w.setup)
	sc.Step(`^"([^"]*)" (?:posts|has posted) an approval card for command "([^"]*)"$`, w.postApproval)
	sc.Step(`^the secretary (?:posts|has posted) a decision card on the task with options "([^"]*)"$`, w.postDecision)
	sc.Step(`^the owner's (first|second) browser approves the approval card$`, func(which string) error {
		return w.actCard(w.devices[which], w.approval, "approve", "")
	})
	sc.Step(`^the owner's (first|second) browser denies the approval card$`, func(which string) error {
		return w.actCard(w.devices[which], w.approval, "deny", "")
	})
	sc.Step(`^the owner's (first|second) browser chooses option "([^"]*)" with text "([^"]*)" on the decision card$`, func(which, opt, text string) error {
		return w.actCard(w.devices[which], w.decision, "choose", fmt.Sprintf(`,"arg":%q,"text":%q`, opt, text))
	})
	sc.Step(`^the owner's (first|second) browser answers the decision card with free text "([^"]*)"$`, func(which, text string) error {
		return w.actCard(w.devices[which], w.decision, "choose", fmt.Sprintf(`,"text":%q`, text))
	})
	sc.Step(`^the owner's (first|second) browser tries to choose option "([^"]*)" on the decision card$`, func(which, opt string) error {
		return w.actCard(w.devices[which], w.decision, "choose", fmt.Sprintf(`,"arg":%q`, opt))
	})
	sc.Step(`^the owner's (first|second) browser tries to choose nothing on the decision card$`, func(which string) error {
		return w.actCard(w.devices[which], w.decision, "choose", "")
	})
	sc.Step(`^(a non-approver member tries to approve the approval card|"ai-one" tries to approve its own approval card|the owner's first browser tries to approve the approval card with an arg|"ai-three" tries to post an approval card on "ai-one"'s task)$`, w.attemptRefusal)
	sc.Step(`^the card home answers ok$`, func() error { return w.checkAnswer("") })
	sc.Step(`^the card home answers "([^"]*)"$`, w.checkAnswer)
	sc.Step(`^the approval card is "([^"]*)" with command "([^"]*)"$`, w.checkApprovalCmd)
	sc.Step(`^the approval card is "([^"]*)" by "([^"]*)"$`, w.checkApprovalBy)
	sc.Step(`^the decision card is "([^"]*)" with choice "([^"]*)" and text "([^"]*)"$`, func(status, choice, text string) error {
		return w.checkDecision(status, &choice, &text)
	})
	sc.Step(`^the decision card is "([^"]*)" with no choice and text "([^"]*)"$`, func(status, text string) error {
		return w.checkDecision(status, nil, &text)
	})
	sc.Step(`^the approval card is "([^"]*)" and the decision card is "([^"]*)"$`, w.checkBothStatuses)
	sc.Step(`^the card task waits for "([^"]*)" on the (approval|decision) card$`, w.checkTaskCardWait)
	sc.Step(`^the card task waits for "nothing"$`, w.checkNoWait)
	sc.Step(`^the card home has queued "([^"]*)"$`, w.checkQueued)
	sc.Step(`^the card home has queued nothing$`, func() error { return w.checkQueued("") })
}

func (w *homeCardWorld) setup(assignee string) error {
	b1, b2 := exampleKey("card browser 1"), exampleKey("card browser 2")
	node, host, guest := exampleKey("card node"), exampleKey("card host"), exampleKey("card guest")
	w.devices = map[string]identity.Key{"first": b1, "second": b2, "host": host, "guest": guest}
	w.members = map[string]string{}
	for _, name := range []string{"owner", "guest", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("card " + name))
	}
	group := string(exampleID("card group"))
	w.state = home.NewState(group, node.ID(), b1.ID())
	ownerLog, err := homeLog([]identity.Key{b1, b2, node, host})
	if err != nil {
		return err
	}
	guestLog, err := homeLog([]identity.Key{guest})
	if err != nil {
		return err
	}
	w.state.Logs[ownerLog.User] = ownerLog
	w.state.Logs[guestLog.User] = guestLog
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: node.ID(),
		Secretary: w.members["secretary"], Forge: "none", Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, ApprovalWaitS: 600},
	}
	w.state.People[w.members["owner"]] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["owner"]}, Group: group, Kind: "human", User: b1.ID(), Name: "owner"},
		Role:      "owner",
	}
	w.state.People[w.members["guest"]] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["guest"]}, Group: group, Kind: "human", User: guest.ID(), Name: "guest"},
		Role:      "member",
	}
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		mid := w.members[name]
		w.state.AIs[mid] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: mid}, Group: group, Kind: "ai", User: b1.ID(), Name: name},
			Host:      host.ID(), Secretary: name == "secretary", Approvers: []string{b1.ID()},
		}
	}
	w.taskID = string(exampleID("card task 1"))
	w.state.Tasks[w.taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID, Seq: 7}, Group: group, Num: 1,
		Title: "Install browser tests", Slug: proto.MakeSlug("Install browser tests"), Origin: "brief",
		State: "active", Step: "local_check", Assignee: w.members[assignee], Reviewer: w.members["ai-two"],
		Pass: 90, Depends: []string{}, Created: w.now.UnixMilli() - 5000, Stats: proto.TaskStats{Started: w.now.UnixMilli() - 5000},
	}
	w.state.Seq, w.state.Num = 7, 1
	w.disk, w.receipts, w.answers, w.queued = homeCardDisk{}, homeCardReceipts{}, nil, nil
	return nil
}

func (w *homeCardWorld) freshNonce() proto.ID {
	w.nonce++
	return exampleID("card nonce " + strconv.Itoa(w.nonce))
}

func (w *homeCardWorld) postApproval(aiName, command string) error {
	aiID := w.members[aiName]
	why := "Run browser timing checks."
	tid := proto.ID(w.taskID)
	ownerMember := proto.ID(w.members["owner"])
	post := proto.Post[proto.ApprovalBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.freshNonce(), Timestamp: w.now.UnixMilli() - 400, Group: proto.ID(w.state.ID), As: proto.ID(aiID)},
		Kind:        "approval", Thread: &tid, Main: true, Mentions: []proto.ID{ownerMember},
		Body: proto.ApprovalBody{
			Task: w.taskID, AI: aiID, Tool: "Bash", Command: command, Why: &why,
			Category: "command", Approvers: []string{w.devices["first"].ID()},
		},
	}
	raw, err := json.Marshal(post)
	if err != nil {
		return err
	}
	if err := w.apply(w.devices["host"].Sign(string(raw))); err != nil {
		return err
	}
	if w.answers[len(w.answers)-1].Answer.OK {
		w.approval = string(post.Nonce)
	}
	return nil
}

func (w *homeCardWorld) postDecision(optList string) error {
	var opts []proto.DecisionOption
	for _, id := range strings.Split(optList, ", ") {
		opts = append(opts, proto.DecisionOption{ID: id, Label: "Option " + id, Effect: "Effect " + id})
	}
	tid := proto.ID(w.taskID)
	taskRef := w.taskID
	post := proto.Post[proto.DecisionBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.freshNonce(), Timestamp: w.now.UnixMilli() - 600, Group: proto.ID(w.state.ID), As: proto.ID(w.members["secretary"])},
		Kind:        "decision", Thread: &tid, Main: true, Text: "Choose where the indicator sits.", Mentions: []proto.ID{},
		Body: proto.DecisionBody{
			Task: &taskRef, Question: "Where should the indicator go?", Options: opts, To: []string{w.devices["first"].ID()},
		},
	}
	raw, err := json.Marshal(post)
	if err != nil {
		return err
	}
	w.decision = string(post.Nonce)
	return w.apply(w.devices["host"].Sign(string(raw)))
}

func (w *homeCardWorld) actCard(by identity.Key, msg, action, extra string) error {
	payload := fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":%q%s}`, w.freshNonce(), w.now.UnixMilli()-100, w.state.ID, msg, action, extra)
	env := by.Sign(payload)
	w.lastAct = env
	return w.apply(env)
}

func (w *homeCardWorld) attemptRefusal(attempt string) error {
	switch attempt {
	case "a non-approver member tries to approve the approval card":
		return w.actCard(w.devices["guest"], w.approval, "approve", "")
	case `"ai-one" tries to approve its own approval card`:
		return w.actCard(w.devices["host"], w.approval, "approve", `,"as":"`+w.members["ai-one"]+`"`)
	case "the owner's first browser tries to approve the approval card with an arg":
		return w.actCard(w.devices["first"], w.approval, "approve", `,"arg":"A"`)
	default:
		return w.postApproval("ai-three", "echo hi")
	}
}

func (w *homeCardWorld) apply(env proto.Envelope) error {
	res, err := home.Commands.Apply(w.state, w.disk, w.receipts, env, w.now)
	if err != nil {
		return err
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			w.disk[msg.ID] = msg
		}
	}
	w.state.Commit(res)
	w.answers = append(w.answers, res)
	w.queued = append(w.queued, res.Items...)
	return nil
}

func (w *homeCardWorld) checkAnswer(code string) error {
	res := w.answers[len(w.answers)-1]
	if res.Answer.Code != code || res.Answer.OK != (code == "") {
		return fmt.Errorf("answered %+v: %s", res.Answer, res.Msg)
	}
	return nil
}

func (w *homeCardWorld) checkApprovalCmd(status, command string) error {
	card := w.disk[w.approval]
	var body proto.ApprovalBody
	json.Unmarshal(card.Body, &body)
	if card.State == nil || card.State.Status != status || body.Command != command {
		return fmt.Errorf("approval card state %+v, command %q", card.State, body.Command)
	}
	return nil
}

func (w *homeCardWorld) checkApprovalBy(status, who string) error {
	card, by, at := w.disk[w.approval], w.members[who], w.now.UnixMilli()
	want := &proto.CardState{Status: status, By: &by, At: &at, Act: &w.lastAct}
	if !reflect.DeepEqual(card.State, want) {
		return fmt.Errorf("approval card state %+v; want %+v", card.State, want)
	}
	return nil
}

func (w *homeCardWorld) checkDecision(status string, choice, text *string) error {
	card, by, at := w.disk[w.decision], w.members["owner"], w.now.UnixMilli()
	want := &proto.CardState{Status: status, By: &by, At: &at, Choice: choice, Text: text, Act: &w.lastAct}
	if !reflect.DeepEqual(card.State, want) {
		return fmt.Errorf("decision card state %+v; want %+v", card.State, want)
	}
	return nil
}

func (w *homeCardWorld) checkBothStatuses(appStatus, decStatus string) error {
	if err := w.checkCardStatus(w.approval, appStatus); err != nil {
		return err
	}
	return w.checkCardStatus(w.decision, decStatus)
}

func (w *homeCardWorld) checkCardStatus(id, status string) error {
	card := w.disk[id]
	if card.State == nil || card.State.Status != status {
		return fmt.Errorf("card %s state is %+v; want %s", id, card.State, status)
	}
	return nil
}

func (w *homeCardWorld) checkTaskCardWait(kind, which string) error {
	wantMsg := w.approval
	if which == "decision" {
		wantMsg = w.decision
	}
	wait := w.state.Tasks[w.taskID].Wait
	if wait == nil || wait.Kind != kind || wait.Msg != wantMsg {
		return fmt.Errorf("task wait is %+v; want %s on %s", wait, kind, wantMsg)
	}
	return nil
}

func (w *homeCardWorld) checkNoWait() error {
	if wait := w.state.Tasks[w.taskID].Wait; wait != nil {
		return fmt.Errorf("task waits for %+v; want nil", wait)
	}
	return nil
}

func (w *homeCardWorld) checkQueued(spec string) error {
	var want []string
	if spec != "" {
		want = strings.Split(spec, ", ")
	}
	var got []string
	for _, it := range w.queued {
		got = append(got, fmt.Sprintf("%s %s task %d", it.Why, w.memberName(it.AI), w.state.Tasks[it.Task].Num))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %q; want %q", got, want)
	}
	w.queued = nil
	return nil
}

func (w *homeCardWorld) memberName(id string) string {
	for name, mid := range w.members {
		if mid == id {
			return name
		}
	}
	return id
}
