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
			t.Logf("log: %q", d.lines)
		}
	})
	return d
}

func (d *diagLines) count(match func(string) bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, l := range d.lines {
		if match(l) {
			n++
		}
	}
	return n
}

func (d *diagLines) is(line string) func(string) bool {
	return func(l string) bool { return l == line }
}

// once fails unless the log holds match exactly once.
func (d *diagLines) once(t *testing.T, what string, match func(string) bool) {
	t.Helper()
	if n := d.count(match); n != 1 {
		d.mu.Lock()
		defer d.mu.Unlock()
		t.Fatalf("%s: %d lines, want 1; log %q", what, n, d.lines)
	}
}

func (d *diagLines) lineOnce(t *testing.T, line string) {
	t.Helper()
	d.once(t, line, d.is(line))
}

func TestDiagDeliveryLines(t *testing.T) {
	d := captureDiag(t)
	r := newDeliverRig(t, false)
	node, home := "node="+r.hostKey.ID()[:8], "home="+r.homeKey.ID()[:8]
	g := node + " g=" + testGroup1
	r.seedGroupAndMembers()
	r.waitUntil("home sync stopped", func() bool { return d.count(d.is("bg.stop "+node+" name=home_sync")) == 1 })

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
		"bg.start " + node + " name=home_sync",
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
	if d.count(d.is("bg.start "+node+" name=deliver g="+testGroup1)) == 0 {
		t.Fatal("no bg.start of a delivery")
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
	if !slices.Equal(d.lines, want) || Listeners["diag"] == nil {
		t.Fatalf("lines %q, want %q; listener %v", d.lines, want, Listeners["diag"] != nil)
	}
}

func TestDiagExternalRunLines(t *testing.T) {
	d := captureDiag(t)
	r := newExternalRig(t, false)
	r.pushExternalItem(proto.Item{Item: 5, Why: "mention", Msg: testMsg1})
	got, ok, err := r.h.ExternalTake(testGroup1, testAI1)
	if !ok || err != nil {
		t.Fatalf("take = %v %v", ok, err)
	}
	r.h.ExternalDone(testGroup1, testAI1, got.Run, "")
	run := fmt.Sprintf("node=%s g=%s ai=%s run=%d", r.hostKey.ID()[:8], testGroup1, testAI1, got.Run)
	d.lineOnce(t, "run.open "+run)
	d.lineOnce(t, "run.close "+run+" why=done")
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
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		d := captureDiag(t)
		h, _, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.ignoreInterrupt = true
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))
		mustLimTurn(t, h, key, Turn{Kind: ItemsTurn, Thread: runTask1})
		synctest.Wait()
		interruptSessionTurn(h, key, "stop", "")
		time.Sleep(stopSeconds * time.Second)
		synctest.Wait()
		s := fmt.Sprintf("node=%s g=%s ai=%s task=%s", h.Key.ID()[:8], runGroup1, runAI1, runTask1)
		d.lineOnce(t, "proc.start "+s+" agent=claude resume=false")
		d.lineOnce(t, "turn.interrupt "+s+" why=stop")
		d.lineOnce(t, "proc.exit "+s+" agent=claude why=stop")
	})
}
