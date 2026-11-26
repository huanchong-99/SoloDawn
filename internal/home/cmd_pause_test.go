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
)

func pauState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Logs[pauAdminKey.ID()] = logOf(t, pauAdminKey)
	s.People[pauAdminID] = person(pauAdminID, pauAdminKey, "admin")
	s.People[pauMemberID] = person(pauMemberID, guest, "member")
	s.AIs[pauWorkA] = ai(pauWorkA, laptop)
	s.AIs[pauWorkB] = ai(pauWorkB, laptop)
	return s
}

func pauCmd(by identity.Key, op string, n byte, more string) proto.Envelope {
	return by.Sign(fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":%q%s}`, id(n), clock-2500, groupID, op, more))
}

func pauSysRow(msgID, code, by string) proto.MsgRow {
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: 12}, Pos: 12, Group: groupID,
		By: "system", Kind: "system", Timestamp: clock, Mentions: []string{},
		Body: json.RawMessage(fmt.Sprintf(`{"code":%q,"by":%q}`, code, by)),
	}
}

func pauTask(n byte, state, step, assignee, reviewer string, wait *proto.TaskWait) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id(n)}, State: state, Step: step,
		Assignee: assignee, Reviewer: reviewer, Report: fmt.Sprintf("m%d", n), Wait: wait,
	}
}

// An owner or an admin pauses an unpaused group: group.paused gets the
// person and the home's clock, and the main line gets system group_paused.
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
			res := briefApply(t, s, disk{}, receipts{}, tc.key, pauCmd(tc.key, "pause", 70, ""))
			want := s.Group
			want.Seq = 11
			want.Paused = &proto.GroupPause{By: tc.member, At: clock}
			equal(t, res, home.Result{Rows: res.Rows, Answer: store.Outcome{OK: true}})
			rows := briefRows(res)
			equal(t, rows, []any{want, pauSysRow(id(70), "group_paused", tc.member)})
			grpNoNulls(t, rows[1], "/thread", "/re", "/state")
		})
	}
}

// Pausing a paused group and resuming an unpaused one are accepted and change
// nothing.
func TestPauseRepeatChangesNothing(t *testing.T) {
	for _, op := range []string{"pause", "resume"} {
		t.Run(op, func(t *testing.T) {
			s := pauState(t)
			if op == "pause" {
				s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 9000}
			}
			s.Tasks[id(31)] = pauTask(31, "active", "work", pauWorkA, "", nil)
			res := briefApply(t, s, disk{}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, op, 71, ""))
			equal(t, res, home.Result{Answer: store.Outcome{OK: true}})
		})
	}
}

// Only an admin pauses or resumes, whether or not the group is paused.
func TestPauseRefusesAllButAdmins(t *testing.T) {
	for name, tc := range map[string]struct {
		op     string
		paused bool
		by     identity.Key
		more   string
	}{
		"member pauses":               {op: "pause", by: guest},
		"member pauses paused group":  {op: "pause", paused: true, by: guest},
		"secretary resumes":           {op: "resume", paused: true, by: laptop, more: fmt.Sprintf(`,"as":%q`, aiID)},
		"working AI resumes unpaused": {op: "resume", by: laptop, more: fmt.Sprintf(`,"as":%q`, pauWorkA)},
	} {
		t.Run(name, func(t *testing.T) {
			s := pauState(t)
			if tc.paused {
				s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
			}
			res := briefApply(t, s, disk{}, receipts{}, tc.by, pauCmd(tc.by, tc.op, 72, tc.more))
			briefRefused(t, res, "forbidden")
		})
	}
}

// Row 24 and the B-18 note: resuming clears group.paused, writes
// group_resumed, and in task-id order wakes active tasks with no wait before
// step merge and reviewers whose review is not already queued. A task that
// waits for an approval is not woken (A4.7).
func TestPauseResumeWakesTasksByRow24(t *testing.T) {
	s := pauState(t)
	s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
	approval := &proto.TaskWait{Kind: "approval", Since: clock - 4000}
	s.Tasks = map[string]proto.TaskRow{
		id(31): pauTask(31, "active", "work", pauWorkA, "", nil),
		id(32): pauTask(32, "active", "work", pauWorkB, "", approval),
		id(33): pauTask(33, "active", "review", pauWorkA, pauWorkB, nil),
		id(34): pauTask(34, "active", "review", pauWorkB, pauWorkA, approval),
		id(35): pauTask(35, "done", "review", pauWorkA, pauWorkB, nil),
		id(36): pauTask(36, "paused", "review", pauWorkB, pauWorkA, nil),
	}
	queued := []store.Item{{Why: "review", AI: pauWorkB, Task: &[]string{id(33)}[0], Msg: &[]string{"m33"}[0]}}

	res := briefApply(t, s, tskDisk{queued: queued}, receipts{}, pauAdminKey, pauCmd(pauAdminKey, "resume", 73, ""))
	want := s.Group
	want.Seq = 11
	want.Paused = nil
	equal(t, res, home.Result{Rows: res.Rows, Answer: store.Outcome{OK: true}, Items: []home.Item{
		{AI: pauWorkA, Why: "resume", Task: id(31)},
		{AI: pauWorkA, Why: "resume", Task: id(33)},
		{AI: pauWorkA, Why: "review", Task: id(34), Msg: "m34"},
	}})
	equal(t, briefRows(res), []any{want, pauSysRow(id(73), "group_resumed", pauAdminID)})
}

// Row 23 while the group is paused: the quiet timer queues nothing, so no
// reminders pile up for the secretary until the group resumes.
func TestPauseStopsQuietReminders(t *testing.T) {
	s := pauState(t)
	s.Group.Limits.IdleNoticeMinutes = 1
	task := pauTask(31, "active", "work", pauWorkA, "", nil)
	task.Created = clock - 60_000
	s.Tasks[task.ID] = task
	quiet := home.Timers["quiet"]
	s.Group.Paused = &proto.GroupPause{By: ownerID, At: clock - 5000}
	equal(t, quiet(s, disk{}, clock), home.Result{})
	s.Group.Paused = nil
	equal(t, quiet(s, disk{}, clock).Items, []home.Item{{AI: aiID, Why: "notice", Task: id(31), Code: "quiet"}})
}
