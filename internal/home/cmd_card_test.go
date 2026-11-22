package home_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	cardA, cardB, cardGuest = id(30), id(31), id(32)
	cardTaskID, cardOtherID = id(33), id(34)
	cardC                   = id(36)
	cardPhone               = device(7)
)

const cardPostTS, cardActTS = clock - 600, clock - 200

type cardDisk map[string]proto.MsgRow

func (d cardDisk) Msg(id string) (proto.MsgRow, bool) {
	m, ok := d[id]
	return m, ok
}

func (d cardDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d cardDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (d cardDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d cardDisk) Queued() []store.Item                      { return nil }

func (d cardDisk) Pending() []proto.MsgRow {
	var list []proto.MsgRow
	for _, m := range d {
		if m.State != nil && m.State.Status == "pending" {
			list = append(list, m)
		}
	}
	slices.SortFunc(list, func(a, b proto.MsgRow) int { return cmp.Compare(a.Pos, b.Pos) })
	return list
}

func cardState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Logs[browser.ID()] = logOf(t, browser, node, laptop, cardPhone)
	s.AIs[cardA] = ai(cardA, laptop)
	s.AIs[cardB] = ai(cardB, laptop)
	s.AIs[cardC] = ai(cardC, laptop)
	s.People[cardGuest] = person(cardGuest, guest, "member")
	s.Tasks[cardTaskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: cardTaskID, Seq: 9}, Group: groupID, Num: 5,
		Title: "Install chromium", Slug: "install-chromium", Origin: "brief",
		State: "active", Step: "local_check", Assignee: cardA, Reviewer: cardB,
		Pass: 90, Depends: []string{}, Thread: proto.ThreadSummary{Count: 2, Rounds: 1},
		Created: clock - 5000, Stats: proto.TaskStats{Started: clock - 5000},
	}
	s.Num = 5
	return s
}

func cardApprovalBody() proto.ApprovalBody {
	full, why := 4096, "Time the path change in a browser."
	return proto.ApprovalBody{
		Task: cardTaskID, AI: cardA, Tool: "Bash", Command: "pnpm exec playwright install chromium",
		Full: &full, Why: &why, Category: "command", Approvers: []string{browser.ID()},
	}
}

func cardApprovalPost(n byte, body proto.ApprovalBody) proto.Post[proto.ApprovalBody] {
	var tid *proto.ID
	if body.Task != "" {
		v := proto.ID(body.Task)
		tid = &v
	}
	return proto.Post[proto.ApprovalBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: cardPostTS, Group: proto.ID(groupID), As: proto.ID(body.AI)},
		Kind:        "approval", Thread: tid, Main: true, Mentions: []proto.ID{proto.ID(ownerID)}, Body: body,
	}
}

func cardDecisionBody(task *string) proto.DecisionBody {
	ctx, rec := "Only space for one label.", "A"
	return proto.DecisionBody{
		Task: task, Question: "Where should the path appear?", Context: &ctx,
		Options:     []proto.DecisionOption{{ID: "A", Label: "Header", Effect: "Always visible."}, {ID: "B", Label: "Badge", Effect: "Quiet."}},
		Recommended: &rec, To: []string{browser.ID()},
	}
}

func cardDecisionPost(n byte, as string, body proto.DecisionBody) proto.Post[proto.DecisionBody] {
	var tid *proto.ID
	if body.Task != nil {
		v := proto.ID(*body.Task)
		tid = &v
	}
	return proto.Post[proto.DecisionBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: cardPostTS, Group: proto.ID(groupID), As: proto.ID(as)},
		Kind:        "decision", Thread: tid, Main: true, Text: "Choose a location.", Mentions: []proto.ID{}, Body: body,
	}
}

func cardActEnv(by identity.Key, n byte, action, msg, extra string) proto.Envelope {
	return by.Sign(fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":%q%s}`, id(n), cardActTS, groupID, msg, action, extra))
}

func cardApply(t *testing.T, s *home.State, d cardDisk, r home.Receipts, _ identity.Key, env proto.Envelope) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, env, now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func cardCommit(s *home.State, d cardDisk, res home.Result) {
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			d[msg.ID] = msg
		}
	}
	s.Commit(res)
}

func cardValues(res home.Result) []any {
	out := []any{}
	for _, r := range res.Rows {
		out = append(out, r.Value())
	}
	return out
}

func cardRefused(t *testing.T, res home.Result, code string) {
	t.Helper()
	if res.Answer.Code != code || res.Msg == "" || len(res.Rows) != 0 || len(res.Items) != 0 {
		t.Fatalf("answered %+v (%q) with %d rows and %d items; want %s", res.Answer, res.Msg, len(res.Rows), len(res.Items), code)
	}
}

// A4.1, post half: posting an approval card stores the pending card in the
// thread and the main line, sets the task wait, and cuts the thread preview at
// 200 characters.
func TestCardPostApprovalWritesAPendingCardAndUpdatesTheTask(t *testing.T) {
	s, d := cardState(t), cardDisk{}
	post := cardApprovalPost(60, cardApprovalBody())
	re := proto.ID(id(35))
	post.Main = false
	post.Text = strings.Repeat("中", 201)
	post.Mentions = []proto.ID{proto.ID(ownerID), proto.ID(cardB)}
	post.Re = &re
	env := briefSign(laptop, post)
	res := cardApply(t, s, d, receipts{}, laptop, env)

	tid, reStr := cardTaskID, id(35)
	wantCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(60), Seq: 11}, Pos: 11, Group: groupID,
		Thread: &tid, Main: true, By: cardA, Kind: "approval", Timestamp: cardPostTS,
		Text: post.Text, Mentions: []string{ownerID, cardB}, Re: &reStr,
		Body: briefRaw(t, post.Body), State: &proto.CardState{Status: "pending"}, Env: &env,
	}
	wantTask := s.Tasks[cardTaskID]
	wantTask.Seq = 12
	wantTask.Wait = &proto.TaskWait{Kind: "approval", Msg: id(60), Who: []string{browser.ID()}, Since: cardPostTS}
	wantTask.Thread = proto.ThreadSummary{
		Count: 3, Rounds: 1,
		Last: &proto.ThreadMessage{By: cardA, Kind: "approval", Text: strings.Repeat("中", 200), Timestamp: cardPostTS},
	}
	equal(t, res, home.Result{Rows: res.Rows, Answer: store.Outcome{OK: true}})
	equal(t, cardValues(res), []any{wantCard, wantTask})

	// Minimal approval card: no stray nulls and matches the pending wire shape.
	s2, d2 := cardState(t), cardDisk{}
	minimal := proto.ApprovalBody{Task: cardTaskID, AI: cardA, Tool: "Bash", Command: "", Category: "other", Approvers: []string{browser.ID()}}
	minPost := cardApprovalPost(61, minimal)
	minPost.Mentions = []proto.ID{}
	minPost.Text = strings.Repeat("中", 200)
	minRes := cardApply(t, s2, d2, receipts{}, laptop, briefSign(laptop, minPost))
	equal(t, minRes.Answer, store.Outcome{OK: true})
	minRaw := briefRaw(t, minRes.Rows[0].Value())
	equal(t, proto.UnexpectedNulls(minRaw, "/re"), []string{})
	var typed proto.MessageRow[proto.ApprovalBody, proto.StatusOnly]
	equal(t, proto.Decode(minRaw, &typed), nil)
	equal(t, minRes.Rows[1].Value().(proto.TaskRow).Thread.Last.Text, strings.Repeat("中", 200))
}

// Main-line approvals (group session), reviewer task approvals (review
// session), and wait priority through Event.Pending.
func TestCardApprovalFromMainLineAndReviewSession(t *testing.T) {
	t.Run("main-line approval from secretary posts and decides without a task", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		body := cardApprovalBody()
		body.Task, body.AI = "", aiID
		post := cardApprovalPost(60, body)
		post.Main = false
		resPost := cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, post))
		equal(t, resPost.Answer, store.Outcome{OK: true})
		equal(t, len(resPost.Rows), 1)
		got := resPost.Rows[0].Value().(proto.MsgRow)
		equal(t, []any{got.Thread == nil, got.Main, got.By, got.State.Status}, []any{true, true, aiID, "pending"})
		raw := briefRaw(t, got)
		equal(t, proto.UnexpectedNulls(raw, "/thread", "/re"), []string{})
		var typed proto.MessageRow[proto.ApprovalBody, proto.StatusOnly]
		equal(t, proto.Decode(raw, &typed), nil)
		cardCommit(s, d, resPost)

		act := cardActEnv(browser, 70, "approve", got.ID, "")
		resAct := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, resAct.Answer, store.Outcome{OK: true})
		equal(t, len(resAct.Rows), 1)
		equal(t, resAct.Rows[0].Value().(proto.MsgRow).State.Status, "approved")
		equal(t, resAct.Items, []home.Item{{AI: aiID, Why: "decided", Msg: got.ID}})
	})

	t.Run("reviewer approval on task sets and clears wait and wakes reviewer", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		body := cardApprovalBody()
		body.AI = cardB
		post := cardApprovalPost(60, body)
		resPost := cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, post))
		equal(t, resPost.Answer, store.Outcome{OK: true})
		equal(t, len(resPost.Rows), 2)
		cardCommit(s, d, resPost)
		equal(t, s.Tasks[cardTaskID].Wait, &proto.TaskWait{Kind: "approval", Msg: id(60), Who: []string{browser.ID()}, Since: cardPostTS})
		equal(t, s.Tasks[cardTaskID].Thread.Last.By, cardB)

		act := cardActEnv(browser, 70, "deny", id(60), "")
		resAct := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, resAct.Answer, store.Outcome{OK: true})
		equal(t, len(resAct.Rows), 2)
		cardCommit(s, d, resAct)
		equal(t, s.Tasks[cardTaskID].Wait, (*proto.TaskWait)(nil))
		equal(t, resAct.Items, []home.Item{{AI: cardB, Why: "decided", Task: cardTaskID, Msg: id(60)}})
	})

	t.Run("older pending approval keeps wait and beats pending decision", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		decPost := cardDecisionPost(58, aiID, cardDecisionBody(&cardTaskID))
		decPost.Timestamp = cardPostTS - 200
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, decPost)))
		equal(t, s.Tasks[cardTaskID].Wait.Kind, "decision")

		app1 := cardApprovalPost(59, cardApprovalBody())
		app1.Timestamp = cardPostTS - 100
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, app1)))
		equal(t, s.Tasks[cardTaskID].Wait, &proto.TaskWait{Kind: "approval", Msg: id(59), Who: []string{browser.ID()}, Since: cardPostTS - 100})

		app2 := cardApprovalPost(60, cardApprovalBody())
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, app2)))
		equal(t, s.Tasks[cardTaskID].Wait, &proto.TaskWait{Kind: "approval", Msg: id(59), Who: []string{browser.ID()}, Since: cardPostTS - 100})
		equal(t, s.Tasks[cardTaskID].Thread.Count, 5)
	})
}

func TestCardPostApprovalRefusals(t *testing.T) {
	existing := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(60)}, Kind: "text"}
	for name, tc := range map[string]struct {
		by    identity.Key
		post  func(*proto.Post[proto.ApprovalBody])
		body  func(*proto.ApprovalBody)
		raw   func(string) string
		state func(*home.State)
		disk  cardDisk
		code  string
	}{
		"by a person":                       {by: browser, post: func(p *proto.Post[proto.ApprovalBody]) { p.As = "" }, code: "forbidden"},
		"malformed payload":                 {raw: func(s string) string { return strings.Replace(s, `"tool":"Bash"`, `"tool":9`, 1) }, code: "bad_request"},
		"thread nil while body.task set":    {post: func(p *proto.Post[proto.ApprovalBody]) { p.Thread = nil }, code: "bad_request"},
		"thread set while body.task empty":  {body: func(b *proto.ApprovalBody) { b.Task = "" }, post: func(p *proto.Post[proto.ApprovalBody]) { tid := proto.ID(cardTaskID); p.Thread = &tid }, code: "bad_request"},
		"thread differs from body.task":     {post: func(p *proto.Post[proto.ApprovalBody]) { other := proto.ID(cardOtherID); p.Thread = &other }, code: "bad_request"},
		"body.ai differs from as":           {post: func(p *proto.Post[proto.ApprovalBody]) { p.As = proto.ID(cardB) }, code: "bad_request"},
		"without approvers":                 {body: func(b *proto.ApprovalBody) { b.Approvers = []string{} }, code: "bad_request"},
		"duplicate mention":                 {post: func(p *proto.Post[proto.ApprovalBody]) { p.Mentions = []proto.ID{proto.ID(ownerID), proto.ID(ownerID)} }, code: "bad_request"},
		"mentioning non-member":             {post: func(p *proto.Post[proto.ApprovalBody]) { p.Mentions = []proto.ID{proto.ID(id(99))} }, code: "conflict"},
		"missing task":                      {state: func(s *home.State) { delete(s.Tasks, cardTaskID) }, code: "not_found"},
		"by third AI even with taken id":    {body: func(b *proto.ApprovalBody) { b.AI = cardC }, disk: cardDisk{id(60): existing}, code: "forbidden"},
		"by secretary on task":              {body: func(b *proto.ApprovalBody) { b.AI = aiID }, code: "forbidden"},
		"existing id on main-line approval": {body: func(b *proto.ApprovalBody) { b.Task = "" }, disk: cardDisk{id(60): existing}, code: "conflict"},
		"existing id on task approval":      {disk: cardDisk{id(60): existing}, code: "conflict"},
		"on a paused task": {state: func(s *home.State) {
			t := s.Tasks[cardTaskID]
			t.State, t.Wait = "paused", &proto.TaskWait{Kind: "manual", Since: 1}
			s.Tasks[cardTaskID] = t
		}, code: "paused"},
		"on a queued task": {state: func(s *home.State) {
			t := s.Tasks[cardTaskID]
			t.State, t.Wait = "queued", &proto.TaskWait{Kind: "depends", Since: 1}
			s.Tasks[cardTaskID] = t
		}, code: "conflict"},
		"on a failed task": {state: func(s *home.State) { t := s.Tasks[cardTaskID]; t.State = "failed"; s.Tasks[cardTaskID] = t }, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := cardState(t)
			if tc.state != nil {
				tc.state(s)
			}
			body := cardApprovalBody()
			if tc.body != nil {
				tc.body(&body)
			}
			post := cardApprovalPost(60, body)
			if tc.post != nil {
				tc.post(&post)
			}
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			raw := briefRaw(t, post)
			if tc.raw != nil {
				raw = json.RawMessage(tc.raw(string(raw)))
			}
			cardRefused(t, cardApply(t, s, tc.disk, receipts{}, by, by.Sign(string(raw))), tc.code)
		})
	}
}

// A4.1 and A4.2, home half: approving from a second device or denying updates
// the card, recomputes the task wait from Event.Pending, and queues decided.
func TestCardApproveAndDeny(t *testing.T) {
	for _, action := range []struct{ action, status string }{{"approve", "approved"}, {"deny", "denied"}} {
		t.Run(action.action, func(t *testing.T) {
			s, d := cardState(t), cardDisk{}
			posted := cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardApprovalPost(60, cardApprovalBody())))
			cardCommit(s, d, posted)
			card := d[id(60)]

			act := cardActEnv(cardPhone, 70, action.action, card.ID, "")
			res := cardApply(t, s, d, receipts{}, cardPhone, act)
			equal(t, res.Answer, store.Outcome{OK: true})

			by, at := ownerID, int64(clock)
			wantCard := card
			wantCard.Seq = 13
			wantCard.State = &proto.CardState{Status: action.status, By: &by, At: &at, Act: &act}
			wantTask := s.Tasks[cardTaskID]
			wantTask.Seq, wantTask.Wait = 14, nil
			equal(t, cardValues(res), []any{wantCard, wantTask})
			equal(t, res.Items, []home.Item{{AI: cardA, Why: "decided", Task: cardTaskID, Msg: card.ID}})

			raw := briefRaw(t, res.Rows[0].Value())
			equal(t, proto.UnexpectedNulls(raw, "/re"), []string{})
			var typed proto.MessageRow[proto.ApprovalBody, proto.ActedState]
			equal(t, proto.Decode(raw, &typed), nil)
		})
	}

	// Non-admin delegate in body.approvers decides; deciding a non-waiting card
	// or a card of a paused task leaves the task row unchanged (one row written).
	t.Run("delegate and wait recomputation", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		decPost := cardDecisionPost(57, aiID, cardDecisionBody(&cardTaskID))
		decPost.Timestamp = cardPostTS - 300
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, decPost)))

		b1 := cardApprovalBody()
		b1.Approvers = []string{guest.ID()}
		p1 := cardApprovalPost(58, b1)
		p1.Timestamp = cardPostTS - 200
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, p1)))

		p2 := cardApprovalPost(59, b1)
		p2.Timestamp = cardPostTS - 100
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, p2)))

		// Deciding the newer approval leaves A1 as the wait: only the card row changes.
		act2 := cardActEnv(guest, 71, "deny", id(59), "")
		res2 := cardApply(t, s, d, receipts{}, guest, act2)
		equal(t, len(res2.Rows), 1)
		equal(t, res2.Rows[0].Value().(proto.MsgRow).State.By, &cardGuest)
		cardCommit(s, d, res2)
		equal(t, s.Tasks[cardTaskID].Wait.Msg, id(58))

		// Deciding A1 moves the wait to the remaining pending decision D1.
		act1 := cardActEnv(guest, 72, "approve", id(58), "")
		res1 := cardApply(t, s, d, receipts{}, guest, act1)
		equal(t, len(res1.Rows), 2)
		cardCommit(s, d, res1)
		equal(t, s.Tasks[cardTaskID].Wait, &proto.TaskWait{Kind: "decision", Msg: id(57), Who: []string{browser.ID()}, Since: cardPostTS - 300})
	})

	t.Run("paused task keeps its wait (R3-1)", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardApprovalPost(60, cardApprovalBody()))))
		task := s.Tasks[cardTaskID]
		task.State, task.Wait = "paused", &proto.TaskWait{Kind: "preempted", Since: clock - 100}
		s.Tasks[cardTaskID] = task

		act := cardActEnv(browser, 73, "approve", id(60), "")
		res := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, len(res.Rows), 1)
		equal(t, res.Items, []home.Item{{AI: cardA, Why: "decided", Task: cardTaskID, Msg: id(60)}})
	})
}

func TestCardApproveAndDenyRefusals(t *testing.T) {
	at := int64(clock - 50)
	for name, tc := range map[string]struct {
		action string
		by     identity.Key
		msg    string
		extra  string
		body   func(*proto.ApprovalBody)
		card   func(*proto.MsgRow)
		code   string
	}{
		"invalid msg id":            {action: "approve", msg: "not-an-id", code: "bad_request"},
		"approve with arg":          {action: "approve", extra: `,"arg":"A"`, code: "bad_request"},
		"approve with empty arg":    {action: "approve", extra: `,"arg":""`, code: "bad_request"},
		"deny with text":            {action: "deny", extra: `,"text":"No."`, code: "bad_request"},
		"deny with empty text":      {action: "deny", extra: `,"text":""`, code: "bad_request"},
		"missing message":           {action: "approve", msg: id(90), code: "not_found"},
		"wrong card kind":           {action: "deny", card: func(m *proto.MsgRow) { m.Kind = "decision" }, code: "bad_request"},
		"by an AI on owner device":  {action: "approve", by: laptop, extra: `,"as":"` + cardA + `"`, code: "forbidden"},
		"by admin not in approvers": {action: "approve", body: func(b *proto.ApprovalBody) { b.Approvers = []string{guest.ID()} }, code: "forbidden"},
		"already approved":          {action: "approve", card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "approved"} }, code: "conflict"},
		"already denied":            {action: "deny", card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "denied"} }, code: "conflict"},
		"expired card":              {action: "approve", card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "expired", At: &at} }, code: "conflict"},
		"reassigned card":           {action: "deny", card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "reassigned", At: &at} }, code: "conflict"},
		"card with nil state":       {action: "approve", card: func(m *proto.MsgRow) { m.State = nil }, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := cardState(t), cardDisk{}
			body := cardApprovalBody()
			if tc.body != nil {
				tc.body(&body)
			}
			cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardApprovalPost(60, body))))
			card := d[id(60)]
			if tc.card != nil {
				tc.card(&card)
				d[card.ID] = card
			}
			by, msg := browser, card.ID
			if tc.by != nil {
				by = tc.by
			}
			if tc.msg != "" {
				msg = tc.msg
			}
			cardRefused(t, cardApply(t, s, d, receipts{}, by, cardActEnv(by, 70, tc.action, msg, tc.extra)), tc.code)
		})
	}
}

// Row 19: posting a decision card works in the main line (no task) and in a
// task's thread, setting wait decision only when the task is active with no wait.
func TestCardPostDecisionInMainLineAndInThread(t *testing.T) {
	t.Run("main line without options", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		body := proto.DecisionBody{Question: "Free-text question?", Options: []proto.DecisionOption{}, To: []string{browser.ID()}}
		post := cardDecisionPost(62, cardB, body)
		post.Main = false
		env := briefSign(laptop, post)
		res := cardApply(t, s, d, receipts{}, laptop, env)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
		got := res.Rows[0].Value().(proto.MsgRow)
		equal(t, []any{got.Thread == nil, got.Main, got.By, got.State.Status}, []any{true, true, cardB, "pending"})
		raw := briefRaw(t, got)
		equal(t, proto.UnexpectedNulls(raw, "/thread", "/re"), []string{})
		var typed proto.MessageRow[proto.DecisionBody, proto.StatusOnly]
		equal(t, proto.Decode(raw, &typed), nil)
	})

	t.Run("task thread sets wait and advances thread", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		post := cardDecisionPost(63, aiID, cardDecisionBody(&cardTaskID))
		res := cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, post))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 2)
		moved := res.Rows[1].Value().(proto.TaskRow)
		equal(t, moved.Wait, &proto.TaskWait{Kind: "decision", Msg: id(63), Who: []string{browser.ID()}, Since: cardPostTS})
		equal(t, moved.Thread, proto.ThreadSummary{
			Count: 3, Rounds: 1,
			Last: &proto.ThreadMessage{By: aiID, Kind: "decision", Text: "Choose a location.", Timestamp: cardPostTS},
		})
		cardCommit(s, d, res)

		// A second decision on the task leaves the first decision's wait in place.
		post2 := cardDecisionPost(64, cardA, cardDecisionBody(&cardTaskID))
		res2 := cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, post2))
		equal(t, res2.Rows[1].Value().(proto.TaskRow).Wait.Msg, id(63))
	})
}

func TestCardPostDecisionRefusals(t *testing.T) {
	existing := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(65)}, Kind: "text"}
	for name, tc := range map[string]struct {
		by    identity.Key
		task  *string
		post  func(*proto.Post[proto.DecisionBody])
		body  func(*proto.DecisionBody)
		raw   func(string) string
		state func(*home.State)
		disk  cardDisk
		code  string
	}{
		"by a person":                       {by: browser, task: &cardTaskID, post: func(p *proto.Post[proto.DecisionBody]) { p.As = "" }, code: "forbidden"},
		"malformed payload":                 {task: &cardTaskID, raw: func(s string) string { return strings.Replace(s, `"options"`, `"choices"`, 1) }, code: "bad_request"},
		"thread nil while body.task set":    {task: &cardTaskID, post: func(p *proto.Post[proto.DecisionBody]) { p.Thread = nil }, code: "bad_request"},
		"thread set while body.task nil":    {task: nil, post: func(p *proto.Post[proto.DecisionBody]) { tid := proto.ID(cardTaskID); p.Thread = &tid }, code: "bad_request"},
		"thread differs from body.task":     {task: &cardTaskID, post: func(p *proto.Post[proto.DecisionBody]) { other := proto.ID(cardOtherID); p.Thread = &other }, code: "bad_request"},
		"empty to":                          {task: &cardTaskID, body: func(b *proto.DecisionBody) { b.To = []string{} }, code: "bad_request"},
		"duplicate mention":                 {task: &cardTaskID, post: func(p *proto.Post[proto.DecisionBody]) { p.Mentions = []proto.ID{proto.ID(ownerID), proto.ID(ownerID)} }, code: "bad_request"},
		"mentioning non-member":             {task: &cardTaskID, post: func(p *proto.Post[proto.DecisionBody]) { p.Mentions = []proto.ID{proto.ID(id(99))} }, code: "conflict"},
		"missing task":                      {task: &cardTaskID, state: func(s *home.State) { delete(s.Tasks, cardTaskID) }, code: "not_found"},
		"merged task":                       {task: &cardTaskID, state: func(s *home.State) { t := s.Tasks[cardTaskID]; t.State = "merged"; s.Tasks[cardTaskID] = t }, code: "conflict"},
		"cancelled task":                    {task: &cardTaskID, state: func(s *home.State) { t := s.Tasks[cardTaskID]; t.State = "cancelled"; s.Tasks[cardTaskID] = t }, code: "conflict"},
		"existing id on main-line decision": {task: nil, disk: cardDisk{id(65): existing}, code: "conflict"},
		"existing id on task decision":      {task: &cardTaskID, disk: cardDisk{id(65): existing}, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := cardState(t)
			if tc.state != nil {
				tc.state(s)
			}
			body := cardDecisionBody(tc.task)
			if tc.body != nil {
				tc.body(&body)
			}
			post := cardDecisionPost(65, aiID, body)
			if tc.post != nil {
				tc.post(&post)
			}
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			raw := briefRaw(t, post)
			if tc.raw != nil {
				raw = json.RawMessage(tc.raw(string(raw)))
			}
			cardRefused(t, cardApply(t, s, tc.disk, receipts{}, by, by.Sign(string(raw))), tc.code)
		})
	}
}

// Row 20: act choose records choice and text, clears or recomputes the wait,
// and queues decided for the asking AI and, when another, the task's assignee.
func TestCardChoose(t *testing.T) {
	maxText := strings.Repeat("x", 1022)
	angleText := strings.Repeat("x", 1021) + "<"

	t.Run("task decision by secretary wakes secretary and assignee", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardDecisionPost(66, aiID, cardDecisionBody(&cardTaskID)))))

		act := cardActEnv(browser, 76, "choose", id(66), `,"arg":"B","text":"`+angleText+`"`)
		res := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, res.Answer, store.Outcome{OK: true})

		by, at, choice := ownerID, int64(clock), "B"
		wantCard := d[id(66)]
		wantCard.Seq = 13
		wantCard.State = &proto.CardState{Status: "decided", By: &by, At: &at, Choice: &choice, Text: &angleText, Act: &act}
		wantTask := s.Tasks[cardTaskID]
		wantTask.Seq, wantTask.Wait = 14, nil
		equal(t, cardValues(res), []any{wantCard, wantTask})
		equal(t, res.Items, []home.Item{
			{AI: aiID, Why: "decided", Task: cardTaskID, Msg: id(66)},
			{AI: cardA, Why: "decided", Task: cardTaskID, Msg: id(66)},
		})

		raw := briefRaw(t, res.Rows[0].Value())
		equal(t, proto.UnexpectedNulls(raw, "/re"), []string{})
		var typed proto.MessageRow[proto.DecisionBody, proto.DecisionDecidedState]
		equal(t, proto.Decode(raw, &typed), nil)
	})

	t.Run("task decision by assignee wakes assignee once, arg without text", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		body := cardDecisionBody(&cardTaskID)
		body.To = []string{guest.ID()}
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardDecisionPost(66, cardA, body))))

		// Non-admin addressee chooses option A without text.
		act := cardActEnv(guest, 76, "choose", id(66), `,"arg":"A"`)
		res := cardApply(t, s, d, receipts{}, guest, act)
		equal(t, res.Answer, store.Outcome{OK: true})
		gotState := res.Rows[0].Value().(proto.MsgRow).State
		equal(t, []any{*gotState.By, *gotState.Choice, gotState.Text == nil}, []any{cardGuest, "A", true})
		equal(t, res.Items, []home.Item{{AI: cardA, Why: "decided", Task: cardTaskID, Msg: id(66)}})
	})

	t.Run("multiple-choice decision answered with free text alone", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		body := cardDecisionBody(&cardTaskID)
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardDecisionPost(66, cardA, body))))

		custom := "Put the path in the status bar."
		act := cardActEnv(browser, 76, "choose", id(66), `,"text":"`+custom+`"`)
		res := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, res.Answer, store.Outcome{OK: true})

		by, at := ownerID, int64(clock)
		wantCard := d[id(66)]
		wantCard.Seq = 13
		wantCard.State = &proto.CardState{Status: "decided", By: &by, At: &at, Choice: nil, Text: &custom, Act: &act}
		wantTask := s.Tasks[cardTaskID]
		wantTask.Seq, wantTask.Wait = 14, nil
		equal(t, cardValues(res), []any{wantCard, wantTask})
		equal(t, res.Items, []home.Item{{AI: cardA, Why: "decided", Task: cardTaskID, Msg: id(66)}})

		raw := briefRaw(t, res.Rows[0].Value())
		equal(t, proto.UnexpectedNulls(raw, "/re"), []string{})
		var typed proto.MessageRow[proto.DecisionBody, proto.DecisionDecidedState]
		equal(t, proto.Decode(raw, &typed), nil)
	})

	t.Run("admin not in body.to decides free-text main-line decision of 1 KiB", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		body := proto.DecisionBody{Question: "How?", Options: []proto.DecisionOption{}, To: []string{guest.ID()}}
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardDecisionPost(66, cardB, body))))

		act := cardActEnv(browser, 76, "choose", id(66), `,"text":"`+maxText+`"`)
		res := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
		gotState := res.Rows[0].Value().(proto.MsgRow).State
		equal(t, []any{*gotState.By, gotState.Choice == nil, *gotState.Text}, []any{ownerID, true, maxText})
		equal(t, res.Items, []home.Item{{AI: cardB, Why: "decided", Msg: id(66)}})
	})

	t.Run("removed asking AI is not queued an item, and next pending decision takes wait", func(t *testing.T) {
		s, d := cardState(t), cardDisk{}
		p1 := cardDecisionPost(66, cardB, cardDecisionBody(&cardTaskID))
		p1.Timestamp = cardPostTS - 100
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, p1)))
		p2 := cardDecisionPost(67, cardA, cardDecisionBody(&cardTaskID))
		cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, p2)))
		delete(s.AIs, cardB)

		act := cardActEnv(browser, 76, "choose", id(66), `,"arg":"A"`)
		res := cardApply(t, s, d, receipts{}, browser, act)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[1].Value().(proto.TaskRow).Wait, &proto.TaskWait{Kind: "decision", Msg: id(67), Who: []string{browser.ID()}, Since: cardPostTS})
		equal(t, res.Items, []home.Item{{AI: cardA, Why: "decided", Task: cardTaskID, Msg: id(66)}})
	})
}

func TestCardChooseRefusals(t *testing.T) {
	at := int64(clock - 50)
	for name, tc := range map[string]struct {
		by       identity.Key
		freeText bool
		msg      string
		extra    string
		card     func(*proto.MsgRow)
		code     string
	}{
		"invalid msg id":                 {msg: "bad-id", extra: `,"arg":"A"`, code: "bad_request"},
		"neither arg nor text":           {extra: "", code: "bad_request"},
		"neither arg nor text free-text": {freeText: true, extra: "", code: "bad_request"},
		"empty arg with text":            {extra: `,"arg":"","text":"ok"`, code: "bad_request"},
		"empty text with arg":            {extra: `,"arg":"A","text":""`, code: "bad_request"},
		"empty text without arg":         {extra: `,"text":""`, code: "bad_request"},
		"text of 1025 bytes":             {extra: `,"arg":"A","text":"` + strings.Repeat("x", 1023) + `"`, code: "bad_request"},
		"text holding < of 1025 bytes":   {extra: `,"arg":"A","text":"` + strings.Repeat("x", 1022) + `<"`, code: "bad_request"},
		"missing message":                {msg: id(91), extra: `,"arg":"A"`, code: "not_found"},
		"wrong card kind":                {extra: `,"arg":"A"`, card: func(m *proto.MsgRow) { m.Kind = "approval" }, code: "bad_request"},
		"arg not in options":             {extra: `,"arg":"C"`, code: "bad_request"},
		"arg not in options with text":   {extra: `,"arg":"C","text":"Custom"`, code: "bad_request"},
		"arg when options empty":         {freeText: true, extra: `,"arg":"A","text":"Custom"`, code: "bad_request"},
		"arg alone when options empty":   {freeText: true, extra: `,"arg":"A"`, code: "bad_request"},
		"by an AI on owner device":       {by: laptop, extra: `,"arg":"A","as":"` + cardA + `"`, code: "forbidden"},
		"by non-admin not in body.to":    {by: guest, extra: `,"arg":"A"`, code: "forbidden"},
		"already decided":                {extra: `,"arg":"A"`, card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "decided"} }, code: "conflict"},
		"expired decision":               {extra: `,"arg":"A"`, card: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "expired", At: &at} }, code: "conflict"},
		"nil state":                      {extra: `,"arg":"A"`, card: func(m *proto.MsgRow) { m.State = nil }, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := cardState(t), cardDisk{}
			body := cardDecisionBody(&cardTaskID)
			if tc.freeText {
				body.Options, body.Recommended = []proto.DecisionOption{}, nil
			}
			cardCommit(s, d, cardApply(t, s, d, receipts{}, laptop, briefSign(laptop, cardDecisionPost(66, aiID, body))))
			card := d[id(66)]
			if tc.card != nil {
				tc.card(&card)
				d[card.ID] = card
			}
			by, msg := browser, card.ID
			if tc.by != nil {
				by = tc.by
			}
			if tc.msg != "" {
				msg = tc.msg
			}
			cardRefused(t, cardApply(t, s, d, receipts{}, by, cardActEnv(by, 76, "choose", msg, tc.extra)), tc.code)
		})
	}
}
