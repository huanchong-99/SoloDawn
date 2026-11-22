package home_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	pauAdminKey = device(6)
	pauAdminID  = id(60)
	pauMemberID = id(61)
	pauWorkA    = id(62)
	pauWorkB    = id(63)
	pauWorkC    = id(64)
)

const pauSignTS = clock - 2500

func pauState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Logs[pauAdminKey.ID()] = logOf(t, pauAdminKey)
	s.People[pauAdminID] = person(pauAdminID, pauAdminKey, "admin")
	s.People[pauMemberID] = person(pauMemberID, guest, "member")
	for _, member := range []string{pauWorkA, pauWorkB, pauWorkC} {
		s.AIs[member] = ai(member, laptop)
	}
	return s
}

func pauCmd(by identity.Key, op string, n byte, more string) proto.Envelope {
	return by.Sign(fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":%q%s}`, id(n), pauSignTS, groupID, op, more))
}

func pauSysRow(msgID string, seq int64, code, by string) proto.MsgRow {
	body := json.RawMessage(fmt.Sprintf(`{"code":%q,"by":%q}`, code, by))
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: seq}, Pos: seq, Group: groupID,
		By: "system", Kind: "system", Timestamp: clock, Mentions: []string{}, Body: body,
	}
}

func pauPtr(v string) *string { return &v }

// A4.7, home half: an owner or an admin pauses an unpaused group, setting
// group.paused with the home's clock and writing main-line system group_paused.
func TestPauseSetsPausedAndWritesSystemMessage(t *testing.T) {
	for name, tc := range map[string]struct {
		key    identity.Key
		member string
	}{
		"by owner": {key: browser, member: ownerID},
		"by admin": {key: pauAdminKey, member: pauAdminID},
	} {
		t.Run(name, func(t *testing.T) {
			s := pauState(t)
			s.Group.Checks = []string{}
			res := briefApply(t, s, disk{}, receipts{}, tc.key, pauCmd(tc.key, "pause", 70, ""))
			if s.Group.Paused != nil {
				t.Fatalf("command mutated state before Commit: %+v", s.Group.Paused)
			}
			wantGroup := s.Group
			wantGroup.Seq = 11
			wantGroup.Paused = &proto.GroupPause{By: tc.member, At: clock}
			equal(t, res, home.Result{Rows: res.Rows, Answer: store.Outcome{OK: true}})
			rows := briefRows(res)
			equal(t, rows, []any{wantGroup, pauSysRow(id(70), 12, "group_paused", tc.member)})
			grpNoNulls(t, rows[0])
			grpNoNulls(t, rows[1], "/thread", "/re", "/state")

			s.Commit(res)
			equal(t, s.Group, wantGroup)
			equal(t, s.Seq, int64(12))
		})
	}
}

// The system message body is encoded without HTML escaping.
func TestPauseBodyDoesNotEscapeHTML(t *testing.T) {
	s := pauState(t)
	delete(s.People, pauAdminID)
	rawID := "<admin&1>"
	s.People[rawID] = person(rawID, pauAdminKey, "admin")

	paused := briefApply(t, s, disk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "pause", 70, ""))
	equal(t, paused.Rows[1].Value().(proto.MsgRow).Body, json.RawMessage(`{"code":"group_paused","by":"<admin&1>"}`))
	s.Commit(paused)

	resumed := briefApply(t, s, disk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "resume", 71, ""))
	equal(t, resumed.Rows[1].Value().(proto.MsgRow).Body, json.RawMessage(`{"code":"group_resumed","by":"<admin&1>"}`))
}

// Pausing a group that is already paused is accepted and changes nothing.
func TestPauseAlreadyPausedChangesNothing(t *testing.T) {
	s := pauState(t)
	original := &proto.GroupPause{By: ownerID, At: clock - 9000}
	s.Group.Paused = original

	res := briefApply(t, s, disk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "pause", 70, ""))
	equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
	s.Commit(res)
	equal(t, s.Group.Paused, original)
	equal(t, s.Seq, int64(10))
}

// Resuming a group that is not paused is accepted and changes nothing, even
// when active tasks exist.
func TestPauseResumeUnpausedGroupChangesNothing(t *testing.T) {
	s := pauState(t)
	s.Tasks[id(31)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(31)}, State: "active", Step: "local_check", Assignee: pauWorkA}
	s.Tasks[id(32)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(32)}, State: "active", Step: "review", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c1"}

	res := briefApply(t, s, disk{}, receipts{}, browser, pauCmd(browser, "resume", 71, ""))
	equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
	s.Commit(res)
	equal(t, s.Seq, int64(10))
}

func TestPauseAndResumeRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		op     string
		paused bool
		by     identity.Key
		more   string
		code   string
	}{
		"pause with empty as":                     {op: "pause", by: laptop, more: `,"as":""`, code: "bad_request"},
		"resume with non-string as":               {op: "resume", paused: true, by: laptop, more: `,"as":1`, code: "bad_request"},
		"pause by ordinary member":                {op: "pause", by: guest, code: "forbidden"},
		"pause already paused by ordinary member": {op: "pause", paused: true, by: guest, code: "forbidden"},
		"resume by ordinary member":               {op: "resume", paused: true, by: guest, code: "forbidden"},
		"resume unpaused by ordinary member":      {op: "resume", by: guest, code: "forbidden"},
		"pause by secretary":                      {op: "pause", by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"pause already paused by secretary":       {op: "pause", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"resume by secretary":                     {op: "resume", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"resume unpaused by secretary":            {op: "resume", by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"pause by working AI":                     {op: "pause", by: laptop, more: fmt.Sprintf(`,"as":%q`, pauWorkA), code: "forbidden"},
		"resume by working AI":                    {op: "resume", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, pauWorkA), code: "forbidden"},
	} {
		t.Run(name, func(t *testing.T) {
			s := pauState(t)
			s.Tasks[id(31)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(31)}, State: "active", Step: "local_check", Assignee: pauWorkA}
			if tc.paused {
				s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
			}
			res := briefApply(t, s, disk{}, receipts{}, tc.by, pauCmd(tc.by, tc.op, 72, tc.more))
			briefRefused(t, res, tc.code)
		})
	}
}

// Row 24 and B-18: resuming clears group.paused, writes group_resumed, wakes
// active wait-free tasks before merge, wakes unqueued active review tasks, and
// skips tasks with waits, tasks at step merge, and non-active tasks.
func TestPauseResumeClearsPausedAndWakesTasksByRow24(t *testing.T) {
	s := pauState(t)
	s.Group.Checks = []string{}
	s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
	wait := func(kind string) *proto.TaskWait { return &proto.TaskWait{Kind: kind, Since: clock - 4000} }
	s.Tasks = map[string]proto.TaskRow{
		id(31): {RowHeader: proto.RowHeader{Type: "task", ID: id(31)}, State: "active", Step: "branch", Assignee: pauWorkA},
		id(32): {RowHeader: proto.RowHeader{Type: "task", ID: id(32)}, State: "active", Step: "local_check", Assignee: pauWorkB},
		id(33): {RowHeader: proto.RowHeader{Type: "task", ID: id(33)}, State: "active", Step: "cloud_check", Assignee: pauWorkC, Commit: "c3"},
		id(34): {RowHeader: proto.RowHeader{Type: "task", ID: id(34)}, State: "active", Step: "review", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c4"},
		id(35): {RowHeader: proto.RowHeader{Type: "task", ID: id(35)}, State: "active", Step: "review", Assignee: pauWorkB, Reviewer: pauWorkC, Commit: "c5", Wait: wait("approval")},
		id(36): {RowHeader: proto.RowHeader{Type: "task", ID: id(36)}, State: "active", Step: "review", Assignee: pauWorkC, Commit: "c6", Wait: wait("reviewer")},
		id(37): {RowHeader: proto.RowHeader{Type: "task", ID: id(37)}, State: "active", Step: "local_check", Assignee: pauWorkA, Wait: wait("approval")},
		id(38): {RowHeader: proto.RowHeader{Type: "task", ID: id(38)}, State: "active", Step: "local_check", Assignee: pauWorkB, Wait: wait("conflict")},
		id(39): {RowHeader: proto.RowHeader{Type: "task", ID: id(39)}, State: "active", Step: "merge", Assignee: pauWorkC, Reviewer: pauWorkA, Commit: "c9"},
		id(40): {RowHeader: proto.RowHeader{Type: "task", ID: id(40)}, State: "paused", Step: "review", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c10", Wait: wait("manual")},
		id(41): {RowHeader: proto.RowHeader{Type: "task", ID: id(41)}, State: "failed", Step: "review", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c11"},
		id(42): {RowHeader: proto.RowHeader{Type: "task", ID: id(42)}, State: "queued", Step: "branch", Assignee: pauWorkA, Wait: wait("depends")},
		id(43): {RowHeader: proto.RowHeader{Type: "task", ID: id(43)}, State: "merged", Step: "merge", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c13"},
		id(44): {RowHeader: proto.RowHeader{Type: "task", ID: id(44)}, State: "cancelled", Step: "review", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c14"},
	}

	res := briefApply(t, s, disk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "resume", 73, ""))
	if s.Group.Paused == nil {
		t.Fatal("command mutated state before Commit")
	}
	wantGroup := s.Group
	wantGroup.Seq = 11
	wantGroup.Paused = nil
	wantItems := []home.Item{
		{AI: pauWorkA, Why: "resume", Task: id(31)},
		{AI: pauWorkB, Why: "resume", Task: id(32)},
		{AI: pauWorkC, Why: "resume", Task: id(33)},
		{AI: pauWorkA, Why: "resume", Task: id(34)},
		{AI: pauWorkB, Why: "review", Task: id(34), Commit: "c4"},
		{AI: pauWorkC, Why: "review", Task: id(35), Commit: "c5"},
	}
	equal(t, res, home.Result{Rows: res.Rows, Items: wantItems, Answer: store.Outcome{OK: true}})
	rows := briefRows(res)
	equal(t, rows, []any{wantGroup, pauSysRow(id(73), 12, "group_resumed", pauAdminID)})
	grpNoNulls(t, rows[0], "/paused")
	grpNoNulls(t, rows[1], "/thread", "/re", "/state")

	s.Commit(res)
	equal(t, s.Group, wantGroup)
	equal(t, s.Seq, int64(12))
}

// Pins every operand of IfNone matching against c.Disk.Queued(): why, AI, task
// and commit, including nil pointers and already-queued resume items.
func TestPauseResumeQueuedFilterPinsEveryField(t *testing.T) {
	taskID, otherTask := id(31), id(32)
	both := []home.Item{
		{AI: pauWorkA, Why: "resume", Task: taskID},
		{AI: pauWorkB, Why: "review", Task: taskID, Commit: "c1"},
	}
	resumeOnly := []home.Item{
		{AI: pauWorkA, Why: "resume", Task: taskID},
	}
	for name, tc := range map[string]struct {
		queued []store.Item
		want   []home.Item
	}{
		"empty queue": {
			want: both,
		},
		"matching review already queued": {
			queued: []store.Item{{Why: "review", AI: pauWorkB, Task: pauPtr(taskID), Commit: pauPtr("c1")}},
			want:   resumeOnly,
		},
		"matching resume already queued still queues resume": {
			queued: []store.Item{{Why: "resume", AI: pauWorkA, Task: pauPtr(taskID), Commit: nil}},
			want:   both,
		},
		"different why does not suppress review": {
			queued: []store.Item{{Why: "merge", AI: pauWorkB, Task: pauPtr(taskID), Commit: pauPtr("c1")}},
			want:   both,
		},
		"different AI does not suppress review": {
			queued: []store.Item{{Why: "review", AI: pauWorkC, Task: pauPtr(taskID), Commit: pauPtr("c1")}},
			want:   both,
		},
		"different task does not suppress review": {
			queued: []store.Item{{Why: "review", AI: pauWorkB, Task: pauPtr(otherTask), Commit: pauPtr("c1")}},
			want:   both,
		},
		"nil task does not suppress review": {
			queued: []store.Item{{Why: "review", AI: pauWorkB, Task: nil, Commit: pauPtr("c1")}},
			want:   both,
		},
		"different commit does not suppress review": {
			queued: []store.Item{{Why: "review", AI: pauWorkB, Task: pauPtr(taskID), Commit: pauPtr("c0")}},
			want:   both,
		},
		"nil commit does not suppress review": {
			queued: []store.Item{{Why: "review", AI: pauWorkB, Task: pauPtr(taskID), Commit: nil}},
			want:   both,
		},
		"matching review after non-matching items": {
			queued: []store.Item{
				{Why: "confirmed"},
				{Why: "review", AI: pauWorkB, Task: pauPtr(taskID), Commit: pauPtr("c0")},
				{Why: "review", AI: pauWorkB, Task: pauPtr(taskID), Commit: pauPtr("c1")},
			},
			want: resumeOnly,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := pauState(t)
			s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
			s.Tasks[taskID] = proto.TaskRow{
				RowHeader: proto.RowHeader{Type: "task", ID: taskID}, State: "active", Step: "review",
				Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c1",
			}
			res := briefApply(t, s, tskDisk{queued: tc.queued}, receipts{}, browser, pauCmd(browser, "resume", 74, ""))
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, res.Items, tc.want)
		})
	}
}

func TestPauseRepeatedCommandUsesReceipt(t *testing.T) {
	s, held := pauState(t), receipts{}
	pauseEnv := pauCmd(browser, "pause", 75, "")

	first := briefApply(t, s, disk{}, held, browser, pauseEnv)
	equal(t, len(first.Rows), 2)
	s.Commit(first)

	second := briefApply(t, s, disk{}, held, browser, pauseEnv)
	equal(t, second, home.Result{Answer: store.Outcome{OK: true}})

	resumeEnv := pauCmd(browser, "resume", 76, "")
	third := briefApply(t, s, disk{}, held, browser, resumeEnv)
	equal(t, len(third.Rows), 2)
	s.Commit(third)

	fourth := briefApply(t, s, disk{}, held, browser, resumeEnv)
	equal(t, fourth, home.Result{Answer: store.Outcome{OK: true}})
}

// End-to-end StartLoop verification: Loop.Sync receives the paused and resumed
// group and system rows, mentions still deliver over Loop.Inbox while paused,
// and group/resume delivers plain InboxItems with live Task copies while
// deduplicating unacknowledged review items and respecting B-18.
func TestPauseLoopSyncAndInbox(t *testing.T) {
	w := runStart(t, runOpen(t), home.NodeTables())
	owner, watcher, host := pauLoopSetup(t, w)
	t0, t1, t2 := pauLoopSeedTasks(t, w, owner, host)
	w.idle()
	watcher.watch(groupID, false)

	pauSendEnv(t, owner, pauCmd(browser, "pause", 85, ""))
	pauCheckSyncPause(t, watcher, &proto.GroupPause{By: grpOwnerID(groupID), At: clock}, "group_paused")
	host.quiet()

	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(86)), Timestamp: clock, Group: proto.ID(groupID)},
		Kind:        "text", Text: "Hold while paused.", Mentions: []proto.ID{proto.ID(pauWorkA)}, Body: proto.EmptyBody{},
	}
	pauSendEnv(t, owner, briefSign(browser, post))
	watcher.next()
	mention := inboxDecode(t, host.next())
	equal(t, len(mention.Items), 1)
	equal(t, []any{mention.Items[0].Item.Why, string(mention.Items[0].Item.AI), string(mention.Items[0].Item.Msg), len(mention.Items[0].Msg) > 0},
		[]any{"mention", pauWorkA, id(86), true})

	pauSendEnv(t, owner, pauCmd(browser, "resume", 87, ""))
	pauCheckSyncPause(t, watcher, nil, "group_resumed")
	pauCheckLoopWake(t, inboxDecode(t, host.next()), t0, t1, t2)
}

func pauLoopSetup(t *testing.T, w *runWorld) (owner, watcher, host *runPeer) {
	t.Helper()
	owner, watcher, host = w.attach(browser), w.attach(node), w.attach(laptop)
	w.loop.Inbox(host.conn)
	create := grpCreatePayload(groupID, groupID)
	create.Forge, create.Checks = "none", []string{}
	pauSendEnv(t, owner, briefSign(browser, create))
	owner.next()
	watcher.next()
	host.next()
	for i, member := range []string{aiID, pauWorkA, pauWorkB} {
		host.does(groupID, "member", fmt.Sprintf(`,"op":"add_ai","id":%q,"cli":"claude","name":"ai-%d","account":"personal","secretary":%t,"approvers":[]`,
			member, i, member == aiID))
	}
	return owner, watcher, host
}

func pauLoopSeedTasks(t *testing.T, w *runWorld, owner, host *runPeer) (t0, t1, t2 string) {
	t.Helper()
	body := briefBody()
	body.Requester = grpOwnerID(groupID)
	entry := proto.BriefTask{Title: "Task", Assignee: pauWorkA, Reviewer: pauWorkB, Depends: []int{}}
	body.Tasks = []proto.BriefTask{entry, entry, entry}
	pauSendEnv(t, host, briefSign(laptop, briefPost(80, body)))
	pauSendEnv(t, owner, briefAct(browser, 81, "confirm", id(80), ""))
	pauAckAll(t, w, host)

	t0, t1, t2 = briefTaskID(81, 0), briefTaskID(81, 1), briefTaskID(81, 2)
	pauSubmitReview(t, host, t1, 2, "c1")
	host.next() // leave t1's review item unacknowledged in the store
	pauSubmitReview(t, host, t2, 3, "c2")
	pauAckAll(t, w, host) // acknowledge t2's review item so row 24 queues review again

	tid := proto.ID(t2)
	approval := proto.Post[proto.ApprovalBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(84)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(pauWorkA)},
		Kind:        "approval", Thread: &tid, Main: true, Mentions: []proto.ID{},
		Body: proto.ApprovalBody{Task: t2, AI: pauWorkA, Tool: "Bash", Command: "make", Category: "command", Approvers: []string{browser.ID()}},
	}
	pauSendEnv(t, host, briefSign(laptop, approval))
	return t0, t1, t2
}

func pauSubmitReview(t *testing.T, host *runPeer, taskID string, num int, commit string) {
	t.Helper()
	host.does(groupID, "task", fmt.Sprintf(`,"op":"branch","id":%q,"branch":"task/%d-task"`, taskID, num))
	host.does(groupID, "task", fmt.Sprintf(`,"op":"checked","id":%q,"commit":%q,"ok":true,"items":[]`, taskID, commit))
	host.does(groupID, "task", fmt.Sprintf(`,"op":"submitted","id":%q,"commit":%q,"stats":{"added":4,"removed":1}`, taskID, commit))
}

func pauAckAll(t *testing.T, w *runWorld, host *runPeer) {
	t.Helper()
	for _, entry := range inboxDecode(t, host.next()).Items {
		w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: entry.Item.Item})
	}
}

func pauSendEnv(t *testing.T, p *runPeer, env proto.Envelope) {
	t.Helper()
	p.re++
	p.w.loop.Cmd(p.conn, p.re, env)
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, p.re))
}

func pauCheckSyncPause(t *testing.T, watcher *runPeer, want *proto.GroupPause, code string) {
	t.Helper()
	var push proto.Rows
	equal(t, json.Unmarshal([]byte(watcher.next()), &push), nil)
	equal(t, len(push.Rows), 2)
	var grp proto.GroupRow
	var msg proto.MsgRow
	equal(t, json.Unmarshal(push.Rows[0], &grp), nil)
	equal(t, json.Unmarshal(push.Rows[1], &msg), nil)
	equal(t, grp.Paused, want)
	equal(t, []any{msg.Kind, msg.By, msg.Body}, []any{"system", "system", json.RawMessage(fmt.Sprintf(`{"code":%q,"by":%q}`, code, grpOwnerID(groupID)))})
}

func pauCheckLoopWake(t *testing.T, frame proto.Inbox, t0, t1, t2 string) {
	t.Helper()
	var got []string
	for _, entry := range frame.Items {
		if entry.Task == nil || entry.Task.ID != string(entry.Item.Task) {
			t.Fatalf("missing or mismatched task copy on %+v", entry)
		}
		got = append(got, fmt.Sprintf("%s:%s:%s:%s", entry.Item.Task, entry.Item.Why, entry.Item.AI, entry.Item.Commit))
	}
	want := []string{
		fmt.Sprintf("%s:resume:%s:", t0, pauWorkA),
		fmt.Sprintf("%s:resume:%s:", t1, pauWorkA),
		fmt.Sprintf("%s:review:%s:c2", t2, pauWorkB),
	}
	slices.Sort(want)
	equal(t, got, want)
}
