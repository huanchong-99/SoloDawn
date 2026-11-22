package home_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type rndDisk struct {
	msgs    map[string]proto.MsgRow
	pending []proto.MsgRow
	queued  []store.Item
}

func (d *rndDisk) Msg(id string) (proto.MsgRow, bool) {
	m, ok := d.msgs[id]
	return m, ok
}

func (d *rndDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d *rndDisk) TypedNotes(string) []proto.NoteRow         { return nil }
func (d *rndDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d *rndDisk) Pending() []proto.MsgRow                   { return d.pending }
func (d *rndDisk) Queued() []store.Item                      { return d.queued }
func (d *rndDisk) OtherMember(string) bool                   { return false }

func rndPtr(v string) *string { return &v }

func rndJumpEnv(by identity.Key, n byte, msg, extra string) proto.Envelope {
	msgPart := ""
	if msg != "" {
		msgPart = fmt.Sprintf(`,"msg":%q`, msg)
	}
	return by.Sign(fmt.Sprintf(`{"t":"jump","n":%q,"ts":%d,"g":%q%s%s}`, id(n), clock-200, groupID, msgPart, extra))
}

func rndPostEnv(by identity.Key, n byte, as, kind, thread, text string, mentions []string, body string) proto.Envelope {
	th := "null"
	if thread != "" {
		th = fmt.Sprintf("%q", thread)
	}
	asPart := ""
	if as != "" {
		asPart = fmt.Sprintf(`,"as":%q`, as)
	}
	mJSON, _ := json.Marshal(mentions)
	if body == "" {
		body = `{}`
	}
	payload := fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q%s,"kind":%q,"thread":%s,"main":false,"text":%q,"mentions":%s,"re":null,"body":%s}`,
		id(n), clock-100, groupID, asPart, kind, th, text, mJSON, body)
	return by.Sign(payload)
}

func rndResumeEnv(by identity.Key, n byte, as, taskID string) proto.Envelope {
	asPart := ""
	if as != "" {
		asPart = fmt.Sprintf(`,"as":%q`, as)
	}
	return by.Sign(fmt.Sprintf(`{"t":"task","op":"resume","n":%q,"ts":%d,"g":%q%s,"id":%q}`, id(n), clock-100, groupID, asPart, taskID))
}

func rndLimitID(n byte) string {
	sum := sha256.Sum256([]byte("SoloDawn rounds limit " + id(n)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func rndState(t *testing.T) *home.State {
	t.Helper()
	s := briefState(t)
	s.Group.Limits = proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, AskDepth: 4, IdleNoticeMinutes: 2}
	return s
}

func rndTaskRow(taskID string, num int) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 8},
		Group:     groupID, Num: num, Title: "Report path", Slug: "report-path",
		Origin: "brief", State: "active", Step: "local_check", Commit: "c1",
		Assignee: briefA, Reviewer: briefB, Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: clock - 120_000}, Created: clock - 200_000,
	}
}

// Jump retires the newest unacknowledged mention item produced by a message of
// the same person in the same thread or in the main line, and queues its
// replacement with Jump set and NotBefore 0.
func TestRoundsJumpRequeuesNewestMatchingMention(t *testing.T) {
	t.Run("main line picks newest of that person and clears hold", func(t *testing.T) {
		s := rndState(t)
		m1, m2, mGuest, mThread := id(60), id(61), id(62), id(63)
		t1 := id(70)
		d := &rndDisk{
			msgs: map[string]proto.MsgRow{
				m1:      {RowHeader: proto.RowHeader{Type: "msg", ID: m1}, By: ownerID, Kind: "text"},
				m2:      {RowHeader: proto.RowHeader{Type: "msg", ID: m2}, By: ownerID, Kind: "text"},
				mGuest:  {RowHeader: proto.RowHeader{Type: "msg", ID: mGuest}, By: briefGuest, Kind: "text"},
				mThread: {RowHeader: proto.RowHeader{Type: "msg", ID: mThread}, By: ownerID, Kind: "text", Thread: rndPtr(t1)},
			},
			queued: []store.Item{
				{ID: 11, AI: briefA, Why: "mention", Msg: rndPtr(m1)},
				{ID: 12, AI: briefB, Why: "mention", Msg: rndPtr(m2), NotBefore: clock + 30_000},
				{ID: 13, AI: briefA, Why: "mention", Msg: rndPtr(mGuest)},
				{ID: 14, AI: briefA, Why: "mention", Task: rndPtr(t1), Msg: rndPtr(mThread)},
				{ID: 15, AI: briefA, Why: "assign", Task: rndPtr(t1), Msg: rndPtr(m1)},
				{ID: 16, AI: briefA, Why: "mention", Msg: nil},
				{ID: 17, AI: briefA, Why: "mention", Msg: rndPtr(id(99))},
			},
		}
		res := briefApply(t, s, d, receipts{}, browser, rndJumpEnv(browser, 80, m1, ""))
		equal(t, res, home.Result{
			Items:  []home.Item{{AI: briefB, Why: "mention", Msg: m2, Jump: true, NotBefore: 0}},
			Retire: []int64{12},
			Answer: store.Outcome{OK: true},
		})
	})

	t.Run("thread message matches oldest index 0 in the same thread", func(t *testing.T) {
		s := rndState(t)
		t1, t2 := id(70), id(71)
		mT1, mT2, mMain := id(60), id(61), id(62)
		// Distinct string copies ensure thread equality compares values, not pointers.
		threadA, threadB := string([]byte(t1)), string([]byte(t1))
		d := &rndDisk{
			msgs: map[string]proto.MsgRow{
				mT1:    {RowHeader: proto.RowHeader{Type: "msg", ID: mT1}, By: ownerID, Kind: "text", Thread: &threadA},
				mT2:    {RowHeader: proto.RowHeader{Type: "msg", ID: mT2}, By: ownerID, Kind: "text", Thread: rndPtr(t2)},
				mMain:  {RowHeader: proto.RowHeader{Type: "msg", ID: mMain}, By: ownerID, Kind: "text"},
				id(64): {RowHeader: proto.RowHeader{Type: "msg", ID: id(64)}, By: ownerID, Kind: "text", Thread: &threadB},
			},
			queued: []store.Item{
				{ID: 21, AI: briefA, Why: "mention", Task: &threadB, Msg: rndPtr(mT1)},
				{ID: 22, AI: briefB, Why: "mention", Task: rndPtr(t2), Msg: rndPtr(mT2)},
				{ID: 23, AI: aiID, Why: "mention", Msg: rndPtr(mMain)},
			},
		}
		res := briefApply(t, s, d, receipts{}, browser, rndJumpEnv(browser, 81, id(64), ""))
		equal(t, res, home.Result{
			Items:  []home.Item{{AI: briefA, Why: "mention", Task: t1, Msg: mT1, Jump: true, NotBefore: 0}},
			Retire: []int64{21},
			Answer: store.Outcome{OK: true},
		})
	})
}

func TestRoundsJumpGoldenAndRefusals(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "jump.json"))
	equal(t, err, nil)
	var golden proto.Jump
	equal(t, proto.Decode(raw, &golden), nil)
	equal(t, proto.UnexpectedNulls(raw), []string{})

	mOwner, mGuest, mAI, mSys := id(60), id(61), id(62), id(63)
	baseMsgs := map[string]proto.MsgRow{
		mOwner: {RowHeader: proto.RowHeader{Type: "msg", ID: mOwner}, By: ownerID, Kind: "text"},
		mGuest: {RowHeader: proto.RowHeader{Type: "msg", ID: mGuest}, By: briefGuest, Kind: "text"},
		mAI:    {RowHeader: proto.RowHeader{Type: "msg", ID: mAI}, By: briefA, Kind: "text"},
		mSys:   {RowHeader: proto.RowHeader{Type: "msg", ID: mSys}, By: "system", Kind: "system"},
	}
	for name, tc := range map[string]struct {
		by     identity.Key
		msg    string
		extra  string
		queued []store.Item
		code   string
	}{
		"by a working AI":       {by: laptop, msg: mOwner, extra: `,"as":"` + briefA + `"`, code: "forbidden"},
		"by the secretary":      {by: laptop, msg: mOwner, extra: `,"as":"` + aiID + `"`, code: "forbidden"},
		"bad msg id":            {by: browser, msg: "not-an-id", code: "bad_request"},
		"missing msg field":     {by: browser, code: "bad_request"},
		"null msg field":        {by: browser, extra: `,"msg":null`, code: "bad_request"},
		"missing message":       {by: browser, msg: id(95), code: "not_found"},
		"other person's msg":    {by: browser, msg: mGuest, code: "forbidden"},
		"AI's msg":              {by: browser, msg: mAI, code: "forbidden"},
		"system msg":            {by: browser, msg: mSys, code: "forbidden"},
		"empty queue":           {by: browser, msg: mOwner, code: "conflict"},
		"only assign for msg":   {by: browser, msg: mOwner, queued: []store.Item{{ID: 1, AI: briefA, Why: "assign", Msg: rndPtr(mOwner)}}, code: "conflict"},
		"only other person due": {by: browser, msg: mOwner, queued: []store.Item{{ID: 2, AI: briefA, Why: "mention", Msg: rndPtr(mGuest)}}, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := rndState(t)
			d := &rndDisk{msgs: baseMsgs, queued: tc.queued}
			briefRefused(t, briefApply(t, s, d, receipts{}, tc.by, rndJumpEnv(tc.by, 82, tc.msg, tc.extra)), tc.code)
		})
	}
}

// A5.16 and row 23: the quiet timer checks thread.last.ts (else stats.started,
// else created) against limits.idle_notice_minutes, notifies the secretary once
// per quiet period of each group, and resets when last activity advances.
func TestRoundsQuietTimerBoundariesAndPeriods(t *testing.T) {
	quietFn := home.Timers["quiet"]
	afterFn := home.After["quiet"]

	t.Run("exact millisecond boundary on started, created and thread.last.ts", func(t *testing.T) {
		s := rndState(t)
		tStarted := rndTaskRow(runID(1), 1)
		tStarted.Stats.Started, tStarted.Created = clock-120_000, clock-500_000

		tCreated := rndTaskRow(runID(2), 2)
		tCreated.Stats.Started, tCreated.Created = 0, clock-120_000

		tLast := rndTaskRow(runID(3), 3)
		tLast.Stats.Started, tLast.Created = clock-500_000, clock-500_000
		tLast.Thread.Last = &proto.ThreadMessage{By: briefA, Kind: "progress", Timestamp: clock - 120_000}

		s.Tasks[tStarted.ID] = tStarted
		s.Tasks[tCreated.ID] = tCreated
		s.Tasks[tLast.ID] = tLast

		d := &rndDisk{}
		// One millisecond before the 2-minute threshold: none is quiet.
		equal(t, quietFn(s, d, clock-1), home.Result{})

		// At the exact 2-minute threshold: all three fire in sorted task ID order.
		res := s.Seal(quietFn(s, d, clock))
		want := []home.Item{
			{AI: aiID, Why: "notice", Task: tStarted.ID, Code: "quiet"},
			{AI: aiID, Why: "notice", Task: tCreated.ID, Code: "quiet"},
			{AI: aiID, Why: "notice", Task: tLast.ID, Code: "quiet"},
		}
		equal(t, res.Items, want)
		s.Commit(res)
		afterFn(s, res)

		// Same quiet period sends nothing on later ticks.
		equal(t, quietFn(s, d, clock+60_000), home.Result{})

		// Another group does not share quiet tracking.
		otherGroup := rndState(t)
		otherGroup.Tasks[tStarted.ID] = tStarted
		equal(t, quietFn(otherGroup, d, clock).Items, []home.Item{{AI: aiID, Why: "notice", Task: tStarted.ID, Code: "quiet"}})

		// Advancing thread.last.ts starts a new quiet period.
		updated := tLast
		updated.Seq = 9
		updated.Thread.Count = 1
		updated.Thread.Last = &proto.ThreadMessage{By: briefA, Kind: "progress", Timestamp: clock + 10_000}
		s.Tasks[updated.ID] = updated
		equal(t, quietFn(s, d, clock+10_000+119_999), home.Result{})
		next := s.Seal(quietFn(s, d, clock+10_000+120_000))
		equal(t, next.Items, []home.Item{{AI: aiID, Why: "notice", Task: updated.ID, Code: "quiet"}})
		s.Commit(next)
		afterFn(s, next)
		equal(t, quietFn(s, d, clock+300_000), home.Result{})
	})

	t.Run("skips tasks without secretary, non-positive idle limit, or non-idle states", func(t *testing.T) {
		s := rndState(t)
		t1, t2 := rndTaskRow(runID(1), 1), rndTaskRow(runID(2), 2)
		tPaused := rndTaskRow(runID(3), 3)
		tPaused.State, tPaused.Wait = "paused", &proto.TaskWait{Kind: "manual", Since: clock - 1000}
		tMerge := rndTaskRow(runID(4), 4)
		tMerge.Step = "merge"
		tWaiting := rndTaskRow(runID(5), 5)
		tWaiting.Wait = &proto.TaskWait{Kind: "reviewer", Since: clock - 1000}
		s.Tasks[t1.ID], s.Tasks[t2.ID] = t1, t2
		s.Tasks[tPaused.ID], s.Tasks[tMerge.ID], s.Tasks[tWaiting.ID] = tPaused, tMerge, tWaiting

		d := &rndDisk{}
		s.Group.Secretary = ""
		equal(t, quietFn(s, d, clock), home.Result{})
		s.Group.Secretary = aiID
		s.Group.Limits.IdleNoticeMinutes = 0
		equal(t, quietFn(s, d, clock), home.Result{})
		s.Group.Limits.IdleNoticeMinutes = 1

		// Non-quiet items passed to After["quiet"] do not mark tasks as quiet-notified.
		afterFn(s, home.Result{Items: []home.Item{
			{AI: aiID, Why: "notice", Task: t2.ID, Code: "task_started"},
			{AI: aiID, Why: "mention", Task: t2.ID, Code: "quiet"},
		}})
		equal(t, quietFn(s, d, clock).Items, []home.Item{
			{AI: aiID, Why: "notice", Task: t1.ID, Code: "quiet"},
			{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"},
		})

		// Recording t1 on tick 1 and t2 on tick 2 keeps both recorded on tick 3.
		afterFn(s, home.Result{Items: []home.Item{{AI: aiID, Why: "notice", Task: t1.ID, Code: "quiet"}}})
		equal(t, quietFn(s, d, clock).Items, []home.Item{{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"}})
		afterFn(s, home.Result{Items: []home.Item{{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"}}})
		equal(t, quietFn(s, d, clock), home.Result{})
	})
}

// A5.8: ten AI-to-AI exchanges in a thread pause the task with wait rounds,
// write rounds_limit in the main line, queue the 10th mention with the paused
// task, and notify the secretary; a person's thread text and an admin's task
// resume each end the wait.
func TestRoundsTenExchangesPauseAndResume(t *testing.T) {
	for _, how := range []string{"person_text", "admin_resume"} {
		t.Run(how, func(t *testing.T) {
			s, d := rndState(t), &rndDisk{msgs: map[string]proto.MsgRow{}}
			taskID := id(70)
			s.Tasks[taskID] = rndTaskRow(taskID, 1)

			// Rounds 1 to 8 via question, round 9 via secretary text, round 10 via call.
			for round := 1; round <= 8; round++ {
				sender, target := briefA, briefB
				if round%2 == 0 {
					sender, target = briefB, briefA
				}
				res := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, byte(30+round), sender, "question", taskID, "q", []string{target}, ""))
				equal(t, res.Items, []home.Item{{AI: target, Why: "mention", Task: taskID, Msg: id(byte(30 + round))}})
				s.Commit(res)
				equal(t, s.Tasks[taskID].Thread.Rounds, round)
			}

			r9 := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 39, aiID, "text", taskID, "check", []string{briefA}, ""))
			s.Commit(r9)
			equal(t, []any{s.Tasks[taskID].State, s.Tasks[taskID].Thread.Rounds}, []any{"active", 9})

			// Round 10: a call with empty mentions[] counts via its mention item (R-114).
			callBody := fmt.Sprintf(`{"to":%q,"depth":1,"wait":true}`, briefB)
			r10 := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 40, briefA, "call", taskID, "ask", []string{}, callBody))
			sysID := rndLimitID(40)
			equal(t, r10.Items, []home.Item{
				{AI: briefB, Why: "mention", Task: taskID, Msg: id(40)},
				{AI: aiID, Why: "notice", Task: taskID, Msg: sysID, Code: "rounds_limit"},
			})
			s.Commit(r10)

			paused := s.Tasks[taskID]
			equal(t, []any{paused.State, paused.Wait.Kind, paused.Wait.Since, paused.Thread.Rounds, paused.Thread.Count},
				[]any{"paused", "rounds", int64(clock), 10, 10})
			sysRow := r10.Rows[2].Value().(proto.MsgRow)
			rawSys := briefRaw(t, sysRow)
			var decodedSys proto.MsgRow
			equal(t, proto.Decode(rawSys, &decodedSys), nil)
			equal(t, proto.UnexpectedNulls(rawSys), []string{"/re", "/state", "/thread"})
			equal(t, []any{sysRow.Group, sysRow.Timestamp, sysRow.Thread == nil, sysRow.Main, sysRow.By, sysRow.Kind, string(sysRow.Body)},
				[]any{groupID, int64(clock), true, false, "system", "system", fmt.Sprintf(`{"code":"rounds_limit","task":%q,"n":10}`, taskID)})

			// Round 11 while already paused at rounds increments rounds to 11 without writing a second rounds_limit.
			r11 := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 41, briefB, "question", taskID, "more", []string{briefA}, ""))
			equal(t, len(r11.Rows), 2)
			equal(t, r11.Items, []home.Item{{AI: briefA, Why: "mention", Task: taskID, Msg: id(41)}})
			s.Commit(r11)
			equal(t, s.Tasks[taskID].Thread.Rounds, 11)

			// Secretary cannot end the round limit via task/resume.
			briefRefused(t, briefApply(t, s, d, receipts{}, laptop, rndResumeEnv(laptop, 42, aiID, taskID)), "forbidden")

			if how == "person_text" {
				resumed := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(guest, 43, "", "text", taskID, "go ahead", []string{briefB}, ""))
				equal(t, resumed.Items, []home.Item{{AI: briefB, Why: "mention", Task: taskID, Msg: id(43)}})
				s.Commit(resumed)
			} else {
				resumed := briefApply(t, s, d, receipts{}, laptop, rndResumeEnv(browser, 44, "", taskID))
				equal(t, resumed.Items, []home.Item{{AI: briefA, Why: "resume", Task: taskID}})
				s.Commit(resumed)
			}
			after := s.Tasks[taskID]
			equal(t, []any{after.State, after.Wait == nil, after.Thread.Rounds}, []any{"active", true, 0})
		})
	}
}

func TestRoundsThreadPostRulesAndStepMerge(t *testing.T) {
	t.Run("main-line, human-only, self-only, review rows, report, step merge, and no secretary", func(t *testing.T) {
		s, d := rndState(t), &rndDisk{msgs: map[string]proto.MsgRow{}}
		taskID := id(70)
		s.Tasks[taskID] = rndTaskRow(taskID, 1)

		// Main-line AI post, thread AI post mentioning only a human, and thread AI post mentioning only itself count no round.
		s.Commit(briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 50, briefA, "question", "", "main", []string{briefB}, "")))
		s.Commit(briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 51, briefA, "progress", taskID, "hi owner", []string{ownerID}, "")))
		s.Commit(briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 52, briefA, "progress", taskID, "self note", []string{briefA}, "")))
		equal(t, s.Tasks[taskID].Thread.Rounds, 0)

		// Review without AI mentions (reviewed item, not mention) counts no round.
		t1 := s.Tasks[taskID]
		t1.Step = "review"
		s.Tasks[taskID] = t1
		revBody := fmt.Sprintf(`{"task":%q,"commit":"c1","score":80,"items":[]}`, taskID)
		s.Commit(briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 53, briefB, "review", taskID, "needs work", []string{}, revBody)))
		equal(t, s.Tasks[taskID].Thread.Rounds, 0)

		// Sixth failed review mentioning another AI writes [reviewMsg, taskFailedMsg, taskRow] and counts 1 round.
		t1 = s.Tasks[taskID]
		t1.Step, t1.Review = "review", &proto.Review{Round: 5, Failed: 5, Score: 80, By: briefB, Commit: "c1", At: clock - 500}
		s.Tasks[taskID] = t1
		revSixth := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 54, briefB, "review", taskID, "stop", []string{briefA}, revBody))
		equal(t, len(revSixth.Rows), 3)
		s.Commit(revSixth)
		equal(t, s.Tasks[taskID].Thread.Rounds, 1)

		// Report mentioning another AI counts 1 round even though report queues no mention item.
		t1 = s.Tasks[taskID]
		t1.State, t1.Step, t1.Failures, t1.Review = "active", "local_check", 0, nil
		s.Tasks[taskID] = t1
		s.Commit(briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 55, briefA, "report", taskID, "", []string{briefB}, `{"commit":"c1","files":1,"added":2,"removed":0}`)))
		equal(t, s.Tasks[taskID].Thread.Rounds, 2)

		// A5.18: at step merge, reaching limits.rounds increments Thread.Rounds without pausing or writing rounds_limit.
		mergeTask := s.Tasks[taskID]
		mergeTask.Step, mergeTask.Thread.Rounds = "merge", 9
		s.Tasks[taskID] = mergeTask
		atMerge := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 56, briefA, "question", taskID, "q", []string{briefB}, ""))
		equal(t, len(atMerge.Rows), 2)
		equal(t, atMerge.Items, []home.Item{{AI: briefB, Why: "mention", Task: taskID, Msg: id(56)}})
		s.Commit(atMerge)
		equal(t, []any{s.Tasks[taskID].State, s.Tasks[taskID].Step, s.Tasks[taskID].Thread.Rounds}, []any{"active", "merge", 10})

		// Without a secretary, reaching limits.rounds writes rounds_limit without queuing a secretary notice.
		activeAgain := s.Tasks[taskID]
		activeAgain.Step, activeAgain.Thread.Rounds = "local_check", 9
		s.Tasks[taskID] = activeAgain
		s.Group.Secretary = ""
		noSec := briefApply(t, s, d, receipts{}, laptop, rndPostEnv(laptop, 57, briefA, "question", taskID, "q", []string{briefB}, ""))
		equal(t, len(noSec.Rows), 3)
		equal(t, noSec.Items, []home.Item{{AI: briefB, Why: "mention", Task: taskID, Msg: id(57)}})

		// Refused post passes through rndPost untouched.
		briefRefused(t, briefApply(t, s, d, receipts{}, browser, rndPostEnv(browser, 58, "", "progress", taskID, "no", []string{}, "")), "forbidden")
	})

	t.Run("person post at step review deduplicates IfNone review item and restores pending card wait", func(t *testing.T) {
		s := rndState(t)
		t1ID, t2ID := id(70), id(71)
		t1 := rndTaskRow(t1ID, 1)
		t1.State, t1.Step, t1.Thread.Rounds = "paused", "review", 10
		t1.Wait = &proto.TaskWait{Kind: "rounds", Since: clock - 2000}
		s.Tasks[t1ID] = t1

		d := &rndDisk{
			pending: []proto.MsgRow{
				{RowHeader: proto.RowHeader{Type: "msg", ID: id(80)}, Thread: &t2ID, Kind: "approval", Timestamp: 100, Body: json.RawMessage(fmt.Sprintf(`{"task":%q,"approvers":["u2"]}`, t2ID))},
				{RowHeader: proto.RowHeader{Type: "msg", ID: id(81)}, Thread: &t1ID, Kind: "decision", Timestamp: 200, Body: json.RawMessage(fmt.Sprintf(`{"task":%q,"to":["u-dec"]}`, t1ID))},
				{RowHeader: proto.RowHeader{Type: "msg", ID: id(82)}, Thread: &t1ID, Kind: "approval", Timestamp: 300, Body: json.RawMessage(fmt.Sprintf(`{"task":%q,"approvers":["u-app"]}`, t1ID))},
			},
			queued: []store.Item{
				{ID: 1, AI: briefB, Why: "review", Task: &t2ID, Commit: rndPtr("c1")},
				{ID: 2, AI: briefB, Why: "review", Task: &t1ID, Commit: rndPtr("c0")},
			},
		}

		res := briefApply(t, s, d, receipts{}, browser, rndPostEnv(browser, 85, "", "text", t1ID, "continue", []string{briefA}, ""))
		equal(t, res.Items, []home.Item{
			{AI: briefB, Why: "review", Task: t1ID, Commit: "c1"},
			{AI: briefA, Why: "mention", Task: t1ID, Msg: id(85)},
		})
		s.Commit(res)
		equal(t, s.Tasks[t1ID].Wait, &proto.TaskWait{Kind: "approval", Msg: id(82), Who: []string{"u-app"}, Since: 300})

		// Matching queued review suppresses duplicate review item on row 22 activation.
		t1 = s.Tasks[t1ID]
		t1.State, t1.Wait, t1.Thread.Rounds = "paused", &proto.TaskWait{Kind: "rounds", Since: clock}, 10
		s.Tasks[t1ID] = t1
		d.queued = []store.Item{{ID: 3, AI: briefB, Why: "review", Task: &t1ID, Commit: rndPtr("c1")}}
		resDup := briefApply(t, s, d, receipts{}, browser, rndPostEnv(browser, 86, "", "text", t1ID, "again", []string{}, ""))
		equal(t, len(resDup.Items), 0)
	})
}

// A mention queued at the round limit reaches the host after failed hosted
// checks turn the rounds-paused task failed and an admin resumes it, and the
// mentions of a rounds-paused task leave the wake-up cap after an hour.
func TestRoundsFailedTaskDeliveryAndWakeCap(t *testing.T) {
	t.Run("mention at rounds limit is delivered after task becomes failed and is resumed", func(t *testing.T) {
		tables := home.NodeTables()
		localCmds := maps.Clone(tables.Commands)
		taskID := id(70)
		localCmds["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
			grp := proto.GroupRow{
				RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Site", Kind: "project",
				Home: s.Home, Secretary: aiID, Forge: "github", Pass: 90, PassFoundation: 70,
				Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: 30},
			}
			task := proto.TaskRow{
				RowHeader: proto.RowHeader{Type: "task", ID: taskID}, Group: s.ID, Num: 1,
				Title: "Cloud task", Slug: "cloud-task", Origin: "direct", State: "active", Step: "cloud_check",
				Commit: "c1", Failures: 2, Assignee: briefA, Reviewer: briefB, Pass: 90, Depends: []string{},
				Thread: proto.ThreadSummary{Count: 9, Rounds: 9}, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
			}
			return home.Result{
				Rows:          []home.Row{home.RowOf(grp), home.RowOf(person(ownerID, browser, "owner")), home.RowOf(ai(aiID, laptop)), home.RowOf(ai(briefA, laptop)), home.RowOf(ai(briefB, laptop)), home.RowOf(task)},
				GroupsChanged: true,
			}, nil
		}
		tables.Commands = localCmds

		w := runStart(t, runOpen(t), tables)
		p, host := w.attach(browser), w.attach(laptop)
		p.does(groupID, "group", `,"op":"create"`)
		p.next()
		host.next()

		// 10th mention pauses task with wait rounds; failed cloud check moves it to failed (wait null);
		// admin task/resume and person thread text make it active again.
		m10 := id(60)
		w.loop.Cmd(host.conn, 2, rndPostEnv(laptop, 60, briefA, "question", taskID, "10th", []string{briefB}, ""))
		host.next()
		w.loop.Cmd(host.conn, 3, laptop.Sign(fmt.Sprintf(`{"t":"task","op":"cloud","n":%q,"ts":%d,"g":%q,"id":%q,"commit":"c1","ok":false,"items":[{"name":"ci","ok":false}]}`, id(61), clock, groupID, taskID)))
		host.next()
		w.loop.Cmd(p.conn, 2, rndResumeEnv(browser, 62, "", taskID))
		p.next()
		w.loop.Cmd(p.conn, 3, rndPostEnv(browser, 63, "", "text", taskID, "fixed", []string{}, ""))
		p.next()

		// When the host takes its inbox, the 10th mention is due at once, its task active.
		w.loop.Inbox(host.conn)
		push := inboxDecode(t, host.next())
		equal(t, []any{push.Items[0].Item.Why, string(push.Items[0].Item.Msg), push.Items[0].Task.State}, []any{"mention", m10, "active"})
	})

	t.Run("mentions at rounds limit age out of wakes_per_hour after an hour", func(t *testing.T) {
		tables := home.NodeTables()
		localCmds := maps.Clone(tables.Commands)
		taskID := id(71)
		localCmds["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
			grp := proto.GroupRow{
				RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Cap", Kind: "project",
				Home: s.Home, Secretary: aiID, Forge: "none", Pass: 90, PassFoundation: 70,
				Limits: proto.Limits{Rounds: 10, WakesPerHour: 2, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: 30},
			}
			task := proto.TaskRow{
				RowHeader: proto.RowHeader{Type: "task", ID: taskID}, Group: s.ID, Num: 1,
				Title: "Cap task", Slug: "cap-task", Origin: "direct", State: "active", Step: "local_check",
				Assignee: briefA, Reviewer: briefB, Pass: 90, Depends: []string{},
				Thread: proto.ThreadSummary{Count: 9, Rounds: 9}, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
			}
			return home.Result{
				Rows:          []home.Row{home.RowOf(grp), home.RowOf(person(ownerID, browser, "owner")), home.RowOf(ai(aiID, laptop)), home.RowOf(ai(briefA, laptop)), home.RowOf(ai(briefB, laptop)), home.RowOf(task)},
				GroupsChanged: true,
			}, nil
		}
		tables.Commands = localCmds

		w := runStart(t, runOpen(t), tables)
		p, host := w.attach(browser), w.attach(laptop)
		p.does(groupID, "group", `,"op":"create"`)
		p.next()
		host.next()
		w.loop.Inbox(host.conn)

		// Rounds 10 and 11 both mention briefB at clock (filling WakesPerHour = 2 at clock).
		for i, n := range []byte{64, 65} {
			w.loop.Cmd(host.conn, int64(2+i), rndPostEnv(laptop, n, briefA, "question", taskID, "q", []string{briefB}, ""))
			host.next()
			inboxDecode(t, host.next())
		}

		// After 61 minutes, both round-limit wakes have aged out: two new mentions go out immediately, and the third is held for 1 hour.
		later := int64(clock + 61*60_000)
		w.clock.Store(later)
		for i, n := range []byte{66, 67} {
			env := browser.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"text","thread":null,"main":false,"text":"m","mentions":[%q],"re":null,"body":{}}`, id(n), later, groupID, briefB))
			w.loop.Cmd(p.conn, int64(10+i), env)
			p.next()
			got := inboxDecode(t, host.next())
			equal(t, string(got.Items[0].Item.Msg), id(n))
		}
		thirdEnv := browser.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"text","thread":null,"main":false,"text":"m3","mentions":[%q],"re":null,"body":{}}`, id(68), later, groupID, briefB))
		w.loop.Cmd(p.conn, 12, thirdEnv)
		p.next()
		host.quiet()
		w.clock.Store(later + 3_600_000)
		w.loop.Tick(time.UnixMilli(later + 3_600_000))
		equal(t, string(inboxDecode(t, host.next()).Items[0].Item.Msg), id(68))
	})
}

// Through StartLoop with NodeTables, main-line posts pass through the post
// wrapper untouched, jump replaces the queued mention in the store, and quiet
// fires once across ticks even after acknowledgement.
func TestRoundsLoopJumpAndQuiet(t *testing.T) {
	tables := home.NodeTables()
	localCmds := maps.Clone(tables.Commands)
	localCmds["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		grp := proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Site", Kind: "project",
			Home: s.Home, Secretary: aiID, Forge: "none", Pass: 90, PassFoundation: 70,
			Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: 1},
		}
		own := person(ownerID, browser, "owner")
		sec, worker := ai(aiID, laptop), ai(briefA, laptop)
		task := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: id(70)}, Group: s.ID, Num: 1,
			Title: "Direct task", Slug: "direct-task", Origin: "direct", State: "active", Step: "local_check",
			Assignee: briefA, Pass: 90, Depends: []string{}, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
		}
		return home.Result{
			Rows:          []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(sec), home.RowOf(worker), home.RowOf(task)},
			GroupsChanged: true,
		}, nil
	}
	tables.Commands = localCmds

	w := runStart(t, runOpen(t), tables)
	p := w.attach(browser)
	host := w.attach(laptop)
	p.does(groupID, "group", `,"op":"create"`)
	p.next()
	host.next()
	w.loop.Inbox(host.conn)

	// Main-line text post mentioning briefA passes through rndWrap and pushes item 1.
	msgID := id(60)
	env := browser.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"text","thread":null,"main":false,"text":"Quick question","mentions":[%q],"re":null,"body":{}}`, msgID, clock, groupID, briefA))
	w.loop.Cmd(p.conn, 2, env)
	p.next()
	firstPush := inboxDecode(t, host.next())
	equal(t, []any{firstPush.Group, len(firstPush.Items), firstPush.Items[0].Item}, []any{
		proto.ID(groupID), 1, proto.Item{Item: 1, AI: proto.ID(briefA), Why: "mention", Msg: proto.ID(msgID)},
	})

	// Jump retires item 1 and pushes item 2 with Jump: true.
	p.does(groupID, "jump", `,"msg":"`+msgID+`"`)
	jumpPush := inboxDecode(t, host.next())
	equal(t, []any{len(jumpPush.Items), jumpPush.Items[0].Item}, []any{
		1, proto.Item{Item: 2, AI: proto.ID(briefA), Why: "mention", Msg: proto.ID(msgID), Jump: true},
	})
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox WHERE id=1"), 0)

	// Acknowledge item 2 via Loop.Ack, then advance the clock to test the quiet timer.
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 2})
	w.idle()
	w.clock.Store(clock + 59_999)
	w.loop.Tick(time.UnixMilli(clock + 59_999))
	host.quiet()

	w.clock.Store(clock + 60_000)
	w.loop.Tick(time.UnixMilli(clock + 60_000))
	quietPush := inboxDecode(t, host.next())
	equal(t, []any{len(quietPush.Items), quietPush.Items[0].Item}, []any{
		1, proto.Item{Item: 3, AI: proto.ID(aiID), Why: "notice", Task: proto.ID(id(70)), Code: "quiet"},
	})

	// Acknowledge the quiet notice; later ticks in the same quiet period send nothing.
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 3})
	w.idle()
	w.clock.Store(clock + 180_000)
	w.loop.Tick(time.UnixMilli(clock + 180_000))
	host.quiet()
}
