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
	tskA     = id(60)
	tskB     = id(61)
	tskC     = id(62)
	tskGuest = id(63)
	tskAdmin = id(64)
	tskPeer  = device(6)
)

// tskDisk is a home.Disk in memory with messages, threads, pending cards and
// queued inbox items.
type tskDisk struct {
	msgs        map[string]proto.MsgRow
	threads     map[string][]proto.MsgRow // newest first
	pending     []proto.MsgRow            // oldest first
	queued      []store.Item              // oldest first
	threadCalls *int
}

func (d tskDisk) Msg(id string) (proto.MsgRow, bool) {
	m, ok := d.msgs[id]
	return m, ok
}

func (d tskDisk) Note(string) (proto.NoteRow, bool)              { return proto.NoteRow{}, false }
func (d tskDisk) NamedNote(string, string) (proto.NoteRow, bool) { return proto.NoteRow{}, false }
func (d tskDisk) NoteNames(string) []string                      { return nil }

func (d tskDisk) countThread() {
	if d.threadCalls != nil {
		*d.threadCalls++
	}
}

func (d tskDisk) Thread(task *string, before int64, limit int) []proto.MsgRow {
	d.countThread()
	if task == nil {
		return nil
	}
	var out []proto.MsgRow
	for _, m := range d.threads[*task] {
		if tskBefore(m.Pos, before) && len(out) < limit {
			out = append(out, m)
		}
	}
	return out
}

func tskBefore(pos, before int64) bool { return before == 0 || pos < before }

func (d tskDisk) Pending() []proto.MsgRow { return d.pending }

func (d tskDisk) Queued() []store.Item { return d.queued }

func tskState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Logs[tskPeer.ID()] = logOf(t, tskPeer)
	s.Group.Pass = 90
	s.Group.PassFoundation = 70
	s.Group.Limits = proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5}
	s.Num = 4
	s.AIs[tskA] = ai(tskA, laptop)
	s.AIs[tskB] = ai(tskB, node)
	s.AIs[tskC] = ai(tskC, node)
	s.People[tskGuest] = person(tskGuest, guest, "member")
	s.People[tskAdmin] = person(tskAdmin, tskPeer, "admin")
	return s
}

func tskActive(taskID string, num int) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 8},
		Group:     groupID, Num: num, Title: "Report path changes", Slug: "report-path-changes",
		Origin: "brief", Brief: id(70), State: "active", Step: "local_check", Commit: "c1",
		Assignee: tskA, Reviewer: tskB, Pass: 90, Depends: []string{},
		Branch: fmt.Sprintf("task/%d-report-path-changes", num),
		Checks: proto.TaskChecks{Local: &proto.Check{OK: true, At: clock - 3000, Commit: "c1"}},
		Review: &proto.Review{Round: 1, Failed: 1, Score: 80, OK: false, By: tskB, Commit: "c0", At: clock - 2000},
		Thread: proto.ThreadSummary{Count: 2, Rounds: 1},
		Stats:  proto.TaskStats{Started: clock - 5000}, Created: clock - 5000,
	}
}

func tskSend(t *testing.T, s *home.State, d tskDisk, r home.Receipts, by identity.Key, payload any) home.Result {
	t.Helper()
	res := briefApply(t, s, d, r, by, briefSign(by, payload))
	for _, row := range res.Rows {
		raw, err := json.Marshal(row.Value())
		equal(t, err, nil)
		equal(t, proto.UnexpectedNulls(raw, "/stats/ended", "/wait", "/re", "/state", "/thread"), []string{})
	}
	return res
}

func tskRawSend(t *testing.T, s *home.State, d tskDisk, by identity.Key, raw string) home.Result {
	t.Helper()
	return briefApply(t, s, d, receipts{}, by, by.Sign(raw))
}

func tskApprovalCard(msgID, taskID string, ts int64, approvers ...string) proto.MsgRow {
	body := proto.ApprovalBody{
		Task: taskID, AI: tskA, Tool: "Bash", Command: "npm test", Category: "command", Approvers: approvers,
	}
	raw, _ := json.Marshal(body)
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: 5}, Pos: 5, Group: groupID,
		Thread: &taskID, Main: true, By: tskA, Kind: "approval", Timestamp: ts,
		Mentions: []string{ownerID}, Body: raw, State: &proto.CardState{Status: "pending"},
	}
}

func tskDecisionCard(msgID, taskID string, ts int64, to ...string) proto.MsgRow {
	var thread *string
	var taskPtr *string
	if taskID != "" {
		cp := taskID
		thread, taskPtr = &cp, &cp
	}
	body := proto.DecisionBody{Task: taskPtr, Question: "Where?", Options: []proto.DecisionOption{}, To: to}
	raw, _ := json.Marshal(body)
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: 6}, Pos: 6, Group: groupID,
		Thread: thread, Main: true, By: aiID, Kind: "decision", Timestamp: ts,
		Mentions: []string{}, Body: raw, State: &proto.CardState{Status: "pending"},
	}
}

func tskQueuedItem(itemID int64, taskID, aiMember, why, commit string) store.Item {
	var taskPtr, commitPtr *string
	if taskID != "" {
		t := taskID
		taskPtr = &t
	}
	if commit != "" {
		c := commit
		commitPtr = &c
	}
	return store.Item{ID: itemID, Group: groupID, AI: aiMember, Why: why, Task: taskPtr, Commit: commitPtr}
}

// A5.7, home half, and row 2: a working AI starts a direct or takeover task,
// receives the group's next task number, writes the task row and its main-line
// task message, and queues assign (except for takeover) and task_started.
func TestTaskCmdStartCreatesDirectAndTakeoverTasks(t *testing.T) {
	t.Run("direct with re and plan", func(t *testing.T) {
		s := tskState(t)
		re := proto.ID(id(71))
		taskID := id(72)
		cmd := proto.TaskStart{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(1)), Timestamp: clock - 200, Group: proto.ID(groupID), As: proto.ID(tskA)},
				Op:          "start", ID: proto.ID(taskID),
			},
			Title: "Choose: a label", Plan: "Ask first.", Re: &re, Origin: "direct",
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true, Num: 5})
		wantTask := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: 11}, Group: groupID, Num: 5,
			Title: "Choose: a label", Slug: "choose-a-label", Origin: "direct", Plan: "Ask first.",
			State: "active", Step: "branch", Assignee: tskA, Pass: 90, Depends: []string{},
			Created: clock, Stats: proto.TaskStats{Started: clock},
		}
		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: taskID, Seq: 12}, Pos: 12, Group: groupID,
			By: "system", Kind: "task", Timestamp: clock, Mentions: []string{},
			Body: json.RawMessage(`{"task":"` + taskID + `"}`),
		}
		equal(t, briefRows(res), []any{wantTask, wantMsg})
		equal(t, res.Items, []home.Item{
			{AI: tskA, Why: "assign", Task: taskID, Msg: string(re)},
			{AI: aiID, Why: "notice", Task: taskID, Msg: taskID, Code: "task_started"},
		})
	})

	t.Run("direct without re and without secretary", func(t *testing.T) {
		s := tskState(t)
		s.Group.Secretary = ""
		taskID := id(73)
		cmd := proto.TaskStart{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(2)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(tskA)},
				Op:          "start", ID: proto.ID(taskID),
			},
			Title: "Quick fix", Origin: "direct",
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true, Num: 5})
		equal(t, res.Items, []home.Item{{AI: tskA, Why: "assign", Task: taskID}})
	})

	t.Run("takeover queues only the secretary notice", func(t *testing.T) {
		s := tskState(t)
		taskID := id(74)
		cmd := proto.TaskStart{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(3)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(tskB)},
				Op:          "start", ID: proto.ID(taskID),
			},
			Title: "Owner takeover", Origin: "takeover",
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, node, cmd)
		equal(t, res.Answer, store.Outcome{OK: true, Num: 5})
		equal(t, res.Rows[0].Value().(proto.TaskRow).Plan, "")
		equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: taskID, Msg: taskID, Code: "task_started"}})
	})
}

func TestTaskCmdStartRefusals(t *testing.T) {
	taskID := id(75)
	valid := func(as string, origin string) proto.TaskStart {
		return proto.TaskStart{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(1)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(as)},
				Op:          "start", ID: proto.ID(taskID),
			},
			Title: "Title", Origin: origin,
		}
	}
	for name, tc := range map[string]struct {
		by    identity.Key
		raw   string
		cmd   proto.TaskStart
		tasks map[string]proto.TaskRow
		msgs  map[string]proto.MsgRow
		code  string
	}{
		"by a person":      {by: browser, cmd: valid("", "direct"), code: "forbidden"},
		"by a host":        {by: laptop, cmd: valid("", "direct"), code: "forbidden"},
		"by the secretary": {by: laptop, cmd: valid(aiID, "direct"), code: "forbidden"},
		"bad payload": {
			by:   laptop,
			raw:  fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"as":%q,"op":"start","id":"bad"}`, id(1), clock, groupID, tskA),
			code: "bad_request",
		},
		"brief origin": {by: laptop, cmd: valid(tskA, "brief"), code: "bad_request"},
		"empty origin": {by: laptop, cmd: valid(tskA, ""), code: "bad_request"},
		"id taken by a task": {
			by: laptop, cmd: valid(tskA, "direct"),
			tasks: map[string]proto.TaskRow{taskID: {RowHeader: proto.RowHeader{ID: taskID}}},
			code:  "conflict",
		},
		"id taken by a message": {
			by: laptop, cmd: valid(tskA, "direct"),
			msgs: map[string]proto.MsgRow{taskID: {RowHeader: proto.RowHeader{ID: taskID}}},
			code: "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := tskState(t)
			for k, v := range tc.tasks {
				s.Tasks[k] = v
			}
			d := tskDisk{msgs: tc.msgs}
			if tc.raw != "" {
				briefRefused(t, tskRawSend(t, s, d, tc.by, tc.raw), tc.code)
				return
			}
			briefRefused(t, tskSend(t, s, d, receipts{}, tc.by, tc.cmd), tc.code)
		})
	}
}

// Row 3: task branch accepts only task/<num>-<slug> from the assignee's host at
// step branch, while active or paused.
func TestTaskCmdBranch(t *testing.T) {
	taskID := id(76)
	for _, state := range []string{"active", "paused"} {
		t.Run(state, func(t *testing.T) {
			s := tskState(t)
			task := tskActive(taskID, 12)
			task.State, task.Step, task.Branch = state, "branch", ""
			if state == "paused" {
				task.Wait = &proto.TaskWait{Kind: "manual", Since: clock - 100}
			}
			s.Tasks[taskID] = task
			cmd := proto.TaskBranch{
				TaskCommand: proto.TaskCommand{
					HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(4)), Timestamp: clock, Group: proto.ID(groupID)},
					Op:          "branch", ID: proto.ID(taskID),
				},
				Branch: "task/12-report-path-changes",
			}
			res := tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd)
			equal(t, res.Answer, store.Outcome{OK: true})
			want := task
			want.Seq, want.Step, want.Branch = 11, "local_check", "task/12-report-path-changes"
			equal(t, briefRows(res), []any{want})
			equal(t, len(res.Items), 0)
		})
	}
}

func TestTaskCmdBranchRefusals(t *testing.T) {
	taskID := id(76)
	for name, tc := range map[string]struct {
		by     identity.Key
		as     string
		raw    string
		id     string
		branch string
		step   string
		code   string
	}{
		"bad payload": {
			by:   laptop,
			raw:  fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"branch","id":%q}`, id(5), clock, groupID, taskID),
			step: "branch", code: "bad_request",
		},
		"missing task":        {by: laptop, id: id(99), branch: "task/12-report-path-changes", step: "branch", code: "not_found"},
		"wrong task number":   {by: laptop, id: taskID, branch: "task/13-report-path-changes", step: "branch", code: "bad_request"},
		"wrong slug":          {by: laptop, id: taskID, branch: "task/12-other", step: "branch", code: "bad_request"},
		"bare branch name":    {by: laptop, id: taskID, branch: "main", step: "branch", code: "bad_request"},
		"by another node":     {by: node, id: taskID, branch: "main", step: "branch", code: "forbidden"},
		"by browser":          {by: browser, id: taskID, branch: "main", step: "branch", code: "forbidden"},
		"by working AI":       {by: laptop, as: tskA, id: taskID, branch: "task/12-report-path-changes", step: "branch", code: "forbidden"},
		"already past branch": {by: laptop, id: taskID, branch: "main", step: "local_check", code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := tskState(t)
			task := tskActive(taskID, 12)
			task.Step, task.Branch = tc.step, ""
			s.Tasks[taskID] = task
			if tc.raw != "" {
				briefRefused(t, tskRawSend(t, s, tskDisk{}, tc.by, tc.raw), tc.code)
				return
			}
			cmd := proto.TaskBranch{
				TaskCommand: proto.TaskCommand{
					HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(6)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(tc.as)},
					Op:          "branch", ID: proto.ID(tc.id),
				},
				Branch: tc.branch,
			}
			briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, tc.by, cmd), tc.code)
		})
	}
}

// Row 11: task assign updates roles, retires superseded items, marks pending
// approval cards reassigned while leaving decision cards and other tasks' cards
// pending, attaches the latest checks or review message, and drops the assign
// item on a queued or paused task.
func TestTaskCmdAssignTransitionsAndEffects(t *testing.T) {
	taskID, otherTask := id(77), id(78)

	t.Run("reassign active task with approval, decision, queued items and thread feedback", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		appCard := tskApprovalCard(id(80), taskID, clock-900, browser.ID())
		decCard := tskDecisionCard(id(81), taskID, clock-800, browser.ID())
		otherCard := tskApprovalCard(id(82), otherTask, clock-700, browser.ID())
		task.Wait = &proto.TaskWait{Kind: "approval", Msg: appCard.ID, Who: []string{browser.ID()}, Since: clock - 900}
		s.Tasks[taskID] = task

		checksMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(83), Seq: 9}, Pos: 9, Kind: "checks"}
		progMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(84), Seq: 10}, Pos: 10, Kind: "progress"}
		d := tskDisk{
			pending: []proto.MsgRow{appCard, decCard, otherCard},
			threads: map[string][]proto.MsgRow{taskID: {progMsg, checksMsg}},
			queued: []store.Item{
				tskQueuedItem(1, taskID, tskA, "assign", ""),
				tskQueuedItem(2, taskID, tskA, "reviewed", ""),
				tskQueuedItem(3, taskID, tskB, "review", "c0"),
				tskQueuedItem(4, otherTask, tskA, "assign", ""),
				tskQueuedItem(5, "", tskA, "mention", ""),
			},
		}
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(7)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Assignee: proto.ID(tskC),
		}
		res := tskSend(t, s, d, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{1, 2})
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "assign", Task: taskID, Msg: checksMsg.ID}})

		at := int64(clock)
		wantCard := appCard
		wantCard.Seq, wantCard.State = 12, &proto.CardState{Status: "reassigned", At: &at}
		wantTask := task
		wantTask.Seq, wantTask.Assignee = 11, tskC
		wantTask.Wait = &proto.TaskWait{Kind: "decision", Msg: decCard.ID, Who: []string{browser.ID()}, Since: clock - 800}
		equal(t, briefRows(res), []any{wantTask, wantCard})
	})

	t.Run("change only reviewer at step review keeps pending approval card untouched", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		task.Step = "review"
		appCard := tskApprovalCard(id(80), taskID, clock-900, browser.ID())
		checksMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(83), Seq: 9}, Pos: 9, Kind: "checks"}
		s.Tasks[taskID] = task
		d := tskDisk{
			pending: []proto.MsgRow{appCard},
			threads: map[string][]proto.MsgRow{taskID: {checksMsg}},
			queued: []store.Item{
				tskQueuedItem(10, taskID, tskB, "review", "c1"),
				tskQueuedItem(11, taskID, tskB, "mention", ""),
				tskQueuedItem(12, taskID, tskA, "assign", ""),
				tskQueuedItem(13, taskID, tskC, "review", "c1"),
			},
		}
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(8)), Timestamp: clock, Group: proto.ID(groupID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Reviewer: proto.ID(tskC),
		}
		res := tskSend(t, s, d, receipts{}, tskPeer, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{10})
		equal(t, len(res.Rows), 1)
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "review", Task: taskID, Commit: "c1"}})
	})

	t.Run("first reviewer at step review retires any stale review item", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		task.Step, task.Reviewer = "review", ""
		task.Wait = &proto.TaskWait{Kind: "reviewer", Since: clock - 500}
		s.Tasks[taskID] = task
		d := tskDisk{
			queued: []store.Item{
				tskQueuedItem(20, taskID, tskB, "review", "c0"),
				tskQueuedItem(21, taskID, tskA, "assign", ""),
			},
		}
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(9)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Reviewer: proto.ID(tskC),
		}
		res := tskSend(t, s, d, receipts{}, laptop, cmd)
		equal(t, res.Retire, []int64{20})
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "review", Task: taskID, Commit: "c1"}})
	})

	t.Run("queued task reassigned queues no assign item", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		task.State, task.Step, task.Wait = "queued", "branch", &proto.TaskWait{Kind: "depends", Since: clock - 500}
		s.Tasks[taskID] = task
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(10)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Assignee: proto.ID(tskC),
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[0].Value().(proto.TaskRow).Assignee, tskC)
		equal(t, len(res.Items), 0)
	})
}

func TestTaskCmdAssignThreadFeedbackPagingAndFailedSwap(t *testing.T) {
	taskID := id(79)

	t.Run("feedback found on second page of 50-message pages", func(t *testing.T) {
		s := tskState(t)
		s.Tasks[taskID] = tskActive(taskID, 8)
		var msgs []proto.MsgRow
		for i := range 99 {
			msgs = append(msgs, proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: fmt.Sprintf("p-%02d", i)}, Pos: int64(110 - i), Kind: "progress",
			})
		}
		revMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(85)}, Pos: 9, Kind: "review"}
		msgs = append(msgs, revMsg)
		calls := 0
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(11)), Timestamp: clock, Group: proto.ID(groupID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Assignee: proto.ID(tskC),
		}
		res := tskSend(t, s, tskDisk{threads: map[string][]proto.MsgRow{taskID: msgs}, threadCalls: &calls}, receipts{}, browser, cmd)
		equal(t, calls, 2)
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "assign", Task: taskID, Msg: revMsg.ID}})
	})

	t.Run("49 progress messages without feedback yields empty msg", func(t *testing.T) {
		s := tskState(t)
		s.Tasks[taskID] = tskActive(taskID, 8)
		var msgs []proto.MsgRow
		for i := range 49 {
			msgs = append(msgs, proto.MsgRow{
				RowHeader: proto.RowHeader{Type: "msg", ID: fmt.Sprintf("p-%02d", i)}, Pos: int64(60 - i), Kind: "progress",
			})
		}
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(12)), Timestamp: clock, Group: proto.ID(groupID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Assignee: proto.ID(tskC),
		}
		res := tskSend(t, s, tskDisk{threads: map[string][]proto.MsgRow{taskID: msgs}}, receipts{}, browser, cmd)
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "assign", Task: taskID, Msg: ""}})
	})

	t.Run("swapping roles from failed at step review retires old item before checking IfNone", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 8)
		task.State, task.Step = "failed", "review"
		s.Tasks[taskID] = task
		// tskA had an old review item queued; making tskB assignee and tskA reviewer
		// retires tskA's old items first, so tskA still receives the review item.
		d := tskDisk{queued: []store.Item{tskQueuedItem(30, taskID, tskA, "review", "c1")}}
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(13)), Timestamp: clock, Group: proto.ID(groupID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Assignee: proto.ID(tskB), Reviewer: proto.ID(tskA),
		}
		res := tskSend(t, s, d, receipts{}, browser, cmd)
		equal(t, res.Retire, []int64{30})
		equal(t, res.Items, []home.Item{
			{AI: tskA, Why: "review", Task: taskID, Commit: "c1"},
			{AI: tskB, Why: "assign", Task: taskID},
		})
	})
}

func TestTaskCmdAssignRefusals(t *testing.T) {
	taskID := id(86)
	for name, tc := range map[string]struct {
		by       identity.Key
		as       string
		raw      string
		id       string
		state    string
		step     string
		assignee string
		reviewer string
		code     string
	}{
		"bad payload": {
			by: browser, raw: fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"assign","id":"bad"}`, id(1), clock, groupID),
			state: "active", step: "local_check", code: "bad_request",
		},
		"neither assignee nor reviewer": {by: browser, id: taskID, state: "active", step: "local_check", code: "bad_request"},
		"missing task":                  {by: browser, id: id(99), state: "active", step: "local_check", assignee: tskC, code: "not_found"},
		"by non-admin person":           {by: guest, id: taskID, state: "active", step: "local_check", assignee: tskC, code: "forbidden"},
		"by host":                       {by: laptop, id: taskID, state: "active", step: "local_check", assignee: tskC, code: "forbidden"},
		"by working AI":                 {by: laptop, as: tskA, id: taskID, state: "active", step: "local_check", assignee: tskC, code: "forbidden"},
		"secretary on failed task":      {by: laptop, as: aiID, id: taskID, state: "failed", step: "local_check", assignee: tskC, code: "forbidden"},
		"at step merge":                 {by: browser, id: taskID, state: "active", step: "merge", assignee: tskC, code: "conflict"},
		"same AI in both roles":         {by: browser, id: taskID, state: "active", step: "local_check", assignee: tskB, code: "conflict"},
		"secretary as assignee":         {by: browser, id: taskID, state: "active", step: "local_check", assignee: aiID, code: "conflict"},
		"person as assignee":            {by: browser, id: taskID, state: "active", step: "local_check", assignee: ownerID, code: "conflict"},
		"unknown id as assignee":        {by: browser, id: taskID, state: "active", step: "local_check", assignee: id(98), code: "conflict"},
		"secretary as reviewer":         {by: browser, id: taskID, state: "active", step: "local_check", reviewer: aiID, code: "conflict"},
		"unknown id as reviewer":        {by: browser, id: taskID, state: "active", step: "local_check", reviewer: id(98), code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := tskState(t)
			task := tskActive(taskID, 9)
			task.State, task.Step = tc.state, tc.step
			s.Tasks[taskID] = task
			if tc.raw != "" {
				briefRefused(t, tskRawSend(t, s, tskDisk{}, tc.by, tc.raw), tc.code)
				return
			}
			cmd := proto.TaskAssign{
				TaskCommand: proto.TaskCommand{
					HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(14)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(tc.as)},
					Op:          "assign", ID: proto.ID(tc.id),
				},
				Assignee: proto.ID(tc.assignee), Reviewer: proto.ID(tc.reviewer),
			}
			briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, tc.by, cmd), tc.code)
		})
	}
}

// Rows 12, 13 and 13b: pause, resume by secretary/admin, resume by host after
// pre-emption, card wait recomputation, and IfNone review deduplication.
func TestTaskCmdPauseAndResume(t *testing.T) {
	taskID, otherTask := id(87), id(88)

	t.Run("pause keeps conflict wait, admin resume restores it", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 10)
		task.Wait = &proto.TaskWait{Kind: "conflict", Since: clock - 400}
		s.Tasks[taskID] = task

		pauseCmd := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(15)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
			Op:          "pause", ID: proto.ID(taskID),
		}
		paused := tskSend(t, s, tskDisk{}, receipts{}, laptop, pauseCmd)
		equal(t, paused.Answer, store.Outcome{OK: true})
		s.Commit(paused)
		equal(t, s.Tasks[taskID].Wait, &proto.TaskWait{Kind: "manual", Since: clock, Kept: "conflict"})

		// Pausing an already paused task answers paused.
		pauseAgain := pauseCmd
		pauseAgain.Nonce = proto.ID(id(16))
		briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, laptop, pauseAgain), "paused")

		// Host cannot end a manual pause.
		hostResume := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(17)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "resume", ID: proto.ID(taskID),
		}
		briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, laptop, hostResume), "forbidden")

		// Admin browser resumes it and queues resume.
		adminResume := hostResume
		adminResume.Nonce = proto.ID(id(18))
		resumed := tskSend(t, s, tskDisk{}, receipts{}, browser, adminResume)
		equal(t, resumed.Answer, store.Outcome{OK: true})
		equal(t, resumed.Rows[0].Value().(proto.TaskRow).Wait, &proto.TaskWait{Kind: "conflict", Since: clock})
		equal(t, resumed.Items, []home.Item{{AI: tskA, Why: "resume", Task: taskID}})
	})

	t.Run("host resumes pre-empted task without resume item and restores pending card wait", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 10)
		task.State, task.Wait = "paused", &proto.TaskWait{Kind: "preempted", Since: clock - 300}
		s.Tasks[taskID] = task

		globalDec := tskDecisionCard(id(89), "", clock-900, browser.ID())
		otherApp := tskApprovalCard(id(90), otherTask, clock-800, browser.ID())
		taskDec := tskDecisionCard(id(91), taskID, clock-700, guest.ID())
		d := tskDisk{pending: []proto.MsgRow{globalDec, otherApp, taskDec}}

		cmd := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(19)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "resume", ID: proto.ID(taskID),
		}
		res := tskSend(t, s, d, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Items), 0)
		equal(t, res.Rows[0].Value().(proto.TaskRow).Wait, &proto.TaskWait{
			Kind: "decision", Msg: taskDec.ID, Who: []string{guest.ID()}, Since: clock - 700,
		})

		taskApp := tskApprovalCard(id(95), taskID, clock-600, browser.ID())
		cmd.Nonce = proto.ID(id(23))
		resApp := tskSend(t, s, tskDisk{pending: []proto.MsgRow{taskDec, taskApp}}, receipts{}, laptop, cmd)
		equal(t, resApp.Rows[0].Value().(proto.TaskRow).Wait, &proto.TaskWait{
			Kind: "approval", Msg: taskApp.ID, Who: []string{browser.ID()}, Since: clock - 600,
		})
	})

	t.Run("IfNone review deduplication checks task, why, AI and commit", func(t *testing.T) {
		for name, queued := range map[string]struct {
			items []store.Item
			want  []home.Item
		}{
			"exact match suppresses duplicate review": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "review", "c1")},
				want:  []home.Item{{AI: tskA, Why: "resume", Task: taskID}},
			},
			"different task does not suppress": {
				items: []store.Item{tskQueuedItem(1, otherTask, tskB, "review", "c1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Commit: "c1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"nil task does not suppress": {
				items: []store.Item{tskQueuedItem(1, "", tskB, "review", "c1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Commit: "c1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"different why does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "mention", "c1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Commit: "c1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"different AI does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskC, "review", "c1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Commit: "c1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"nil commit does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "review", "")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Commit: "c1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"different commit does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "review", "c0")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Commit: "c1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
		} {
			t.Run(name, func(t *testing.T) {
				s := tskState(t)
				task := tskActive(taskID, 10)
				task.State, task.Step, task.Wait = "paused", "review", &proto.TaskWait{Kind: "manual", Since: clock - 200}
				s.Tasks[taskID] = task
				cmd := proto.TaskCommand{
					HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(20)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
					Op:          "resume", ID: proto.ID(taskID),
				}
				res := tskSend(t, s, tskDisk{queued: queued.items}, receipts{}, laptop, cmd)
				equal(t, res.Items, queued.want)
			})
		}
	})

	t.Run("pause and resume bad payloads and missing tasks", func(t *testing.T) {
		s := tskState(t)
		for _, op := range []string{"pause", "resume"} {
			bad := fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":%q,"id":"bad"}`, id(21), clock, groupID, op)
			briefRefused(t, tskRawSend(t, s, tskDisk{}, browser, bad), "bad_request")
			missing := proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(22)), Timestamp: clock, Group: proto.ID(groupID)},
				Op:          op, ID: proto.ID(id(99)),
			}
			briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, browser, missing), "not_found")
		}
	})
}

// Row 15 and A5.18: task cancel expires pending approval and decision cards,
// retires every queued item of the task, notifies the secretary of each queued
// task that depended on it in deterministic id order, and allows an admin's
// cancel at step merge while refusing the secretary's.
func TestTaskCmdCancelAndOrphanedDependencies(t *testing.T) {
	taskID := id(70)
	dep1, dep2, activeDep, unrelated := id(71), id(72), id(73), id(74)
	ordered := []string{dep1, dep2}
	slices.Sort(ordered)

	t.Run("admin cancels at step merge, expires cards, retires items and notifies dependent queued tasks", func(t *testing.T) {
		s := tskState(t)
		merging := tskActive(taskID, 1)
		merging.Step = "merge"
		s.Tasks[taskID] = merging
		s.Tasks[dep2] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: dep2}, State: "queued", Assignee: tskB, Depends: []string{taskID}}
		s.Tasks[dep1] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: dep1}, State: "queued", Assignee: tskB, Depends: []string{unrelated, taskID}}
		s.Tasks[activeDep] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: activeDep}, State: "cancelled", Assignee: tskB, Depends: []string{taskID}}
		s.Tasks[unrelated] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: unrelated}, State: "queued", Assignee: tskB, Depends: []string{}}

		appCard := tskApprovalCard(id(80), taskID, clock-600, browser.ID())
		decCard := tskDecisionCard(id(81), taskID, clock-500, browser.ID())
		otherCard := tskDecisionCard(id(82), unrelated, clock-400, browser.ID())
		d := tskDisk{
			pending: []proto.MsgRow{appCard, decCard, otherCard},
			queued: []store.Item{
				tskQueuedItem(1, taskID, tskA, "merge", "c1"),
				tskQueuedItem(2, taskID, tskB, "mention", ""),
				tskQueuedItem(3, unrelated, tskA, "assign", ""),
			},
		}

		// Secretary is refused at step merge.
		secCancel := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(25)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
			Op:          "cancel", ID: proto.ID(taskID),
		}
		briefRefused(t, tskSend(t, s, d, receipts{}, laptop, secCancel), "forbidden")

		// Admin succeeds.
		adminCancel := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(26)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "cancel", ID: proto.ID(taskID),
		}
		res := tskSend(t, s, d, receipts{}, browser, adminCancel)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{1, 2})
		equal(t, res.Items, []home.Item{
			{AI: aiID, Why: "notice", Task: ordered[0], Code: "dependency_cancelled"},
			{AI: aiID, Why: "notice", Task: ordered[1], Code: "dependency_cancelled"},
		})

		at := int64(clock)
		ended := int64(clock)
		wantTask := merging
		wantTask.Seq, wantTask.State, wantTask.Wait, wantTask.Stats.Ended = 11, "cancelled", nil, &ended
		expApp, expDec := appCard, decCard
		expApp.Seq, expApp.State = 12, &proto.CardState{Status: "expired", At: &at}
		expDec.Seq, expDec.State = 13, &proto.CardState{Status: "expired", At: &at}
		equal(t, briefRows(res), []any{wantTask, expApp, expDec})
	})

	t.Run("host cancels its takeover task; secretaryless group queues no orphan notice", func(t *testing.T) {
		s := tskState(t)
		s.Group.Secretary = ""
		task := tskActive(taskID, 1)
		task.Origin = "takeover"
		s.Tasks[taskID] = task
		s.Tasks[dep1] = proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: dep1}, State: "queued", Assignee: tskB, Depends: []string{taskID}}
		cmd := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(27)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "cancel", ID: proto.ID(taskID),
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Items), 0)
	})

	t.Run("bad payload and missing task", func(t *testing.T) {
		s := tskState(t)
		bad := fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"cancel","id":"bad"}`, id(28), clock, groupID)
		briefRefused(t, tskRawSend(t, s, tskDisk{}, browser, bad), "bad_request")
		missing := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(29)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "cancel", ID: proto.ID(id(99)),
		}
		briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, browser, missing), "not_found")
	})
}

// Rows 10 and 14, A5.5 and A5.15: task fail counts once per receipt, sends
// crash/timeout notices with detail below the limit, and at the limit writes
// system task_failed mentioning the brief's requester and the assignee's owner.
func TestTaskCmdFailAndStop(t *testing.T) {
	taskID, briefID := id(92), id(70)
	s := tskState(t)
	task := tskActive(taskID, 3)
	task.Failures = 0
	s.Tasks[taskID] = task

	briefCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: briefID}, Kind: "brief",
		Body: json.RawMessage(`{"requester":"` + tskGuest + `"}`),
	}
	d := tskDisk{msgs: map[string]proto.MsgRow{briefID: briefCard}}
	r := receipts{}

	fail1 := proto.TaskFail{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(31)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "fail", ID: proto.ID(taskID),
		},
		Reason: "crash", Detail: "signal: killed",
	}
	res1 := tskSend(t, s, d, r, laptop, fail1)
	equal(t, res1.Answer, store.Outcome{OK: true})
	equal(t, res1.Items, []home.Item{{AI: aiID, Why: "notice", Task: taskID, Code: "crash", Detail: "signal: killed"}})
	s.Commit(res1)
	equal(t, s.Tasks[taskID].Failures, 1)

	// Sending the exact same fail command again counts once (A5.15).
	again := tskSend(t, s, d, r, laptop, fail1)
	equal(t, again, home.Result{Answer: store.Outcome{OK: true}})
	equal(t, s.Tasks[taskID].Failures, 1)

	fail2 := fail1
	fail2.Nonce, fail2.Reason, fail2.Detail = proto.ID(id(32)), "timeout", "60m limit"
	res2 := tskSend(t, s, d, r, laptop, fail2)
	s.Commit(res2)
	equal(t, s.Tasks[taskID].Failures, 2)
	equal(t, res2.Items, []home.Item{{AI: aiID, Why: "notice", Task: taskID, Code: "timeout", Detail: "60m limit"}})

	// Third failure reaches limits.failures: writes task_failed mentioning
	// the brief's requester (tskGuest) and the assignee's owner (ownerID).
	fail3 := fail1
	fail3.Nonce, fail3.Detail = proto.ID(id(33)), "segfault"
	res3 := tskSend(t, s, d, r, laptop, fail3)
	equal(t, res3.Answer, store.Outcome{OK: true})
	sysID := id(33)
	equal(t, res3.Items, []home.Item{{AI: aiID, Why: "notice", Task: taskID, Msg: sysID, Code: "task_failed"}})
	stopped := res3.Rows[0].Value().(proto.TaskRow)
	sysMsg := res3.Rows[1].Value().(proto.MsgRow)
	equal(t, []any{stopped.State, stopped.Failures, stopped.Thread.Count}, []any{"failed", 3, 3})
	equal(t, stopped.Thread.Last, &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock})
	equal(t, sysMsg, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: sysID, Seq: 14}, Pos: 14, Group: groupID,
		Thread: &taskID, By: "system", Kind: "system", Timestamp: clock,
		Mentions: []string{tskGuest, ownerID},
		Body:     json.RawMessage(`{"code":"task_failed","task":"` + taskID + `","reason":"failures"}`),
	})
}

func TestTaskCmdFailMentionsAndRefusals(t *testing.T) {
	taskID, briefID := id(92), id(70)

	for name, tc := range map[string]struct {
		requester string
		dropOwner bool
		want      []string
	}{
		"requester equals owner": {requester: ownerID, want: []string{ownerID}},
		"AI requester":           {requester: tskC, want: []string{tskC, ownerID}},
		"no brief":               {requester: "", want: []string{ownerID}},
		"no human row for user":  {requester: "", dropOwner: true, want: []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			s := tskState(t)
			s.Group.Limits.Failures = 1
			task := tskActive(taskID, 3)
			if tc.requester == "" {
				task.Origin, task.Brief = "direct", ""
			}
			if tc.dropOwner {
				orphanAI := ai(tskA, laptop)
				orphanAI.User = device(7).ID()
				s.AIs[tskA] = orphanAI
			}
			s.Tasks[taskID] = task
			d := tskDisk{msgs: map[string]proto.MsgRow{}}
			if tc.requester != "" {
				d.msgs[briefID] = proto.MsgRow{
					RowHeader: proto.RowHeader{Type: "msg", ID: briefID}, Kind: "brief",
					Body: json.RawMessage(`{"requester":"` + tc.requester + `"}`),
				}
			}
			cmd := proto.TaskFail{
				TaskCommand: proto.TaskCommand{
					HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(34)), Timestamp: clock, Group: proto.ID(groupID)},
					Op:          "fail", ID: proto.ID(taskID),
				},
				Reason: "crash", Detail: "exit 1",
			}
			res := tskSend(t, s, d, receipts{}, laptop, cmd)
			equal(t, res.Rows[1].Value().(proto.MsgRow).Mentions, tc.want)
		})
	}

	for name, tc := range map[string]struct {
		by     identity.Key
		raw    string
		id     string
		reason string
		state  string
		taken  bool
		code   string
	}{
		"bad payload": {
			by: laptop, raw: fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"fail","id":%q}`, id(35), clock, groupID, taskID),
			state: "active", code: "bad_request",
		},
		"unknown reason":   {by: laptop, id: taskID, reason: "oom", state: "active", code: "bad_request"},
		"missing task":     {by: laptop, id: id(99), reason: "crash", state: "active", code: "not_found"},
		"by person":        {by: browser, id: taskID, reason: "crash", state: "active", code: "forbidden"},
		"while paused":     {by: laptop, id: taskID, reason: "crash", state: "paused", code: "paused"},
		"msg id collision": {by: laptop, id: taskID, reason: "crash", state: "active", taken: true, code: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			s := tskState(t)
			s.Group.Limits.Failures = 1
			task := tskActive(taskID, 3)
			task.State = tc.state
			if tc.state == "paused" {
				task.Wait = &proto.TaskWait{Kind: "manual", Since: clock - 100}
			}
			s.Tasks[taskID] = task
			d := tskDisk{msgs: map[string]proto.MsgRow{}}
			if tc.taken {
				d.msgs[id(36)] = proto.MsgRow{RowHeader: proto.RowHeader{ID: id(36)}}
			}
			if tc.raw != "" {
				briefRefused(t, tskRawSend(t, s, d, tc.by, tc.raw), tc.code)
				return
			}
			cmd := proto.TaskFail{
				TaskCommand: proto.TaskCommand{
					HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(36)), Timestamp: clock, Group: proto.ID(groupID)},
					Op:          "fail", ID: proto.ID(tc.id),
				},
				Reason: tc.reason, Detail: "err",
			}
			briefRefused(t, tskSend(t, s, d, receipts{}, tc.by, cmd), tc.code)
		})
	}
}

// Row 16: task preempted pauses the task, keeps a reviewer or conflict wait,
// and writes the preempted system message naming the jump message's author.
func TestTaskCmdPreempted(t *testing.T) {
	taskID, jumpID := id(93), id(94)

	t.Run("preempts active task and records jump author in thread", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 6)
		task.Wait = &proto.TaskWait{Kind: "conflict", Since: clock - 400}
		s.Tasks[taskID] = task
		d := tskDisk{msgs: map[string]proto.MsgRow{
			jumpID: {RowHeader: proto.RowHeader{Type: "msg", ID: jumpID}, By: ownerID, Kind: "text"},
		}}
		cmd := proto.TaskPreempted{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(37)), Timestamp: clock, Group: proto.ID(groupID)},
				Op:          "preempted", ID: proto.ID(taskID),
			},
			Msg: proto.ID(jumpID),
		}
		res := tskSend(t, s, d, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Items), 0)

		gotTask := res.Rows[0].Value().(proto.TaskRow)
		gotMsg := res.Rows[1].Value().(proto.MsgRow)
		equal(t, gotTask.State, "paused")
		equal(t, gotTask.Wait, &proto.TaskWait{Kind: "preempted", Since: clock, Kept: "conflict"})
		equal(t, gotTask.Thread.Count, 3)
		equal(t, gotTask.Thread.Last, &proto.ThreadMessage{By: "system", Kind: "system", Text: "", Timestamp: clock})
		equal(t, gotMsg, proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(37), Seq: 12}, Pos: 12, Group: groupID,
			Thread: &taskID, By: "system", Kind: "system", Timestamp: clock, Mentions: []string{},
			Body: json.RawMessage(`{"code":"preempted","task":"` + taskID + `","by":"` + ownerID + `"}`),
		})
	})

	t.Run("preempted refusals", func(t *testing.T) {
		for name, tc := range map[string]struct {
			by    identity.Key
			raw   string
			id    string
			msg   string
			state string
			msgs  map[string]proto.MsgRow
			code  string
		}{
			"bad payload": {
				by: laptop, raw: fmt.Sprintf(`{"t":"task","n":%q,"ts":%d,"g":%q,"op":"preempted","id":%q}`, id(38), clock, groupID, taskID),
				state: "active", code: "bad_request",
			},
			"missing task":         {by: laptop, id: id(99), msg: jumpID, state: "active", code: "not_found"},
			"by person":            {by: browser, id: taskID, msg: jumpID, state: "active", code: "forbidden"},
			"while paused":         {by: laptop, id: taskID, msg: jumpID, state: "paused", code: "paused"},
			"missing jump message": {by: laptop, id: taskID, msg: jumpID, state: "active", code: "not_found"},
			"command nonce taken by message": {
				by: laptop, id: taskID, msg: jumpID, state: "active",
				msgs: map[string]proto.MsgRow{
					jumpID: {RowHeader: proto.RowHeader{ID: jumpID}, By: ownerID},
					id(39): {RowHeader: proto.RowHeader{ID: id(39)}, By: ownerID},
				},
				code: "conflict",
			},
		} {
			t.Run(name, func(t *testing.T) {
				s := tskState(t)
				task := tskActive(taskID, 6)
				task.State = tc.state
				if tc.state == "paused" {
					task.Wait = &proto.TaskWait{Kind: "manual", Since: clock - 100}
				}
				s.Tasks[taskID] = task
				d := tskDisk{msgs: tc.msgs}
				if tc.raw != "" {
					briefRefused(t, tskRawSend(t, s, d, tc.by, tc.raw), tc.code)
					return
				}
				cmd := proto.TaskPreempted{
					TaskCommand: proto.TaskCommand{
						HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(39)), Timestamp: clock, Group: proto.ID(groupID)},
						Op:          "preempted", ID: proto.ID(tc.id),
					},
					Msg: proto.ID(tc.msg),
				}
				briefRefused(t, tskSend(t, s, d, receipts{}, tc.by, cmd), tc.code)
			})
		}
	})
}
