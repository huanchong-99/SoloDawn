package home_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func revEmptyDisk() revDisk {
	return revDisk{msgs: map[string]proto.MsgRow{}}
}

func revState(t *testing.T, forge string) (*home.State, revDisk) {
	t.Helper()
	s := empty(t)
	s.Logs[browser.ID()] = logOf(t, browser, node, laptop, revReviewBox)
	s.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: groupID},
		Forge:     forge, Secretary: aiID, Pass: 90, PassFoundation: 70,
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
		Slug: "show-connection-paths", Origin: "brief", Brief: revBriefID,
		State: "active", Step: "local_check", Assignee: revAssignee, Reviewer: revReviewer,
		Pass: 90, Depends: []string{}, Branch: "task/12-show-connection-paths",
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

func revDerivedID(n byte) string {
	sum := sha256.Sum256([]byte("SoloDawn task failed " + id(n)))
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

func revCheckedPayload(n byte, op string, ok bool, commit string, items []proto.CheckItem) proto.TaskChecked {
	return proto.TaskChecked{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(n)), Timestamp: revCmdTS, Group: proto.ID(groupID)},
			Op:          op, ID: proto.ID(revTaskID),
		},
		OK: ok, Items: items, Commit: commit,
	}
}

func revSubmittedPayload(n byte, commit string, pr int, url string, added, removed int) proto.TaskSubmitted {
	return proto.TaskSubmitted{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(n)), Timestamp: revCmdTS, Group: proto.ID(groupID)},
			Op:          "submitted", ID: proto.ID(revTaskID),
		},
		Commit: commit, PR: pr, PRURL: url,
		Stats: proto.DiffStats{Added: added, Removed: removed},
	}
}

func revReviewPayload(n byte, as, commit string, score int, items []proto.ReviewItem) proto.Post[proto.ReviewSigned] {
	tid := proto.ID(revTaskID)
	return proto.Post[proto.ReviewSigned]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: revCmdTS, Group: proto.ID(groupID), As: proto.ID(as)},
		Kind:        "review", Thread: &tid, Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: tid, Commit: commit, Score: score, Items: items},
	}
}

func TestReviewCmdCheckedRecordsChecksAndThreadMessage(t *testing.T) {
	t.Run("passing checks with details and paused task", func(t *testing.T) {
		s, d := revState(t, "github")
		items := []proto.CheckItem{
			{Name: "go test ./...", OK: true, Detail: revStr("42 tests")},
			{Name: "secrets", OK: true},
		}
		res := revApply(t, s, d, laptop, revCheckedPayload(70, "checked", true, "9f2c1ab", items))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Items), 0)
		equal(t, len(res.Retire), 0)
		revAssertNoNulls(t, res)

		tid := revTaskID
		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(70), Seq: 11}, Pos: 11, Group: groupID,
			Thread: &tid, By: "system", Kind: "checks", Timestamp: clock, Mentions: []string{},
			Body: briefRaw(t, proto.ChecksBody{Stage: "local", OK: true, Commit: "9f2c1ab", Items: items}),
		}
		wantTask := s.Tasks[revTaskID]
		wantTask.Seq = 12
		wantTask.Checks.Local = &proto.Check{OK: true, At: clock, Commit: "9f2c1ab"}
		wantTask.Thread = proto.ThreadSummary{
			Count: 3, Rounds: 1,
			Last: &proto.ThreadMessage{By: "system", Kind: "checks", Text: "", Timestamp: clock},
		}
		equal(t, briefRows(res), []any{wantMsg, wantTask})
	})

	t.Run("failing checks with empty item list while paused at review", func(t *testing.T) {
		s, d := revState(t, "none")
		task := s.Tasks[revTaskID]
		task.State, task.Step, task.Wait = "paused", "review", &proto.TaskWait{Kind: "manual", Since: clock - 100}
		s.Tasks[revTaskID] = task

		res := revApply(t, s, d, laptop, revCheckedPayload(71, "checked", false, "c2", []proto.CheckItem{}))
		equal(t, res.Answer, store.Outcome{OK: true})
		revAssertNoNulls(t, res)
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotTask.Checks.Local, &proto.Check{OK: false, At: clock, Commit: "c2"})
		equal(t, gotTask.State, "paused")
	})
}

func TestReviewCmdCheckedRefusals(t *testing.T) {
	validItems := []proto.CheckItem{{Name: "go test ./...", OK: true}}
	for name, tc := range map[string]struct {
		by    identity.Key
		raw   string
		patch func(*proto.TaskChecked)
		state func(*home.State)
		code  string
	}{
		"malformed payload": {
			raw: fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"checked","id":%q,"ok":"yes","items":[],"commit":"c1"}`,
				id(70), revCmdTS, groupID, revTaskID),
			code: "bad_request",
		},
		"unknown task": {
			patch: func(p *proto.TaskChecked) { p.ID = proto.ID(id(99)) },
			code:  "not_found",
		},
		"by owner browser instead of host": {
			by:   browser,
			code: "forbidden",
		},
		"by another host node": {
			by:   revReviewBox,
			code: "forbidden",
		},
		"by AI instead of host": {
			patch: func(p *proto.TaskChecked) { p.As = proto.ID(revAssignee) },
			code:  "forbidden",
		},
		"at step branch": {
			state: func(s *home.State) {
				row := s.Tasks[revTaskID]
				row.Step = "branch"
				s.Tasks[revTaskID] = row
			},
			code: "conflict",
		},
		"at step merge": {
			state: func(s *home.State) {
				row := s.Tasks[revTaskID]
				row.Step = "merge"
				s.Tasks[revTaskID] = row
			},
			code: "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := revState(t, "github")
			if tc.state != nil {
				tc.state(s)
			}
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			if tc.raw != "" {
				briefRefused(t, revApplyRaw(t, s, d, by, tc.raw), tc.code)
				return
			}
			p := revCheckedPayload(70, "checked", true, "c1", validItems)
			if tc.patch != nil {
				tc.patch(&p)
			}
			briefRefused(t, revApply(t, s, d, by, p), tc.code)
		})
	}
}

func TestReviewCmdSubmittedWithAndWithoutRemote(t *testing.T) {
	t.Run("with remote retiring review items and recomputing card waits", func(t *testing.T) {
		s, d := revState(t, "github")
		task := s.Tasks[revTaskID]
		task.Checks.Local = &proto.Check{OK: true, At: clock - 200, Commit: "c1"}
		task.Wait = &proto.TaskWait{Kind: "conflict", Since: clock - 300}
		s.Tasks[revTaskID] = task

		tid, other := revTaskID, revOtherTask
		d.queued = []store.Item{
			{ID: 1, Why: "review"},
			{ID: 2, Task: &other, Why: "review"},
			{ID: 3, Task: &tid, Why: "assign"},
			{ID: 4, Task: &tid, Why: "review"},
		}
		d.pending = []proto.MsgRow{
			{RowHeader: proto.RowHeader{ID: id(80)}, Kind: "brief", Body: briefRaw(t, proto.BriefBody{Requester: ownerID})},
			{RowHeader: proto.RowHeader{ID: id(81)}, Kind: "approval", Timestamp: 15,
				Body: briefRaw(t, proto.ApprovalBody{Task: other, Approvers: []string{"u-other"}})},
			{RowHeader: proto.RowHeader{ID: id(82)}, Kind: "decision", Timestamp: 20,
				Body: briefRaw(t, proto.DecisionBody{Task: &tid, To: []string{"u-dec"}})},
		}

		res := revApply(t, s, d, laptop, revSubmittedPayload(72, "c1", 1, "https://git.example/pull/1", 0, 0))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{4})
		equal(t, len(res.Items), 0)
		revAssertNoNulls(t, res)

		got := res.Rows[0].Value().(proto.TaskRow)
		equal(t, got.Step, "cloud_check")
		equal(t, got.Commit, "c1")
		equal(t, got.PR, 1)
		equal(t, got.PRURL, "https://git.example/pull/1")
		equal(t, got.Stats, proto.TaskStats{Added: 0, Removed: 0, Started: clock - 5000})
		equal(t, got.Wait, &proto.TaskWait{Kind: "decision", Msg: id(82), Who: []string{"u-dec"}, Since: 20})
	})

	t.Run("approval card wait takes precedence over decision on submitted", func(t *testing.T) {
		s, d := revState(t, "github")
		task := s.Tasks[revTaskID]
		task.Checks.Local = &proto.Check{OK: true, At: clock - 200, Commit: "c1"}
		task.Wait = &proto.TaskWait{Kind: "conflict", Since: clock - 300}
		s.Tasks[revTaskID] = task
		tid := revTaskID
		d.pending = []proto.MsgRow{
			{RowHeader: proto.RowHeader{ID: id(82)}, Kind: "decision", Timestamp: 10,
				Body: briefRaw(t, proto.DecisionBody{Task: &tid, To: []string{"u-dec"}})},
			{RowHeader: proto.RowHeader{ID: id(83)}, Kind: "approval", Timestamp: 25,
				Body: briefRaw(t, proto.ApprovalBody{Task: tid, Approvers: []string{"u-app"}})},
		}
		res := revApply(t, s, d, laptop, revSubmittedPayload(73, "c1", 31, "https://git.example/pull/31", 212, 18))
		equal(t, res.Answer, store.Outcome{OK: true})
		got := res.Rows[0].Value().(proto.TaskRow)
		equal(t, got.Stats.Added, 212)
		equal(t, got.Stats.Removed, 18)
		equal(t, got.Wait, &proto.TaskWait{Kind: "approval", Msg: id(83), Who: []string{"u-app"}, Since: 25})
	})

	t.Run("without remote moves straight to review and queues review item", func(t *testing.T) {
		s, d := revState(t, "none")
		task := s.Tasks[revTaskID]
		task.Checks.Local = &proto.Check{OK: true, At: clock - 200, Commit: "c1"}
		task.Failures = 2
		s.Tasks[revTaskID] = task

		res := revApply(t, s, d, laptop, revSubmittedPayload(74, "c1", 0, "", 10, 4))
		equal(t, res.Answer, store.Outcome{OK: true})
		got := res.Rows[0].Value().(proto.TaskRow)
		equal(t, got.Step, "review")
		equal(t, got.Failures, 0)
		equal(t, got.PR, 0)
		equal(t, got.PRURL, "")
		equal(t, res.Items, []home.Item{{AI: revReviewer, Why: "review", Task: revTaskID, Commit: "c1"}})
	})

	t.Run("without remote and without reviewer notifies secretary when present", func(t *testing.T) {
		for _, hasSec := range []bool{true, false} {
			s, d := revState(t, "none")
			if !hasSec {
				s.Group.Secretary = ""
			}
			task := s.Tasks[revTaskID]
			task.Reviewer = ""
			task.Checks.Local = &proto.Check{OK: true, At: clock - 200, Commit: "c1"}
			s.Tasks[revTaskID] = task

			res := revApply(t, s, d, laptop, revSubmittedPayload(75, "c1", 0, "", 5, 1))
			equal(t, res.Answer, store.Outcome{OK: true})
			got := res.Rows[0].Value().(proto.TaskRow)
			equal(t, got.Wait, &proto.TaskWait{Kind: "reviewer", Since: clock})
			if hasSec {
				equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: revTaskID, Code: "no_reviewer"}})
			} else {
				equal(t, len(res.Items), 0)
			}
		}
	})
}

func TestReviewCmdSubmittedRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		by    identity.Key
		raw   string
		patch func(*proto.TaskSubmitted)
		state func(*home.State)
		code  string
	}{
		"malformed stats": {
			raw: fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"submitted","id":%q,"commit":"c1","stats":null}`,
				id(76), revCmdTS, groupID, revTaskID),
			code: "bad_request",
		},
		"unknown task": {
			patch: func(p *proto.TaskSubmitted) { p.ID = proto.ID(id(99)) },
			code:  "not_found",
		},
		"non-host sender": {
			by:   browser,
			code: "forbidden",
		},
		"failing local checks": {
			state: func(s *home.State) {
				row := s.Tasks[revTaskID]
				row.Checks.Local = &proto.Check{OK: false, At: clock - 100, Commit: "c1"}
				s.Tasks[revTaskID] = row
			},
			code: "conflict",
		},
		"commit differs from checked commit": {
			patch: func(p *proto.TaskSubmitted) { p.Commit = "c2" },
			code:  "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := revState(t, "none")
			row := s.Tasks[revTaskID]
			row.Checks.Local = &proto.Check{OK: true, At: clock - 100, Commit: "c1"}
			s.Tasks[revTaskID] = row
			if tc.state != nil {
				tc.state(s)
			}
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			if tc.raw != "" {
				briefRefused(t, revApplyRaw(t, s, d, by, tc.raw), tc.code)
				return
			}
			p := revSubmittedPayload(76, "c1", 0, "", 1, 0)
			if tc.patch != nil {
				tc.patch(&p)
			}
			briefRefused(t, revApply(t, s, d, by, p), tc.code)
		})
	}
}

func TestReviewCmdCloudOutcomesAndStop(t *testing.T) {
	items := []proto.CheckItem{{Name: "ci / test", OK: true}}

	t.Run("passed hosted checks write cloud checks message and queue review without pr", func(t *testing.T) {
		s, d := revState(t, "github")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit, task.PR, task.Failures = "cloud_check", "c1", 31, 2
		s.Tasks[revTaskID] = task

		res := revApply(t, s, d, laptop, revCheckedPayload(80, "cloud", true, "c1", items))
		equal(t, res.Answer, store.Outcome{OK: true})
		revAssertNoNulls(t, res)
		gotMsg := res.Rows[0].Value().(proto.MsgRow)
		equal(t, gotMsg.Body, briefRaw(t, proto.ChecksBody{Stage: "cloud", OK: true, Commit: "c1", Items: items}))
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotTask.Step, "review")
		equal(t, gotTask.Failures, 0)
		equal(t, gotTask.Checks.Cloud, &proto.Check{OK: true, At: clock, Commit: "c1"})
		equal(t, res.Items, []home.Item{{AI: revReviewer, Why: "review", Task: revTaskID, Commit: "c1"}})
	})

	t.Run("failed hosted checks below limit return to local_check and queue checks item", func(t *testing.T) {
		s, d := revState(t, "github")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit, task.Failures = "cloud_check", "c1", 1
		s.Tasks[revTaskID] = task
		failItems := []proto.CheckItem{{Name: "ci / test", OK: false, Detail: revStr("1 failed")}}

		res := revApply(t, s, d, laptop, revCheckedPayload(81, "cloud", false, "c1", failItems))
		equal(t, res.Answer, store.Outcome{OK: true})
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotTask.State, "active")
		equal(t, gotTask.Step, "local_check")
		equal(t, gotTask.Failures, 2)
		equal(t, res.Items, []home.Item{{AI: revAssignee, Why: "checks", Task: revTaskID, Msg: id(81)}})
	})

	t.Run("failed hosted checks at limit stop task with derived task_failed row", func(t *testing.T) {
		s, d := revState(t, "github")
		d.msgs[revBriefID] = proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: revBriefID},
			Kind:      "brief", Body: briefRaw(t, proto.BriefBody{Requester: revGuestMem}),
		}
		task := s.Tasks[revTaskID]
		task.Step, task.Commit, task.Failures = "cloud_check", "c1", 2
		s.Tasks[revTaskID] = task
		failItems := []proto.CheckItem{{Name: "ci / test", OK: false}}

		res := revApply(t, s, d, laptop, revCheckedPayload(82, "cloud", false, "c1", failItems))
		equal(t, res.Answer, store.Outcome{OK: true})
		revAssertNoNulls(t, res)
		equal(t, len(res.Rows), 3)

		checksRow := res.Rows[0].Value().(proto.MsgRow)
		sysRow := res.Rows[1].Value().(proto.MsgRow)
		gotTask := res.Rows[2].Value().(proto.TaskRow)
		sysID := revDerivedID(82)

		equal(t, checksRow.ID, id(82))
		equal(t, checksRow.Pos, int64(11))
		equal(t, sysRow.ID, sysID)
		equal(t, sysRow.Seq, int64(12))
		equal(t, sysRow.Pos, int64(12))
		equal(t, sysRow.By, "system")
		equal(t, sysRow.Kind, "system")
		equal(t, sysRow.Timestamp, int64(clock))
		equal(t, sysRow.Mentions, []string{revGuestMem, ownerID})
		equal(t, sysRow.Body, briefRaw(t, proto.TaskFailedBody{
			SystemCode: proto.SystemCode{Code: "task_failed"}, Task: revTaskID, Reason: "failures",
		}))
		equal(t, gotTask.Seq, int64(13))
		equal(t, gotTask.State, "failed")
		equal(t, gotTask.Step, "local_check")
		equal(t, gotTask.Wait, (*proto.TaskWait)(nil))
		equal(t, gotTask.Failures, 3)
		equal(t, gotTask.Thread, proto.ThreadSummary{
			Count: 4, Rounds: 1,
			Last: &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock},
		})
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: revTaskID, Msg: sysID, Code: "task_failed"}})
	})

	t.Run("wrong commit or step on cloud is conflict", func(t *testing.T) {
		s, d := revState(t, "github")
		briefRefused(t, revApply(t, s, d, laptop, revCheckedPayload(84, "cloud", true, "c1", items)), "conflict")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit = "cloud_check", "c1"
		s.Tasks[revTaskID] = task
		briefRefused(t, revApply(t, s, d, laptop, revCheckedPayload(85, "cloud", true, "c2", items)), "conflict")
	})
}

func TestReviewCmdPostPassFailVetoAndSixReviewStop(t *testing.T) {
	t.Run("passing review at exact threshold writes review row, truncates thread preview, and queues merge with pr", func(t *testing.T) {
		s, d := revState(t, "github")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit, task.PR = "review", "c1", 31
		s.Tasks[revTaskID] = task

		longText := strings.Repeat("评", 205)
		reID := proto.ID(id(55))
		items := []proto.ReviewItem{
			{Text: "Clean design", Veto: false, File: revStr("path.go"), Line: revInt(1)},
			{Text: "Line only", Veto: false, Line: revInt(42)},
		}
		p := revReviewPayload(90, revReviewer, "c1", 90, items)
		p.Text = longText
		p.Mentions = []proto.ID{proto.ID(ownerID), proto.ID(revOtherAI)}
		p.Re = &reID
		env := briefSign(revReviewBox, p)

		res, err := home.Commands.Apply(s, d, receipts{}, env, now)
		equal(t, err, nil)
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(true), Round: 1})
		revAssertNoNulls(t, res)

		tid, reStr := revTaskID, id(55)
		wantBody := proto.ReviewBody{
			Task: revTaskID, Num: 12, Title: "Show connection paths", Commit: "c1",
			Round: 1, Score: 90, Pass: 90, OK: true, Items: items,
		}
		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(90), Seq: 11}, Pos: 11, Group: groupID,
			Thread: &tid, By: revReviewer, Kind: "review", Timestamp: revCmdTS, Text: longText,
			Mentions: []string{ownerID, revOtherAI}, Re: &reStr, Body: briefRaw(t, wantBody), Env: &env,
		}
		gotMsg := res.Rows[0].Value().(proto.MsgRow)
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotMsg, wantMsg)
		equal(t, gotTask.Step, "merge")
		equal(t, gotTask.Review, &proto.Review{Round: 1, Failed: 0, Score: 90, OK: true, By: revReviewer, Commit: "c1", At: clock})
		equal(t, gotTask.Thread, proto.ThreadSummary{
			Count: 3, Rounds: 2,
			Last: &proto.ThreadMessage{By: revReviewer, Kind: "review", Text: strings.Repeat("评", 200), Timestamp: revCmdTS},
		})
		equal(t, res.Items, []home.Item{{AI: revAssignee, Why: "merge", Task: revTaskID, Commit: "c1", PR: 31}})
	})

	t.Run("passing review without remote omits pr on merge item and keeps 200-rune text intact", func(t *testing.T) {
		for _, score := range []int{0, 100} {
			s, d := revState(t, "none")
			task := s.Tasks[revTaskID]
			task.Step, task.Commit, task.Pass, task.PR = "review", "c1", score, 31
			s.Tasks[revTaskID] = task

			exact200 := strings.Repeat("x", 200)
			p := revReviewPayload(91, revReviewer, "c1", score, []proto.ReviewItem{})
			p.Text = exact200
			res := revApply(t, s, d, revReviewBox, p)
			equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(true), Round: 1})
			revAssertNoNulls(t, res)
			gotTask := res.Rows[1].Value().(proto.TaskRow)
			equal(t, gotTask.Thread.Last.Text, exact200)
			equal(t, res.Items, []home.Item{{AI: revAssignee, Why: "merge", Task: revTaskID, Commit: "c1", PR: 0}})
		}
	})

	t.Run("score below pass returns task to local_check and queues reviewed", func(t *testing.T) {
		s, d := revState(t, "none")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit = "review", "c1"
		s.Tasks[revTaskID] = task

		p := revReviewPayload(92, revReviewer, "c1", 89, []proto.ReviewItem{{Text: "One point short"}})
		res := revApply(t, s, d, revReviewBox, p)
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 1})
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotTask.Step, "local_check")
		equal(t, gotTask.Review, &proto.Review{Round: 1, Failed: 1, Score: 89, OK: false, By: revReviewer, Commit: "c1", At: clock})
		equal(t, res.Items, []home.Item{{AI: revAssignee, Why: "reviewed", Task: revTaskID, Msg: id(92)}})
	})

	t.Run("veto forces score to zero and ok false even when pass is zero", func(t *testing.T) {
		s, d := revState(t, "none")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit, task.Pass = "review", "c1", 0
		s.Tasks[revTaskID] = task

		items := []proto.ReviewItem{{Text: "Looks clean"}, {Text: "Fake test", Veto: true}}
		res := revApply(t, s, d, revReviewBox, revReviewPayload(93, revReviewer, "c1", 95, items))
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 1})
		var body proto.ReviewBody
		equal(t, json.Unmarshal(res.Rows[0].Value().(proto.MsgRow).Body, &body), nil)
		equal(t, body.Score, 0)
		equal(t, body.OK, false)
		gotTask := res.Rows[1].Value().(proto.TaskRow)
		equal(t, gotTask.Review.Score, 0)
		equal(t, gotTask.Review.OK, false)
		equal(t, gotTask.Step, "local_check")
	})

	t.Run("fifth failed review returns to local_check; sixth failed review stops task", func(t *testing.T) {
		s, d := revState(t, "none")
		task := s.Tasks[revTaskID]
		task.Step, task.Commit = "review", "c1"
		task.Review = &proto.Review{Round: 4, Failed: 4, Score: 70, By: revReviewer, Commit: "c0", At: clock - 900}
		s.Tasks[revTaskID] = task

		fifth := revApply(t, s, d, revReviewBox, revReviewPayload(94, revReviewer, "c1", 86, []proto.ReviewItem{{Text: "Round 5"}}))
		equal(t, fifth.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 5})
		s.Commit(fifth)
		equal(t, s.Tasks[revTaskID].State, "active")
		equal(t, s.Tasks[revTaskID].Step, "local_check")
		equal(t, fifth.Items, []home.Item{{AI: revAssignee, Why: "reviewed", Task: revTaskID, Msg: id(94)}})

		task = s.Tasks[revTaskID]
		task.Step, task.Commit = "review", "c2"
		s.Tasks[revTaskID] = task

		sixth := revApply(t, s, d, revReviewBox, revReviewPayload(95, revReviewer, "c2", 86, []proto.ReviewItem{{Text: "Round 6"}}))
		equal(t, sixth.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 6})
		revAssertNoNulls(t, sixth)
		equal(t, len(sixth.Rows), 3)

		revRow := sixth.Rows[0].Value().(proto.MsgRow)
		sysRow := sixth.Rows[1].Value().(proto.MsgRow)
		gotTask := sixth.Rows[2].Value().(proto.TaskRow)
		sysID := revDerivedID(95)

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
		equal(t, gotTask.Thread.Count, 5)
		equal(t, gotTask.Thread.Last, &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock})
		equal(t, sixth.Items, []home.Item{{AI: aiID, Why: "notice", Task: revTaskID, Msg: sysID, Code: "task_failed"}})
	})

	t.Run("task_failed mentions when brief is absent and assignee owner left", func(t *testing.T) {
		s, d := revState(t, "none")
		delete(s.People, ownerID)
		s.People[revGuestMem] = person(revGuestMem, guest, "owner")
		s.AIs[revReviewer] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: revReviewer}, User: guest.ID()},
			Host:      guest.ID(),
		}
		s.Group.Secretary = ""
		task := s.Tasks[revTaskID]
		task.Origin, task.Brief, task.Step, task.Commit = "direct", "", "review", "c1"
		task.Review = &proto.Review{Round: 5, Failed: 5, Score: 50, By: revReviewer, Commit: "c0", At: clock - 900}
		s.Tasks[revTaskID] = task

		res := revApply(t, s, d, guest, revReviewPayload(96, revReviewer, "c1", 50, []proto.ReviewItem{}))
		equal(t, res.Answer, store.Outcome{OK: true, Passed: revBool(false), Round: 6})
		revAssertNoNulls(t, res)
		sysRow := res.Rows[1].Value().(proto.MsgRow)
		equal(t, sysRow.Mentions, []string{})
		equal(t, len(res.Items), 0)
	})
}

func TestReviewCmdPostRefusals(t *testing.T) {
	otherID := proto.ID(revOtherTask)
	for name, tc := range map[string]struct {
		by    identity.Key
		raw   string
		patch func(*proto.Post[proto.ReviewSigned])
		state func(*home.State)
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
			by: revReviewBox,
			raw: fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"as":%q,"kind":"review","thread":%q,"main":false,"text":"","mentions":[],"re":null,"body":{"task":%q,"commit":"c1","score":"high","items":[]}}`,
				id(97), revCmdTS, groupID, revReviewer, revTaskID, revTaskID),
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
			state: func(s *home.State) {
				row := s.Tasks[revTaskID]
				row.State, row.Wait = "paused", &proto.TaskWait{Kind: "manual", Since: clock - 100}
				s.Tasks[revTaskID] = row
			},
			code: "paused",
		},
		"commit differs from submitted commit": {
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.Body.Commit = "c2" },
			code:  "conflict",
		},
		"outside step review": {
			state: func(s *home.State) {
				row := s.Tasks[revTaskID]
				row.Step = "local_check"
				s.Tasks[revTaskID] = row
			},
			code: "conflict",
		},
		"mentions non-member": {
			patch: func(p *proto.Post[proto.ReviewSigned]) { p.Mentions = []proto.ID{proto.ID(id(98))} },
			code:  "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, d := revState(t, "none")
			row := s.Tasks[revTaskID]
			row.Step, row.Commit = "review", "c1"
			s.Tasks[revTaskID] = row
			if tc.state != nil {
				tc.state(s)
			}
			by := revReviewBox
			if tc.by != nil {
				by = tc.by
			}
			if tc.raw != "" {
				briefRefused(t, revApplyRaw(t, s, d, by, tc.raw), tc.code)
				return
			}
			p := revReviewPayload(97, revReviewer, "c1", 95, []proto.ReviewItem{{Text: "Good"}})
			if tc.patch != nil {
				tc.patch(&p)
			}
			briefRefused(t, revApply(t, s, d, by, p), tc.code)
		})
	}
}
