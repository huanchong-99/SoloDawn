package host

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The queue jump of flows/agent-turn.md C, with delivery and the session
// runtime as they are and a fake driver, home and clock.

const jumpHome = "home-dev"

type jumpRig struct {
	t    *testing.T
	h    *Host
	db   *store.Store
	home *fakeDeliverHome
	drv  *fakeDriver

	mu     sync.Mutex
	ai     string // the AI whose items and tasks the rig makes; runAI1 unless a test changes it
	tasks  map[string]proto.TaskRow
	refuse map[string]string // "task <op>" to the code the home refuses it with
	gone   map[string]int    // "task <op>" to how often the home is not reached
	lost   map[string]int    // "task <op>" to how often the home does it and the answer is lost
	during map[string]func() // "task <op>" to what happens while the home's answer is on its way
}

// newJumpRig makes a host on the store in dir, inside a synctest bubble, with
// one working AI and its home.
func newJumpRig(t *testing.T, dir string) *jumpRig {
	t.Helper()
	jump := Listeners["jump"]
	r := &jumpRig{t: t, home: newFakeDeliverHome(), drv: &fakeDriver{}, ai: runAI1, tasks: map[string]proto.TaskRow{}, refuse: map[string]string{}, gone: map[string]int{}, lost: map[string]int{}, during: map[string]func(){}}
	Drivers = map[string]Driver{"claude": r.drv}
	Listeners = map[string]func(*Host, SessionKey, Event){}
	if jump != nil {
		Listeners["jump"] = jump
	}
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
	Deliver = deliverNow
	Instructions = func(Session) string { return "" }
	Compose = jumpCompose
	db, err := store.Open(filepath.Join(dir, "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := New(db, seamKey(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	r.h, r.db = h, db
	h.Shim = []string{"solodawn", "mcp"}
	r.home.onCmd = r.answer
	h.Attach(jumpHome, r.home)
	r.putGroup(false)
	r.putAI(runAI1)
	return r
}

func (r *jumpRig) putAI(id string) {
	member := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 2}, Group: runGroup1, Kind: "ai", Name: "Claude Code"},
		CLI:       "claude", Host: r.h.Key.ID(), Allow: []string{}, Approvers: []string{},
	}
	r.h.Rows.Put(runGroup1, "member", id, json.RawMessage(marshalWire(member)))
}

// close ends the host as a node that stops does: the CLIs are killed and the
// turns they ran are not ended.
func (r *jumpRig) close() {
	r.home.closeConn()
	r.h.Settle()
	synctest.Wait()
	r.db.Close()
}

// jumpCompose names the items of a turn made of items, and writes the other
// turns as the host does.
func jumpCompose(h *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
	if kind != ItemsTurn {
		return composeTurn(h, s, kind, items)
	}
	nums := make([]string, len(items))
	for i, raw := range items {
		var wire proto.InboxItem
		proto.Decode(raw, &wire)
		nums[i] = strconv.FormatInt(wire.Item.Item, 10)
	}
	return "items " + strings.Join(nums, ","), Turn{Kind: kind, Thread: s.Task}, nil
}

func jumpGroupRow(paused bool, seq int64) json.RawMessage {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: seq}, Name: "Website", Kind: "project",
		Base: "main", Forge: "none", Home: jumpHome, Checks: []string{},
	}
	if paused {
		grp.Paused = &proto.GroupPause{By: testHuman, At: 1}
	}
	return json.RawMessage(marshalWire(grp))
}

func (r *jumpRig) putGroup(paused bool) {
	r.h.Rows.Put(runGroup1, "group", runGroup1, jumpGroupRow(paused, 1))
}

// pushGroup pushes the group's row as the home does when the group is paused
// or resumed, and waits until the host has acted on it.
func (r *jumpRig) pushGroup(paused bool, seq int64) {
	frame := proto.Rows{Type: "rows", Group: runGroup1, Rows: []json.RawMessage{jumpGroupRow(paused, seq)}}
	r.h.Push(jumpHome, json.RawMessage(marshalWire(frame)))
	synctest.Wait()
}

// putTask writes the row of a task of the AI, as the home pushes it before it
// answers the command that changed it.
func (r *jumpRig) putTask(id, state, step, wait string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		num := len(r.tasks) + 1
		task = proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: id}, Group: runGroup1, Num: num, Title: "Fix it",
			Slug: proto.Slug("fix-" + strconv.Itoa(num)), Origin: "direct", Assignee: r.ai, Depends: []string{},
		}
	}
	task.Seq++
	task.State, task.Step, task.Wait = state, step, nil
	if wait != "" {
		task.Wait = &proto.TaskWait{Kind: wait}
	}
	r.tasks[id] = task
	r.h.Rows.Put(runGroup1, "task", id, json.RawMessage(marshalWire(task)))
}

// answer is the home's answer to a command: it pauses and resumes a task as
// rows 16 and 13b of the task machine do, unless the test has it refuse the
// command or be out of reach.
func (r *jumpRig) answer(_ context.Context, cmd proto.Cmd) (json.RawMessage, error) {
	var p struct {
		T  string `json:"t"`
		Op string `json:"op"`
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(cmd.Env.Payload), &p)
	op := p.T + " " + p.Op
	r.mu.Lock()
	code, gone, lost, during := r.refuse[op], r.gone[op] > 0, r.lost[op] > 0, r.during[op]
	r.gone[op]--
	r.lost[op]--
	r.mu.Unlock()
	switch {
	case gone:
		return nil, errors.New("the home is gone")
	case code != "":
		return json.RawMessage(`{"re":1,"err":{"code":"` + code + `","msg":"refused"}}`), nil
	}
	r.apply(op, p.ID)
	if during != nil {
		during()
	}
	if lost {
		return nil, errors.New("the connection ended")
	}
	return json.RawMessage(`{"re":1,"ok":true}`), nil
}

func (r *jumpRig) apply(op, id string) {
	switch op {
	case "task preempted":
		r.putTask(id, "paused", "local_check", "preempted")
	case "task resume":
		r.putTask(id, "active", "local_check", "")
	}
}

func (r *jumpRig) set(table map[string]string, op, code string) {
	r.mu.Lock()
	table[op] = code
	r.mu.Unlock()
}

// push delivers one inbox item from the home and waits until the host has
// done all it can with it.
func (r *jumpRig) push(it proto.Item) {
	r.send(it)
	synctest.Wait()
}

func (r *jumpRig) send(it proto.Item) {
	it.AI = proto.ID(r.ai)
	frame := proto.Inbox{Type: "inbox", Group: runGroup1, Items: []proto.InboxItem{{Item: it}}}
	r.h.Push(jumpHome, json.RawMessage(marshalWire(frame)))
}

// deliver looks at everything again, as when a hold has been lifted.
func (r *jumpRig) deliver() {
	Deliver(r.h, runGroup1, runAI1)
	synctest.Wait()
}

// takeOver begins a takeover of a session of the AI, whose CLI it closes, and
// returns what ends it without a hand-back turn.
func (r *jumpRig) takeOver(key SessionKey) func() {
	slot, p := aiSlot{group: runGroup1, member: runAI1}, takeoverState(r.h)
	p.claim(slot, &takeover{key: key}, false)
	Runner.Close(r.h, key)
	r.h.Sessions.Move(key, Dormant, Takeover)
	return func() {
		r.h.Sessions.Move(key, Takeover, Dormant)
		p.mu.Lock()
		delete(p.open, slot)
		p.mu.Unlock()
		r.deliver()
	}
}

func jumpMention(item int64, msg, task string, jump bool) proto.Item {
	return proto.Item{Item: item, Why: "mention", Msg: proto.ID(msg), Task: proto.ID(task), Jump: proto.True(jump)}
}

// end ends the turn a CLI is running, with a status.
func (r *jumpRig) end(p *fakeProc, status string) {
	p.emit(Event{Kind: TurnEnded, Status: status})
	synctest.Wait()
}

// work starts the task session of a task with its assign item and returns its
// CLI, which is then running that turn.
func (r *jumpRig) work(item int64, task, step string) *fakeProc {
	r.putTask(task, "active", step, "")
	r.push(proto.Item{Item: item, Why: "assign", Task: proto.ID(task)})
	return r.drv.lastProc()
}

func jumpTurns(p *fakeProc) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.turns)
}

// reports is what the host told the home beside plain member statuses, in
// order: "task <op> <id> <msg or reason>" and "event <code> <reason>".
func (r *jumpRig) reports() []string {
	var out []string
	for _, raw := range r.home.cmdPayloads() {
		var p struct {
			T      string `json:"t"`
			Op     string `json:"op"`
			ID     string `json:"id"`
			Msg    string `json:"msg"`
			Reason string `json:"reason"`
			Event  *struct {
				Code   string `json:"code"`
				Reason string `json:"reason"`
			} `json:"event"`
		}
		json.Unmarshal([]byte(raw), &p)
		if p.T == "task" {
			out = append(out, strings.TrimSpace("task "+p.Op+" "+p.ID+" "+p.Msg+p.Reason))
		} else if p.Event != nil {
			out = append(out, "event "+p.Event.Code+" "+p.Event.Reason)
		}
	}
	return out
}

func (r *jumpRig) wantReports(want ...string) {
	r.t.Helper()
	if got := r.reports(); !slices.Equal(got, want) {
		r.t.Fatalf("reports = %q, want %q", got, want)
	}
}

func (r *jumpRig) wantTurns(p *fakeProc, want ...string) {
	r.t.Helper()
	if got := jumpTurns(p); !slices.Equal(got, want) {
		r.t.Fatalf("turns = %q, want %q", got, want)
	}
}

// wantHeld checks which items the host holds, none of them acknowledged.
func (r *jumpRig) wantHeld(items ...int64) {
	r.t.Helper()
	if n := r.h.QueuedCount(runGroup1, runAI1); n != len(items) {
		r.t.Fatalf("%d items are held, want %v", n, items)
	}
	for _, item := range items {
		if r.home.ackCountFor(item) != 0 {
			r.t.Fatalf("item %d is held and acknowledged", item)
		}
	}
}

func (r *jumpRig) wantAcked(items ...int64) {
	r.t.Helper()
	for _, item := range items {
		if n := r.home.ackCountFor(item); n != 1 {
			r.t.Fatalf("item %d has %d acknowledgements, want 1", item, n)
		}
	}
}

// wantSettled checks that delivery owes the AI nothing more.
func (r *jumpRig) wantSettled() {
	r.t.Helper()
	if keys := deliv(r.h).heldKeys(); len(keys) != 0 {
		r.t.Fatalf("delivery still owes %v", keys)
	}
}

// wantStates checks the states of the session of the first task and of the
// group session; "" is no session.
func (r *jumpRig) wantStates(task, group State) {
	r.t.Helper()
	if r.state(runTask1) != task || r.state("") != group {
		r.t.Fatalf("task session %q, group session %q, want %q and %q", r.state(runTask1), r.state(""), task, group)
	}
}

func (r *jumpRig) state(task string) State {
	s, _ := r.h.Sessions.Get(SessionKey{Group: runGroup1, Member: runAI1, Task: task})
	return s.State
}

func jumpTest(t *testing.T, run func(t *testing.T, r *jumpRig)) {
	t.Helper()
	saveRuntimeGlobals(t)
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		r := newJumpRig(t, dir)
		defer r.close()
		run(t, r)
	})
}

const (
	jumpContinueTask  = "[SoloDawn] group \"Website\" · you are \"Claude Code\" · task session T-1\n== 1 of 1 · continue · instruction\nYou were interrupted by a direct request from your owner. Continue the task."
	jumpContinueGroup = "[SoloDawn] group \"Website\" · you are \"Claude Code\" · group session\n== 1 of 1 · continue · instruction\nYou were interrupted by a direct request from your owner. Continue."
)

// A jump at a busy AI keeps the item, unacknowledged, until the turn it
// interrupted has ended, and then delivers it once. Before the host half of
// the jump existed the item was acknowledged at once and its turn refused
// with busy: the message was gone.
func TestJumpAtABusyAIDeliversTheMessageAfterTheInterruptedTurn(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.wantHeld(2)
		if n := task.interrupts.Load(); n != 1 || r.drv.count() != 1 {
			t.Fatalf("the running turn got %d interrupts and %d CLIs run, want 1 and 1", n, r.drv.count())
		}
		r.end(task, "interrupted")
		r.wantAcked(1, 2)
		r.wantTurns(r.drv.proc(1), "items 2")
	})
}

func TestJumpForAnotherSessionPreemptsTheTaskAndContinuesIt(t *testing.T) {
	d := captureDiag(t)
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", false))
		r.push(jumpMention(3, testMsg2, runTask1, false))
		r.wantHeld(2, 3)
		r.push(jumpMention(4, testMsg1, "", true))
		// Item 4 is the jump of the message of item 2, which the home retired.
		r.wantHeld(3, 4)
		r.wantReports()
		r.wantStates(Working, Dormant)
		if n := task.interrupts.Load(); n != 1 {
			t.Fatalf("the running turn got %d interrupts, want 1", n)
		}

		r.end(task, "interrupted")
		group := r.drv.proc(1)
		r.wantReports("task preempted " + runTask1 + " " + testMsg1)
		r.wantTurns(group, "items 4")
		r.wantAcked(4)
		r.wantHeld(3)
		r.wantStates(Waiting, Working)

		r.end(group, "completed")
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantHeld(3)
		if c, _ := r.h.Sessions.Caller(tokenFromLaunch(task.launch)); c.Turn.Kind != ContinueTurn {
			t.Fatalf("the turn of its own is noted as %q", c.Turn.Kind)
		}

		r.end(task, "completed")
		r.wantTurns(task, "items 1", jumpContinueTask, "items 3")
		r.wantAcked(3)
		r.end(task, "completed")
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
		r.wantSettled()
		if r.home.ackCountFor(2) != 0 {
			t.Fatal("the retired item was acknowledged")
		}
	})
	where := " node=" + seamKey().ID()[:8] + " g=" + runGroup1 + " ai=" + runAI1
	d.lineOnce(t, "turn.interrupt"+where+" task="+runTask1+" why=jump")
	if busy := "inbox.take" + where + ` task="" took="" left=4,3 paused=false`; d.count(d.is(busy)) == 0 {
		t.Fatalf("no line %q for the look at the queue of the busy AI", busy)
	}
}

// The jump item goes first, then the interrupted session gets the turn of its
// own as after any jump, then what waited.
func TestJumpForTheRunningSessionGoesFirstAndReportsNothing(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, runTask1, false))
		r.push(jumpMention(3, testMsg2, runTask1, true))
		r.push(jumpMention(3, testMsg2, runTask1, true))
		r.wantHeld(2, 3)
		if n := task.interrupts.Load(); n != 1 {
			t.Fatalf("the running turn got %d interrupts, want 1", n)
		}
		r.end(task, "interrupted")
		r.wantTurns(task, "items 1", "items 3")
		r.wantAcked(3)
		r.wantHeld(2)
		r.end(task, "completed")
		r.wantTurns(task, "items 1", "items 3", jumpContinueTask)
		r.end(task, "completed")
		r.wantTurns(task, "items 1", "items 3", jumpContinueTask, "items 2")
		r.end(task, "completed")
		r.wantReports()
		r.wantSettled()
		if r.drv.count() != 1 {
			t.Fatal("another session ran")
		}
	})
}

// At step merge the home refuses task preempted: nothing more is reported for
// the task, its session still gets its turn, and nothing of its own comes
// before that turn.
func TestJumpAtStepMergeReportsNoResume(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "merge")
		r.set(r.refuse, "task preempted", "conflict")
		r.push(jumpMention(2, testMsg2, runTask1, false))
		r.push(jumpMention(3, testMsg1, "", true))
		r.end(task, "interrupted")
		r.wantHeld(2)
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.end(task, "completed")
		r.wantTurns(task, "items 1", jumpContinueTask, "items 2")
		r.wantReports("task preempted " + runTask1 + " " + testMsg1)
	})
}

func TestJumpInterruptingAGroupSessionReportsNothingAndContinuesIt(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.end(task, "completed")
		r.push(jumpMention(2, testMsg1, "", false))
		group := r.drv.proc(1)
		r.push(jumpMention(3, testMsg2, runTask1, true))
		if group.interrupts.Load() != 1 {
			t.Fatal("the group session's turn was not interrupted")
		}
		r.end(group, "interrupted")
		r.wantTurns(task, "items 1", "items 3")
		r.end(task, "completed")
		r.wantTurns(group, "items 2", jumpContinueGroup)
		r.end(group, "completed")
		r.wantReports()
		r.wantSettled()
	})
}

// The author jumps a message a second time, as after a reload of the page:
// the home retires the first jump item and sends another, and the message is
// delivered once.
func TestMessageJumpedTwiceIsDeliveredOnce(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.push(jumpMention(3, testMsg1, "", true))
		r.wantHeld(3)
		if n := task.interrupts.Load(); n != 1 {
			t.Fatalf("the running turn got %d interrupts, want 1", n)
		}
		r.end(task, "interrupted")
		group := r.drv.proc(1)
		r.wantTurns(group, "items 3")
		r.end(group, "completed")
		r.wantTurns(group, "items 3")
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantAcked(3)
		if r.home.ackCountFor(2) != 0 {
			t.Fatal("the retired jump item was acknowledged")
		}
	})
}

// A group paused during the jump turn stops that turn and leaves the task
// pre-empted; the resume of the group brings the task back by row 13b and
// continues it.
func TestGroupPausedDuringTheJumpTurnContinuesTheTaskOnResume(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		group := r.drv.proc(1)
		r.pushGroup(true, 2)
		if group.interrupts.Load() != 1 {
			t.Fatal("the pause did not interrupt the jump turn")
		}
		r.end(group, "interrupted")
		r.wantTurns(task, "items 1")
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "event turn_interrupted stop")
		r.pushGroup(false, 3)
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "event turn_interrupted stop", "task resume "+runTask1)
		r.end(task, "completed")
		r.wantSettled()
	})
}

// A message that mentions two AIs of this host is jumped for both: each is
// interrupted, answers and continues by itself.
func TestJumpOfAMessageForTwoAIsRunsForEach(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		first := r.work(1, runTask1, "local_check")
		r.putAI(runAI2)
		r.ai = runAI2
		second := r.work(2, runTask2, "local_check")
		r.push(jumpMention(4, testMsg1, "", true))
		r.ai = runAI1
		r.push(jumpMention(3, testMsg1, "", true))
		if first.interrupts.Load() != 1 || second.interrupts.Load() != 1 {
			t.Fatalf("interrupts = %d and %d, want 1 and 1", first.interrupts.Load(), second.interrupts.Load())
		}
		r.end(first, "interrupted")
		answer1 := r.drv.lastProc()
		r.end(second, "interrupted")
		answer2 := r.drv.lastProc()
		r.wantTurns(answer1, "items 3")
		r.wantTurns(answer2, "items 4")
		r.end(answer1, "completed")
		r.end(answer2, "completed")
		r.wantTurns(first, "items 1", jumpContinueTask)
		r.wantTurns(second, "items 2", strings.Replace(jumpContinueTask, "T-1", "T-2", 1))
		r.wantReports(
			"task preempted "+runTask1+" "+testMsg1, "task preempted "+runTask2+" "+testMsg1,
			"task resume "+runTask1, "task resume "+runTask2,
		)
		r.wantAcked(1, 2, 3, 4)
	})
}

// A CLI that ignores the interrupt is killed after 10 s: that is a crash, and
// nothing retries by itself (flows/agent-turn.md E, G). The jump item is
// delivered after it.
func TestJumpWhoseInterruptIsIgnoredIsDeliveredAfterTheCrash(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		time.Sleep(stopSeconds * time.Second)
		synctest.Wait()
		r.wantReports("event turn_interrupted crash", "task fail "+runTask1+" crash")
		group := r.drv.proc(1)
		r.wantTurns(group, "items 2")
		r.wantAcked(2)
		r.end(group, "completed")
		r.wantTurns(task, "items 1")
		r.wantReports("event turn_interrupted crash", "task fail "+runTask1+" crash")
		r.wantSettled()
		if r.state(runTask1) != Dormant || r.drv.count() != 2 {
			t.Fatalf("task session %s after %d starts", r.state(runTask1), r.drv.count())
		}
	})
}

func TestJumpAtATurnThatEndsByItselfReportsNothing(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "completed")
		group := r.drv.proc(1)
		r.wantTurns(group, "items 2")
		r.end(group, "completed")
		r.wantTurns(task, "items 1")
		r.wantReports()
		r.wantSettled()
	})
}

// A turn that is ending for another reason keeps that reason: the jump waits
// for its end and pre-empts nothing.
func TestJumpAtATurnInterruptedForAnotherReasonWaitsForItsEnd(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		interruptSessionTurn(r.h, key, "timeout", "Turn time limit reached.")
		r.push(jumpMention(2, testMsg1, "", true))
		if n := task.interrupts.Load(); n != 1 {
			t.Fatalf("the turn got %d interrupts, want the one of the timeout", n)
		}
		r.wantHeld(2)
		r.end(task, "interrupted")
		group := r.drv.proc(1)
		r.wantTurns(group, "items 2")
		r.end(group, "completed")
		r.wantTurns(task, "items 1")
		r.wantReports("event turn_interrupted timeout", "task fail "+runTask1+" timeout")
		r.wantSettled()
	})
}

// A jump item that something else holds interrupts nothing.
func TestHeldJumpInterruptsNothing(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		other := r.work(1, runTask2, "local_check")
		r.end(other, "completed")
		task := r.work(2, runTask1, "local_check")
		r.putTask(runTask2, "paused", "local_check", "manual")
		r.push(jumpMention(3, testMsg1, runTask2, true))
		r.putGroup(true)
		r.push(jumpMention(4, testMsg2, "", true))
		r.wantHeld(3, 4)
		if task.interrupts.Load() != 0 {
			t.Fatal("a held jump interrupted the running turn")
		}
	})
}

// A group that is paused between the interrupt and the jump turn holds the
// jump item and reports nothing; when it resumes, the jump runs as before.
func TestJumpInAGroupPausedMeanwhileWaitsForTheResume(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.putGroup(true)
		r.end(task, "interrupted")
		r.wantHeld(2)
		r.wantReports()
		r.putGroup(false)
		r.deliver()
		r.wantAcked(2)
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
	})
}

// The turn of its own is held as a delivery is. When the hold lifts, delivery
// gives it although no item is held for the AI.
func TestInterruptedSessionContinuesWhenItsHoldLifts(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		var held sync.Mutex
		offline := true
		Holds["offline"] = func(*Host, SessionKey) (string, bool) {
			held.Lock()
			defer held.Unlock()
			return "offline", offline
		}
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.wantReports("task preempted " + runTask1 + " " + testMsg1)
		held.Lock()
		offline = false
		held.Unlock()
		Deliver(r.h, runGroup1, runAI2)
		synctest.Wait()
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
	})
}

// A session that has ended meanwhile, as when its task was cancelled, is owed
// nothing, and what waited behind its turn goes on.
func TestInterruptedSessionThatEndedGetsNoTurn(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg2, "", false))
		r.push(jumpMention(3, testMsg1, "", true))
		r.end(task, "interrupted")
		group := r.drv.proc(1)
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		Runner.Close(r.h, key)
		r.h.Sessions.End(key)
		r.end(group, "completed")
		r.wantTurns(group, "items 3", "items 2")
		r.wantTurns(task, "items 1")
		r.wantReports("task preempted " + runTask1 + " " + testMsg1)
		r.end(group, "completed")
		r.wantSettled()
	})
}

// Paused or failed means no turns (contracts/task-machine.md section 4): a
// task that failed while it was pre-empted is not continued.
func TestInterruptedSessionOfAStoppedTaskGetsNoTurn(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.putTask(runTask1, "failed", "local_check", "")
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.wantReports("task preempted " + runTask1 + " " + testMsg1)
		r.wantSettled()
	})
}

// A task resume that does not reach the home is reported again when delivery
// next runs, and the turn of its own waits for it.
func TestTaskResumeWaitsForTheHome(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.mu.Lock()
		r.gone["task resume"] = 1
		r.mu.Unlock()
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.deliver()
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1, "task resume "+runTask1)
		r.end(task, "completed")
		r.wantSettled()
	})
}

// While a pre-emption is open, a jump for the session that is running
// interrupts it, and one for yet another session interrupts nothing and runs
// when the jump turn has ended. Both go before the interrupted sessions
// continue, the one interrupted last first.
func TestFurtherJumpsRunBeforeTheInterruptedSessionContinues(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		second := r.work(1, runTask2, "local_check")
		r.end(second, "completed")
		task := r.work(2, runTask1, "local_check")
		r.push(jumpMention(3, testMsg1, "", true))
		r.end(task, "interrupted")
		group := r.drv.proc(2)
		r.wantTurns(group, "items 3")

		r.push(jumpMention(4, testCall1, "", true))
		if group.interrupts.Load() != 1 {
			t.Fatal("a jump for the running session did not interrupt it")
		}
		r.end(group, "interrupted")
		r.wantTurns(group, "items 3", "items 4")

		r.push(jumpMention(5, testMsg2, runTask2, true))
		if group.interrupts.Load() != 1 {
			t.Fatal("a jump for a third session interrupted the jump turn")
		}
		r.wantHeld(5)
		r.end(group, "completed")
		r.wantTurns(second, "items 1", "items 5")
		r.wantTurns(task, "items 2")
		r.end(second, "completed")
		r.wantTurns(group, "items 3", "items 4", jumpContinueGroup)
		r.wantTurns(task, "items 2")
		r.end(group, "completed")
		r.wantTurns(task, "items 2", strings.Replace(jumpContinueTask, "T-1", "T-2", 1))
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
		r.end(task, "completed")
		r.wantSettled()
	})
}

// A turn of its own that cannot start is logged and not tried again.
func TestTurnOfItsOwnThatCannotStartIsNotRetried(t *testing.T) {
	logs := captureLogs(t)
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		Runner.Close(r.h, SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1})
		r.drv.mu.Lock()
		r.drv.startErr = errors.New("no such CLI")
		r.drv.mu.Unlock()
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.wantSettled()
	})
	if !strings.Contains(logs.String(), "host: continue") || !strings.Contains(logs.String(), "no such CLI") {
		t.Fatalf("the failed turn was not logged: %s", logs.String())
	}
}

// A host that stops between the interrupt and the jump turn has not
// acknowledged the item: the home sends it again and it is delivered.
func TestJumpSurvivesAHostRestartBeforeItsTurn(t *testing.T) {
	saveRuntimeGlobals(t)
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		r := newJumpRig(t, dir)
		r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.wantHeld(2)
		r.close()

		r = newJumpRig(t, dir)
		defer r.close()
		r.putTask(runTask1, "active", "local_check", "")
		r.push(jumpMention(2, testMsg1, "", true))
		r.wantAcked(2)
		r.wantTurns(r.drv.lastProc(), "items 2")
		if r.state(runTask1) != Dormant {
			t.Fatalf("the interrupted session is %s after the restart", r.state(runTask1))
		}
	})
}

// The home sends an unacknowledged item again when it connects. A copy that
// arrives while the jump turn is being prepared is of that turn.
func TestResentJumpItemDoesNotInterruptItsOwnTurn(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		Compose = func(h *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
			r.send(jumpMention(2, testMsg1, "", true))
			Compose = jumpCompose
			return jumpCompose(h, s, kind, items)
		}
		r.end(task, "interrupted")
		group := r.drv.proc(1)
		r.wantTurns(group, "items 2")
		if n := group.interrupts.Load(); n != 0 {
			t.Fatalf("the jump turn got %d interrupts from its own item", n)
		}
		r.end(group, "completed")
		r.wantTurns(group, "items 2")
		r.wantTurns(task, "items 1", jumpContinueTask)
	})
}

// The home accepts task preempted and the answer is lost: the row it pushed
// says that task resume is owed.
func TestTaskPreemptedWithALostAnswerIsResumed(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		r.lost["task preempted"] = 1
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
		r.end(task, "completed")
		r.wantSettled()
	})
}

// As before, and the row comes only when the host has synced, after the
// interrupted session was continued: the task goes on all the same, and
// before what waits for the AI.
func TestTaskFoundPreemptedAfterALostAnswerIsResumed(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		r.gone["task preempted"] = 1
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.push(jumpMention(3, testMsg2, "", false))
		r.putTask(runTask1, "paused", "local_check", "preempted")
		r.pushGroup(false, 2)
		r.end(task, "interrupted")
		r.wantTurns(task, "items 1", jumpContinueTask, jumpContinueTask)
		r.wantHeld(3)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "event turn_interrupted stop", "task resume "+runTask1)
		r.end(task, "completed")
		r.wantAcked(3)
	})
}

// A host that stops after task preempted finds the task pre-empted when it
// has synced again, and brings it back although no item is held for the AI.
func TestPreemptedTaskIsBroughtBackAfterAHostRestart(t *testing.T) {
	saveRuntimeGlobals(t)
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		r := newJumpRig(t, dir)
		r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(r.drv.proc(0), "interrupted")
		r.wantStates(Waiting, Working)
		r.close()

		r = newJumpRig(t, dir)
		defer r.close()
		r.putTask(runTask1, "paused", "local_check", "preempted")
		r.putTask(runTask2, "paused", "local_check", "preempted")
		r.pushGroup(false, 2)
		r.wantReports("task resume " + runTask1)
		task := r.drv.lastProc()
		r.wantTurns(task, jumpContinueTask)
		r.end(task, "completed")
		r.wantSettled()
	})
}

// A jump goes before a task that is found pre-empted, which is not reported
// again.
func TestJumpGoesBeforeATaskFoundPreempted(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.end(task, "completed")
		r.putTask(runTask1, "paused", "local_check", "preempted")
		r.push(jumpMention(2, testMsg1, "", true))
		group := r.drv.proc(1)
		r.wantTurns(group, "items 2")
		r.wantReports()
		r.end(group, "completed")
		r.wantReports("task resume " + runTask1)
		r.wantTurns(task, "items 1", jumpContinueTask)
	})
}

// The task of a review session, and one paused otherwise, are not the host's
// to resume.
func TestOnlyAPreemptedTaskOfATaskSessionIsBroughtBack(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.end(task, "completed")
		r.putTask(runTask1, "paused", "local_check", "manual")
		r.putTask(runTask2, "paused", "review", "preempted")
		r.h.Sessions.Create(makeRunSession(SessionKey{Group: runGroup1, Member: runAI1, Task: runTask2}, ReviewSession, "review/2"))
		r.deliver()
		r.wantReports()
		r.wantSettled()
	})
}

// A task resume that the home refuses is not repeated until delivery next
// runs, and the turn of its own follows the task's row.
func TestRefusedTaskResumeWaitsForTheRow(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.set(r.refuse, "task resume", "conflict")
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
		r.putTask(runTask1, "active", "local_check", "")
		r.deliver()
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
	})
}

// A jump whose session cannot be prepared, as when a git fetch fails, stops
// nothing: the turn goes on and the item waits. The next look tries again.
func TestJumpWhoseSessionCannotBePreparedInterruptsNothing(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		var grp proto.GroupRow
		json.Unmarshal(jumpGroupRow(false, 1), &grp)
		grp.Repo = filepath.Join(t.TempDir(), "no-such-repo")
		r.h.Rows.Put(runGroup1, "group", runGroup1, json.RawMessage(marshalWire(grp)))
		r.push(jumpMention(2, testMsg1, "", true))
		r.wantHeld(2)
		r.wantReports()
		if n := task.interrupts.Load(); n != 0 {
			t.Fatalf("the turn got %d interrupts for a jump that cannot go", n)
		}
		r.putGroup(false)
		r.deliver()
		if n := task.interrupts.Load(); n != 1 {
			t.Fatalf("the turn got %d interrupts once the session can be prepared, want 1", n)
		}
		r.end(task, "interrupted")
		r.wantAcked(2)
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1", jumpContinueTask)
	})
}

// The home is waited for when task resume is sent. What comes meanwhile, a
// takeover or a pause of the group, holds the turn of its own.
func TestHoldDuringTaskResumeKeepsTheTurnOfItsOwn(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		var handBack func()
		r.during["task resume"] = func() { handBack = r.takeOver(SessionKey{Group: runGroup1, Member: runAI1}) }
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.during["task resume"] = nil
		handBack()
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.end(task, "completed")
		r.wantSettled()
	})
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.during["task resume"] = func() { r.putGroup(true) }
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1")
		r.putGroup(false)
		r.deliver()
		r.wantTurns(task, "items 1", jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
	})
}

// A second jump, in the thread of the pre-empted task, is held until the task
// is resumed. Then it goes first, and the turn of its own follows.
func TestSecondJumpInTheThreadOfThePreemptedTask(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		r.end(task, "interrupted")
		r.push(jumpMention(3, testMsg2, runTask1, true))
		r.wantHeld(3)
		r.end(r.drv.proc(1), "completed")
		r.wantTurns(task, "items 1", "items 3")
		if n := task.interrupts.Load(); n != 1 {
			t.Fatalf("the task session got %d interrupts, want the one of the first jump", n)
		}
		r.end(task, "completed")
		r.wantTurns(task, "items 1", "items 3", jumpContinueTask)
		r.end(task, "completed")
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
		r.wantSettled()
	})
}

// A takeover that starts while task preempted is on its way holds the jump
// item, which is not acknowledged.
func TestTakeoverDuringTaskPreemptedHoldsTheJump(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, "", true))
		var handBack func()
		r.during["task preempted"] = func() {
			handBack = r.takeOver(SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1})
			r.send(jumpMention(2, testMsg1, "", true))
		}
		r.end(task, "interrupted")
		r.wantHeld(2)
		if n := r.drv.count(); n != 1 {
			t.Fatalf("%d CLIs ran during the takeover, want 1", n)
		}
		handBack()
		r.wantAcked(2)
		group := r.drv.proc(1)
		r.wantTurns(group, "items 2")
		r.end(group, "completed")
		r.wantTurns(r.drv.lastProc(), jumpContinueTask)
		r.wantReports("task preempted "+runTask1+" "+testMsg1, "task resume "+runTask1)
	})
}

// A further jump for the session that is running the jump turn interrupts
// that turn. A session interrupted twice still gets one turn of its own.
func TestJumpTurnThatAFurtherJumpInterruptsIsContinuedOnce(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.push(jumpMention(2, testMsg1, runTask1, true))
		r.end(task, "interrupted")
		r.push(jumpMention(3, testMsg2, runTask1, true))
		if n := task.interrupts.Load(); n != 2 {
			t.Fatalf("the session got %d interrupts, want 2", n)
		}
		r.end(task, "interrupted")
		r.wantTurns(task, "items 1", "items 2", "items 3")
		r.end(task, "completed")
		r.wantTurns(task, "items 1", "items 2", "items 3", jumpContinueTask)
		r.end(task, "completed")
		r.wantReports()
		r.wantSettled()
	})
}

// A hand-back takes a session from takeover to working. Delivery that looks
// at the AI in between, having seen no running turn yet, must still see one
// of the two and hold the items.
func TestItemsStayHeldAcrossAHandBackStart(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		key := SessionKey{Group: runGroup1, Member: runAI1}
		// The takeover is registered, so that reading the group's state does not hand the row back as one a restart left.
		r.h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
		r.takeOver(key)
		r.push(jumpMention(1, testMsg1, "", false))
		r.wantHeld(1)
		Holds["hand_back"] = func(h *Host, _ SessionKey) (string, bool) {
			h.Sessions.Move(key, Takeover, Working)
			return "", false
		}
		r.deliver()
		r.wantHeld(1)
		if r.state("") != Working || r.drv.count() != 0 {
			t.Fatalf("state %s, %d CLIs started: an item was delivered into the hand-back turn", r.state(""), r.drv.count())
		}
	})
}

// The turn of its own is held across a hand-back start as items are.
func TestTurnOfItsOwnStaysHeldAcrossAHandBackStart(t *testing.T) {
	jumpTest(t, func(t *testing.T, r *jumpRig) {
		task := r.work(1, runTask1, "local_check")
		r.end(task, "completed")
		r.push(jumpMention(2, testMsg1, "", false))
		group := r.drv.proc(1)
		r.push(jumpMention(3, testMsg2, runTask1, true))
		r.end(group, "interrupted")
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		r.putGroup(true)
		r.end(task, "completed")
		handBack := r.takeOver(key)
		r.putGroup(false)
		Holds["hand_back"] = func(h *Host, _ SessionKey) (string, bool) {
			h.Sessions.Move(key, Takeover, Working)
			return "", false
		}
		r.deliver()
		r.wantTurns(group, "items 2")
		delete(Holds, "hand_back")
		r.h.Sessions.Move(key, Working, Dormant)
		handBack()
		r.wantTurns(group, "items 2", jumpContinueGroup)
	})
}
