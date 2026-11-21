package home_test

import (
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	mrgWorkA  = id(60)
	mrgWorkB  = id(61)
	mrgWorkC  = id(62)
	mrgMember = id(63)
	mrgHostB  = device(6)
)

const mrgSignedTS = clock - 500

type mrgDisk struct {
	pending []proto.MsgRow
}

func (*mrgDisk) Msg(string) (proto.MsgRow, bool)           { return proto.MsgRow{}, false }
func (*mrgDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (*mrgDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }

func (d *mrgDisk) Pending() []proto.MsgRow {
	if d == nil {
		return nil
	}
	return d.pending
}

func (*mrgDisk) Queued() []store.Item { return nil }

func mrgState(t *testing.T) *home.State {
	t.Helper()
	s := empty(t)
	s.Logs[browser.ID()] = logOf(t, browser, node, laptop, mrgHostB)
	s.Group = proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: groupID},
		Home:           node.ID(),
		Forge:          "github",
		Secretary:      aiID,
		Pass:           90,
		PassFoundation: 70,
		Limits:         proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	s.People[ownerID] = person(ownerID, browser, "owner")
	s.People[mrgMember] = person(mrgMember, guest, "member")
	s.AIs[aiID] = ai(aiID, laptop)
	s.AIs[mrgWorkA] = ai(mrgWorkA, laptop)
	s.AIs[mrgWorkB] = ai(mrgWorkB, mrgHostB)
	s.AIs[mrgWorkC] = ai(mrgWorkC, laptop)
	s.Seq, s.Num = 20, 3
	return s
}

func mrgTaskAtMerge(taskID string, num int) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 15},
		Group:     groupID,
		Num:       num,
		Title:     "Merge path changes",
		Slug:      "merge-path-changes",
		Origin:    "brief",
		Brief:     id(70),
		State:     "active",
		Step:      "merge",
		Commit:    "9f2c1ab",
		Assignee:  mrgWorkA,
		Reviewer:  mrgWorkB,
		Pass:      90,
		Depends:   []string{},
		Branch:    "task/1-merge-path-changes",
		PR:        28,
		PRURL:     "https://git.example/pull/28",
		Checks: proto.TaskChecks{
			Local: &proto.Check{OK: true, At: clock - 4000, Commit: "9f2c1ab"},
			Cloud: &proto.Check{OK: true, At: clock - 3000, Commit: "9f2c1ab"},
		},
		Review: &proto.Review{
			Round: 2, Failed: 1, Score: 94, OK: true,
			By: mrgWorkB, Commit: "9f2c1ab", At: clock - 2000,
		},
		Failures: 1,
		Thread: proto.ThreadSummary{
			Count:  7,
			Rounds: 5,
			Last:   &proto.ThreadMessage{By: mrgWorkB, Kind: "review", Text: "Ship it.", Timestamp: clock - 2000},
		},
		Stats:   proto.TaskStats{Added: 120, Removed: 10, Started: clock - 72*60_000},
		Created: clock - 72*60_000,
	}
}

func mrgMergedCmd(n byte, taskID, commit, mergeCommit string, added, removed int) proto.TaskMerged {
	return proto.TaskMerged{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: proto.ID(id(n)), Timestamp: mrgSignedTS, Group: proto.ID(groupID),
			},
			Op: "merged",
			ID: proto.ID(taskID),
		},
		Commit:      commit,
		MergeCommit: mergeCommit,
		Stats:       proto.DiffStats{Added: added, Removed: removed},
	}
}

func mrgConflictCmd(n byte, taskID, commit, detail string) proto.TaskConflict {
	return proto.TaskConflict{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: proto.ID(id(n)), Timestamp: mrgSignedTS, Group: proto.ID(groupID),
			},
			Op: "conflict",
			ID: proto.ID(taskID),
		},
		Commit: commit,
		Detail: detail,
	}
}

func mrgSummaryCmd(n byte, as string, thread *string, main bool, text string, mentions []proto.ID, re *string) proto.Post[proto.EmptyBody] {
	var tid *proto.ID
	if thread != nil {
		v := proto.ID(*thread)
		tid = &v
	}
	var rid *proto.ID
	if re != nil {
		v := proto.ID(*re)
		rid = &v
	}
	return proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(id(n)), Timestamp: mrgSignedTS,
			Group: proto.ID(groupID), As: proto.ID(as),
		},
		Kind:     "summary",
		Thread:   tid,
		Main:     main,
		Text:     text,
		Mentions: mentions,
		Re:       rid,
		Body:     proto.EmptyBody{},
	}
}

func mrgRun(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, payload any) home.Result {
	t.Helper()
	return briefApply(t, s, d, r, by, briefSign(by, payload))
}

func mrgAssertNoNulls(t *testing.T, v any, allowed ...string) []byte {
	t.Helper()
	raw := briefRaw(t, v)
	equal(t, proto.UnexpectedNulls(raw, allowed...), []string{})
	return raw
}

// Row 8: task merged updates stats, marks the task merged with stats.ended,
// expires pending cards of that task only, writes system task_merged in the
// thread, advances thread summary, and notifies the secretary.
func TestMergeMergedUpdatesTaskExpiresCardsAndWritesSystemMessage(t *testing.T) {
	s := mrgState(t)
	tid := id(71)
	otherID := id(72)
	s.Tasks[tid] = mrgTaskAtMerge(tid, 1)
	s.Tasks[otherID] = mrgTaskAtMerge(otherID, 2)

	appCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(80), Seq: 9}, Pos: 9, Group: groupID,
		Thread: &tid, Main: true, By: mrgWorkA, Kind: "approval", Timestamp: clock - 1500,
		Mentions: []string{ownerID},
		Body:     briefRaw(t, proto.ApprovalBody{Task: tid, AI: mrgWorkA, Tool: "Bash", Command: "ls", Category: "command", Approvers: []string{browser.ID()}}),
		State:    &proto.CardState{Status: "pending"},
	}
	decCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(81), Seq: 10}, Pos: 10, Group: groupID,
		Thread: &tid, Main: true, By: mrgWorkA, Kind: "decision", Timestamp: clock - 1200,
		Mentions: []string{},
		Body:     briefRaw(t, proto.DecisionBody{Task: &tid, Question: "Where?", Options: []proto.DecisionOption{}, To: []string{browser.ID()}}),
		State:    &proto.CardState{Status: "pending"},
	}
	mainCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(82), Seq: 11}, Pos: 11, Group: groupID,
		Thread: nil, Main: false, By: aiID, Kind: "brief", Timestamp: clock - 1100,
		Mentions: []string{}, State: &proto.CardState{Status: "pending"},
	}
	otherCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(83), Seq: 12}, Pos: 12, Group: groupID,
		Thread: &otherID, Main: true, By: mrgWorkA, Kind: "approval", Timestamp: clock - 1000,
		Mentions: []string{ownerID}, State: &proto.CardState{Status: "pending"},
	}
	d := &mrgDisk{pending: []proto.MsgRow{mainCard, appCard, otherCard, decCard}}

	res := mrgRun(t, s, d, receipts{}, laptop, mrgMergedCmd(90, tid, "9f2c1ab", "c1d9e7a", 318, 42))
	equal(t, res.Answer, store.Outcome{OK: true})

	ended := int64(clock)
	wantTask := mrgTaskAtMerge(tid, 1)
	wantTask.Seq = 21
	wantTask.State = "merged"
	wantTask.Wait = nil
	wantTask.Stats = proto.TaskStats{Added: 318, Removed: 42, Started: clock - 72*60_000, Ended: &ended}
	wantTask.Thread = proto.ThreadSummary{
		Count:  8,
		Rounds: 5,
		Last:   &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock},
	}

	wantApp := appCard
	wantApp.Seq, wantApp.State = 22, &proto.CardState{Status: "expired", At: &ended}
	wantDec := decCard
	wantDec.Seq, wantDec.State = 23, &proto.CardState{Status: "expired", At: &ended}

	wantSys := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(90), Seq: 24},
		Pos:       24,
		Group:     groupID,
		Thread:    &tid,
		Main:      false,
		By:        "system",
		Kind:      "system",
		Timestamp: clock,
		Mentions:  []string{},
		Body:      briefRaw(t, proto.TaskMergedBody{SystemCode: proto.SystemCode{Code: "task_merged"}, Task: tid, Commit: "9f2c1ab", MergeCommit: "c1d9e7a"}),
	}

	equal(t, briefRows(res), []any{wantTask, wantApp, wantDec, wantSys})
	equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: tid, Msg: id(90), Code: "task_merged"}})

	var decodedSys proto.MessageRow[proto.TaskMergedBody, proto.NoCard]
	equal(t, proto.Decode(mrgAssertNoNulls(t, wantSys, "/re", "/state"), &decodedSys), nil)
	mrgAssertNoNulls(t, wantTask, "/wait")
}

// Boundary check: zero diff stats are accepted, and a group without a secretary
// queues no notice item.
func TestMergeMergedZeroStatsAndNoSecretary(t *testing.T) {
	s := mrgState(t)
	s.Group.Secretary = ""
	tid := id(71)
	s.Tasks[tid] = mrgTaskAtMerge(tid, 1)

	res := mrgRun(t, s, &mrgDisk{}, receipts{}, laptop, mrgMergedCmd(90, tid, "9f2c1ab", "c1d9e7a", 0, 0))
	equal(t, res.Answer, store.Outcome{OK: true})
	moved := res.Rows[0].Value().(proto.TaskRow)
	equal(t, moved.Stats.Added, 0)
	equal(t, moved.Stats.Removed, 0)
	equal(t, len(res.Items), 0)
}

// Row 8 release: dependent queued tasks release in sorted ID order when all
// their dependencies are merged, set stats.started to c.Now, recompute card
// waits from Pending, and set Msg to the task ID on released assign items.
func TestMergeMergedReleasesDependentTasksInSortedOrder(t *testing.T) {
	s := mrgState(t)
	doneID := id(25)
	mergedID := id(26)
	stillOpenID := id(27)
	depFirst := id(31)
	depSecond := id(32)
	depBlocked := id(33)
	depUnrelated := id(34)
	depAlreadyActive := id(35)
	depCancelled := id(36)

	done := mrgTaskAtMerge(doneID, 1)
	done.State = "merged"
	s.Tasks[doneID] = done
	s.Tasks[mergedID] = mrgTaskAtMerge(mergedID, 2)
	s.Tasks[stillOpenID] = mrgTaskAtMerge(stillOpenID, 3)

	confirmedAt := int64(clock - 600_000)
	makeQueued := func(tid string, num int, assignee string, brief string, deps []string) proto.TaskRow {
		return proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: tid, Seq: 6},
			Group:     groupID, Num: num, Title: "Dependent", Slug: "dependent",
			Origin: "brief", Brief: brief, State: "queued", Step: "branch",
			Assignee: assignee, Reviewer: mrgWorkC, Pass: 90, Depends: deps,
			Wait:    &proto.TaskWait{Kind: "depends", Since: confirmedAt},
			Stats:   proto.TaskStats{Started: confirmedAt},
			Created: confirmedAt,
		}
	}

	// depSecond is inserted alongside depFirst to verify sorted ID iteration.
	s.Tasks[depSecond] = makeQueued(depSecond, 5, mrgWorkB, id(70), []string{doneID, mergedID})
	s.Tasks[depFirst] = makeQueued(depFirst, 4, mrgWorkA, id(70), []string{mergedID})
	// Blocked by stillOpenID: stays queued and is omitted from Rows and Items.
	s.Tasks[depBlocked] = makeQueued(depBlocked, 6, mrgWorkA, id(70), []string{mergedID, stillOpenID})
	// Queued on an already-merged task without listing mergedID: untouched.
	s.Tasks[depUnrelated] = makeQueued(depUnrelated, 7, mrgWorkA, id(70), []string{doneID})
	// Active and cancelled tasks listing mergedID: untouched.
	activeDep := makeQueued(depAlreadyActive, 8, mrgWorkA, id(70), []string{mergedID})
	activeDep.State, activeDep.Wait = "active", nil
	s.Tasks[depAlreadyActive] = activeDep
	cancelledDep := makeQueued(depCancelled, 9, mrgWorkA, id(70), []string{mergedID})
	cancelledDep.State, cancelledDep.Wait = "cancelled", nil
	s.Tasks[depCancelled] = cancelledDep

	// depFirst has a pending decision card; depSecond has a pending approval card.
	decPending := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(84), Seq: 8}, Pos: 8, Group: groupID,
		Thread: &depFirst, Main: true, By: aiID, Kind: "decision", Timestamp: clock - 900,
		Mentions: []string{},
		Body:     briefRaw(t, proto.DecisionBody{Task: &depFirst, Question: "Format?", Options: []proto.DecisionOption{}, To: []string{guest.ID()}}),
		State:    &proto.CardState{Status: "pending"},
	}
	appPending := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(85), Seq: 9}, Pos: 9, Group: groupID,
		Thread: &depSecond, Main: true, By: mrgWorkB, Kind: "approval", Timestamp: clock - 800,
		Mentions: []string{ownerID},
		Body:     briefRaw(t, proto.ApprovalBody{Task: depSecond, AI: mrgWorkB, Tool: "Bash", Command: "make", Category: "command", Approvers: []string{browser.ID()}}),
		State:    &proto.CardState{Status: "pending"},
	}
	mainPending := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(86), Seq: 10}, Pos: 10, Group: groupID,
		Thread: nil, Kind: "brief", State: &proto.CardState{Status: "pending"},
	}

	d := &mrgDisk{pending: []proto.MsgRow{mainPending, decPending, appPending}}

	res := mrgRun(t, s, d, receipts{}, laptop, mrgMergedCmd(91, mergedID, "9f2c1ab", "c1d9e7a", 10, 2))
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, len(res.Rows), 4)

	relFirst := res.Rows[2].Value().(proto.TaskRow)
	equal(t, relFirst.ID, depFirst)
	equal(t, relFirst.State, "active")
	equal(t, relFirst.Step, "branch")
	equal(t, relFirst.Stats.Started, int64(clock))
	equal(t, relFirst.Created, confirmedAt)
	equal(t, relFirst.Wait, &proto.TaskWait{Kind: "decision", Msg: decPending.ID, Who: []string{guest.ID()}, Since: decPending.Timestamp})

	relSecond := res.Rows[3].Value().(proto.TaskRow)
	equal(t, relSecond.ID, depSecond)
	equal(t, relSecond.State, "active")
	equal(t, relSecond.Stats.Started, int64(clock))
	equal(t, relSecond.Wait, &proto.TaskWait{Kind: "approval", Msg: appPending.ID, Who: []string{browser.ID()}, Since: appPending.Timestamp})

	equal(t, res.Items, []home.Item{
		{AI: aiID, Why: "notice", Task: mergedID, Msg: id(91), Code: "task_merged"},
		{AI: mrgWorkA, Why: "assign", Task: depFirst, Msg: depFirst},
		{AI: mrgWorkB, Why: "assign", Task: depSecond, Msg: depSecond},
	})
}

func TestMergeMergedRefusals(t *testing.T) {
	tid := id(71)
	for name, tc := range map[string]struct {
		by   identity.Key
		raw  string
		cmd  func(*proto.TaskMerged)
		task func(*proto.TaskRow)
		code string
	}{
		"invalid json field":  {raw: `{"t":"task","n":"` + id(90) + `","ts":1763600000000,"g":"` + groupID + `","op":"merged","id":"` + tid + `","commit":5}`, code: "bad_request"},
		"missing task":        {cmd: func(p *proto.TaskMerged) { p.ID = proto.ID(id(99)) }, code: "not_found"},
		"by owner browser":    {by: browser, code: "forbidden"},
		"by another host":     {by: mrgHostB, code: "forbidden"},
		"by assignee with as": {cmd: func(p *proto.TaskMerged) { p.As = proto.ID(mrgWorkA) }, code: "forbidden"},
		"wrong step":          {task: func(r *proto.TaskRow) { r.Step = "review" }, code: "conflict"},
		"paused task":         {task: func(r *proto.TaskRow) { r.State = "paused" }, code: "conflict"},
		"already merged":      {task: func(r *proto.TaskRow) { r.State = "merged" }, code: "conflict"},
		"wrong commit":        {cmd: func(p *proto.TaskMerged) { p.Commit = "other" }, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := mrgState(t)
			row := mrgTaskAtMerge(tid, 1)
			if tc.task != nil {
				tc.task(&row)
			}
			s.Tasks[tid] = row
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			var env proto.Envelope
			if tc.raw != "" {
				env = by.Sign(tc.raw)
			} else {
				cmd := mrgMergedCmd(90, tid, "9f2c1ab", "c1d9e7a", 10, 2)
				if tc.cmd != nil {
					tc.cmd(&cmd)
				}
				env = briefSign(by, cmd)
			}
			briefRefused(t, briefApply(t, s, &mrgDisk{}, receipts{}, by, env), tc.code)
		})
	}
}

// Row 9: task conflict sets step local_check and wait conflict without
// counting a failure, writes a checks message at stage merge (omitting detail
// when empty), and notifies the secretary when present.
func TestMergeConflictMovesTaskToLocalCheckAndWritesMergeChecks(t *testing.T) {
	detail := "CONFLICT (content): Merge conflict in path.go"
	for name, tc := range map[string]struct {
		detail     string
		wantDetail *string
		secretary  string
		wantItems  []home.Item
	}{
		"with detail and secretary": {
			detail:     detail,
			wantDetail: &detail,
			secretary:  aiID,
			wantItems:  []home.Item{{AI: aiID, Why: "notice", Task: id(71), Msg: id(90), Code: "conflict"}},
		},
		"empty detail and no secretary": {
			detail:     "",
			wantDetail: nil,
			secretary:  "",
			wantItems:  nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := mrgState(t)
			s.Group.Secretary = tc.secretary
			tid := id(71)
			s.Tasks[tid] = mrgTaskAtMerge(tid, 1)

			res := mrgRun(t, s, &mrgDisk{}, receipts{}, laptop, mrgConflictCmd(90, tid, "9f2c1ab", tc.detail))
			equal(t, res.Answer, store.Outcome{OK: true})

			wantTask := mrgTaskAtMerge(tid, 1)
			wantTask.Seq = 21
			wantTask.Step = "local_check"
			wantTask.Wait = &proto.TaskWait{Kind: "conflict", Since: clock}
			wantTask.Failures = 1
			wantTask.Thread = proto.ThreadSummary{
				Count:  8,
				Rounds: 5,
				Last:   &proto.ThreadMessage{By: "system", Kind: "checks", Text: "", Timestamp: clock},
			}

			wantBody := proto.ChecksBody{
				Stage:  "merge",
				OK:     false,
				Commit: "9f2c1ab",
				Items:  []proto.CheckItem{{Name: "merge", OK: false, Detail: tc.wantDetail}},
			}
			wantMsg := proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: id(90), Seq: 22},
				Pos:       22,
				Group:     groupID,
				Thread:    &tid,
				Main:      false,
				By:        "system",
				Kind:      "checks",
				Timestamp: clock,
				Mentions:  []string{},
				Body:      briefRaw(t, wantBody),
			}
			equal(t, briefRows(res), []any{wantTask, wantMsg})
			equal(t, res.Items, tc.wantItems)

			rawMsg := mrgAssertNoNulls(t, wantMsg, "/re", "/state")
			var decoded proto.MessageRow[proto.ChecksBody, proto.NoCard]
			equal(t, proto.Decode(rawMsg, &decoded), nil)
			equal(t, strings.Contains(string(wantMsg.Body), `"detail"`), tc.wantDetail != nil)
		})
	}
}

func TestMergeConflictRefusals(t *testing.T) {
	tid := id(71)
	for name, tc := range map[string]struct {
		by   identity.Key
		raw  string
		cmd  func(*proto.TaskConflict)
		task func(*proto.TaskRow)
		code string
	}{
		"invalid json":        {raw: `{"t":"task","n":"` + id(90) + `","ts":1763600000000,"g":"` + groupID + `","op":"conflict","id":"` + tid + `","commit":5}`, code: "bad_request"},
		"missing task":        {cmd: func(p *proto.TaskConflict) { p.ID = proto.ID(id(99)) }, code: "not_found"},
		"by owner browser":    {by: browser, code: "forbidden"},
		"by another host":     {by: mrgHostB, code: "forbidden"},
		"by assignee with as": {cmd: func(p *proto.TaskConflict) { p.As = proto.ID(mrgWorkA) }, code: "forbidden"},
		"wrong step":          {task: func(r *proto.TaskRow) { r.Step = "review" }, code: "conflict"},
		"paused task":         {task: func(r *proto.TaskRow) { r.State = "paused" }, code: "conflict"},
		"wrong commit":        {cmd: func(p *proto.TaskConflict) { p.Commit = "other" }, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := mrgState(t)
			row := mrgTaskAtMerge(tid, 1)
			if tc.task != nil {
				tc.task(&row)
			}
			s.Tasks[tid] = row
			by := laptop
			if tc.by != nil {
				by = tc.by
			}
			var env proto.Envelope
			if tc.raw != "" {
				env = by.Sign(tc.raw)
			} else {
				cmd := mrgConflictCmd(90, tid, "9f2c1ab", "conflict")
				if tc.cmd != nil {
					tc.cmd(&cmd)
				}
				env = briefSign(by, cmd)
			}
			briefRefused(t, briefApply(t, s, &mrgDisk{}, receipts{}, by, env), tc.code)
		})
	}
}

// post/summary writes the summary message with main=true, fills SummaryBody
// from the merged task row (score and rounds from task.Review, minutes from
// stats.started to stats.ended, optional PR when PR > 0), and advances the
// task's thread summary with a 200-character preview.
func TestMergeSummaryBuildsBodyAndUpdatesThread(t *testing.T) {
	tid := id(71)
	reID := id(79)
	exact200 := strings.Repeat("界", 200)
	over200 := exact200 + "尾"
	pr1, pr28 := 1, 28

	for name, tc := range map[string]struct {
		started     int64
		ended       int64
		pr          int
		wantPR      *int
		postMain    bool
		text        string
		wantPreview string
		mentions    []proto.ID
		wantMent    []string
		re          *string
		wantMinutes int
	}{
		"remote pr 28, 72 minutes, 201 runes cut to 200, main forced true": {
			started: clock - 72*60_000, ended: clock, pr: 28, wantPR: &pr28,
			postMain: false, text: over200, wantPreview: exact200,
			mentions: []proto.ID{proto.ID(ownerID), proto.ID(mrgWorkA)}, wantMent: []string{ownerID, mrgWorkA},
			re: &reID, wantMinutes: 72,
		},
		"pr boundary 1, exact 1 minute, exact 200 runes": {
			started: clock - 60_000, ended: clock, pr: 1, wantPR: &pr1,
			postMain: true, text: exact200, wantPreview: exact200,
			mentions: []proto.ID{}, wantMent: []string{},
			re: nil, wantMinutes: 1,
		},
		"no remote pr 0 omitted, sub-minute 59999 ms is 0": {
			started: clock - 59_999, ended: clock, pr: 0, wantPR: nil,
			postMain: true, text: "Done.", wantPreview: "Done.",
			mentions: []proto.ID{proto.ID(mrgMember)}, wantMent: []string{mrgMember},
			re: nil, wantMinutes: 0,
		},
		"zero elapsed ms and empty text": {
			started: clock, ended: clock, pr: 0, wantPR: nil,
			postMain: true, text: "", wantPreview: "",
			mentions: []proto.ID{proto.ID(mrgWorkB)}, wantMent: []string{mrgWorkB},
			re: nil, wantMinutes: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := mrgState(t)
			merged := mrgTaskAtMerge(tid, 1)
			ended := tc.ended
			merged.State = "merged"
			merged.PR = tc.pr
			merged.Stats = proto.TaskStats{Added: 318, Removed: 42, Started: tc.started, Ended: &ended}
			s.Tasks[tid] = merged

			cmd := mrgSummaryCmd(95, aiID, &tid, tc.postMain, tc.text, tc.mentions, tc.re)
			env := briefSign(laptop, cmd)
			res := briefApply(t, s, &mrgDisk{}, receipts{}, laptop, env)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, len(res.Items), 0)

			wantBody := proto.SummaryBody{
				Task:    tid,
				Score:   94,
				Rounds:  2,
				Minutes: tc.wantMinutes,
				Added:   318,
				Removed: 42,
				PR:      tc.wantPR,
			}
			wantMsg := proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: id(95), Seq: 21},
				Pos:       21,
				Group:     groupID,
				Thread:    &tid,
				Main:      true,
				By:        aiID,
				Kind:      "summary",
				Timestamp: mrgSignedTS,
				Text:      tc.text,
				Mentions:  tc.wantMent,
				Re:        tc.re,
				Body:      briefRaw(t, wantBody),
				Env:       &env,
			}
			wantTask := merged
			wantTask.Seq = 22
			wantTask.Thread = proto.ThreadSummary{
				Count:  8,
				Rounds: 5,
				Last:   &proto.ThreadMessage{By: aiID, Kind: "summary", Text: tc.wantPreview, Timestamp: mrgSignedTS},
			}
			equal(t, briefRows(res), []any{wantMsg, wantTask})

			rawMsg := mrgAssertNoNulls(t, wantMsg, "/re", "/state")
			var decoded proto.MessageRow[proto.SummaryBody, proto.NoCard]
			equal(t, proto.Decode(rawMsg, &decoded), nil)
			equal(t, strings.Contains(string(wantMsg.Body), `"pr"`), tc.wantPR != nil)
		})
	}
}

type mrgSummaryRefusalCase struct {
	by        identity.Key
	as        string
	noSec     bool
	raw       string
	thread    *string
	mentions  []proto.ID
	taskState string
	code      string
}

func mrgSetupSummaryRefusal(t *testing.T, tid string, tc mrgSummaryRefusalCase) (*home.State, identity.Key, proto.Envelope) {
	t.Helper()
	s := mrgState(t)
	if tc.noSec {
		s.Group.Secretary = ""
	}
	ended := int64(clock)
	row := mrgTaskAtMerge(tid, 1)
	row.State, row.Stats.Ended = mrgOrDefault(tc.taskState, "merged"), &ended
	s.Tasks[tid] = row
	by, as := mrgSummarySigner(tc)
	return s, by, mrgSummaryRefusalEnv(by, as, tc)
}

func mrgOrDefault(val, def string) string {
	if val != "" {
		return val
	}
	return def
}

func mrgSummarySigner(tc mrgSummaryRefusalCase) (identity.Key, string) {
	if tc.by != nil {
		return tc.by, tc.as
	}
	if tc.as != "" {
		return laptop, tc.as
	}
	return laptop, aiID
}

func mrgSummaryRefusalEnv(by identity.Key, as string, tc mrgSummaryRefusalCase) proto.Envelope {
	if tc.raw != "" {
		return by.Sign(tc.raw)
	}
	mentions := tc.mentions
	if mentions == nil {
		mentions = []proto.ID{}
	}
	return briefSign(by, mrgSummaryCmd(95, as, tc.thread, true, "Summary.", mentions, nil))
}

func TestMergeSummaryRefusals(t *testing.T) {
	tid := id(71)
	missingID := id(99)
	strangerID := id(98)

	for name, tc := range map[string]mrgSummaryRefusalCase{
		"by person":               {by: browser, as: "", thread: &tid, code: "forbidden"},
		"by host without as":      {by: laptop, as: "", thread: &tid, code: "forbidden"},
		"by working AI":           {by: laptop, as: mrgWorkA, thread: &tid, code: "forbidden"},
		"group without secretary": {by: laptop, as: aiID, noSec: true, thread: &tid, code: "forbidden"},
		"invalid json":            {raw: `{"t":"post","n":"` + id(95) + `","ts":1763599999500,"g":"` + groupID + `","as":"` + aiID + `","kind":"summary","thread":"` + tid + `","main":true,"text":7}`, code: "bad_request"},
		"null thread":             {thread: nil, code: "bad_request"},
		"duplicate mentions":      {thread: &tid, mentions: []proto.ID{proto.ID(ownerID), proto.ID(mrgWorkA), proto.ID(ownerID)}, code: "bad_request"},
		"missing task":            {thread: &missingID, mentions: []proto.ID{proto.ID(strangerID)}, code: "not_found"},
		"queued task":             {thread: &tid, taskState: "queued", code: "conflict"},
		"active task":             {thread: &tid, taskState: "active", code: "conflict"},
		"paused task":             {thread: &tid, taskState: "paused", code: "conflict"},
		"failed task":             {thread: &tid, taskState: "failed", code: "conflict"},
		"cancelled task":          {thread: &tid, taskState: "cancelled", code: "conflict"},
		"non-member mention":      {thread: &tid, mentions: []proto.ID{proto.ID(ownerID), proto.ID(strangerID)}, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s, by, env := mrgSetupSummaryRefusal(t, tid, tc)
			briefRefused(t, briefApply(t, s, &mrgDisk{}, receipts{}, by, env), tc.code)
		})
	}
}

func TestMergeCommandsAnswerRepeatedNonceFromReceipt(t *testing.T) {
	s := mrgState(t)
	tid := id(71)
	s.Tasks[tid] = mrgTaskAtMerge(tid, 1)
	r := receipts{}
	d := &mrgDisk{}

	mergeEnv := briefSign(laptop, mrgMergedCmd(90, tid, "9f2c1ab", "c1d9e7a", 50, 5))
	first := briefApply(t, s, d, r, laptop, mergeEnv)
	equal(t, first.Answer, store.Outcome{OK: true})
	s.Commit(first)
	second := briefApply(t, s, d, r, laptop, mergeEnv)
	equal(t, second, home.Result{Answer: store.Outcome{OK: true}})
	equal(t, s.Tasks[tid].Thread.Count, 8)
}
