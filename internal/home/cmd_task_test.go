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

func (d tskDisk) Note(string) (proto.NoteRow, bool) { return proto.NoteRow{}, false }
func (d tskDisk) TypedNotes(string) []proto.NoteRow { return nil }

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

func (d tskDisk) Queued() []store.Item    { return d.queued }
func (d tskDisk) OtherMember(string) bool { return false }

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
		Group:     groupID, Num: num, Title: "Report path changes",
		Origin: "brief", Brief: id(70), State: "active", Step: "work", Report: "m1",
		Assignee: tskA, Reviewer: tskB, Pass: 90, Depends: []string{},
		Review: &proto.Review{Round: 1, Failed: 1, Score: 80, OK: false, By: tskB, Report: "m0", At: clock - 2000},
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

func tskQueuedItem(itemID int64, taskID, aiMember, why, msg string) store.Item {
	var taskPtr, msgPtr *string
	if taskID != "" {
		t := taskID
		taskPtr = &t
	}
	if msg != "" {
		m := msg
		msgPtr = &m
	}
	return store.Item{ID: itemID, Group: groupID, AI: aiMember, Why: why, Task: taskPtr, Msg: msgPtr}
}

// tskBehind is a task that waits for the folder of its assignee.
func tskBehind(taskID string, num int, assignee string) proto.TaskRow {
	task := tskActive(taskID, num)
	task.State, task.Assignee, task.Review, task.Report = "queued", assignee, nil, ""
	task.Wait = &proto.TaskWait{Kind: "busy", Since: clock - 500}
	return task
}

// tskShare gives two AIs of one host the same folder.
func tskShare(s *home.State, a, b string) {
	shared := s.AIs[b]
	shared.Workspace = s.AIs[a].Workspace
	s.AIs[b] = shared
}

func tskAssignCmd(n byte, as, taskID, assignee, reviewer string) proto.TaskAssign {
	return proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(as)},
			Op:          "assign", ID: proto.ID(taskID),
		},
		Assignee: proto.ID(assignee), Reviewer: proto.ID(reviewer),
	}
}

// A5.7, home half, A5.20 and row 2: a working AI starts a direct task,
// receives the group's next task number, writes the task row and its main-line
// task message, and queues assign and task_started. While its folder is busy
// the task is queued and only the secretary is told.
func TestTaskCmdStartCreatesDirectTasks(t *testing.T) {
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
			Title: "Choose: a label", Origin: "direct", Plan: "Ask first.",
			State: "active", Step: "work", Assignee: tskA, Pass: 90, Depends: []string{},
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

	t.Run("while the AI's folder is busy the task is queued behind the other one", func(t *testing.T) {
		holders := map[string][]string{
			"its own task":                    {tskB, "active"},
			"its own paused task":             {tskB, "paused"},
			"its own task that was stopped":   {tskB, "failed"},
			"a task of an AI in its folder":   {tskC, "active"},
			"a paused task of that other AI":  {tskC, "paused"},
			"a stopped task of that other AI": {tskC, "failed"},
		}
		for name, holder := range holders {
			t.Run(name, func(t *testing.T) {
				s := tskState(t)
				tskShare(s, tskB, tskC)
				running := tskActive(id(76), 4)
				running.Assignee, running.State, running.Reviewer = holder[0], holder[1], tskA
				s.Tasks[running.ID] = running
				taskID := id(74)
				cmd := proto.TaskStart{
					TaskCommand: proto.TaskCommand{
						HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(3)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(tskB)},
						Op:          "start", ID: proto.ID(taskID),
					},
					Title: "Second change", Plan: "After the first.", Origin: "direct",
				}
				res := tskSend(t, s, tskDisk{}, receipts{}, node, cmd)
				equal(t, res.Answer, store.Outcome{OK: true, Num: 5})
				got := res.Rows[0].Value().(proto.TaskRow)
				equal(t, []string{got.State, got.Step, got.Plan}, []string{"queued", "work", "After the first."})
				equal(t, got.Wait, &proto.TaskWait{Kind: "busy", Since: clock})
				equal(t, res.Items, []home.Item{{AI: aiID, Why: "notice", Task: taskID, Msg: taskID, Code: "task_started"}})
			})
		}
	})

	t.Run("a folder is a host and a path: only a started task in the very folder holds it", func(t *testing.T) {
		s := tskState(t)
		s.Tasks[id(76)] = tskBehind(id(76), 3, tskB)
		for n, state := range []string{"done", "cancelled"} {
			final := tskActive(id(77+byte(n)), 4)
			final.State, final.Assignee, final.Reviewer = state, tskB, tskA
			s.Tasks[final.ID] = final
		}
		// tskC works in another folder of the same computer, tskA in a folder
		// of that name on another one.
		elsewhere := s.AIs[tskA]
		elsewhere.Workspace = s.AIs[tskB].Workspace
		s.AIs[tskA] = elsewhere
		neighbour := tskActive(id(79), 2)
		neighbour.Assignee, neighbour.Reviewer = tskC, tskA
		s.Tasks[neighbour.ID], s.Tasks[id(80)] = neighbour, tskActive(id(80), 1)
		cmd := proto.TaskStart{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(3)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(tskB)},
				Op:          "start", ID: proto.ID(id(74)),
			},
			Title: "Free folder", Origin: "direct",
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, node, cmd)
		equal(t, res.Rows[0].Value().(proto.TaskRow).State, "active")
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
		"brief origin":    {by: laptop, cmd: valid(tskA, "brief"), code: "bad_request"},
		"takeover origin": {by: laptop, cmd: valid(tskA, "takeover"), code: "bad_request"},
		"empty origin":    {by: laptop, cmd: valid(tskA, ""), code: "bad_request"},
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

// Row 11: task assign updates roles, retires superseded items, marks pending
// approval cards reassigned, by its sender, while leaving decision cards and
// other tasks' cards pending, attaches the latest review message, and drops
// the assign item on a queued or paused task.
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

		checksMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(83), Seq: 9}, Pos: 9, Kind: "review"}
		progMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(84), Seq: 10}, Pos: 10, Kind: "progress"}
		d := tskDisk{
			pending: []proto.MsgRow{appCard, decCard, otherCard},
			threads: map[string][]proto.MsgRow{taskID: {progMsg, checksMsg}},
			queued: []store.Item{
				tskQueuedItem(1, taskID, tskA, "assign", ""),
				tskQueuedItem(2, taskID, tskA, "reviewed", ""),
				tskQueuedItem(3, taskID, tskB, "review", "m0"),
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

		at, by := int64(clock), aiID
		wantCard := appCard
		wantCard.Seq, wantCard.State = 12, &proto.CardState{Status: "reassigned", At: &at, By: &by}
		wantTask := task
		wantTask.Seq, wantTask.Assignee = 11, tskC
		wantTask.Wait = &proto.TaskWait{Kind: "decision", Msg: decCard.ID, Who: []string{browser.ID()}, Since: clock - 800}
		equal(t, briefRows(res), []any{wantTask, wantCard})
	})

	t.Run("foreign reviewer waits for consent without changing the approval card", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		task.Step = "review"
		appCard := tskApprovalCard(id(80), taskID, clock-900, browser.ID())
		checksMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(83), Seq: 9}, Pos: 9, Kind: "review"}
		s.Tasks[taskID] = task
		d := tskDisk{
			pending: []proto.MsgRow{appCard},
			threads: map[string][]proto.MsgRow{taskID: {checksMsg}},
			queued: []store.Item{
				tskQueuedItem(10, taskID, tskB, "review", "m1"),
				tskQueuedItem(11, taskID, tskB, "mention", ""),
				tskQueuedItem(12, taskID, tskA, "assign", ""),
				tskQueuedItem(13, taskID, tskC, "review", "m1"),
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
		equal(t, len(res.Rows), 2)
		equal(t, res.Items, []home.Item{})
		card := cfmCard(t, res)
		equal(t, card.Kind, "confirm")
		equal(t, card.Re, &task.Brief)
		equal(t, card.Body, briefRaw(t, proto.ConfirmBody{What: "direct", AI: tskC, To: []string{browser.ID()}}))
		want := task
		want.Seq, want.Reviewer, want.Asked = 11, tskC, []string{tskC}
		want.Wait = &proto.TaskWait{Kind: "confirm", Msg: card.ID, Who: []string{browser.ID()}, Since: clock}
		equal(t, res.Rows[0].Value(), want)
	})

	t.Run("first reviewer at step review retires any stale review item", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		task.Step, task.Reviewer = "review", ""
		task.Wait = &proto.TaskWait{Kind: "reviewer", Since: clock - 500}
		s.Tasks[taskID] = task
		d := tskDisk{
			queued: []store.Item{
				tskQueuedItem(20, taskID, tskB, "review", "m0"),
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
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "review", Task: taskID, Msg: "m1"}})
	})

	t.Run("queued task that still waits for a dependency is reassigned without an assign item", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 7)
		task.State, task.Depends, task.Wait = "queued", []string{otherTask}, &proto.TaskWait{Kind: "depends", Since: clock - 500}
		s.Tasks[taskID] = task
		s.Tasks[otherTask] = tskActive(otherTask, 6)
		cmd := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{
				HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(10)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
				Op:          "assign", ID: proto.ID(taskID),
			},
			Assignee: proto.ID(tskC),
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		want := task
		want.Seq, want.Assignee = 11, tskC
		equal(t, briefRows(res), []any{want})
		equal(t, len(res.Items), 0)
	})
}

// A5.20 and row 11: one folder takes one task at a time. A started task is
// not given to an AI whose folder is busy with another task; giving it to an
// AI in another folder starts the task that was queued behind it; and a queued
// task that is given another assignee is looked at again.
func TestTaskCmdAssignAndTheBusyFolder(t *testing.T) {
	first, second, third := id(77), id(78), id(79)

	t.Run("refused for an AI whose folder is busy, its own or one it shares", func(t *testing.T) {
		for name, to := range map[string]string{"its own": tskC, "shared": tskB} {
			for _, state := range []string{"active", "paused", "failed"} {
				s := tskState(t)
				tskShare(s, tskC, tskB)
				task := tskActive(first, 7)
				task.State, task.Reviewer = state, ""
				s.Tasks[first] = task
				holder := tskActive(second, 8)
				holder.Assignee, holder.Reviewer = tskC, tskA
				s.Tasks[second] = holder
				res := tskSend(t, s, tskDisk{}, receipts{}, browser, tskAssignCmd(7, "", first, to, ""))
				if res.Answer.Code != "conflict" || len(res.Rows) != 0 {
					t.Errorf("%s folder, %s task: got %+v", name, state, res.Answer)
				}
			}
		}
	})

	t.Run("an AI in another folder gets it, and the task queued behind it starts with its task message", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(first, 7)
		appCard := tskApprovalCard(id(80), first, clock-900, browser.ID())
		s.Tasks[first], s.Tasks[second], s.Tasks[third] = task, tskBehind(second, 9, tskA), tskBehind(third, 8, tskA)
		res := tskSend(t, s, tskDisk{pending: []proto.MsgRow{appCard}}, receipts{}, browser, tskAssignCmd(7, "", first, tskC, ""))
		equal(t, res.Answer, store.Outcome{OK: true})

		at, by := int64(clock), ownerID
		moved, card, started := task, appCard, tskBehind(third, 8, tskA)
		moved.Seq, moved.Assignee = 11, tskC
		card.Seq, card.State = 12, &proto.CardState{Status: "reassigned", At: &at, By: &by}
		started.Seq, started.State, started.Wait, started.Stats.Started = 13, "active", nil, clock
		equal(t, briefRows(res), []any{moved, card, started})
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "assign", Task: first}, {AI: tskA, Why: "assign", Task: third, Msg: third}})
	})

	t.Run("an AI that shares the folder gets it, and the folder stays busy", func(t *testing.T) {
		s := tskState(t)
		tskShare(s, tskC, tskB)
		task := tskActive(first, 7)
		task.Assignee, task.Reviewer = tskC, tskA
		s.Tasks[first], s.Tasks[second] = task, tskBehind(second, 8, tskB)
		res := tskSend(t, s, tskDisk{}, receipts{}, browser, tskAssignCmd(7, "", first, tskB, ""))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
		equal(t, res.Items, []home.Item{{AI: tskB, Why: "assign", Task: first}})
	})

	t.Run("a queued task starts when it is given to an AI whose folder is free", func(t *testing.T) {
		s := tskState(t)
		s.Tasks[first], s.Tasks[second] = tskActive(first, 7), tskBehind(second, 8, tskA)
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, tskAssignCmd(7, aiID, second, tskC, ""))
		equal(t, res.Answer, store.Outcome{OK: true})
		want := tskBehind(second, 8, tskC)
		want.Seq, want.State, want.Wait, want.Stats.Started = 11, "active", nil, clock
		equal(t, briefRows(res), []any{want})
		equal(t, res.Items, []home.Item{{AI: tskC, Why: "assign", Task: second, Msg: second}})
	})

	t.Run("a queued task waits on when it is given to an AI whose folder is busy too", func(t *testing.T) {
		s := tskState(t)
		holder := tskActive(third, 6)
		holder.Assignee, holder.Reviewer = tskC, tskA
		s.Tasks[first], s.Tasks[second], s.Tasks[third] = tskActive(first, 7), tskBehind(second, 8, tskA), holder
		res := tskSend(t, s, tskDisk{}, receipts{}, laptop, tskAssignCmd(7, aiID, second, tskC, ""))
		equal(t, res.Answer, store.Outcome{OK: true})
		want := tskBehind(second, 8, tskC)
		want.Seq = 11
		equal(t, briefRows(res), []any{want})
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
		d := tskDisk{queued: []store.Item{tskQueuedItem(30, taskID, tskA, "review", "m1")}}
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
			{AI: tskA, Why: "review", Task: taskID, Msg: "m1"},
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
			state: "active", step: "work", code: "bad_request",
		},
		"neither assignee nor reviewer": {by: browser, id: taskID, state: "active", step: "work", code: "bad_request"},
		"missing task":                  {by: browser, id: id(99), state: "active", step: "work", assignee: tskC, code: "not_found"},
		"by non-admin person":           {by: guest, id: taskID, state: "active", step: "work", assignee: tskC, code: "forbidden"},
		"by host":                       {by: laptop, id: taskID, state: "active", step: "work", assignee: tskC, code: "forbidden"},
		"by working AI":                 {by: laptop, as: tskA, id: taskID, state: "active", step: "work", assignee: tskC, code: "forbidden"},
		"secretary on failed task":      {by: laptop, as: aiID, id: taskID, state: "failed", step: "work", assignee: tskC, code: "forbidden"},
		"done task":                     {by: browser, id: taskID, state: "done", step: "review", assignee: tskC, code: "conflict"},
		"same AI in both roles":         {by: browser, id: taskID, state: "active", step: "work", assignee: tskB, code: "conflict"},
		"secretary as assignee":         {by: browser, id: taskID, state: "active", step: "work", assignee: aiID, code: "conflict"},
		"person as assignee":            {by: browser, id: taskID, state: "active", step: "work", assignee: ownerID, code: "conflict"},
		"unknown id as assignee":        {by: browser, id: taskID, state: "active", step: "work", assignee: id(98), code: "conflict"},
		"secretary as reviewer":         {by: browser, id: taskID, state: "active", step: "work", reviewer: aiID, code: "conflict"},
		"unknown id as reviewer":        {by: browser, id: taskID, state: "active", step: "work", reviewer: id(98), code: "conflict"},
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

	t.Run("pause names who paused and keeps a reviewer wait, admin resume restores it", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 10)
		task.Step, task.Reviewer = "review", ""
		task.Wait = &proto.TaskWait{Kind: "reviewer", Since: clock - 400}
		s.Tasks[taskID] = task

		pauseCmd := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(15)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
			Op:          "pause", ID: proto.ID(taskID),
		}
		paused := tskSend(t, s, tskDisk{}, receipts{}, laptop, pauseCmd)
		equal(t, paused.Answer, store.Outcome{OK: true})
		s.Commit(paused)
		equal(t, s.Tasks[taskID].Wait, &proto.TaskWait{Kind: "manual", Since: clock, Kept: "reviewer", By: aiID})

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
		equal(t, resumed.Rows[0].Value().(proto.TaskRow).Wait, &proto.TaskWait{Kind: "reviewer", Since: clock})
		equal(t, resumed.Items, []home.Item{{AI: tskA, Why: "resume", Task: taskID}})
	})

	t.Run("a person's pause names the person", func(t *testing.T) {
		s := tskState(t)
		s.Tasks[taskID] = tskActive(taskID, 10)
		cmd := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(15)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "pause", ID: proto.ID(taskID),
		}
		res := tskSend(t, s, tskDisk{}, receipts{}, tskPeer, cmd)
		equal(t, res.Rows[0].Value().(proto.TaskRow).Wait, &proto.TaskWait{Kind: "manual", Since: clock, By: tskAdmin})
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

	t.Run("IfNone review deduplication checks task, why, AI and report", func(t *testing.T) {
		for name, queued := range map[string]struct {
			items []store.Item
			want  []home.Item
		}{
			"exact match suppresses duplicate review": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "review", "m1")},
				want:  []home.Item{{AI: tskA, Why: "resume", Task: taskID}},
			},
			"different task does not suppress": {
				items: []store.Item{tskQueuedItem(1, otherTask, tskB, "review", "m1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Msg: "m1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"nil task does not suppress": {
				items: []store.Item{tskQueuedItem(1, "", tskB, "review", "m1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Msg: "m1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"different why does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "mention", "m1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Msg: "m1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"different AI does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskC, "review", "m1")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Msg: "m1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"an item without a message does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "review", "")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Msg: "m1"}, {AI: tskA, Why: "resume", Task: taskID}},
			},
			"another report does not suppress": {
				items: []store.Item{tskQueuedItem(1, taskID, tskB, "review", "m0")},
				want:  []home.Item{{AI: tskB, Why: "review", Task: taskID, Msg: "m1"}, {AI: tskA, Why: "resume", Task: taskID}},
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

// Row 15 and A5.20: task cancel expires pending approval and decision cards,
// retires every queued item of the task, notifies the secretary of each queued
// task that depended on it in deterministic id order, and starts the first of
// the tasks that were queued behind it in its folder, leaving the next queued.
func TestTaskCmdCancelAndOrphanedDependencies(t *testing.T) {
	taskID := id(70)
	dep1, dep2, endedDep, unrelated := id(71), id(72), id(73), id(74)
	behind, behindToo := id(75), id(76)
	ordered := []string{dep1, dep2}
	slices.Sort(ordered)
	waiting := func(taskID string, state string, depends ...string) proto.TaskRow {
		return proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: taskID}, State: state, Assignee: tskB,
			Depends: depends, Wait: &proto.TaskWait{Kind: "depends", Since: clock - 100}}
	}

	t.Run("the secretary cancels: cards expire, items are retired, dependents are reported, the folder passes on", func(t *testing.T) {
		s := tskState(t)
		cancelled := tskActive(taskID, 1)
		s.Tasks[taskID] = cancelled
		s.Tasks[dep2] = waiting(dep2, "queued", taskID)
		s.Tasks[dep1] = waiting(dep1, "queued", unrelated, taskID)
		s.Tasks[endedDep] = waiting(endedDep, "cancelled", taskID)
		running := tskActive(unrelated, 2)
		running.Assignee, running.Reviewer = tskC, tskA
		s.Tasks[unrelated] = running
		s.Tasks[behindToo], s.Tasks[behind] = tskBehind(behindToo, 6, tskA), tskBehind(behind, 5, tskA)

		appCard := tskApprovalCard(id(80), taskID, clock-600, browser.ID())
		decCard := tskDecisionCard(id(81), taskID, clock-500, browser.ID())
		otherCard := tskDecisionCard(id(82), unrelated, clock-400, browser.ID())
		d := tskDisk{
			pending: []proto.MsgRow{appCard, decCard, otherCard},
			queued: []store.Item{
				tskQueuedItem(1, taskID, tskB, "review", "m1"),
				tskQueuedItem(2, taskID, tskB, "mention", ""),
				tskQueuedItem(3, unrelated, tskA, "assign", ""),
			},
		}
		cancel := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(26)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(aiID)},
			Op:          "cancel", ID: proto.ID(taskID),
		}
		res := tskSend(t, s, d, receipts{}, laptop, cancel)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Retire, []int64{1, 2})
		equal(t, res.Items, []home.Item{
			{AI: aiID, Why: "notice", Task: ordered[0], Code: "dependency_cancelled"},
			{AI: aiID, Why: "notice", Task: ordered[1], Code: "dependency_cancelled"},
			{AI: tskA, Why: "assign", Task: behind, Msg: behind},
		})

		at := int64(clock)
		ended := int64(clock)
		wantTask := cancelled
		wantTask.Seq, wantTask.State, wantTask.Wait, wantTask.Stats.Ended = 11, "cancelled", nil, &ended
		expApp, expDec := appCard, decCard
		expApp.Seq, expApp.State = 12, &proto.CardState{Status: "expired", At: &at}
		expDec.Seq, expDec.State = 13, &proto.CardState{Status: "expired", At: &at}
		started := tskBehind(behind, 5, tskA)
		started.Seq, started.State, started.Wait, started.Stats.Started = 14, "active", nil, clock
		equal(t, briefRows(res), []any{wantTask, expApp, expDec, started})
	})

	t.Run("the host of the assignee cancels nothing; a secretaryless group queues no orphan notice", func(t *testing.T) {
		s := tskState(t)
		s.Group.Secretary = ""
		s.Tasks[taskID] = tskActive(taskID, 1)
		s.Tasks[dep1] = waiting(dep1, "queued", taskID)
		cmd := proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(27)), Timestamp: clock, Group: proto.ID(groupID)},
			Op:          "cancel", ID: proto.ID(taskID),
		}
		briefRefused(t, tskSend(t, s, tskDisk{}, receipts{}, laptop, cmd), "forbidden")
		res := tskSend(t, s, tskDisk{}, receipts{}, browser, cmd)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 1)
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

// The release goes through the queued tasks in the order of num, and a task it
// starts keeps its folder from the ones after it: of the tasks behind one
// folder exactly the one with the lowest number starts.
func TestTaskCmdReleaseStartsTheLowestNumber(t *testing.T) {
	s := tskState(t)
	s.Tasks[id(70)] = tskActive(id(70), 1)
	for n := byte(71); n <= 78; n++ {
		s.Tasks[id(n)] = tskBehind(id(n), 100-int(n), tskA)
	}
	cmd := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: proto.ID(id(26)), Timestamp: clock, Group: proto.ID(groupID)},
		Op:          "cancel", ID: proto.ID(id(70)),
	}
	res := tskSend(t, s, tskDisk{}, receipts{}, browser, cmd)
	equal(t, len(res.Rows), 2)
	started := res.Rows[1].Value().(proto.TaskRow)
	equal(t, []any{started.ID, started.Num, started.State}, []any{id(78), 22, "active"})
	equal(t, res.Items, []home.Item{{AI: tskA, Why: "assign", Task: id(78), Msg: id(78)}})
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

// Row 16: task preempted pauses the task, keeps a reviewer wait, and writes the preempted system message naming the jump message's author.
func TestTaskCmdPreempted(t *testing.T) {
	taskID, jumpID := id(93), id(94)

	t.Run("preempts active task and records jump author in thread", func(t *testing.T) {
		s := tskState(t)
		task := tskActive(taskID, 6)
		task.Step, task.Reviewer = "review", ""
		task.Wait = &proto.TaskWait{Kind: "reviewer", Since: clock - 400}
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
		equal(t, gotTask.Wait, &proto.TaskWait{Kind: "preempted", Since: clock, Kept: "reviewer"})
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
