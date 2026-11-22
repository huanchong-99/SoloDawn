package home_test

import (
	"encoding/json"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func cftBrief(t *testing.T) (*home.State, cardDisk, home.Result) {
	t.Helper()
	s, d := briefState(t), cardDisk{}
	for _, id := range []string{briefA, briefB} {
		a := s.AIs[id]
		a.User, a.Host, a.Account = guest.ID(), guest.ID(), "personal"
		s.AIs[id] = a
	}
	post := briefPost(180, briefBody())
	made := cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, post))
	equal(t, made.Answer.OK, true)
	cardCommit(s, d, made)
	res := cardApply(t, s, d, receipts{}, browser, cfmAct(browser, 181, id(180), "confirm", ""))
	equal(t, res.Answer.OK, true)
	cardCommit(s, d, res)
	return s, d, res
}

func cftFind(t *testing.T, d cardDisk, ai string) proto.MsgRow {
	t.Helper()
	for _, card := range d {
		var b proto.ConfirmBody
		json.Unmarshal(card.Body, &b)
		if card.Kind == "confirm" && b.AI == ai {
			return card
		}
	}
	t.Fatal("missing role card")
	return proto.MsgRow{}
}

func TestRoleBriefCardsAndDecisions(t *testing.T) {
	for _, action := range []string{"confirm", "deny"} {
		t.Run(action, func(t *testing.T) {
			s, d, res := cftBrief(t)
			taskID := briefTaskID(181, 0)
			queuedID := briefTaskID(181, 1)
			card := cftFind(t, d, briefA)
			equal(t, res.Items, []home.Item{{AI: aiID, Why: "confirmed", Msg: id(180)}})
			equal(t, s.Tasks[taskID].Wait, &proto.TaskWait{Kind: "confirm", Msg: card.ID, Who: []string{guest.ID()}, Since: clock})
			equal(t, s.Tasks[queuedID].Wait.Kind, "depends")
			equal(t, card.Re, cardOptPtr(id(180)))
			equal(t, card.Thread, (*string)(nil))
			equal(t, card.Env, (*proto.Envelope)(nil))
			// A held role does not suppress a new confirmation for a thread mention.
			_, mention := postSend(t, s, d, browser, postMake(182, "", "text", &taskID, []string{briefA}, nil, "Context", proto.EmptyBody{}))
			equal(t, len(mention.Items), 0)
			equal(t, cfmCard(t, mention).State.Status, "pending")
			act := cfmAct(guest, 183, card.ID, action, "")
			res = cardApply(t, s, d, receipts{}, guest, act)
			equal(t, res.Answer, store.Outcome{OK: true})
			cardCommit(s, d, res)
			if action == "confirm" {
				equal(t, res.Items, []home.Item{{AI: briefA, Why: "assign", Task: taskID, Msg: taskID}})
				equal(t, s.Tasks[taskID].Wait, (*proto.TaskWait)(nil))
				_, mention = postSend(t, s, d, browser, postMake(184, "", "text", &taskID, []string{briefA}, nil, "Context", proto.EmptyBody{}))
				equal(t, mention.Items, []home.Item{{AI: briefA, Why: "mention", Task: taskID, Msg: id(184)}})
				equal(t, len(mention.Rows), 2)
			} else {
				equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Msg: card.ID, Code: "confirm_denied"}})
				equal(t, s.Tasks[taskID].Wait, (*proto.TaskWait)(nil))
			}
		})
	}
}

func cardOptPtr(s string) *string { return &s }

func TestRoleReassignmentAndReviewerWait(t *testing.T) {
	s, d := cfmState(t), cardDisk{}
	// The task's request, as written by task start.
	d[calTaskID] = proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: calTaskID}, Kind: "task"}
	task := s.Tasks[calTaskID]
	task.Brief, task.Step, task.Reviewer = "", "review", calA
	task.Assignee, task.Commit = calC, "commit"
	s.Tasks[calTaskID] = task
	assign := proto.TaskAssign{TaskCommand: proto.TaskCommand{HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(185)), Timestamp: clock, Group: proto.ID(groupID)}, Op: "assign", ID: proto.ID(calTaskID)}, Reviewer: proto.ID(calB)}
	_, res := postSend(t, s, d, browser, assign)
	equal(t, res.Answer.OK, true)
	cardCommit(s, d, res)
	card := cftFind(t, d, calB)
	equal(t, s.Tasks[calTaskID].Wait.Kind, "confirm")
	equal(t, len(res.Items), 0)
	res = cardApply(t, s, d, receipts{}, guest, cfmAct(guest, 186, card.ID, "confirm", ""))
	cardCommit(s, d, res)
	equal(t, res.Answer.OK, true)
	equal(t, res.Items, []home.Item{{AI: calB, Why: "review", Task: calTaskID, Commit: "commit"}})
	equal(t, s.Tasks[calTaskID].Wait, (*proto.TaskWait)(nil))
}
