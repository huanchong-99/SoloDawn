package home_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	revAssignee  = id(60)
	revReviewer  = id(61)
	revOtherAI   = id(62)
	revGuestMem  = id(63)
	revTaskID    = id(64)
	revOtherTask = id(65)
	revBriefID   = id(66)
	revReport    = id(67) // the report under review
	revEarlier   = id(68) // a report before it
	revReviewBox = device(6)
)

const revCmdTS = clock - 500

type revDisk struct {
	msgs    map[string]proto.MsgRow
	pending []proto.MsgRow
	queued  []store.Item
}

func (d revDisk) Msg(id string) (proto.MsgRow, bool) {
	m, ok := d.msgs[id]
	return m, ok
}

func (d revDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d revDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (d revDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d revDisk) Pending() []proto.MsgRow                   { return d.pending }
func (d revDisk) Queued() []store.Item                      { return d.queued }
func (d revDisk) OtherMember(string) bool                   { return false }

func revEmptyDisk() revDisk {
	return revDisk{msgs: map[string]proto.MsgRow{}}
}

// revState is a group with a secretary, three working AIs, each in a folder
// of its own, and one active task at step work.
func revState(t *testing.T) (*home.State, revDisk) {
	t.Helper()
	s := empty(t)
	s.Logs[browser.ID()] = logOf(t, browser, node, laptop, revReviewBox)
	s.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: groupID},
		Secretary: aiID, Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	s.People[ownerID] = person(ownerID, browser, "owner")
	s.People[revGuestMem] = person(revGuestMem, guest, "member")
	s.AIs[aiID] = ai(aiID, laptop)
	s.AIs[revAssignee] = ai(revAssignee, laptop)
	s.AIs[revReviewer] = ai(revReviewer, revReviewBox)
	s.AIs[revOtherAI] = ai(revOtherAI, laptop)
	s.Tasks[revTaskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: revTaskID, Seq: 8},
		Group:     groupID, Num: 12, Title: "Show connection paths",
		Origin: "brief", Brief: revBriefID,
		State: "active", Step: "work", Assignee: revAssignee, Reviewer: revReviewer,
		Pass: 90, Depends: []string{},
		Thread: proto.ThreadSummary{Count: 2, Rounds: 1},
		Stats:  proto.TaskStats{Started: clock - 5000}, Created: clock - 5000,
	}
	s.Seq, s.Num = 10, 12
	d := revEmptyDisk()
	d.msgs[revBriefID] = proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: revBriefID},
		Kind:      "brief", Body: briefRaw(t, proto.BriefBody{Requester: ownerID}),
	}
	return s, d
}

// revAtReview puts the task of revState at step review with revReport.
func revAtReview(s *home.State, change func(*proto.TaskRow)) {
	task := s.Tasks[revTaskID]
	task.Step, task.Report = "review", revReport
	change(&task)
	s.Tasks[revTaskID] = task
}

func revApply(t *testing.T, s *home.State, d revDisk, by identity.Key, payload any) home.Result {
	t.Helper()
	env := briefSign(by, payload)
	res, err := home.Commands.Apply(s, d, receipts{}, env, now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func revApplyRaw(t *testing.T, s *home.State, d revDisk, by identity.Key, raw string) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, receipts{}, by.Sign(raw), now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func revStr(v string) *string { return &v }
func revInt(v int) *int       { return &v }
func revBool(v bool) *bool    { return &v }

// revDerivedID is the id of the system message that a review of nonce n
// writes for that code.
func revDerivedID(code string, n byte) string {
	sum := sha256.Sum256([]byte("SoloDawn " + code + " " + id(n)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func revAssertNoNulls(t *testing.T, res home.Result) {
	t.Helper()
	for _, r := range res.Rows {
		raw, err := json.Marshal(r.Value())
		if err != nil {
			t.Fatal(err)
		}
		allowed := []string{"/re", "/state"}
		if _, isTask := r.Value().(proto.TaskRow); isTask {
			allowed = []string{"/wait", "/stats/ended"}
		}
		equal(t, proto.UnexpectedNulls(raw, allowed...), []string{})
	}
}

func revReportPayload(n byte, as, text string) proto.Post[proto.EmptyBody] {
	tid := proto.ID(revTaskID)
	return proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: revCmdTS, Group: proto.ID(groupID), As: proto.ID(as)},
		Kind:        "report", Thread: &tid, Text: text, Mentions: []proto.ID{},
	}
}

func revReviewPayload(n byte, as, report string, score int, items []proto.ReviewItem) proto.Post[proto.ReviewSigned] {
	tid := proto.ID(revTaskID)
	return proto.Post[proto.ReviewSigned]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: revCmdTS, Group: proto.ID(groupID), As: proto.ID(as)},
		Kind:        "review", Thread: &tid, Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: tid, Report: proto.ID(report), Score: score, Items: items},
	}
}

func TestReviewCmdReportIsTheSubmission(t *testing.T) {
	t.Run("the report becomes the task's, retires the review of an earlier one and reaches the reviewer", func(t *testing.T) {
		s, d := revState(t)
		task := s.Tasks[revTaskID]
		task.Step, task.Report, task.Failures = "review", revEarlier, 2
		task.Wait = &proto.TaskWait{Kind: "approval", Msg: id(83), Who: []string{"u-app"}, Since: 25}
		s.Tasks[revTaskID] = task
		tid, other, earlier := revTaskID, revOtherTask, revEarlier
		d.queued = []store.Item{
			{ID: 1, Why: "review"},
			{ID: 2, Task: &other, Why: "review"},
			{ID: 3, Task: &tid, Why: "assign"},
			{ID: 4, Task: &tid, Why: "review", AI: revReviewer, Msg: &earlier},
		}

		p := revReportPayload(72, revAssignee, "Path events are sent on every switch. Ran the tests: 42 passed.")
		env := briefSign(laptop, p)
		res, err := home.Commands.Apply(s, d, receipts{}, env, now)
		equal(t, err, nil)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{4})
		revAssertNoNulls(t, res)

		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(72), Seq: 11}, Pos: 11, Group: groupID,
			Thread: &tid, By: revAssignee, Kind: "report", Timestamp: revCmdTS, Text: p.Text,
			Mentions: []string{}, Body: json.RawMessage(`{}`), Env: &env,
		}
		wantTask := task
		wantTask.Seq, wantTask.Report, wantTask.Failures = 12, id(72), 0
		wantTask.Thread = proto.ThreadSummary{
			Count: 3, Rounds: 1,
			Last: &proto.ThreadMessage{By: revAssignee, Kind: "report", Text: p.Text, Timestamp: revCmdTS},
		}
		equal(t, briefRows(res), []any{wantMsg, wantTask})
		equal(t, res.Items, []home.Item{{AI: revReviewer, Why: "review", Task: revTaskID, Msg: id(72)}})
	})

	t.Run("a first report takes the task from work to review", func(t *testing.T) {
		s, d := revState(t)
		res := revApply(t, s, d, laptop, revReportPayload(73, revAssignee, "Done."))
		equal(t, res.Answer, store.Outcome{OK: true})
		got := res.Rows[1].Value().(proto.TaskRow)
		equal(t, []string{got.Step, got.Report}, []string{"review", id(73)})
	})

	t.Run("what the assignee puts into the body is not read", func(t *testing.T) {
		s, d := revState(t)
		raw := strings.Replace(briefSign(laptop, revReportPayload(74, revAssignee, "Done.")).Payload, `"body":{}`, `"body":{"commit":"c1"}`, 1)
		res := revApplyRaw(t, s, d, laptop, raw)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[0].Value().(proto.MsgRow).Body, json.RawMessage(`{}`))
	})

	t.Run("without a reviewer the task waits for one and the secretary is told when there is one", func(t *testing.T) {
		for _, hasSec := range []bool{true, false} {
			s, d := revState(t)
			if !hasSec {
				s.Group.Secretary = ""
			}
			task := s.Tasks[revTaskID]
			task.Reviewer = ""
			s.Tasks[revTaskID] = task

			res := revApply(t, s, d, laptop, revReportPayload(75, revAssignee, "Done."))
			equal(t, res.Answer, store.Outcome{OK: true})
			got := res.Rows[1].Value().(proto.TaskRow)
			equal(t, got.Wait, &proto.TaskWait{Kind: "reviewer", Since: clock})
			if hasSec {
				equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: revTaskID, Code: "no_reviewer"}})
			} else {
				equal(t, len(res.Items), 0)
			}
		}
	})

	t.Run("while paused the report is taken, the wait stays and nobody is woken", func(t *testing.T) {
		s, d := revState(t)
		task := s.Tasks[revTaskID]
		task.State, task.Wait = "paused", &proto.TaskWait{Kind: "manual", Since: clock - 100, By: ownerID}
		s.Tasks[revTaskID] = task
		res := revApply(t, s, d, laptop, revReportPayload(76, revAssignee, "Done."))
		equal(t, res.Answer, store.Outcome{OK: true})
		got := res.Rows[1].Value().(proto.TaskRow)
		equal(t, []string{got.State, got.Step, got.Report}, []string{"paused", "review", id(76)})
		equal(t, got.Wait, task.Wait)
		equal(t, len(res.Items), 0)
	})
}

func TestReviewCmdReportRefusals(t *testing.T) {
	inState := func(state string) func(*home.State) {
		return func(s *home.State) {
			row := s.Tasks[revTaskID]
			row.State = state
			s.Tasks[revTaskID] = row
		}
	}
	for name, tc := range map[string]struct {
		by    identity.Key
		patch func(*proto.Post[proto.EmptyBody])
		state func(*home.State)
		code  string
	}{
		"by a person": {
			by:    browser,
			patch: func(p *proto.Post[proto.EmptyBody]) { p.As = "" },
			code:  "forbidden",
		},
		"by the reviewer": {
			by:    revReviewBox,
			patch: func(p *proto.Post[proto.EmptyBody]) { p.As = proto.ID(revReviewer) },
			code:  "forbidden",
		},
		"by a third working AI": {
			patch: func(p *proto.Post[proto.EmptyBody]) { p.As = proto.ID(revOtherAI) },
			code:  "forbidden",
		},
		"by the secretary": {
			patch: func(p *proto.Post[proto.EmptyBody]) { p.As = proto.ID(aiID) },
			code:  "forbidden",
		},
		"in the main line": {
			patch: func(p *proto.Post[proto.EmptyBody]) { p.Thread = nil },
			code:  "bad_request",
		},
		"unknown task": {
			patch: func(p *proto.Post[proto.EmptyBody]) {
				unk := proto.ID(id(99))
				p.Thread = &unk
			},
			code: "not_found",
		},
		"mentions non-member": {
			patch: func(p *proto.Post[proto.EmptyBody]) { p.Mentions = []proto.ID{proto.ID(id(98))} },
			code:  "conflict",
		},
		"queued task": {state: inState("queued"), code: "conflict"},
		"failed task": {state: inState("failed"), code: "conflict"},
		"done task":   {state: inState("done"), code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := revState(t)
			if tc.state != nil {
				tc.state(s)
			}
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			p := revReportPayload(77, revAssignee, "Done.")
			if tc.patch != nil {
				tc.patch(&p)
			}
			res := revApply(t, s, d, by, p)
			briefRefused(t, res, tc.code)
			equal(t, len(res.Rows), 0)
		})
	}
}

func TestReviewCmdPostPassFailVetoAndSixReviewStop(t *testing.T) {
	t.Run("a review at the exact threshold ends the task: review, task_done, the task, its expired cards", func(t *testing.T) {
		s, d := revState(t)
		revAtReview(s, func(*proto.TaskRow) {})
		tid, other := revTaskID, revOtherTask
		approval := tskApprovalCard(id(81), tid, 15, "u-app")
		d.pending = []proto.MsgRow{
			approval,
			{RowHeader: proto.RowHeader{ID: id(82)}, Kind: "decision", Timestamp: 20,
				Body: briefRaw(t, proto.DecisionBody{Task: &other, To: []string{"u-dec"}})},
		}

		reID := proto.ID(id(55))
		items := []proto.ReviewItem{
			{Text: "Clean design", Veto: false, File: revStr("path.go"), Line: revInt(1)},
			{Text: "Line only", Veto: false, Line: revInt(42)},
		}
		p := revReviewPayload(90, revReviewer, revReport, 90, items)
		p.Text = "Covers every transition."
		p.Mentions = []proto.ID{proto.ID(ownerID), proto.ID(revOtherAI)}
		p.Re = &reID
		env := briefSign(revReviewBox, p)

		res, err := home.Commands.Apply(s, d, receipts{}, env, now)
		equal(t, err, nil)
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(true), Round: 1})
		revAssertNoNulls(t, res)

		reStr, sysID, ended := id(55), revDerivedID("task_done", 90), int64(clock)
		wantBody := proto.ReviewBody{
			Task: revTaskID, Num: 12, Title: "Show connection paths", Report: revReport,
			Round: 1, Score: 90, Pass: 90, OK: true, Items: items,
		}
		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(90), Seq: 11}, Pos: 11, Group: groupID,
			Thread: &tid, By: revReviewer, Kind: "review", Timestamp: revCmdTS, Text: p.Text,
			Mentions: []string{ownerID, revOtherAI}, Re: &reStr, Body: briefRaw(t, wantBody), Env: &env,
		}
		wantSys := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: sysID, Seq: 12}, Pos: 12, Group: groupID,
			Thread: &tid, By: "system", Kind: "system", Timestamp: clock, Mentions: []string{},
			Body: briefRaw(t, proto.TaskDoneBody{SystemCode: proto.SystemCode{Code: "task_done"}, Task: revTaskID}),
		}
		wantTask := s.Tasks[revTaskID]
		wantTask.Seq, wantTask.State, wantTask.Stats.Ended = 13, "done", &ended
		wantTask.Review = &proto.Review{Round: 1, Failed: 0, Score: 90, OK: true, By: revReviewer, Report: revReport, At: clock}
		wantTask.Thread = proto.ThreadSummary{
			Count: 4, Rounds: 2,
			Last: &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock},
		}
		wantCard := approval
		wantCard.Seq, wantCard.State = 14, &proto.CardState{Status: "expired", At: &ended}
		equal(t, briefRows(res), []any{wantMsg, wantSys, wantTask, wantCard})
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: revTaskID, Msg: sysID, Code: "task_done"}})
	})

	t.Run("the pass score is the task's, whatever it is", func(t *testing.T) {
		for _, score := range []int{0, 100} {
			s, d := revState(t)
			revAtReview(s, func(task *proto.TaskRow) { task.Pass = score })
			res := revApply(t, s, d, revReviewBox, revReviewPayload(91, revReviewer, revReport, score, []proto.ReviewItem{}))
			equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(true), Round: 1})
			revAssertNoNulls(t, res)
			equal(t, res.Rows[2].Value().(proto.TaskRow).State, "done")
		}
	})

	t.Run("a score below pass returns the task to work, queues reviewed and cuts the thread preview", func(t *testing.T) {
		for _, tc := range []struct{ text, preview string }{
			{strings.Repeat("评", 205), strings.Repeat("评", 200)},
			{strings.Repeat("x", 200), strings.Repeat("x", 200)},
		} {
			s, d := revState(t)
			revAtReview(s, func(*proto.TaskRow) {})
			p := revReviewPayload(92, revReviewer, revReport, 89, []proto.ReviewItem{{Text: "One point short"}})
			p.Text = tc.text
			res := revApply(t, s, d, revReviewBox, p)
			equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 1})
			equal(t, len(res.Rows), 2)
			gotTask := res.Rows[1].Value().(proto.TaskRow)
			equal(t, []string{gotTask.State, gotTask.Step}, []string{"active", "work"})
			equal(t, gotTask.Review, &proto.Review{Round: 1, Failed: 1, Score: 89, OK: false, By: revReviewer, Report: revReport, At: clock})
			equal(t, gotTask.Thread.Last.Text, tc.preview)
			equal(t, res.Items, []home.Item{{AI: revAssignee, Why: "reviewed", Task: revTaskID, Msg: id(92)}})
		}
	})

	t.Run("veto forces score to zero and ok false even when pass is zero", func(t *testing.T) {
		s, d := revState(t)
		revAtReview(s, func(task *proto.TaskRow) { task.Pass = 0 })

		items := []proto.ReviewItem{{Text: "Looks clean"}, {Text: "Fake test", Veto: true}}
		res := revApply(t, s, d, revReviewBox, revReviewPayload(93, revReviewer, revReport, 95, items))
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 1})
		var body proto.ReviewBody
		equal(t, json.Unmarshal(res.Rows[0].Value().(proto.MsgRow).Body, &body), nil)
		equal(t, body.Score, 0)
		equal(t, body.OK, false)
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotTask.Review.Score, 0)
		equal(t, gotTask.Review.OK, false)
		equal(t, gotTask.Step, "work")
	})

	t.Run("fifth failed review returns to work; sixth failed review stops task", func(t *testing.T) {
		s, d := revState(t)
		revAtReview(s, func(task *proto.TaskRow) {
			task.Review = &proto.Review{Round: 4, Failed: 4, Score: 70, By: revReviewer, Report: revEarlier, At: clock - 900}
		})

		fifth := revApply(t, s, d, revReviewBox, revReviewPayload(94, revReviewer, revReport, 86, []proto.ReviewItem{{Text: "Round 5"}}))
		equal(t, fifth.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 5})
		s.Commit(fifth)
		equal(t, s.Tasks[revTaskID].State, "active")
		equal(t, s.Tasks[revTaskID].Step, "work")
		equal(t, fifth.Items, []home.Item{{AI: revAssignee, Why: "reviewed", Task: revTaskID, Msg: id(94)}})

		again := revApply(t, s, d, laptop, revReportPayload(100, revAssignee, "Fixed."))
		equal(t, again.Answer, store.Outcome{OK: true})
		s.Commit(again)

		sixth := revApply(t, s, d, revReviewBox, revReviewPayload(95, revReviewer, id(100), 86, []proto.ReviewItem{{Text: "Round 6"}}))
		equal(t, sixth.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 6})
		revAssertNoNulls(t, sixth)
		equal(t, len(sixth.Rows), 3)

		revRow := sixth.Rows[0].Value().(proto.MsgRow)
		sysRow := sixth.Rows[1].Value().(proto.MsgRow)
		gotTask := sixth.Rows[2].Value().(proto.TaskRow)
		sysID := revDerivedID("task_failed", 95)

		equal(t, revRow.ID, id(95))
		equal(t, sysRow.ID, sysID)
		equal(t, sysRow.Mentions, []string{ownerID})
		equal(t, sysRow.Body, briefRaw(t, proto.TaskFailedBody{
			SystemCode: proto.SystemCode{Code: "task_failed"}, Task: revTaskID, Reason: "review_rounds",
		}))
		equal(t, gotTask.State, "failed")
		equal(t, gotTask.Step, "review")
		equal(t, gotTask.Wait, (*proto.TaskWait)(nil))
		equal(t, gotTask.Review.Failed, 6)
		equal(t, gotTask.Thread.Count, 6)
		equal(t, gotTask.Thread.Last, &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock})
		equal(t, sixth.Items, []home.Item{{AI: aiID, Why: "notice", Task: revTaskID, Msg: sysID, Code: "task_failed"}})
	})

	t.Run("task_failed mentions the requester who is a member, and nobody when brief and owner are gone", func(t *testing.T) {
		s, d := revState(t)
		d.msgs[revBriefID] = proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: revBriefID},
			Kind:      "brief", Body: briefRaw(t, proto.BriefBody{Requester: revGuestMem}),
		}
		failing := func(task *proto.TaskRow) {
			task.Review = &proto.Review{Round: 5, Failed: 5, Score: 50, By: revReviewer, Report: revEarlier, At: clock - 900}
		}
		revAtReview(s, failing)
		res := revApply(t, s, d, revReviewBox, revReviewPayload(96, revReviewer, revReport, 50, []proto.ReviewItem{}))
		equal(t, res.Rows[1].Value().(proto.MsgRow).Mentions, []string{revGuestMem, ownerID})

		s, d = revState(t)
		delete(s.People, ownerID)
		s.People[revGuestMem] = person(revGuestMem, guest, "owner")
		s.AIs[revReviewer] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: revReviewer}, User: guest.ID()},
			Host:      guest.ID(),
		}
		s.Group.Secretary = ""
		revAtReview(s, func(task *proto.TaskRow) {
			failing(task)
			task.Origin, task.Brief = "direct", ""
		})

		res = revApply(t, s, d, guest, revReviewPayload(96, revReviewer, revReport, 50, []proto.ReviewItem{}))
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 6})
		revAssertNoNulls(t, res)
		equal(t, res.Rows[1].Value().(proto.MsgRow).Mentions, []string{})
		equal(t, len(res.Items), 0)
	})
}

// A passed review is where a task ends, so it is where the tasks that waited
// for it start: the one that depends on it, and the one behind it in its folder.
func TestReviewCmdPassStartsTheTasksThatWaited(t *testing.T) {
	s, d := revState(t)
	revAtReview(s, func(*proto.TaskRow) {})
	queued := func(taskID string, num int, assignee, wait string, depends ...string) proto.TaskRow {
		return proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: taskID}, Group: groupID, Num: num, Origin: "brief", Brief: revBriefID,
			State: "queued", Step: "work", Assignee: assignee, Reviewer: revReviewer, Depends: append([]string{}, depends...),
			Wait: &proto.TaskWait{Kind: wait, Since: clock - 50}, Stats: proto.TaskStats{Started: clock - 50}, Created: clock - 50,
		}
	}
	dependent, behind, later := id(101), id(102), id(103)
	s.Tasks[dependent] = queued(dependent, 15, revOtherAI, "depends", revTaskID)
	s.Tasks[behind] = queued(behind, 14, revAssignee, "busy")
	s.Tasks[later] = queued(later, 16, revAssignee, "depends", revTaskID)
	s.Num = 16

	res := revApply(t, s, d, revReviewBox, revReviewPayload(104, revReviewer, revReport, 95, []proto.ReviewItem{}))
	equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(true), Round: 1})
	revAssertNoNulls(t, res)
	equal(t, len(res.Rows), 6)
	started := func(row any) []any {
		task := row.(proto.TaskRow)
		return []any{task.ID, task.State, task.Wait, task.Stats.Started}
	}
	rows := briefRows(res)
	equal(t, started(rows[3]), []any{behind, "active", (*proto.TaskWait)(nil), int64(clock)})
	equal(t, started(rows[4]), []any{dependent, "active", (*proto.TaskWait)(nil), int64(clock)})
	equal(t, started(rows[5]), []any{later, "queued", &proto.TaskWait{Kind: "busy", Since: clock}, int64(clock - 50)})
	equal(t, res.Items, []home.Item{
		{AI: aiID, Why: "notice", Task: revTaskID, Msg: revDerivedID("task_done", 104), Code: "task_done"},
		{AI: revAssignee, Why: "assign", Task: behind, Msg: behind},
		{AI: revOtherAI, Why: "assign", Task: dependent, Msg: dependent},
	})

	t.Run("a review that mentions an AI counts its round on the reviewed task alone", func(t *testing.T) {
		p := revReviewPayload(105, revReviewer, revReport, 95, []proto.ReviewItem{})
		p.Mentions = []proto.ID{proto.ID(revOtherAI)}
		rows := briefRows(revApply(t, s, d, revReviewBox, p))
		equal(t, rows[2].(proto.TaskRow).Thread.Rounds, 2)
		equal(t, rows[3].(proto.TaskRow).Thread.Rounds, 0)
		equal(t, rows[4].(proto.TaskRow).ID, dependent)
	})

	t.Run("a review that fails starts nothing", func(t *testing.T) {
		res := revApply(t, s, d, revReviewBox, revReviewPayload(106, revReviewer, revReport, 10, []proto.ReviewItem{}))
		equal(t, len(res.Rows), 2)
		equal(t, len(res.Items), 1)
	})
}

func TestReviewCmdPostRefusals(t *testing.T) {
	otherID := proto.ID(revOtherTask)
	for name, tc := range map[string]struct {
		by    identity.Key
		raw   func(string) string
		patch func(*proto.Post[proto.ReviewSigned])
		state func(*proto.TaskRow)
		code  string
	}{
		"by a person": {
			by:    browser,
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.As = "" },
			code:  "forbidden",
		},
		"by the secretary": {
			by:    laptop,
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.As = proto.ID(aiID) },
			code:  "forbidden",
		},
		"by the assignee": {
			by:    laptop,
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.As = proto.ID(revAssignee) },
			code:  "forbidden",
		},
		"by a third working AI": {
			by:    laptop,
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.As = proto.ID(revOtherAI) },
			code:  "forbidden",
		},
		"malformed body": {
			raw:  func(p string) string { return strings.Replace(p, `"score":95`, `"score":"high"`, 1) },
			code: "bad_request",
		},
		"a report that is no id": {
			raw:  func(p string) string { return strings.Replace(p, revReport, "c1", 1) },
			code: "bad_request",
		},
		"null thread": {
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.Thread = nil },
			code:  "bad_request",
		},
		"thread differs from body task": {
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.Thread = &otherID },
			code:  "bad_request",
		},
		"duplicate mention": {
			patch: func(p *proto.Post[proto.ReviewSigned]) {
				p.Mentions = []proto.ID{proto.ID(ownerID), proto.ID(revOtherAI), proto.ID(ownerID)}
			},
			code: "bad_request",
		},
		"unknown task": {
			patch: func(p *proto.Post[proto.ReviewSigned]) {
				unk := proto.ID(id(99))
				p.Thread, p.Body.Task = &unk, unk
			},
			code: "not_found",
		},
		"paused task": {
			state: func(row *proto.TaskRow) {
				row.State, row.Wait = "paused", &proto.TaskWait{Kind: "manual", Since: clock - 100}
			},
			code: "paused",
		},
		"a report other than the one under review": {
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.Body.Report = proto.ID(revEarlier) },
			code:  "conflict",
		},
		"outside step review": {
			state: func(row *proto.TaskRow) { row.Step = "work" },
			code:  "conflict",
		},
		"a task that is done": {
			state: func(row *proto.TaskRow) { row.State = "done" },
			code:  "conflict",
		},
		"mentions non-member": {
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.Mentions = []proto.ID{proto.ID(id(98))} },
			code:  "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := revState(t)
			revAtReview(s, func(row *proto.TaskRow) {
				if tc.state != nil {
					tc.state(row)
				}
			})
			by := revReviewBox
			if tc.by != nil {
				by = tc.by
			}
			p := revReviewPayload(97, revReviewer, revReport, 95, []proto.ReviewItem{{Text: "Good"}})
			if tc.patch != nil {
				tc.patch(&p)
			}
			if tc.raw != nil {
				briefRefused(t, revApplyRaw(t, s, d, by, tc.raw(briefSign(by, p).Payload)), tc.code)
				return
			}
			briefRefused(t, revApply(t, s, d, by, p), tc.code)
		})
	}
}
