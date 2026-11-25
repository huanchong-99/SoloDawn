package home

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func cftFixture() (*State, cftDisk, proto.TaskRow, proto.MsgRow, Command) {
	s := NewState("g", "h", "p")
	s.AIs["a"] = proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{ID: "a"}, User: "q"}}
	s.AIs["r"] = proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{ID: "r"}, User: "q"}}
	s.Group.Secretary = "sec"
	task := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: "t"}, State: taskActive, Step: stepBranch, Assignee: "a", Reviewer: "r", Asked: []string{"a", "r"}, Brief: "b"}
	s.Tasks["t"] = task
	req := "b"
	card := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: cfmCardID("b", "a")}, Kind: cfmKind, Re: &req,
		Body: briefJSON(proto.ConfirmBody{What: cfmWhatDirect, AI: "a", To: []string{"q"}}), State: &proto.CardState{Status: cardStatusPending}}
	d := cftDisk{Disk: cardTestDisk{}, msgs: map[string]proto.MsgRow{"b": {RowHeader: proto.RowHeader{ID: "b"}, Kind: briefKind}, card.ID: card}}
	c := Command{Disk: d, Now: 10, Who: Who{User: "p"}}
	return s, d, task, card, c
}

func TestRoleGateWaitsActivationAndExpiry(t *testing.T) {
	s, d, task, card, c := cftFixture()
	cardTestSame(t, cftRequest(proto.TaskRow{RowHeader: proto.RowHeader{ID: "direct"}}), "direct")
	cardTestSame(t, s.cfmAllowed(d, "p", &task.ID, "a"), false)
	held := s.cftFinish(c, Result{Rows: []Row{RowOf(task)}, Items: []Item{{AI: "a", Task: "t", Why: itemAssign}, {AI: "sec", Why: itemNotice}}})
	cardTestSame(t, held.Items, []Item{{AI: "sec", Why: itemNotice}})
	cardTestSame(t, held.Rows[0].Value().(proto.TaskRow).Wait, &proto.TaskWait{Kind: waitConfirm, Msg: card.ID, Who: []string{"q"}})
	confirmed := card
	confirmed.State = &proto.CardState{Status: briefConfirmed}
	task.Wait = &proto.TaskWait{Kind: waitConfirm, Msg: card.ID}
	s.Tasks["t"] = task
	d.msgs[card.ID] = confirmed
	cardTestSame(t, s.cfmAllowed(d, "p", &task.ID, "a"), true)
	res := s.cftFinish(c, Result{Rows: []Row{RowOf(task)}, Items: []Item{{AI: "a", Task: "t", Why: itemResume}}})
	cardTestSame(t, res.Rows[0].Value().(proto.TaskRow).Wait, (*proto.TaskWait)(nil))
	cardTestSame(t, res.Items, []Item{{AI: "a", Task: "t", Why: itemResume}})
	task.State = taskPaused
	cardTestSame(t, s.cftDecide(c, confirmed, proto.ConfirmBody{AI: "a"}).Items, []Item{{AI: "a", Why: itemAssign, Task: "t", Msg: "t"}})
	s.Tasks["t"] = task
	cardTestSame(t, s.cftDecide(c, confirmed, proto.ConfirmBody{AI: "a"}).Items, []Item(nil))
	task.State = taskQueued
	s.Tasks["t"] = task
	cardTestSame(t, s.cftDecide(c, confirmed, proto.ConfirmBody{AI: "a"}).Items, []Item(nil))
	d.msgs[card.ID] = card
	task.State = taskCancelled
	s.Tasks["t"] = task
	cardTestSame(t, s.cftFinish(c, Result{}), Result{})
	d.Disk = cardTestDisk{card}
	res = cfmExpire(s, d, c.Now)
	at := int64(10)
	cardTestSame(t, res.Rows[0].Value().(proto.MsgRow).State, &proto.CardState{Status: "expired", At: &at})
	cardTestSame(t, s.cftHasRole("missing", "a"), false)
	cardTestSame(t, s.cftTasksFor("missing", "a"), []proto.TaskRow(nil))
	cardTestSame(t, cftActivation(d, task, "r"), []Item(nil))
	task.Step = stepReview
	task.Commit = "c"
	cardTestSame(t, cftActivation(d, task, "r"), []Item{{AI: "r", Why: itemReview, Task: "t", Commit: "c"}})
}

func TestRoleCardOverlayAndOrdinaryWaits(t *testing.T) {
	s, d, task, card, c := cftFixture()
	closed := card
	closed.State = &proto.CardState{Status: cardStatusDenied}
	d.Disk = cardTestDisk{card}
	d.msgs[card.ID] = closed
	cardTestSame(t, cftHeld(closed), true)
	closed.State = &proto.CardState{Status: "expired"}
	cardTestSame(t, cftHeld(closed), true)
	task.Wait = &proto.TaskWait{Kind: waitConfirm}
	approval := proto.MsgRow{RowHeader: proto.RowHeader{ID: "app"}, Kind: waitApproval, Pos: 3, Timestamp: 1, Body: briefJSON(proto.ApprovalBody{Task: "t", Approvers: []string{"p"}}), State: &proto.CardState{Status: cardStatusPending}}
	decision := proto.MsgRow{RowHeader: proto.RowHeader{ID: "dec"}, Kind: waitDecision, Pos: 2, Body: briefJSON(proto.DecisionBody{Task: &task.ID, To: []string{"p"}}), State: &proto.CardState{Status: cardStatusPending}}
	d.Disk = cardTestDisk{approval, decision}
	cardTestSame(t, cftWait(d, task), &proto.TaskWait{Kind: waitApproval, Msg: "app", Who: []string{"p"}, Since: 1})
	d.Disk = cardTestDisk{decision}
	cardTestSame(t, cftWait(d, task), &proto.TaskWait{Kind: waitDecision, Msg: "dec", Who: []string{"p"}})
	d.Disk = cardTestDisk{}
	cardTestSame(t, cftWait(d, task), (*proto.TaskWait)(nil))
	task.Wait = &proto.TaskWait{Kind: waitConflict}
	cardTestSame(t, cftWait(d, task), task.Wait)
	task.Step = stepReview
	rcard := s.cfmNewCard(c, cfmWhatDirect, d.msgs["b"], s.AIs["r"])
	d.msgs[rcard.ID] = rcard
	cardTestSame(t, cftWait(d, task).Msg, rcard.ID)
	out := Result{Rows: []Row{RowOf(card)}}
	cftPutTask(&out, task)
	cardTestSame(t, len(out.Rows), 2)
	task.Commit = "new"
	cftPutTask(&out, task)
	cardTestSame(t, len(out.Rows), 2)
	cardTestSame(t, out.Rows[1].Value(), task)
}

func TestRoleFinishingWithoutTaskWorkNeedsNoDisk(t *testing.T) {
	s := NewState("g", "h", "owner")
	for _, res := range []Result{
		{},
		{Rows: []Row{RowOf(proto.NoteRow{})}},
		{Rows: []Row{RowOf(proto.MsgRow{})}, Items: []Item{{AI: "a", Why: itemNotice}}},
	} {
		cardTestSame(t, s.cftFinish(Command{}, res), res)
	}
}

func TestRoleDecisionsAndItemsLeaveUnchangedTasksAlone(t *testing.T) {
	s, d, task, card, c := cftFixture()
	task.Wait = &proto.TaskWait{Kind: waitConfirm, Msg: card.ID, Who: []string{"q"}}
	s.Tasks[task.ID] = task
	res := s.cftFinish(c, Result{Items: []Item{{AI: "a", Task: task.ID, Why: postMentionWhy}}})
	cardTestSame(t, res.Rows, []Row(nil))
	rcard := s.cfmNewCard(c, cfmWhatDirect, d.msgs["b"], s.AIs["r"])
	rcard.State = &proto.CardState{Status: briefConfirmed}
	res = s.cftDecide(c, rcard, proto.ConfirmBody{AI: "r"})
	cardTestSame(t, res.Rows, []Row{RowOf(rcard)})
	cardTestSame(t, res.Items, []Item(nil))
	for _, state := range []string{taskQueued, taskPaused, taskMerged} {
		task.State = state
		s.Tasks[task.ID] = task
		res = s.cftDecide(c, rcard, proto.ConfirmBody{AI: "r"})
		cardTestSame(t, res.Rows, []Row{RowOf(rcard)})
		cardTestSame(t, res.Items, []Item(nil))
	}
}

func TestNewRoleAuthorityAndDuplicates(t *testing.T) {
	s, d, task, card, c := cftFixture()
	res := Result{}
	s.cftAsk(c, task, s.AIs["a"], d, &res)
	cardTestSame(t, len(res.Rows), 0)
	given := task
	given.Asked = nil
	s.Tasks[task.ID] = given
	s.cftAsk(c, task, s.AIs["a"], d, &res)
	cardTestSame(t, len(res.Rows), 0)
	delete(d.msgs, card.ID)
	s.cftAsk(c, task, s.AIs["a"], d, &res)
	cardTestSame(t, res.Rows[0].Value().(proto.MsgRow).ID, card.ID)
	ai := s.AIs["a"]
	ai.Account, ai.Allow = "team", []string{"p"}
	s.AIs["a"] = ai
	cardTestSame(t, s.cftAsks("q", task, "a", "old", false), false)
	cardTestSame(t, s.cftAsks("p", task, "a", "old", true), false)
	cardTestSame(t, s.cftAsks("p", task, "a", "old", false), true)
	cardTestSame(t, s.cftAsks("x", task, "a", "old", true), true)
	cardTestSame(t, s.cftAsks("q", task, "a", "a", false), true)
	cardTestSame(t, s.cftAsks("p", proto.TaskRow{}, "a", "a", false), false)
}

// A text in the thread of a finished task, whose decided cards retention has
// deleted, asks nobody again.
func TestFinishedTaskWritesNoCardAgain(t *testing.T) {
	s, d, task, card, c := cftFixture()
	task.State = taskMerged
	s.Tasks[task.ID] = task
	delete(d.msgs, card.ID)
	task.Thread.Count++
	cardTestSame(t, s.cftFinish(c, Result{Rows: []Row{RowOf(task)}}).Rows, []Row{RowOf(task)})
}

func TestTaskItemWithoutRowsIsGated(t *testing.T) {
	s, _, task, card, c := cftFixture()
	res := s.cftFinish(c, Result{Items: []Item{{AI: task.Assignee, Task: task.ID, Why: itemAssign}}})
	cardTestSame(t, res.Items, []Item{})
	task.Wait = &proto.TaskWait{Kind: waitConfirm, Msg: card.ID, Who: []string{"q"}}
	cardTestSame(t, res.Rows, []Row{RowOf(task)})
}

// A card for another task of the request neither holds nor, confirmed, starts
// again a role that asks nobody.
func TestRoleThatAsksNobodyIgnoresTheCardOfItsRequest(t *testing.T) {
	s, _, task, card, c := cftFixture()
	task.Asked = nil
	s.Tasks[task.ID] = task
	resume := Result{Items: []Item{{AI: "a", Task: "t", Why: itemResume}}}
	cardTestSame(t, s.cftFinish(c, resume), resume)
	card.State = &proto.CardState{Status: briefConfirmed}
	cardTestSame(t, s.cftDecide(c, card, proto.ConfirmBody{AI: "a"}).Items, []Item(nil))
}

func TestRoleGrantOnDependencyActivationAndMultipleActiveTasks(t *testing.T) {
	for _, status := range []string{cardStatusPending, briefConfirmed, cardStatusDenied} {
		s, d, task, card, c := cftFixture()
		card.State = &proto.CardState{Status: status}
		d.msgs[card.ID] = card
		task.State, task.Depends = taskQueued, []string{"dependency"}
		task.Wait = &proto.TaskWait{Kind: waitDepends}
		s.Tasks[task.ID] = task
		row, items, ok := s.mrgReleaseOne(d, task, "dependency", 10)
		cardTestSame(t, ok, true)
		res := s.cftFinish(c, Result{Rows: []Row{row}, Items: items})
		active := res.Rows[0].Value().(proto.TaskRow)
		cardTestSame(t, active.State, taskActive)
		cardTestSame(t, active.Stats.Started, int64(10))
		if status == briefConfirmed {
			cardTestSame(t, res.Items, []Item{{AI: "a", Task: "t", Why: itemAssign, Msg: "t"}})
			cardTestSame(t, active.Wait, (*proto.TaskWait)(nil))
		} else {
			cardTestSame(t, len(res.Items), 0)
		}
	}
	s, d, task, card, c := cftFixture()
	task.ID = "second"
	s.Tasks[task.ID] = task
	card.State = &proto.CardState{Status: briefConfirmed}
	res := s.cftDecide(c, card, proto.ConfirmBody{AI: "a"})
	cardTestSame(t, res.Items, []Item{{AI: "a", Why: itemAssign, Task: "second", Msg: "second"}, {AI: "a", Why: itemAssign, Task: "t", Msg: "t"}})
	task.Step, task.Commit = stepReview, "c"
	s.Tasks[task.ID] = task
	d.msgs[cfmCardID("b", "r")] = proto.MsgRow{State: &proto.CardState{Status: briefConfirmed}}
	item := Item{AI: "r", Task: task.ID, Why: itemReview, Commit: "c"}
	cardTestSame(t, s.cftFinish(c, Result{Items: []Item{item}}).Items, []Item{item})
}
