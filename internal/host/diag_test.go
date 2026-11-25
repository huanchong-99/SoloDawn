package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// diagLines keeps the lines the standard logger writes, without "INFO ".
type diagLines struct {
	mu    sync.Mutex
	lines []string
}

func (d *diagLines) Write(b []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if text, ok := strings.CutPrefix(strings.TrimSuffix(string(b), "\n"), "INFO "); ok {
		d.lines = append(d.lines, text)
	}
	return len(b), nil
}

func captureDiag(t *testing.T) *diagLines {
	t.Helper()
	d := &diagLines{}
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(d)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		if t.Failed() {
			t.Logf("log: %q", d.all())
		}
	})
	return d
}

func (d *diagLines) all() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.lines)
}

// with is the lines whose text begins with prefix.
func (d *diagLines) with(prefix string) []string {
	return slices.DeleteFunc(d.all(), func(l string) bool { return !strings.HasPrefix(l, prefix) })
}

func (d *diagLines) count(match func(string) bool) int {
	return len(slices.DeleteFunc(d.all(), func(l string) bool { return !match(l) }))
}

func (d *diagLines) is(line string) func(string) bool {
	return func(l string) bool { return l == line }
}

// once fails unless the log holds match exactly once.
func (d *diagLines) once(t *testing.T, what string, match func(string) bool) {
	t.Helper()
	if n := d.count(match); n != 1 {
		t.Fatalf("%s: %d lines, want 1", what, n)
	}
}

func (d *diagLines) lineOnce(t *testing.T, line string) {
	t.Helper()
	d.once(t, line, d.is(line))
}

// wait waits until the log holds line.
func (d *diagLines) wait(t *testing.T, line string) {
	t.Helper()
	poll(t, 10*time.Second, fmt.Sprintf("%q", line), func() string {
		if d.count(d.is(line)) > 0 {
			return ""
		}
		lines := d.all()
		return fmt.Sprintf("%d lines, last %q", len(lines), lines[max(0, len(lines)-5):])
	})
}

// ran fails unless the log holds bg.start and bg.stop of the work n times
// each, the start first.
func (d *diagLines) ran(t *testing.T, n int, node, work string) {
	t.Helper()
	start, stop := "bg.start "+node+" name="+work, "bg.stop "+node+" name="+work
	lines := d.all()
	if i, j := slices.Index(lines, start), slices.Index(lines, stop); i < 0 || j < i || d.count(d.is(start)) != n || d.count(d.is(stop)) != n {
		t.Fatalf("name=%s: %d starts (first %d) and %d stops (first %d), want %d", work, d.count(d.is(start)), i, d.count(d.is(stop)), j, n)
	}
}

func TestDiagDeliveryLines(t *testing.T) {
	d := captureDiag(t)
	r := newDeliverRig(t, false)
	node, home := "node="+r.hostKey.ID()[:8], "home="+r.homeKey.ID()[:8]
	g := node + " g=" + testGroup1
	r.seedGroupAndMembers()
	r.waitUntil("home sync stopped", func() bool { return d.count(d.is("bg.stop "+node+" name=home_sync")) == 1 })
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup2, Rows: []json.RawMessage{}})))
	r.waitGroupLive(testGroup2)

	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 7, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "hi"))
	take7 := "inbox.take " + g + " ai=" + testAI1 + ` task="" took=7 left="" paused=false`
	r.waitUntil("item 7 taken", func() bool { return d.count(d.is(take7)) == 1 })

	_, done := r.h.Await(testMsg2, nil)
	defer done()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 8, AI: testAI1, Why: "decided", Msg: testMsg2}, nil)
	if err := recordAudit(r.h, AuditRow{Group: testGroup1, What: AuditDelivered, Item: 9, Home: r.homeKey.ID(), By: r.homeKey.ID()}); err != nil {
		t.Fatal(err)
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 9, AI: testAI1, Why: "mention", Msg: testCall1}, nil)
	r.waitUntil("item 9 acknowledged", func() bool { return r.home.ackCountFor(9) == 1 })

	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 2}, Name: "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}, Paused: &proto.GroupPause{By: testHuman, At: testStartMS}})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 10, AI: testAI1, Why: "mention", Msg: testCard1}, r.makeTextMsg(testCard1, nil, "hi"))
	take10 := "inbox.take " + g + " ai=" + testAI1 + ` task="" took="" left=10 paused=true`
	r.waitUntil("item 10 left", func() bool { return d.count(d.is(take10)) > 0 })

	rows := []json.RawMessage{json.RawMessage(marshalWire(proto.RowHeader{Type: "x", ID: "a", Seq: 20})), json.RawMessage(marshalWire(proto.RowHeader{Type: "x", ID: "b", Seq: 21}))}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: rows})))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{}})))
	r.home.closeConn()
	r.waitUntil("detach", func() bool { return d.count(d.is("host.detach "+node+" "+home)) == 1 })
	r.h.Settle()

	for _, line := range []string{
		"host.attach " + node + " " + home,
		"sync.begin " + g + " " + home,
		"sync.end " + g + " " + home + " seq=0 ok=true",
		"inbox.recv " + g + " item=7 ai=" + testAI1 + " kind=mention seen=false",
		take7,
		"inbox.recv " + g + " item=8 ai=" + testAI1 + " kind=decided seen=false",
		"inbox.hand " + g + " item=8 wait=" + testMsg2,
		"inbox.recv " + g + " item=9 ai=" + testAI1 + " kind=mention seen=true",
		"ack.send " + g + " " + home + " item=9",
		"rows.recv " + g + " " + home + " seq=20-21",
		"rows.recv " + g + " " + home + ` seq=""`,
		"settle.begin " + node,
		"settle.end " + node,
	} {
		d.lineOnce(t, line)
	}
	d.ran(t, 1, node, "home_sync")
	d.ran(t, 1, node, "home_watch")
	d.ran(t, 1, node, "group_sync g="+testGroup2)
	d.ran(t, 3, node, "group_state g="+testGroup1)
	d.ran(t, 2, node, "deliver g="+testGroup1)
}

func TestDiagSettleWaitsForTheHomeWatch(t *testing.T) {
	d := captureDiag(t)
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.h.Settle()
	d.ran(t, 1, "node="+r.hostKey.ID()[:8], "home_watch")
}

// The caller of bg may hold a mutex, so the work writes its own bg.start.
func TestDiagWorkWritesItsStart(t *testing.T) {
	d := captureDiag(t)
	before := -1
	r := newDeliverRig(t, false)
	r.h.bg(func(f func()) {
		before = len(d.with("bg."))
		f()
	}, "work", func() {})
	d.ran(t, 1, "node="+r.hostKey.ID()[:8], "work")
	if before != 0 {
		t.Fatalf("%d lines before the work started", before)
	}
}

func TestDiagTurnLines(t *testing.T) {
	d := captureDiag(t)
	r := newDeliverRig(t, false)
	key := SessionKey{Group: testGroup1, Member: testAI1, Task: "T1"}
	s := "node=" + r.hostKey.ID()[:8] + " g=" + testGroup1 + " ai=" + testAI1 + " task=T1"
	diagTurn(r.h, key, Event{Kind: TurnBegan})
	d.lineOnce(t, "turn.begin "+s)
	diagTurn(r.h, key, Event{Kind: Used})
	diagTurn(r.h, key, Event{Kind: TurnEnded, Status: "completed"})
	want := []string{"turn.begin " + s, "turn.end " + s + " status=completed"}
	if got := d.with("turn."); !slices.Equal(got, want) || Listeners["diag"] == nil {
		t.Fatalf("lines %q, want %q; listener %v", got, want, Listeners["diag"] != nil)
	}
}

func TestDiagExternalRunLines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := captureDiag(t)
		h := externalBubble(t)
		node := "node=" + h.Key.ID()[:8]
		h.ExternalPresent(testGroup1, testAI1)()
		time.Sleep(externalPresence)
		synctest.Wait()
		d.ran(t, 1, node, "presence g="+testGroup1)

		s, _ := h.Sessions.Get(SessionKey{Group: testGroup1, Member: testAI1})
		for run, end := range map[int64]struct {
			why string
			do  func(run int64)
		}{
			3: {"timeout", func(int64) { time.Sleep(time.Minute) }},
			4: {"crash", func(run int64) { h.ExternalDone(testGroup1, testAI1, run, "boom") }},
			5: {"done", func(run int64) { h.ExternalDone(testGroup1, testAI1, run, "") }},
		} {
			extOpenRun(h, extOf(h), aiGroupKey{group: testGroup1, member: testAI1}, s, run, Turn{})
			end.do(run)
			synctest.Wait()
			line := fmt.Sprintf("%s g=%s ai=%s run=%d", node, testGroup1, testAI1, run)
			d.lineOnce(t, "run.open "+line)
			d.lineOnce(t, "run.close "+line+" why="+end.why)
		}
		d.ran(t, 1, node, "run_timeout g="+testGroup1)
	})
}

func TestDiagToolLine(t *testing.T) {
	d := captureDiag(t)
	r := newDeliverRig(t, false)
	key := SessionKey{Group: testGroup1, Member: testAI1}
	if err := r.h.Sessions.Create(Session{SessionKey: key, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "Claude"}); err != nil {
		t.Fatal(err)
	}
	saveTools(t, map[string]Tool{"probe": {Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
		return nil, &Error{Code: "secret", Message: "refused"}
	}}})
	r.h.invokeTool(t.Context(), r.h.Sessions.Admit(key, 1, false), "probe", nil)
	prefix := "tool node=" + r.hostKey.ID()[:8] + " g=" + testGroup1 + " ai=" + testAI1 + ` task="" name=probe code=secret ms=`
	d.once(t, prefix, func(l string) bool { return strings.HasPrefix(l, prefix) })
	codes := [3]string{diagCode(nil), diagCode(errors.New("x")), diagCode(fmt.Errorf("w: %w", &Error{Code: "busy"}))}
	if codes != [3]string{"ok", "internal", "busy"} {
		t.Fatalf("codes = %v", codes)
	}
}

func TestDiagProcessAndInterruptLines(t *testing.T) {
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	for name, end := range map[string]struct {
		resume  bool
		why     string
		deliver int
		do      func(h *Host, p *limProc)
	}{
		"interrupt": {false, "stop", 1, func(h *Host, _ *limProc) {
			interruptSessionTurn(h, key, "stop", "")
			time.Sleep(stopSeconds * time.Second)
		}},
		"no session":    {true, "no_session", 0, func(_ *Host, p *limProc) { p.emit(Event{Kind: NoSession}) }},
		"login invalid": {true, "login_invalid", 1, func(_ *Host, p *limProc) { p.emit(Event{Kind: LoginInvalid}) }},
		"exit":          {false, `""`, 1, func(_ *Host, p *limProc) { p.finish() }},
	} {
		t.Run(name, func(t *testing.T) {
			saveRuntimeGlobals(t)
			base := t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				d := captureDiag(t)
				h, _, drv, shutdown := setupLimBubble(t, base)
				defer shutdown()
				drv.ignoreInterrupt = true
				session := makeRunSession(key, TaskSession, "wt/1")
				if end.resume {
					session.CLISession = "old"
				}
				h.Sessions.Create(session)
				mustLimTurn(t, h, key, Turn{Kind: ItemsTurn, Thread: runTask1})
				synctest.Wait()
				end.do(h, drv.proc(0))
				synctest.Wait()
				node := "node=" + h.Key.ID()[:8]
				s := fmt.Sprintf("%s g=%s ai=%s task=%s", node, runGroup1, runAI1, runTask1)
				d.lineOnce(t, fmt.Sprintf("proc.start %s agent=claude resume=%t", s, end.resume))
				d.lineOnce(t, "proc.exit "+s+" agent=claude why="+end.why)
				d.ran(t, 1, node, "home_flush")
				if got := d.with("turn.interrupt " + node); name == "interrupt" != slices.Equal(got, []string{"turn.interrupt " + s + " why=stop"}) {
					t.Fatalf("interrupts %q", got)
				}
				if got := d.count(d.is("bg.stop " + node + " name=deliver g=" + runGroup1)); got != end.deliver {
					t.Fatalf("%d deliveries, want %d", got, end.deliver)
				}
			})
		})
	}
}

func TestDiagLimitLines(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		d := captureDiag(t)
		h, _, _, shutdown := setupLimBubble(t, base)
		defer shutdown()
		s := makeRunSession(SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}, TaskSession, "wt/1")
		onRateLimited(h, limitsOf(h), s, time.Now().Add(time.Second).UnixMilli())
		time.Sleep(time.Second)
		synctest.Wait()
		node := "node=" + h.Key.ID()[:8]
		d.ran(t, 1, node, "member_status g="+runGroup1+" task="+runTask1)
		d.ran(t, 1, node, "deliver")
	})
}

func TestDiagTakeoverLines(t *testing.T) {
	d := captureDiag(t)
	r := newTkRig(t)
	r.putTask(tkTask, "active", runAI1, "", 1)
	if err := r.start(runAI1, tkTask); err != nil {
		t.Fatal(err)
	}
	r.end(runAI1)
	r.waitReleased(runAI1)
	saved := makeRunSession(SessionKey{Group: runGroup1, Member: tkSec, Task: runTask2}, TaskSession, "wt/2")
	r.h.Sessions.Create(saved)
	r.h.Sessions.Move(saved.SessionKey, Dormant, Takeover)
	handBackSaved(r.h, runGroup1)
	r.waitReleased(tkSec)
	r.h.Settle()
	node := "node=" + r.h.Key.ID()[:8]
	d.ran(t, 1, node, "path_watch")
	d.ran(t, 1, node, "hand_back g="+runGroup1+" task="+tkTask)
	d.ran(t, 1, node, "hand_back g="+runGroup1+" task="+runTask2)
}

func TestDiagReleasedSlotDeliveryLines(t *testing.T) {
	d := captureDiag(t)
	r := newDeliverRig(t, false)
	takeoverState(r.h).release(r.h, aiSlot{group: testGroup1, member: testAI1}, &takeover{})
	r.h.Settle()
	d.ran(t, 1, "node="+r.hostKey.ID()[:8], "deliver g="+testGroup1)
}

func TestDiagCloudPollLines(t *testing.T) {
	d := captureDiag(t)
	h, _, _ := customForgeHost(t)
	fakeForgeGH(t, forgeAnswer{})
	startHostedCheck(h, "home-device", tskTestGroupID, HostedCheck{Task: testTask1, Branch: "task/1-poll", Commit: "head", PR: 7, Since: time.Now()})
	node, work := "node="+h.Key.ID()[:8], "cloud_poll g="+tskTestGroupID+" task="+testTask1
	d.wait(t, "bg.stop "+node+" name="+work)
	d.ran(t, 1, node, work)
}

func TestDiagAskLines(t *testing.T) {
	d := captureDiag(t)
	fx := newApproveFixture(t)
	posted := make(chan struct{}, 1)
	fx.home.onPost = func(proto.Post[proto.ApprovalBody]) { posted <- struct{}{} }
	fx.h.Report(fx.sessKey, Event{Kind: Asked, Ask: &Ask{ID: "c1", Tool: "commandExecution", Category: "command", Command: "make a"}})
	recvApproveChan(t, posted, "the card")
	fx.h.Report(fx.sessKey, Event{Kind: TurnEnded, Status: "interrupted"})
	node, work := "node="+fx.hostKey.ID()[:8], "ask g="+fx.groupID+" task="+fx.taskID
	d.wait(t, "bg.stop "+node+" name="+work)
	d.ran(t, 1, node, work)
}
