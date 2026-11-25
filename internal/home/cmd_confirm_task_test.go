package home_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
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
			queued := s.Tasks[queuedID]
			res = cardApply(t, s, d, receipts{}, guest, act)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, len(res.Rows), 2)
			cardCommit(s, d, res)
			equal(t, s.Tasks[queuedID], queued)
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

// The owner of an AI gives it, as an admin, a role that they had denied: all
// items of that task reach it. Their denial still holds for the other task of
// the request, and for this one when somebody else gives the role again.
func TestRoleDeniedThenGivenByTheOwner(t *testing.T) {
	s, d, _ := cftBrief(t)
	s.People[briefGuest] = person(briefGuest, guest, "admin")
	s.Group.Limits.Failures = 3
	first, second := briefTaskID(181, 0), briefTaskID(181, 1)
	equal(t, s.Tasks[first].Asked, []string{briefA, briefB})
	n := byte(190)
	send := func(by identity.Key, kind, rest string) []home.Item {
		t.Helper()
		n++
		res := cardApply(t, s, d, receipts{}, by, signed(by, kind, n, rest))
		equal(t, res.Answer, store.Outcome{OK: true})
		cardCommit(s, d, res)
		return res.Items
	}
	task := func(by identity.Key, op, rest string) []home.Item {
		t.Helper()
		return send(by, "task", fmt.Sprintf(`,"op":%q,"id":%q%s`, op, first, rest))
	}
	give := func(by identity.Key, ai string) []home.Item {
		t.Helper()
		return task(by, "assign", fmt.Sprintf(`,"assignee":%q`, ai))
	}
	send(guest, "act", fmt.Sprintf(`,"msg":%q,"action":"deny"`, cftFind(t, d, briefA).ID))
	equal(t, give(browser, briefC), []home.Item{{AI: briefC, Why: "assign", Task: first}})
	equal(t, s.Tasks[first].Asked, []string{briefB})

	equal(t, give(guest, briefA), []home.Item{{AI: briefA, Why: "assign", Task: first}})
	equal(t, s.Tasks[first].Asked, []string{briefB})
	task(guest, "branch", `,"branch":"task/5-report-path-changes"`)
	task(guest, "checked", `,"ok":true,"items":[],"commit":"c1"`)
	task(guest, "submitted", `,"commit":"c1","pr":1,"pr_url":"https://x/1","stats":{"added":1,"removed":0}`)
	checks := task(guest, "cloud", `,"ok":false,"items":[],"commit":"c1"`)
	equal(t, checks, []home.Item{{AI: briefA, Why: "checks", Task: first, Msg: id(n)}})
	equal(t, task(browser, "pause", ""), []home.Item(nil))
	equal(t, task(browser, "resume", ""), []home.Item{{AI: briefA, Why: "resume", Task: first}})
	equal(t, s.Tasks[second].Asked, []string{briefB, briefA})

	give(browser, briefC)
	equal(t, give(browser, briefA), []home.Item{})
	equal(t, s.Tasks[first].Asked, []string{briefA, briefB})
	task(browser, "pause", "")
	equal(t, task(browser, "resume", ""), []home.Item{})
}
