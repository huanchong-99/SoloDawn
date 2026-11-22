package host

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type limDriver struct {
	mu              sync.Mutex
	procs           []*limProc
	autoEnd         bool
	endOnInterrupt  bool
	ignoreClose     bool
	ignoreInterrupt bool
}

func (d *limDriver) Start(ctx context.Context, l Launch) (Process, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := &limProc{
		ctx:             ctx,
		launch:          l,
		events:          make(chan Event, 32),
		autoEnd:         d.autoEnd,
		endOnInterrupt:  d.endOnInterrupt,
		ignoreClose:     d.ignoreClose,
		ignoreInterrupt: d.ignoreInterrupt,
		exited:          make(chan struct{}),
	}
	go p.watchCtx()
	d.procs = append(d.procs, p)
	return p, nil
}

func (d *limDriver) Interactive(l Launch) ([]string, string, error) {
	return []string{"lim", l.Session}, l.Session, nil
}

func (d *limDriver) proc(i int) *limProc {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.procs[i]
}

func (d *limDriver) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.procs)
}

type limProc struct {
	ctx             context.Context
	launch          Launch
	mu              sync.Mutex
	events          chan Event
	closed          bool
	autoEnd         bool
	endOnInterrupt  bool
	ignoreClose     bool
	ignoreInterrupt bool
	interrupted     atomic.Int32
	exited          chan struct{}
}

func (p *limProc) watchCtx() {
	select {
	case <-p.ctx.Done():
		p.finish()
	case <-p.exited:
	}
}

func (p *limProc) Events() <-chan Event { return p.events }

func (p *limProc) Turn(string) error {
	p.emit(Event{Kind: Ready, Session: "lim-sess"})
	if p.autoEnd {
		p.emit(Event{Kind: TurnEnded, Status: "completed"})
	}
	return nil
}

func (p *limProc) Interrupt() error {
	p.interrupted.Add(1)
	if p.endOnInterrupt && !p.ignoreInterrupt {
		p.emit(Event{Kind: TurnEnded, Status: "interrupted"})
	}
	return nil
}

func (p *limProc) Answer(string, bool) error { return nil }

func (p *limProc) Close() error {
	if !p.ignoreClose {
		p.finish()
	}
	return nil
}

func (p *limProc) emit(e Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.events <- e
	}
}

func (p *limProc) finish() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.events)
		close(p.exited)
	}
	p.mu.Unlock()
}

func setupLimEnv(t *testing.T) (*Host, *runHome, *limDriver, *store.Store) {
	t.Helper()
	stopAllTestHosts()
	saveRuntimeGlobals(t)
	drv := &limDriver{}
	Drivers = map[string]Driver{"claude": drv, "codex": drv}
	installDefaultHooks()
	db, _ := openFileStore(t, t.TempDir())
	t.Cleanup(func() { db.Close() })
	h := newRunHost(t, db)
	t.Cleanup(func() { h.Settle() })
	h.Shim = []string{"solodawn", "mcp"}
	rh := newRunHome("local")
	h.Attach("home-dev", rh)
	return h, rh, drv, db
}

func setupLimBubble(t *testing.T, baseDir string) (*Host, *runHome, *limDriver, func()) {
	t.Helper()
	drv := &limDriver{}
	Drivers = map[string]Driver{"claude": drv, "codex": drv}
	installDefaultHooks()
	db, err := store.Open(filepath.Join(baseDir, "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := New(db, seamKey(), time.Now)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	h.Shim = []string{"solodawn", "mcp"}
	rh := newRunHome("local")
	h.Attach("home-dev", rh)
	shutdown := func() {
		h.Settle()
		stopHostNow(h)
		synctest.Wait()
		db.Close()
	}
	return h, rh, drv, shutdown
}

func limHostCmdJSON(h *Host, kind string) json.RawMessage {
	cmd := proto.HostCommand{
		Type:      kind,
		Nonce:     proto.NewNonce(),
		Timestamp: h.Now().UnixMilli(),
		To:        h.Key.ID(),
	}
	return json.RawMessage(marshalWire(cmd))
}

func putLimCaps(t *testing.T, db *store.Store, agent string, caps proto.Caps) {
	t.Helper()
	raw := marshalWire(caps)
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO agent(agent, cli_version, account, caps) VALUES(?, '1.0.0', 'personal', ?)", agent, raw)
		return err
	}
	if err := db.Write(write); err != nil {
		t.Fatal(err)
	}
}

func putLimGroupRow(h *Host, group string, turnMinutes int, paused *proto.GroupPause) {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group, Seq: 1},
		Name:      "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: "home-dev", Secretary: runAI1, Checks: []string{},
		Limits: proto.Limits{TurnMinutes: turnMinutes},
		Paused: paused,
	}
	h.Rows.Put(group, "group", group, json.RawMessage(marshalWire(grp)))
}

func putLimLocalAI(h *Host, group, member, cli string) {
	row := proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: member, Seq: 2},
			Group:     group, Kind: "ai", User: "u", Name: "AI",
		},
		CLI: cli, Host: h.Key.ID(), HostName: "host", Account: "personal",
		Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local",
	}
	h.Rows.Put(group, "member", member, json.RawMessage(marshalWire(row)))
}

func mustLimTurn(t *testing.T, h *Host, key SessionKey, text string, turn Turn) {
	t.Helper()
	if err := Runner.Turn(t.Context(), h, key, text, turn); err != nil {
		t.Fatal(err)
	}
}

func mustLimSurface(t *testing.T, h *Host, kind string) {
	t.Helper()
	if _, err := Surface[kind](t.Context(), h, nil, "owner", limHostCmdJSON(h, kind)); err != nil {
		t.Fatal(err)
	}
}

func TestHostOfflineAndOnlineLifecycleAndPersistence(t *testing.T) {
	h, rh, drv, db := setupLimEnv(t)
	delivered := make(chan aiSlot, 8)
	Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	grpKey := SessionKey{Group: runGroup1, Member: runAI2}
	dormKey := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(dormKey, GroupSession, "repo"))

	mustLimTurn(t, h, taskKey, "work", Turn{Kind: ItemsTurn, Thread: runTask1})
	rh.waitCmd(t)
	mustLimTurn(t, h, grpKey, "chat", Turn{Kind: ItemsTurn})
	rh.waitCmd(t)
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, grpKey, Waiting)
	rh.waitCmd(t)
	waitDeliveredSlot(t, delivered, aiSlot{runGroup1, runAI2})

	mustLimSurface(t, h, "host.offline")
	assertAllDormantAndHeldOffline(t, h, db, taskKey, grpKey, dormKey)
	assertOfflineStatuses(t, rh)

	mustLimSurface(t, h, "host.online")
	reason, held := h.Held(taskKey)
	if [2]any{reason, held} != [2]any{"", false} {
		t.Fatalf("still held after online: %q %v", reason, held)
	}
	waitDeliveredSlot(t, delivered, aiSlot{runGroup2, runAI1})
}

func assertAllDormantAndHeldOffline(t *testing.T, h *Host, db *store.Store, keys ...SessionKey) {
	t.Helper()
	for _, k := range keys {
		reason, held := h.Held(k)
		if [3]any{seamState(t, h.Sessions, k), reason, held} != [3]any{Dormant, "offline", true} {
			t.Fatalf("key %v state=%s held=(%q,%v)", k, seamState(t, h.Sessions, k), reason, held)
		}
	}
	st, task := h.MemberState(runGroup1, runAI1)
	if [2]string{st, task} != [2]string{"offline", ""} {
		t.Fatalf("MemberState = (%q, %q), want (offline, empty)", st, task)
	}
	assertOfflinePersisted(t, db, keys[0])
}

func assertOfflinePersisted(t *testing.T, db *store.Store, key SessionKey) {
	t.Helper()
	h2, _, err := New(db, seamKey(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	reason, held := h2.Held(key)
	if [2]any{reason, held} != [2]any{"offline", true} {
		t.Fatalf("offline did not persist across restart: (%q, %v)", reason, held)
	}
}

func assertOfflineStatuses(t *testing.T, rh *runHome) {
	t.Helper()
	var sawStop bool
	for range 3 {
		cmd := rh.waitCmd(t)
		if cmd.payload["state"] != "offline" {
			t.Fatalf("expected offline state in %v", cmd.payload)
		}
		ev, _ := cmd.payload["event"].(map[string]any)
		if [3]any{ev["code"], ev["reason"], ev["task"]} == [3]any{"turn_interrupted", "stop", runTask1} {
			sawStop = true
		}
	}
	if !sawStop {
		t.Fatal("missing turn_interrupted stop for active task session")
	}
}

func TestOfflineEdgeCasesValidationErrorsAndCloseWait(t *testing.T) {
	h, _, _, _ := setupLimEnv(t)

	if _, err := Surface["host.offline"](t.Context(), h, nil, "owner", json.RawMessage("{")); err == nil {
		t.Fatal("expected bad_request on invalid offline JSON")
	}
	if _, err := Surface["host.online"](t.Context(), h, nil, "owner", json.RawMessage("{")); err == nil {
		t.Fatal("expected bad_request on invalid online JSON")
	}

	delivered := make(chan aiSlot, 4)
	Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }
	if _, err := Surface["host.online"](t.Context(), h, nil, "owner", limHostCmdJSON(h, "host.online")); err != nil {
		t.Fatal(err)
	}
	waitDeliveredSlot(t, delivered, aiSlot{"", ""})

	Deliver = nil
	if _, err := Surface["host.online"](t.Context(), h, nil, "owner", limHostCmdJSON(h, "host.online")); err != nil {
		t.Fatal(err)
	}

	verifyOfflineCloseBranches(t, h)
	verifyOfflineStoreWriteError(t)
}

func verifyOfflineCloseBranches(t *testing.T, h *Host) {
	t.Helper()
	waitKey := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(waitKey, GroupSession, "repo"))
	h.Sessions.Move(waitKey, Dormant, Working)
	h.Sessions.Move(waitKey, Working, Waiting)

	rt := stateOf(h)
	rt.mu.Lock()
	rt.closing[waitKey] = true
	rt.mu.Unlock()
	go func() {
		time.Sleep(5 * time.Millisecond)
		rt.mu.Lock()
		delete(rt.closing, waitKey)
		rt.notifyLocked()
		rt.mu.Unlock()
	}()
	task, wasTurn := closeSessionForOffline(h, waitKey)
	if wasTurn || task != "" || seamState(t, h.Sessions, waitKey) != Dormant {
		t.Fatalf("unexpected closeSessionForOffline result: task=%q wasTurn=%v state=%s", task, wasTurn, seamState(t, h.Sessions, waitKey))
	}
	if moveSessionDormant(h, SessionKey{Group: "missing", Member: "missing"}) {
		t.Fatal("moveSessionDormant succeeded on missing session")
	}
}

func verifyOfflineStoreWriteError(t *testing.T) {
	t.Helper()
	h, _, _, db := setupLimEnv(t)
	drop := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE setting")
		return err
	}
	if err := db.Write(drop); err != nil {
		t.Fatal(err)
	}
	if _, err := Surface["host.offline"](t.Context(), h, nil, "owner", limHostCmdJSON(h, "host.offline")); err == nil {
		t.Fatal("expected store error on offline when setting table dropped")
	}
	if _, err := Surface["host.online"](t.Context(), h, nil, "owner", limHostCmdJSON(h, "host.online")); err == nil {
		t.Fatal("expected store error on online when setting table dropped")
	}
}

func TestOfflineUnresponsiveWaitingProcWritesKilledAudit(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.autoEnd = true
		drv.ignoreClose = true

		grpKey := SessionKey{Group: runGroup1, Member: runAI1}
		h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
		if err := Runner.Turn(t.Context(), h, grpKey, "hi", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		done := make(chan error, 1)
		go func() {
			_, err := Surface["host.offline"](t.Context(), h, nil, "owner", limHostCmdJSON(h, "host.offline"))
			done <- err
		}()
		synctest.Wait()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(seamRecord(t, h), "|claude|killed|") {
			t.Fatalf("expected killed audit row in %q", seamRecord(t, h))
		}
	})
}

func TestTurnMinutesTimeoutInterruptAndUnresponsiveKill(t *testing.T) {
	saveRuntimeGlobals(t)
	base1 := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		runResponsiveTurnTimeoutBubble(t, base1)
	})
	base2 := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		runUnresponsiveTurnTimeoutBubble(t, base2)
	})
}

func runResponsiveTurnTimeoutBubble(t *testing.T, base string) {
	t.Helper()
	h, rh, drv, shutdown := setupLimBubble(t, base)
	defer shutdown()
	drv.endOnInterrupt = true
	putLimGroupRow(h, runGroup1, 1, nil)

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	grpKey := SessionKey{Group: runGroup1, Member: runAI2}
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))

	if err := Runner.Turn(t.Context(), h, taskKey, "slow", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	if err := Runner.Turn(t.Context(), h, grpKey, "chat", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	synctest.Wait()

	time.Sleep(time.Minute - time.Nanosecond)
	synctest.Wait()
	if drv.proc(0).interrupted.Load() != 0 {
		t.Fatal("interrupted before 1 minute")
	}
	time.Sleep(time.Nanosecond)
	synctest.Wait()
	if drv.proc(0).interrupted.Load() != 1 || drv.proc(1).interrupted.Load() != 1 {
		t.Fatal("expected both turns interrupted at 1 minute")
	}
	assertTimeoutCommands(t, rh, true)
}

func runUnresponsiveTurnTimeoutBubble(t *testing.T, base string) {
	t.Helper()
	h, rh, drv, shutdown := setupLimBubble(t, base)
	defer shutdown()
	drv.ignoreInterrupt = true
	putLimGroupRow(h, runGroup1, 1, nil)

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))
	if err := Runner.Turn(t.Context(), h, taskKey, "hung", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	synctest.Wait()

	time.Sleep(time.Minute)
	synctest.Wait()
	interruptSessionTurn(h, taskKey, "stop", "ignored duplicate")
	interruptSessionTurn(h, SessionKey{Group: "none", Member: "none"}, "stop", "missing")

	time.Sleep(10 * time.Second)
	synctest.Wait()
	if seamState(t, h.Sessions, taskKey) != Dormant {
		t.Fatalf("state = %s, want dormant after 10s kill", seamState(t, h.Sessions, taskKey))
	}
	assertTimeoutCommands(t, rh, true)
}

func assertTimeoutCommands(t *testing.T, rh *runHome, wantTaskFail bool) {
	t.Helper()
	var sawIntr, sawFail bool
	for _, c := range rh.snapshotCmds() {
		ev, _ := c.payload["event"].(map[string]any)
		if [3]any{ev["code"], ev["reason"], ev["detail"]} == [3]any{"turn_interrupted", "timeout", "Turn time limit reached."} {
			sawIntr = true
		}
		if [3]any{c.payload["t"], c.payload["op"], c.payload["reason"]} == [3]any{"task", "fail", "timeout"} {
			sawFail = true
		}
	}
	if [2]bool{sawIntr, !wantTaskFail || sawFail} != [2]bool{true, true} {
		t.Fatalf("missing timeout commands: intr=%v fail=%v cmds=%+v", sawIntr, sawFail, rh.snapshotCmds())
	}
}

func TestStopInterruptReportingAndCodexSilentWatch(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.endOnInterrupt = true

		verifyManualStopInterrupt(t, h, rh)
		verifyCodexSilentTimer(t, h, rh, drv)
	})
}

func verifyManualStopInterrupt(t *testing.T, h *Host, rh *runHome) {
	t.Helper()
	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
	mustLimTurn(t, h, grpKey, "stop-me", Turn{Kind: ItemsTurn})
	synctest.Wait()
	interruptSessionTurn(h, grpKey, "stop", "Paused.")
	synctest.Wait()

	for _, c := range rh.snapshotCmds() {
		ev, _ := c.payload["event"].(map[string]any)
		if [3]any{ev["code"], ev["reason"], ev["detail"]} == [3]any{"turn_interrupted", "stop", "Paused."} {
			return
		}
	}
	t.Fatalf("missing stop interrupt status in %+v", rh.snapshotCmds())
}

func verifyCodexSilentTimer(t *testing.T, h *Host, rh *runHome, drv *limDriver) {
	t.Helper()
	cdxKey := SessionKey{Group: runGroup1, Member: runAI2}
	sess := makeRunSession(cdxKey, GroupSession, "repo")
	sess.Agent = "codex"
	h.Sessions.Create(sess)
	mustLimTurn(t, h, cdxKey, "codex-turn", Turn{Kind: ItemsTurn})
	p := drv.proc(1)
	h.Report(cdxKey, Event{Kind: TurnBegan})
	h.Report(SessionKey{Group: "missing", Member: "missing"}, Event{Kind: Active})
	synctest.Wait()

	advanceCodexWithActivityAndOpenCall(t, h, cdxKey, p)
	if p.interrupted.Load() != 0 {
		t.Fatal("codex interrupted while active or tool call open")
	}

	time.Sleep(5 * time.Minute)
	synctest.Wait()
	if p.interrupted.Load() != 1 {
		t.Fatalf("codex silent interrupt count = %d, want 1", p.interrupted.Load())
	}
	assertSilentDetail(t, rh)
}

func advanceCodexWithActivityAndOpenCall(t *testing.T, h *Host, key SessionKey, p *limProc) {
	t.Helper()
	pct := 25.0
	resets := time.Now().Add(time.Hour).UnixMilli()
	time.Sleep(3 * time.Minute)
	p.emit(Event{Kind: Active})
	synctest.Wait()
	time.Sleep(3 * time.Minute)
	onTurnActivity(limitsOf(h), Session{SessionKey: key, Agent: "codex"}, Event{Kind: Asked})
	p.emit(Event{Kind: Used, Usage: Usage{WindowPct: &pct, WindowResets: &resets}})
	synctest.Wait()

	doneCall := h.Sessions.Calling(key)
	time.Sleep(5 * time.Minute)
	synctest.Wait()
	doneCall()
}

func assertSilentDetail(t *testing.T, rh *runHome) {
	t.Helper()
	for _, c := range rh.snapshotCmds() {
		ev, _ := c.payload["event"].(map[string]any)
		if ev["detail"] == "Turn was silent for 5 minutes." {
			return
		}
	}
	t.Fatalf("missing silent timeout detail in %+v", rh.snapshotCmds())
}

func TestStaleTurnGenIgnoredAndSettleStopsAllTimers(t *testing.T) {
	h, _, drv, db := setupLimEnv(t)
	putLimCaps(t, db, "claude", proto.Caps{Day: &proto.Cap{Unit: "turns", Max: 1}})

	claudeKey := SessionKey{Group: runGroup1, Member: runAI1}
	codexKey := SessionKey{Group: runGroup1, Member: runAI2}
	h.Sessions.Create(makeRunSession(claudeKey, GroupSession, "repo"))
	cdx := makeRunSession(codexKey, GroupSession, "repo")
	cdx.Agent = "codex"
	h.Sessions.Create(cdx)

	mustLimTurn(t, h, claudeKey, "t1", Turn{Kind: ItemsTurn})
	resets := h.Now().Add(time.Hour).UnixMilli()
	drv.proc(0).emit(Event{Kind: RateLimited, Resets: resets})
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, claudeKey, Waiting)

	armDayCapTimer(h, limitsOf(h), "claude", h.Now())
	armDayCapTimer(h, limitsOf(h), "claude", h.Now())

	mustLimTurn(t, h, claudeKey, "t2", Turn{Kind: ItemsTurn})
	mustLimTurn(t, h, codexKey, "c1", Turn{Kind: ItemsTurn})

	lp := limitsOf(h)
	fireTurnDeadline(h, lp, claudeKey, 999999)
	fireCodexSilent(h, lp, codexKey, 999999)
	h.Settle()

	lp.mu.Lock()
	rem := len(lp.rateTimers) + len(lp.dayTimers) + len(lp.turns)
	lp.mu.Unlock()
	if rem != 0 {
		t.Fatalf("Settle left %d active timers", rem)
	}
}

func TestRateLimitedInTurnOutOfTurnBoundaryAndRelease(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		delivered := make(chan aiSlot, 8)
		Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }

		putLimCaps(t, h.Store, "claude", proto.Caps{Day: &proto.Cap{Unit: "turns", Max: 1}})
		key := SessionKey{Group: runGroup1, Member: runAI1}
		h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
		mustLimTurn(t, h, key, "t1", Turn{Kind: ItemsTurn})
		synctest.Wait()

		nowMS := time.Now().UnixMilli()
		p := drv.proc(0)
		p.emit(Event{Kind: RateLimited, Resets: nowMS - 100})
		p.emit(Event{Kind: RateLimited, Resets: nowMS + 20_000})
		p.emit(Event{Kind: RateLimited, Resets: nowMS + 30_000})
		p.emit(Event{Kind: TurnEnded, Status: "completed"})
		synctest.Wait()

		assertRateHoldAndStatus(t, h, rh, key)
		verifyRateReleaseAndOutOfTurnEvent(t, h, rh, p, key, delivered)
	})
}

func assertRateHoldAndStatus(t *testing.T, h *Host, rh *runHome, key SessionKey) {
	t.Helper()
	reason, held := h.Held(key)
	st, _ := h.MemberState(key.Group, key.Member)
	if [3]any{reason, held, st} != [3]any{"rate_limit", true, "limited"} {
		t.Fatalf("Held=(%q,%v) MemberState=%q", reason, held, st)
	}
	for _, c := range rh.snapshotCmds() {
		ev, _ := c.payload["event"].(map[string]any)
		if ev["code"] == "rate_limited" {
			return
		}
	}
	t.Fatalf("expected rate_limited event on turn end: %+v", rh.snapshotCmds())
}

func verifyRateReleaseAndOutOfTurnEvent(t *testing.T, h *Host, rh *runHome, p *limProc, key SessionKey, delivered <-chan aiSlot) {
	t.Helper()
	lp := limitsOf(h)
	exactUntil := time.Now().UnixMilli() + 30_000
	if [2]bool{lp.isRateLimited("claude", exactUntil-1), lp.isRateLimited("claude", exactUntil)} != [2]bool{true, false} {
		t.Fatal("isRateLimited boundary at exact reset timestamp failed")
	}

	time.Sleep(30 * time.Second)
	synctest.Wait()
	drainDelivered(delivered)

	p.emit(Event{Kind: RateLimited, Resets: time.Now().UnixMilli()})
	synctest.Wait()
	if len(delivered) != 0 {
		t.Fatal("equal-timestamp RateLimited scheduled a zero-delay release")
	}

	before := len(rh.snapshotCmds())
	p.emit(Event{Kind: RateLimited, Resets: time.Now().Add(5 * time.Second).UnixMilli()})
	synctest.Wait()
	if len(rh.snapshotCmds()) <= before {
		t.Fatal("out-of-turn RateLimited did not send member status")
	}
}

func drainDelivered(ch <-chan aiSlot) {
	for len(ch) > 0 {
		<-ch
	}
}

func TestDayCapMidnightReleaseAndTaskCapMultiDay(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.autoEnd = true
		putLimCaps(t, h.Store, "claude", proto.Caps{
			Day:  &proto.Cap{Unit: "turns", Max: 2},
			Task: &proto.Cap{Unit: "turns", Max: 2},
		})
		seedPriorDayTaskUsage(t, h.Store)

		grpKey := SessionKey{Group: runGroup1, Member: runAI1}
		taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
		h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))

		runTaskCapThenDayCapTurns(t, h, grpKey, taskKey)
		assertCapHoldsBeforeMidnight(t, h, rh, grpKey, taskKey)

		now := time.Now()
		time.Sleep(nextLocalMidnight(now).Sub(now))
		synctest.Wait()
		assertCapHoldsAfterMidnight(t, h, grpKey, taskKey)
	})
}

func assertCapHoldsAfterMidnight(t *testing.T, h *Host, grpKey, taskKey SessionKey) {
	t.Helper()
	_, grpHeld := h.Held(grpKey)
	tReason, tHeld := h.Held(taskKey)
	st, _ := h.MemberState(runGroup1, runAI1)
	if [4]any{grpHeld, tReason, tHeld, st} != [4]any{false, "cap", true, "limited"} {
		t.Fatalf("after midnight: grpHeld=%v task=(%q,%v) st=%q", grpHeld, tReason, tHeld, st)
	}
}

func seedPriorDayTaskUsage(t *testing.T, db *store.Store) {
	t.Helper()
	prior := storedDayUsage{
		usageTotals: usageTotals{Turns: 1, Tokens: 100, HasTokens: true, USD: 0.25, HasUSD: true},
		Tasks: map[string]usageTotals{
			runTask1: {Turns: 1, Tokens: 100, HasTokens: true, USD: 0.25, HasUSD: true},
		},
	}
	execSQL(t, db, "INSERT INTO usage(agent, day, data) VALUES('claude', '1999-12-31', ?)", marshalWire(prior))
}

func runTaskCapThenDayCapTurns(t *testing.T, h *Host, grpKey, taskKey SessionKey) {
	t.Helper()
	mustLimTurn(t, h, taskKey, "task-turn-1", Turn{Kind: ItemsTurn, Thread: runTask1})
	synctest.Wait()
	if _, held := h.Held(grpKey); held {
		t.Fatal("group key held when only task cap was reached")
	}
	mustLimTurn(t, h, taskKey, "task-turn-already-capped", Turn{Kind: ItemsTurn, Thread: runTask1})
	synctest.Wait()
	mustLimTurn(t, h, grpKey, "extra-after-day-cap", Turn{Kind: ItemsTurn})
	synctest.Wait()
}

func assertCapHoldsBeforeMidnight(t *testing.T, h *Host, rh *runHome, grpKey, taskKey SessionKey) {
	t.Helper()
	gReason, gHeld := h.Held(grpKey)
	tReason, tHeld := h.Held(taskKey)
	if [4]any{gReason, gHeld, tReason, tHeld} != [4]any{"cap", true, "cap", true} {
		t.Fatalf("holds before midnight: grp=(%q,%v) task=(%q,%v)", gReason, gHeld, tReason, tHeld)
	}
	assertCapEventCounts(t, rh.snapshotCmds(), 1, 1)
}

func assertCapEventCounts(t *testing.T, cmds []recordedCmd, wantTask, wantDay int) {
	t.Helper()
	var taskCapCount, dayCapCount int
	for _, c := range cmds {
		ev, _ := c.payload["event"].(map[string]any)
		if [3]any{ev["code"], ev["cap"], ev["task"]} == [3]any{"cap_reached", "task", runTask1} {
			taskCapCount++
		}
		if [2]any{ev["code"], ev["cap"]} == [2]any{"cap_reached", "day"} {
			dayCapCount++
		}
	}
	if [2]int{taskCapCount, dayCapCount} != [2]int{wantTask, wantDay} {
		t.Fatalf("cap events: task=%d day=%d, want %d and %d", taskCapCount, dayCapCount, wantTask, wantDay)
	}
}

func TestCapUnitsMemberUsageOverlaysAndMemberAgentFallbacks(t *testing.T) {
	h, _, _, db := setupLimEnv(t)

	if u := memberUsage(h, "unknown-g", "unknown-m"); u != (proto.Usage{}) {
		t.Fatalf("unknown memberUsage = %+v, want zero", u)
	}
	reason, held := h.Held(SessionKey{Group: "unknown-g", Member: "unknown-m"})
	if [2]any{reason, held} != [2]any{"", false} {
		t.Fatalf("unknown Held = (%q, %v)", reason, held)
	}

	putLimLocalAI(h, runGroup1, runAI1, "claude")
	if got := memberUsage(h, runGroup1, runAI1); got != (proto.Usage{}) {
		t.Fatalf("empty memberUsage = %+v, want zero", got)
	}
	if _, held := h.Held(SessionKey{Group: runGroup1, Member: runAI1}); held {
		t.Fatal("held with empty caps")
	}

	verifyWindowAndCapUnits(t, h, db)
	verifyTokensAndUSDUsage(t, h, db)
	verifyOverlayStatesAndTaskFallback(t, h, db)
	verifyCorruptAndMissingTables(t)
}

func verifyWindowAndCapUnits(t *testing.T, h *Host, db *store.Store) {
	t.Helper()
	lp := limitsOf(h)
	resets := int64(1763609999000)
	lp.recordWindow("claude", Usage{WindowResets: &resets})
	got := memberUsage(h, runGroup1, runAI1)
	if got.WindowResets == nil || *got.WindowResets != resets {
		t.Fatalf("window resets only usage = %+v", got)
	}

	putLimCaps(t, db, "claude", proto.Caps{Day: &proto.Cap{Unit: "window_pct", Max: 80}})
	if agentReachedCap(h, lp, "claude", "") {
		t.Fatal("window_pct cap met before WindowPct reported")
	}
	pct := 80.0
	lp.recordWindow("claude", Usage{WindowPct: &pct})
	if !agentReachedCap(h, lp, "claude", "") {
		t.Fatal("window_pct cap not met at exact max 80")
	}
	checkCapMetBoundaries(t)
}

func verifyTokensAndUSDUsage(t *testing.T, h *Host, db *store.Store) {
	t.Helper()
	tokens := int64(500)
	usd := 0.0
	pct := 80.0
	today := h.Now().Format(time.DateOnly)
	putLimCaps(t, db, "claude", proto.Caps{Day: &proto.Cap{Unit: "tokens", Max: 500}})
	if hit := recordTurnUsage(h, "claude", today, "", Usage{Tokens: &tokens, USD: &usd}, &pct); hit != "day" {
		t.Fatalf("tokens cap hit = %q, want day", hit)
	}

	putLimCaps(t, db, "claude", proto.Caps{Day: &proto.Cap{Unit: "usd", Max: 2.0}})
	u := memberUsage(h, runGroup1, runAI1)
	if u.SpendToday == nil || u.SpendCap == nil || u.DayCapPct == nil {
		t.Fatalf("missing usd pointers in %+v", u)
	}
	if [3]float64{*u.SpendToday, *u.SpendCap, *u.DayCapPct} != [3]float64{0, 2.0, 0} {
		t.Fatalf("usd usage = %+v", u)
	}
}

func checkCapMetBoundaries(t *testing.T) {
	t.Helper()
	tot := usageTotals{Turns: 0, USD: 1.0, HasUSD: true}
	if isCapMet(nil, tot, nil) || isCapMet(&proto.Cap{Unit: "turns", Max: 0}, tot, nil) {
		t.Fatal("nil or zero-max cap reported met")
	}
	if isCapMet(&proto.Cap{Unit: "bogus", Max: 1}, usageTotals{Turns: 5}, nil) {
		t.Fatal("unknown unit reported met")
	}
	uZeroTurns := buildProtoUsage(&proto.Cap{Unit: "turns", Max: 10}, tot, true, windowState{})
	uZeroMax := buildProtoUsage(&proto.Cap{Unit: "turns", Max: 0}, usageTotals{Turns: 1}, true, windowState{})
	if [3]bool{uZeroTurns.TurnsToday == nil, uZeroTurns.DayCapPct == nil, uZeroMax.DayCapPct == nil} != [3]bool{true, true, true} {
		t.Fatalf("zero boundary usage mismatch: zeroTurns=%+v zeroMax=%+v", uZeroTurns, uZeroMax)
	}
}

func verifyOverlayStatesAndTaskFallback(t *testing.T, h *Host, db *store.Store) {
	t.Helper()
	putLimCaps(t, db, "claude", proto.Caps{Day: &proto.Cap{Unit: "turns", Max: 1}})
	grpKey := SessionKey{Group: runGroup2, Member: runAI2}
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
	unmadeTaskKey := SessionKey{Group: runGroup2, Member: runAI2, Task: runTask2}
	reason, held := h.Held(unmadeTaskKey)
	if [2]any{reason, held} != [2]any{"cap", true} {
		t.Fatalf("task fallback to group session failed: (%q, %v)", reason, held)
	}

	putLimCaps(t, db, "claude", proto.Caps{})
	paused := proto.GroupPause{By: "owner", At: 1}
	putLimGroupRow(h, runGroup2, 0, &paused)
	if st, _ := h.MemberState(runGroup2, runAI2); st != "paused" {
		t.Fatalf("group paused MemberState = %q, want paused", st)
	}

	Holds["custom_pause"] = func(*Host, SessionKey) (string, bool) { return "paused", true }
	defer delete(Holds, "custom_pause")
	if st, _ := h.MemberState(runGroup2, runAI2); st != "paused" {
		t.Fatalf("hold paused MemberState = %q, want paused", st)
	}
}

func verifyCorruptAndMissingTables(t *testing.T) {
	t.Helper()
	h, _, _, db := setupLimEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
	today := h.Now().Format(time.DateOnly)

	execSQL(t, db, "INSERT INTO agent(agent, cli_version, account, caps) VALUES('claude', '1.0.0', 'personal', '{bad')")
	assertDBErrorClean(t, h, today)

	execSQL(t, db, "UPDATE agent SET caps = '{\"day\":{\"unit\":\"turns\",\"max\":1}}' WHERE agent = 'claude'")
	execSQL(t, db, "INSERT INTO usage(agent, day, data) VALUES('claude', ?, '{bad')", today)
	assertDBErrorClean(t, h, today)

	execSQL(t, db, "DROP TABLE usage")
	if agentReachedCap(h, limitsOf(h), "claude", "") {
		t.Fatal("dropped usage table reported cap met")
	}
	execSQL(t, db, "DROP TABLE agent")
	if agentReachedCap(h, limitsOf(h), "claude", "") {
		t.Fatal("dropped agent table reported cap met")
	}
}

func assertDBErrorClean(t *testing.T, h *Host, today string) {
	t.Helper()
	got := [3]any{
		recordTurnUsage(h, "claude", today, "", Usage{}, nil),
		agentReachedCap(h, limitsOf(h), "claude", ""),
		memberUsage(h, runGroup1, runAI1),
	}
	if got != [3]any{"", false, proto.Usage{}} {
		t.Fatalf("expected clean zero fallbacks on DB error, got %+v", got)
	}
}

func execSQL(t *testing.T, db *store.Store, sqlText string, args ...any) {
	t.Helper()
	write := func(tx *store.Tx) error {
		_, err := tx.Exec(sqlText, args...)
		return err
	}
	if err := db.Write(write); err != nil {
		t.Fatal(err)
	}
}

func TestGroupPauseAndTaskPauseInterruptRunningTurns(t *testing.T) {
	h, rh, drv, _ := setupLimEnv(t)
	drv.endOnInterrupt = true
	d := deliv(h)
	d.mu.Lock()
	d.groupLocked(runGroup1, "home-dev").live = true
	d.mu.Unlock()

	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	taskKey := SessionKey{Group: runGroup1, Member: runAI2, Task: runTask1}
	otherKey := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))
	h.Sessions.Create(makeRunSession(otherKey, GroupSession, "repo"))

	mustLimTurn(t, h, grpKey, "g-work", Turn{Kind: ItemsTurn})
	mustLimTurn(t, h, taskKey, "t-work", Turn{Kind: ItemsTurn, Thread: runTask1})
	rh.waitCmd(t)
	rh.waitCmd(t)

	verifyTaskPauseInterruptsOnlyPausedTask(t, h, rh, drv, grpKey, taskKey)
	verifyGroupPauseInterruptsRemainingTurn(t, h, rh, drv, grpKey)
}

func pushLimRows(h *Host, group string, rows ...any) {
	raws := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		raws[i] = json.RawMessage(marshalWire(r))
	}
	frame := proto.Rows{Type: "rows", Group: group, Rows: raws}
	h.Push("home-dev", json.RawMessage(marshalWire(frame)))
}

func verifyTaskPauseInterruptsOnlyPausedTask(t *testing.T, h *Host, rh *runHome, drv *limDriver, grpKey, taskKey SessionKey) {
	t.Helper()
	activeTask := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: runTask1, Seq: 1},
		Group:     runGroup1, Num: 1, Title: "T", Slug: "t", Origin: "brief",
		State: "active", Step: "branch", Assignee: runAI2, Depends: []string{},
	}
	pushLimRows(h, runGroup1, activeTask)
	time.Sleep(15 * time.Millisecond)
	if drv.proc(0).interrupted.Load() != 0 || drv.proc(1).interrupted.Load() != 0 {
		t.Fatal("active task row update interrupted running turn")
	}

	pausedTask := activeTask
	pausedTask.Seq = 2
	pausedTask.State = "paused"
	pushLimRows(h, runGroup1, pausedTask)
	waitForState(t, h, taskKey, Waiting)
	cmd := rh.waitCmd(t)
	ev, _ := cmd.payload["event"].(map[string]any)
	if [3]any{ev["code"], ev["reason"], ev["task"]} != [3]any{"turn_interrupted", "stop", runTask1} {
		t.Fatalf("unexpected task pause status: %+v", cmd.payload)
	}
	if seamState(t, h.Sessions, grpKey) != Working {
		t.Fatal("group session interrupted by unrelated task pause")
	}
}

func verifyGroupPauseInterruptsRemainingTurn(t *testing.T, h *Host, rh *runHome, drv *limDriver, grpKey SessionKey) {
	t.Helper()
	pause := proto.GroupPause{By: "owner", At: 1}
	pausedGrp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 3},
		Name:      "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: "home-dev", Secretary: runAI1, Checks: []string{},
		Paused: &pause,
	}
	pushLimRows(h, runGroup1, pausedGrp)
	waitForState(t, h, grpKey, Waiting)
	cmd := rh.waitCmd(t)
	ev, _ := cmd.payload["event"].(map[string]any)
	if [3]any{cmd.payload["state"], ev["code"], ev["reason"]} != [3]any{"paused", "turn_interrupted", "stop"} {
		t.Fatalf("unexpected group pause status: %+v", cmd.payload)
	}
	if drv.proc(0).interrupted.Load() != 1 {
		t.Fatalf("group proc interrupt count = %d, want 1", drv.proc(0).interrupted.Load())
	}
}
