package home_test

import (
	"encoding/json"
	"fmt"
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
func (d *rndDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d *rndDisk) Pending() []proto.MsgRow                   { return d.pending }
func (d *rndDisk) Queued() []store.Item                      { return d.queued }

func rndPtr(v string) *string { return &v }

func rndJumpEnv(by identity.Key, n byte, msg, extra string) proto.Envelope {
	msgPart := ""
	if msg != "" {
		msgPart = fmt.Sprintf(`,"msg":%q`, msg)
	}
	return by.Sign(fmt.Sprintf(`{"t":"jump","n":%q,"ts":%d,"g":%q%s%s}`, id(n), clock-200, groupID, msgPart, extra))
}

func rndState(t *testing.T) *home.State {
	t.Helper()
	s := briefState(t)
	s.Group.Limits = proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: 2}
	return s
}

func rndTaskRow(taskID string, num int) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 8},
		Group:     groupID, Num: num, Title: "Report path", Slug: "report-path",
		Origin: "brief", State: "active", Step: "local_check",
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
				mT1:   {RowHeader: proto.RowHeader{Type: "msg", ID: mT1}, By: ownerID, Kind: "text", Thread: &threadA},
				mT2:   {RowHeader: proto.RowHeader{Type: "msg", ID: mT2}, By: ownerID, Kind: "text", Thread: rndPtr(t2)},
				mMain: {RowHeader: proto.RowHeader{Type: "msg", ID: mMain}, By: ownerID, Kind: "text"},
			},
			queued: []store.Item{
				{ID: 21, AI: briefA, Why: "mention", Task: &threadB, Msg: rndPtr(mT1)},
				{ID: 22, AI: briefB, Why: "mention", Task: rndPtr(t2), Msg: rndPtr(mT2)},
				{ID: 23, AI: aiID, Why: "mention", Msg: rndPtr(mMain)},
			},
		}
		target := mT1
		d.msgs[id(64)] = proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(64)}, By: ownerID, Kind: "text", Thread: &threadB}
		res := briefApply(t, s, d, receipts{}, browser, rndJumpEnv(browser, 81, id(64), ""))
		equal(t, res, home.Result{
			Items:  []home.Item{{AI: briefA, Why: "mention", Task: t1, Msg: target, Jump: true, NotBefore: 0}},
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
			res := briefApply(t, s, d, receipts{}, tc.by, rndJumpEnv(tc.by, 82, tc.msg, tc.extra))
			equal(t, res.Answer, store.Outcome{Code: tc.code})
			equal(t, res.Msg != "", true)
			equal(t, len(res.Items)+len(res.Retire), 0)
		})
	}
}

// A5.16 and row 23: the quiet timer checks thread.last.ts (else stats.started,
// else created) against limits.idle_notice_minutes, notifies the secretary once
// per quiet period, and resets when a thread message arrives or the task changes.
func TestRoundsQuietTimerBoundariesAndPeriods(t *testing.T) {
	quietFn := home.Timers["quiet"]
	afterFn := home.After["quiet"]

	t.Run("exact millisecond boundary on started, created and thread.last.ts", func(t *testing.T) {
		s := rndState(t)
		// Limits.IdleNoticeMinutes is 2 (120,000 ms).
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

		// Even with an empty queue (all notices acknowledged), the same quiet period sends nothing.
		equal(t, quietFn(s, d, clock+60_000), home.Result{})

		// A separate group state with the same group ID and task IDs does not share quiet tracking.
		otherGroup := rndState(t)
		otherGroup.Tasks[tStarted.ID] = tStarted
		equal(t, quietFn(otherGroup, d, clock).Items, []home.Item{{AI: aiID, Why: "notice", Task: tStarted.ID, Code: "quiet"}})

		// Updating a task row (new thread message or task change) starts a new quiet period.
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

	t.Run("skips tasks without secretary, non-idle states, or queued quiet item", func(t *testing.T) {
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

		// Without a secretary, nothing is queued and nothing is remembered as notified.
		s.Group.Secretary = ""
		d := &rndDisk{}
		equal(t, quietFn(s, d, clock), home.Result{})
		s.Group.Secretary = aiID

		// Non-quiet notices, notices for other tasks, main-line notices, and mentions do not suppress t2.
		// Seam JSON on t1 and signed-envelope JSON on t2 both suppress only their own quiet notice.
		d.queued = []store.Item{
			{ID: 1, AI: aiID, Why: "notice", Task: nil, Env: json.RawMessage(`{"Code":"quiet"}`)},
			{ID: 2, AI: aiID, Why: "mention", Task: rndPtr(t2.ID), Env: json.RawMessage(`{"Code":"quiet"}`)},
			{ID: 3, AI: aiID, Why: "notice", Task: rndPtr(t2.ID), Env: json.RawMessage(`{"Code":"task_started"}`)},
			{ID: 4, AI: aiID, Why: "notice", Task: rndPtr(t1.ID), Env: json.RawMessage(`{"Code":"quiet"}`)},
		}
		res := quietFn(s, d, clock)
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"}})

		// Signed-envelope form in Queued also suppresses duplicate quiet notices.
		d.queued = append(d.queued, store.Item{
			ID: 5, AI: aiID, Why: "notice", Task: rndPtr(t2.ID),
			Env: json.RawMessage(`{"p":"{\"t\":\"item\",\"why\":\"notice\",\"code\":\"quiet\"}","k":"k","s":"s"}`),
		})
		equal(t, quietFn(s, d, clock), home.Result{})

		// Non-quiet items passed to After["quiet"] do not mark t2 as quiet-notified.
		d.queued = nil
		afterFn(s, home.Result{Items: []home.Item{
			{AI: aiID, Why: "notice", Task: t2.ID, Code: "task_started"},
			{AI: aiID, Why: "mention", Task: t2.ID, Code: "quiet"},
		}})
		equal(t, quietFn(s, d, clock).Items, []home.Item{
			{AI: aiID, Why: "notice", Task: t1.ID, Code: "quiet"},
			{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"},
		})

		// Recording t1 on tick 1 and t2 on tick 2 keeps both recorded on tick 3.
		r1 := home.Result{Items: []home.Item{{AI: aiID, Why: "notice", Task: t1.ID, Code: "quiet"}}}
		afterFn(s, r1)
		equal(t, quietFn(s, d, clock).Items, []home.Item{{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"}})
		r2 := home.Result{Items: []home.Item{{AI: aiID, Why: "notice", Task: t2.ID, Code: "quiet"}}}
		afterFn(s, r2)
		equal(t, quietFn(s, d, clock), home.Result{})
	})
}

// Through StartLoop with NodeTables, main-line posts pass through the post
// wrapper untouched, jump replaces the queued mention in the store, and quiet
// fires once across ticks even after acknowledgement.
func TestRoundsLoopJumpAndQuiet(t *testing.T) {
	tables := home.NodeTables()
	localCmds := home.Table{}
	for k, v := range tables.Commands {
		localCmds[k] = v
	}
	localCmds["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		grp := proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Site", Kind: "project",
			Home: s.Home, Secretary: aiID, Forge: "none", Pass: 90, PassFoundation: 70,
			Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: 1},
		}
		own := person(ownerID, browser, "owner")
		sec := ai(aiID, laptop)
		worker := ai(briefA, laptop)
		task := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: id(70)}, Group: s.ID, Num: 1,
			Title: "Direct task", Slug: "direct-task", Origin: "direct", State: "active", Step: "local_check",
			Assignee: briefA, Pass: 90, Depends: []string{}, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
		}
		msg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(60)}, Group: s.ID, By: ownerID, Kind: "text",
			Timestamp: c.Now, Text: "Quick question", Mentions: []string{briefA}, Body: json.RawMessage(`{}`),
		}
		return home.Result{
			Rows:          []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(sec), home.RowOf(worker), home.RowOf(task), home.RowOf(msg)},
			Items:         []home.Item{{AI: briefA, Why: "mention", Msg: id(60)}},
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
	firstPush := host.next()
	equal(t, firstPush, runFrame(home.ItemsPush{
		Group: groupID,
		Items: []home.QueuedItem{{ID: 1, Item: home.Item{AI: briefA, Why: "mention", Msg: id(60)}}},
	}))

	// Jump retires item 1 and pushes item 2 with Jump: true.
	p.does(groupID, "jump", `,"msg":"`+id(60)+`"`)
	jumpPush := host.next()
	equal(t, jumpPush, runFrame(home.ItemsPush{
		Group: groupID,
		Items: []home.QueuedItem{{ID: 2, Item: home.Item{AI: briefA, Why: "mention", Msg: id(60), Jump: true}}},
	}))
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox WHERE id=1"), 0)

	// Acknowledge item 2, then advance the clock to test the quiet timer.
	runExec(t, w.db, "DELETE FROM inbox WHERE id=2")
	w.clock.Store(clock + 59_999)
	w.loop.Tick(time.UnixMilli(clock + 59_999))
	host.quiet()

	w.clock.Store(clock + 60_000)
	w.loop.Tick(time.UnixMilli(clock + 60_000))
	quietPush := host.next()
	equal(t, quietPush, runFrame(home.ItemsPush{
		Group: groupID,
		Items: []home.QueuedItem{{ID: 3, Item: home.Item{AI: aiID, Why: "notice", Task: id(70), Code: "quiet"}}},
	}))

	// Acknowledge the quiet notice; later ticks in the same quiet period send nothing.
	runExec(t, w.db, "DELETE FROM inbox WHERE id=3")
	w.clock.Store(clock + 180_000)
	w.loop.Tick(time.UnixMilli(clock + 180_000))
	host.quiet()
}
