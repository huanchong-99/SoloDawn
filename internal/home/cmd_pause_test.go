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

var (
	pauAdminKey = device(6)
	pauAdminID  = id(60)
	pauMemberID = id(61)
	pauWorkA    = id(62)
	pauWorkB    = id(63)
	pauWorkC    = id(64)
)

const pauSignTS = clock - 2500

type pauDisk struct {
	msgs   map[string]proto.MsgRow
	queued []store.Item
}

func (d pauDisk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d.msgs[id]; return m, ok }
func (d pauDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d pauDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d pauDisk) Pending() []proto.MsgRow                   { return nil }
func (d pauDisk) Queued() []store.Item                      { return d.queued }

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

func pauApply(t *testing.T, s *home.State, d pauDisk, r home.Receipts, by identity.Key, env proto.Envelope) home.Result {
	t.Helper()
	res, err := home.Commands.Apply(s, d, r, by.ID(), env, now)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func pauRows(res home.Result) []any {
	out := []any{}
	for _, r := range res.Rows {
		out = append(out, r.Value())
	}
	return out
}

func pauRefused(t *testing.T, res home.Result, code string) {
	t.Helper()
	if res.Answer.Code != code || res.Answer.OK || res.Msg == "" || len(res.Rows) != 0 || len(res.Items) != 0 {
		t.Fatalf("answered %+v (%q) with %d rows and %d items; want %s", res.Answer, res.Msg, len(res.Rows), len(res.Items), code)
	}
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
			res := pauApply(t, s, pauDisk{}, receipts{}, tc.key, pauCmd(tc.key, "pause", 70, ""))
			if s.Group.Paused != nil {
				t.Fatalf("command mutated state before Commit: %+v", s.Group.Paused)
			}
			wantGroup := s.Group
			wantGroup.Seq = 11
			wantGroup.Paused = &proto.GroupPause{By: tc.member, At: clock}
			equal(t, res, home.Result{Rows: res.Rows, Answer: store.Outcome{OK: true}})
			equal(t, pauRows(res), []any{wantGroup, pauSysRow(id(70), 12, "group_paused", tc.member)})

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

	paused := pauApply(t, s, pauDisk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "pause", 70, ""))
	equal(t, paused.Rows[1].Value().(proto.MsgRow).Body, json.RawMessage(`{"code":"group_paused","by":"<admin&1>"}`))
	s.Commit(paused)

	resumed := pauApply(t, s, pauDisk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "resume", 71, ""))
	equal(t, resumed.Rows[1].Value().(proto.MsgRow).Body, json.RawMessage(`{"code":"group_resumed","by":"<admin&1>"}`))
}

// Pausing a group that is already paused is accepted and changes nothing, even
// when a message already holds the command's nonce.
func TestPauseAlreadyPausedChangesNothing(t *testing.T) {
	s := pauState(t)
	original := &proto.GroupPause{By: ownerID, At: clock - 9000}
	s.Group.Paused = original
	taken := pauDisk{msgs: map[string]proto.MsgRow{id(70): {RowHeader: proto.RowHeader{Type: "msg", ID: id(70)}, Kind: "text"}}}

	res := pauApply(t, s, taken, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "pause", 70, ""))
	equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
	s.Commit(res)
	equal(t, s.Group.Paused, original)
	equal(t, s.Seq, int64(10))
}

// Resuming a group that is not paused is accepted and changes nothing, even
// when active tasks exist and a message holds the command's nonce.
func TestPauseResumeUnpausedGroupChangesNothing(t *testing.T) {
	s := pauState(t)
	s.Tasks[id(31)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(31)}, State: "active", Step: "local_check", Assignee: pauWorkA}
	s.Tasks[id(32)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(32)}, State: "active", Step: "review", Assignee: pauWorkA, Reviewer: pauWorkB, Commit: "c1"}
	taken := pauDisk{msgs: map[string]proto.MsgRow{id(71): {RowHeader: proto.RowHeader{Type: "msg", ID: id(71)}, Kind: "text"}}}

	res := pauApply(t, s, taken, receipts{}, browser, pauCmd(browser, "resume", 71, ""))
	equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
	s.Commit(res)
	equal(t, s.Seq, int64(10))
}

func TestPauseAndResumeRefusals(t *testing.T) {
	existing := pauDisk{msgs: map[string]proto.MsgRow{id(72): {RowHeader: proto.RowHeader{Type: "msg", ID: id(72)}, Kind: "text"}}}
	for name, tc := range map[string]struct {
		op     string
		paused bool
		by     identity.Key
		more   string
		state  func(*home.State)
		disk   pauDisk
		code   string
	}{
		"pause with non-id as": {
			op: "pause", by: laptop, more: `,"as":"not-a-random-id"`,
			state: func(s *home.State) { s.AIs["not-a-random-id"] = ai("not-a-random-id", laptop) },
			code:  "bad_request",
		},
		"resume with non-id as": {
			op: "resume", paused: true, by: laptop, more: `,"as":"not-a-random-id"`,
			state: func(s *home.State) { s.AIs["not-a-random-id"] = ai("not-a-random-id", laptop) },
			code:  "bad_request",
		},
		"pause by ordinary member":                 {op: "pause", by: guest, code: "forbidden"},
		"pause already paused by ordinary member":  {op: "pause", paused: true, by: guest, code: "forbidden"},
		"resume by ordinary member":                {op: "resume", paused: true, by: guest, code: "forbidden"},
		"resume unpaused by ordinary member":       {op: "resume", by: guest, code: "forbidden"},
		"pause by secretary":                       {op: "pause", by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"pause already paused by secretary":        {op: "pause", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"resume by secretary":                      {op: "resume", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"resume unpaused by secretary":             {op: "resume", by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID), code: "forbidden"},
		"pause by working AI":                      {op: "pause", by: laptop, more: fmt.Sprintf(`,"as":%q`, pauWorkA), code: "forbidden"},
		"resume by working AI":                     {op: "resume", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, pauWorkA), code: "forbidden"},
		"pause with taken message id":              {op: "pause", by: browser, disk: existing, code: "conflict"},
		"resume with taken message id wakes no AI": {op: "resume", paused: true, by: browser, disk: existing, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := pauState(t)
			s.Tasks[id(31)] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id(31)}, State: "active", Step: "local_check", Assignee: pauWorkA}
			if tc.paused {
				s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
			}
			if tc.state != nil {
				tc.state(s)
			}
			res := pauApply(t, s, tc.disk, receipts{}, tc.by, pauCmd(tc.by, tc.op, 72, tc.more))
			pauRefused(t, res, tc.code)
		})
	}
}

// Row 24 and B-18: resuming clears group.paused, writes group_resumed, wakes
// active wait-free tasks before merge, wakes unqueued active review tasks, and
// skips tasks with waits, tasks at step merge, and non-active tasks.
func TestPauseResumeClearsPausedAndWakesTasksByRow24(t *testing.T) {
	s := pauState(t)
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

	res := pauApply(t, s, pauDisk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "resume", 73, ""))
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
	equal(t, pauRows(res), []any{wantGroup, pauSysRow(id(73), 12, "group_resumed", pauAdminID)})

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
			res := pauApply(t, s, pauDisk{queued: tc.queued}, receipts{}, browser, pauCmd(browser, "resume", 74, ""))
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, res.Items, tc.want)
		})
	}
}

func TestPauseRepeatedCommandUsesReceipt(t *testing.T) {
	s, held := pauState(t), receipts{}
	pauseEnv := pauCmd(browser, "pause", 75, "")

	first := pauApply(t, s, pauDisk{}, held, browser, pauseEnv)
	equal(t, len(first.Rows), 2)
	s.Commit(first)

	second := pauApply(t, s, pauDisk{}, held, browser, pauseEnv)
	equal(t, second, home.Result{Answer: store.Outcome{OK: true}})

	resumeEnv := pauCmd(browser, "resume", 76, "")
	third := pauApply(t, s, pauDisk{}, held, browser, resumeEnv)
	equal(t, len(third.Rows), 2)
	s.Commit(third)

	fourth := pauApply(t, s, pauDisk{}, held, browser, resumeEnv)
	equal(t, fourth, home.Result{Answer: store.Outcome{OK: true}})
}
