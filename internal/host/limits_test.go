package host

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"slices"
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
	endOnInterrupt  bool
	ignoreInterrupt bool
}

func (d *limDriver) Start(ctx context.Context, l Launch) (Process, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := &limProc{
		ctx:            ctx,
		events:         make(chan Event, 32),
		endOnInterrupt: d.endOnInterrupt && !d.ignoreInterrupt,
		exited:         make(chan struct{}),
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

type limProc struct {
	ctx            context.Context
	mu             sync.Mutex
	events         chan Event
	closed         bool
	endOnInterrupt bool
	interrupted    atomic.Int32
	exited         chan struct{}
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
	return nil
}

func (p *limProc) Interrupt() error {
	p.interrupted.Add(1)
	if p.endOnInterrupt {
		p.emit(Event{Kind: TurnEnded, Status: "interrupted"})
	}
	return nil
}

func (p *limProc) Answer(string, bool) error { return nil }

func (p *limProc) Close() error {
	p.finish()
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
	t.Cleanup(h.Settle)
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
		t.Fatal(err)
	}
	rh := newRunHome("local")
	h.Attach("home-dev", rh)
	shutdown := func() {
		h.Settle()
		synctest.Wait()
		db.Close()
	}
	return h, rh, drv, shutdown
}

func limHostCmdJSON(h *Host, kind string) json.RawMessage {
	cmd := proto.HostCommand{Type: kind, Nonce: proto.NewNonce(), Timestamp: h.Now().UnixMilli(), To: h.Key.ID()}
	return json.RawMessage(marshalWire(cmd))
}

func limSurface(t *testing.T, h *Host, kind string) error {
	t.Helper()
	_, err := Surface[kind](t.Context(), h, nil, "owner", limHostCmdJSON(h, kind))
	return err
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

func mustLimTurn(t *testing.T, h *Host, key SessionKey, turn Turn) {
	t.Helper()
	if err := Runner.Turn(t.Context(), h, key, "turn", turn); err != nil {
		t.Fatal(err)
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

func eventOf(c recordedCmd) [3]any {
	ev, _ := c.payload["event"].(map[string]any)
	return [3]any{ev["code"], ev["reason"], ev["task"]}
}

func countCmds(cmds []recordedCmd, match func(recordedCmd) bool) int {
	n := 0
	for _, c := range cmds {
		if match(c) {
			n++
		}
	}
	return n
}

func isTaskFail(c recordedCmd) bool {
	return [2]any{c.payload["t"], c.payload["op"]} == [2]any{"task", "fail"}
}

func TestOfflineClosesEverySessionAndOnlineReleasesTheHold(t *testing.T) {
	h, rh, drv, db := setupLimEnv(t)
	delivered := make(chan aiSlot, 8)
	Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	grpKey := SessionKey{Group: runGroup1, Member: runAI2}
	dormKey := SessionKey{Group: runGroup2, Member: runAI1}
	for _, s := range []Session{makeRunSession(taskKey, TaskSession, "wt/1"), makeRunSession(grpKey, GroupSession, "repo"), makeRunSession(dormKey, GroupSession, "repo")} {
		h.Sessions.Create(s)
	}
	mustLimTurn(t, h, taskKey, Turn{Kind: ItemsTurn, Thread: runTask1})
	mustLimTurn(t, h, grpKey, Turn{Kind: ItemsTurn})
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, grpKey, Waiting)
	waitDeliveredSlot(t, delivered, aiSlot{runGroup1, runAI2})

	before := len(rh.snapshotCmds())
	if err := limSurface(t, h, "host.offline"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []SessionKey{taskKey, grpKey, dormKey} {
		reason, held := h.Held(k)
		if [3]any{seamState(t, h.Sessions, k), reason, held} != [3]any{Dormant, "offline", true} {
			t.Fatalf("%v after offline: %s %q %v", k, seamState(t, h.Sessions, k), reason, held)
		}
	}
	cmds := rh.snapshotCmds()[before:]
	if countCmds(cmds, func(c recordedCmd) bool { return c.payload["state"] != "offline" }) != 0 {
		t.Fatalf("a status after offline is not offline: %+v", cmds)
	}
	last := cmds[len(cmds)-3:]
	stops := countCmds(last, func(c recordedCmd) bool { return eventOf(c) == [3]any{"turn_interrupted", "stop", runTask1} })
	if stops != 1 || countCmds(cmds, isTaskFail) != 0 {
		t.Fatalf("want one turn_interrupted stop for the running task and no task fail, got %+v", cmds)
	}

	reloaded, _, err := New(db, seamKey(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if reason, held := reloaded.Held(grpKey); [2]any{reason, held} != [2]any{"offline", true} {
		t.Fatalf("offline not loaded at build: %q %v", reason, held)
	}

	before = len(rh.snapshotCmds())
	if err := limSurface(t, h, "host.online"); err != nil {
		t.Fatal(err)
	}
	if reason, held := h.Held(taskKey); held {
		t.Fatalf("still held after online: %q", reason)
	}
	for slot := range delivered {
		if slot == (aiSlot{}) {
			break
		}
	}
	cmds = rh.snapshotCmds()[before:]
	if len(cmds) != 3 || countCmds(cmds, func(c recordedCmd) bool { return c.payload["state"] == "dormant" }) != 3 {
		t.Fatalf("online statuses: %+v", cmds)
	}
}

func TestOfflineRefusesABadPayloadAndKeepsMemoryWhenTheStoreFails(t *testing.T) {
	h, _, _, db := setupLimEnv(t)
	for _, kind := range []string{"host.offline", "host.online"} {
		_, err := Surface[kind](t.Context(), h, nil, "owner", json.RawMessage(`{"t":"`+kind+`","extra":1}`))
		var herr *Error
		if !errors.As(err, &herr) || herr.Code != "bad_request" {
			t.Fatalf("%s with an unknown field: %v", kind, err)
		}
	}
	execSQL(t, db, "DROP TABLE setting")
	for _, kind := range []string{"host.offline", "host.online"} {
		if limSurface(t, h, kind) == nil {
			t.Fatalf("%s succeeded without its setting table", kind)
		}
	}
	if _, held := h.Held(SessionKey{Group: runGroup1, Member: runAI1}); held {
		t.Fatal("a failed host.offline still holds items")
	}
}

func TestTurnLimitsInterruptTheTurn(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.endOnInterrupt = true
		putLimGroupRow(h, runGroup1, 1, nil)
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))
		mustLimTurn(t, h, key, Turn{Kind: ItemsTurn, Thread: runTask1})
		synctest.Wait()

		time.Sleep(time.Minute - time.Nanosecond)
		synctest.Wait()
		if drv.proc(0).interrupted.Load() != 0 {
			t.Fatal("interrupted before turn_minutes")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		cmds := rh.snapshotCmds()
		intr := countCmds(cmds, func(c recordedCmd) bool {
			ev, _ := c.payload["event"].(map[string]any)
			return [3]any{ev["code"], ev["reason"], ev["detail"]} == [3]any{"turn_interrupted", "timeout", "Turn time limit reached."}
		})
		fails := countCmds(cmds, func(c recordedCmd) bool { return isTaskFail(c) && c.payload["reason"] == "timeout" })
		if [3]any{drv.proc(0).interrupted.Load(), intr, fails} != [3]any{int32(1), 1, 1} {
			t.Fatalf("timeout: interrupts=%d cmds=%+v", drv.proc(0).interrupted.Load(), cmds)
		}
		if seamState(t, h.Sessions, key) != Waiting {
			t.Fatal("the interrupted turn did not leave the session waiting")
		}
	})
}

func TestAStopThatEndsInAKillDoesNotFailTheTask(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.ignoreInterrupt = true
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))
		mustLimTurn(t, h, key, Turn{Kind: ItemsTurn, Thread: runTask1})
		synctest.Wait()

		interruptSessionTurn(h, key, "stop", "")
		interruptSessionTurn(h, key, "timeout", "")
		interruptSessionTurn(h, SessionKey{Group: "none", Member: "none"}, "stop", "")
		time.Sleep(stopSeconds*time.Second - time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Working {
			t.Fatal("killed before the stop timeout")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		cmds := rh.snapshotCmds()
		stops := countCmds(cmds, func(c recordedCmd) bool { return eventOf(c) == [3]any{"turn_interrupted", "stop", runTask1} })
		if [4]any{seamState(t, h.Sessions, key), drv.proc(0).interrupted.Load(), stops, countCmds(cmds, isTaskFail)} != [4]any{Dormant, int32(1), 1, 0} {
			t.Fatalf("stop then kill: %+v", cmds)
		}
	})
}

func TestCodexSilentLimitWaitsForActivityAndOpenCalls(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.endOnInterrupt = true
		key := SessionKey{Group: runGroup1, Member: runAI2}
		s := makeRunSession(key, GroupSession, "repo")
		s.Agent = "codex"
		h.Sessions.Create(s)
		mustLimTurn(t, h, key, Turn{Kind: ItemsTurn})
		p := drv.proc(0)
		for _, kind := range []EventKind{Active, Asked, Used} {
			time.Sleep(4 * time.Minute)
			p.emit(Event{Kind: kind})
			synctest.Wait()
		}
		done := h.Sessions.Calling(key)
		time.Sleep(4 * time.Minute)
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		if p.interrupted.Load() != 0 {
			t.Fatal("interrupted while active or in a tool call")
		}
		done()
		time.Sleep(codexSilentLimit)
		synctest.Wait()
		silent := countCmds(rh.snapshotCmds(), func(c recordedCmd) bool {
			ev, _ := c.payload["event"].(map[string]any)
			return ev["detail"] == "Turn was silent for 5 minutes."
		})
		if [2]any{p.interrupted.Load(), silent} != [2]any{int32(1), 1} {
			t.Fatalf("silent limit: interrupts=%d cmds=%+v", p.interrupted.Load(), rh.snapshotCmds())
		}
	})
}

func TestAStaleTurnTimerDoesNothingAndSettleStopsTheTimers(t *testing.T) {
	h, _, drv, _ := setupLimEnv(t)
	claudeKey := SessionKey{Group: runGroup1, Member: runAI1}
	codexKey := SessionKey{Group: runGroup1, Member: runAI2}
	h.Sessions.Create(makeRunSession(claudeKey, GroupSession, "repo"))
	cdx := makeRunSession(codexKey, GroupSession, "repo")
	cdx.Agent = "codex"
	h.Sessions.Create(cdx)
	mustLimTurn(t, h, claudeKey, Turn{Kind: ItemsTurn})
	mustLimTurn(t, h, codexKey, Turn{Kind: ItemsTurn})
	h.Report(claudeKey, Event{Kind: TurnBegan})
	drv.proc(1).emit(Event{Kind: RateLimited, Resets: h.Now().Add(time.Hour).UnixMilli()})
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, codexKey, Waiting)
	h.Report(codexKey, Event{Kind: TurnBegan})

	lp := limitsOf(h)
	fireTurnLimit(h, lp, claudeKey, 0, false)
	fireTurnLimit(h, lp, codexKey, 0, true)
	if drv.proc(0).interrupted.Load()+drv.proc(1).interrupted.Load() != 0 {
		t.Fatal("a stale timer interrupted a turn")
	}
	h.Settle()
	var delivered atomic.Int32
	Deliver = func(*Host, string, string) { delivered.Add(1) }
	releaseRateLimit(h, lp, "claude")
	onRateLimited(h, lp, cdx, h.Now().Add(time.Hour).UnixMilli())
	lp.wg.Wait()
	lp.mu.Lock()
	left := len(lp.rateTimers) + len(lp.watches)
	lp.mu.Unlock()
	if left != 0 || delivered.Load() != 0 {
		t.Fatalf("after Settle: %d timers, %d deliveries", left, delivered.Load())
	}
}

func TestRateLimitsHoldUntilTheReset(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		delivered := make(chan aiSlot, 8)
		Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }
		key := SessionKey{Group: runGroup1, Member: runAI1}
		h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
		mustLimTurn(t, h, key, Turn{Kind: ItemsTurn})
		synctest.Wait()

		p := drv.proc(0)
		reset := time.Now().Add(30 * time.Second).UnixMilli()
		p.emit(Event{Kind: RateLimited, Resets: time.Now().Add(20 * time.Second).UnixMilli()})
		p.emit(Event{Kind: RateLimited, Resets: reset})
		synctest.Wait()
		if countCmds(rh.snapshotCmds(), func(c recordedCmd) bool { return eventOf(c)[0] == "rate_limited" }) != 0 {
			t.Fatal("rate_limited sent before the turn ended")
		}
		p.emit(Event{Kind: TurnEnded, Status: "completed"})
		synctest.Wait()
		<-delivered
		end := rh.snapshotCmds()[len(rh.snapshotCmds())-1]
		ev, _ := end.payload["event"].(map[string]any)
		if [3]any{end.payload["state"], ev["code"], ev["resets"]} != [3]any{"limited", "rate_limited", float64(reset)} {
			t.Fatalf("turn end status: %+v", end.payload)
		}

		time.Sleep(30*time.Second - time.Millisecond)
		synctest.Wait()
		if reason, held := h.Held(key); [2]any{reason, held} != [2]any{"rate_limit", true} {
			t.Fatalf("not held before the reset: %q %v", reason, held)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if _, held := h.Held(key); held || <-delivered != (aiSlot{}) {
			t.Fatal("the reset did not release the hold")
		}
		if st := rh.snapshotCmds()[len(rh.snapshotCmds())-1].payload["state"]; st != "waiting" {
			t.Fatalf("status after the reset: %v", st)
		}

		p.emit(Event{Kind: RateLimited, Resets: time.Now().Add(5 * time.Second).UnixMilli()})
		synctest.Wait()
		out := rh.snapshotCmds()[len(rh.snapshotCmds())-1]
		if [2]any{out.payload["state"], eventOf(out)[0]} != [2]any{"limited", "rate_limited"} {
			t.Fatalf("rate limit outside a turn: %+v", out.payload)
		}
		time.Sleep(5 * time.Second)
		synctest.Wait()
		<-delivered
	})
}

func TestMemberUsageComesFromMemory(t *testing.T) {
	saveRuntimeGlobals(t)
	installDefaultHooks()
	db, _ := openFileStore(t, t.TempDir())
	t.Cleanup(func() { db.Close() })
	now := time.Date(2026, 11, 22, 23, 0, 0, 0, time.Local)
	execSQL(t, db, `INSERT INTO usage(agent, day, data) VALUES('claude', '2026-11-22', '{"turns":4}'), ('codex', '2026-11-21', '{"turns":9}')`)
	h, _, err := New(db, seamKey(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	key := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
	putLimLocalAI(h, runGroup2, runAI2, "claude")

	if u := memberUsage(h, runGroup1, runAI1); u.TurnsToday == nil || *u.TurnsToday != 4 {
		t.Fatalf("turns loaded at build: %+v", u)
	}
	h.Report(key, Event{Kind: TurnEnded})
	h.Report(SessionKey{Group: "none", Member: "none"}, Event{Kind: TurnEnded})
	if u := memberUsage(h, runGroup2, runAI2); u.TurnsToday == nil || *u.TurnsToday != 5 {
		t.Fatalf("turn counted for the member row's CLI: %+v", u)
	}
	var stored string
	db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT data FROM usage WHERE agent = 'claude' AND day = '2026-11-22'").Scan(&stored)
	})
	if stored != `{"turns":5}` {
		t.Fatalf("stored usage %s", stored)
	}

	pct, resets := 62.0, int64(1763611200000)
	h.Report(key, Event{Kind: Used, Usage: Usage{WindowPct: &pct}})
	h.Report(key, Event{Kind: Used, Usage: Usage{WindowResets: &resets}})
	now = now.Add(2 * time.Hour)
	got := memberUsage(h, runGroup1, runAI1)
	if got.TurnsToday != nil || *got.WindowPct != pct || *got.WindowResets != resets {
		t.Fatalf("usage on the next day: %+v", got)
	}
	h.Report(key, Event{Kind: TurnEnded})
	if u := memberUsage(h, runGroup1, runAI1); *u.TurnsToday != 1 {
		t.Fatalf("first turn of the new day: %+v", u)
	}
	if u := memberUsage(h, "none", "none"); u != (proto.Usage{}) {
		t.Fatalf("usage of an unknown member: %+v", u)
	}
}

func TestMemberStateOverlays(t *testing.T) {
	h, _, _, _ := setupLimEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
	putLimGroupRow(h, runGroup1, 0, &proto.GroupPause{By: "owner", At: 1})
	if st, _ := h.MemberState(runGroup1, runAI1); st != "paused" {
		t.Fatalf("paused group: %s", st)
	}
	lp := limitsOf(h)
	lp.mu.Lock()
	lp.rateUntil["claude"] = h.Now().Add(time.Hour).UnixMilli()
	lp.mu.Unlock()
	if st, _ := h.MemberState(runGroup1, runAI1); st != "limited" {
		t.Fatalf("rate limited: %s", st)
	}
	lp.mu.Lock()
	lp.offline = true
	lp.mu.Unlock()
	if st, _ := h.MemberState(runGroup1, runAI1); st != "offline" {
		t.Fatalf("offline: %s", st)
	}
}

func TestPausingInterruptsTheTurnsOfThePausedGroupOrTask(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.endOnInterrupt = true
		grpKey := SessionKey{Group: runGroup1, Member: runAI1}
		taskKey := SessionKey{Group: runGroup1, Member: runAI2, Task: runTask1}
		otherKey := SessionKey{Group: runGroup2, Member: runAI1}
		for _, s := range []Session{makeRunSession(grpKey, GroupSession, "repo"), makeRunSession(taskKey, TaskSession, "wt/1"), makeRunSession(otherKey, GroupSession, "repo")} {
			h.Sessions.Create(s)
		}
		mustLimTurn(t, h, grpKey, Turn{Kind: ItemsTurn})
		mustLimTurn(t, h, taskKey, Turn{Kind: ItemsTurn, Thread: runTask1})
		mustLimTurn(t, h, otherKey, Turn{Kind: ItemsTurn})
		d := deliv(h)
		d.mu.Lock()
		d.groupLocked(runGroup1, "home-dev").live = true
		d.mu.Unlock()

		task := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: runTask1, Seq: 1},
			Group:     runGroup1, Num: 1, Title: "T", Slug: "t", Origin: "brief",
			State: "active", Step: "branch", Assignee: runAI2, Depends: []string{},
		}
		pushLimRows(h, runGroup1, task)
		synctest.Wait()
		interrupts := func() [3]int32 {
			return [3]int32{drv.proc(0).interrupted.Load(), drv.proc(1).interrupted.Load(), drv.proc(2).interrupted.Load()}
		}
		if interrupts() != [3]int32{} {
			t.Fatalf("an active task interrupted a turn: %v", interrupts())
		}

		task.Seq, task.State = 2, "paused"
		pushLimRows(h, runGroup1, task)
		synctest.Wait()
		if interrupts() != [3]int32{0, 1, 0} {
			t.Fatalf("a paused task: %v", interrupts())
		}
		last := rh.snapshotCmds()[len(rh.snapshotCmds())-1]
		if eventOf(last) != [3]any{"turn_interrupted", "stop", runTask1} {
			t.Fatalf("task pause status: %+v", last.payload)
		}

		grp := proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 3},
			Name:      "Proj", Kind: "project", Base: "main", Forge: "none",
			Home: "home-dev", Secretary: runAI1, Checks: []string{},
			Paused: &proto.GroupPause{By: "owner", At: 1},
		}
		pushLimRows(h, runGroup1, grp)
		synctest.Wait()
		if interrupts() != [3]int32{1, 1, 0} {
			t.Fatalf("a paused group: %v", interrupts())
		}
		last = rh.snapshotCmds()[len(rh.snapshotCmds())-1]
		if [2]any{last.payload["state"], eventOf(last)[1]} != [2]any{"paused", "stop"} {
			t.Fatalf("group pause status: %+v", last.payload)
		}
	})
}

// limStates is the last state reported of each AI of a group since the first n
// commands.
func limStates(rh *runHome, group string, n int) map[string]string {
	states := map[string]string{}
	for _, c := range rh.snapshotCmds()[n:] {
		if c.payload["op"] == "status" && c.payload["g"] == group {
			states[c.payload["id"].(string)] = c.payload["state"].(string)
		}
	}
	return states
}

// A pause and a resume of the group, and the owner's offline switch, are
// reported of every AI this node hosts: of one that waits, and of one that
// never had a session.
func TestPauseAndOfflineAreReportedOfEveryHostedAI(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupLimBubble(t, base)
		defer shutdown()
		drv.endOnInterrupt = true
		d := deliv(h)
		d.mu.Lock()
		d.groupLocked(runGroup1, "home-dev").live = true
		d.mu.Unlock()
		putLimLocalAI(h, runGroup1, runAI1, "claude")
		putLimLocalAI(h, runGroup1, runAI2, "codex")
		h.Rows.Put(runGroup1, "member", "elsewhere", json.RawMessage(marshalWire(proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "elsewhere", Seq: 2}, Group: runGroup1, Kind: "ai", Name: "Elsewhere"},
			CLI:       "claude", Host: "another", Allow: []string{}, Approvers: []string{},
		})))
		key := SessionKey{Group: runGroup1, Member: runAI1}
		h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
		mustLimTurn(t, h, key, Turn{Kind: ItemsTurn})
		check := func(what string, act func(), want map[string]string) {
			t.Helper()
			n := len(rh.snapshotCmds())
			act()
			synctest.Wait()
			if got := limStates(rh, runGroup1, n); !maps.Equal(got, want) {
				t.Fatalf("%s: reported %v, want %v", what, got, want)
			}
		}
		push := func(seq int64, paused *proto.GroupPause) func() {
			return func() {
				pushLimRows(h, runGroup1, proto.GroupRow{
					RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: seq}, Name: "Proj", Kind: "project",
					Base: "main", Forge: "none", Home: "home-dev", Checks: []string{}, Paused: paused,
				})
			}
		}
		paused := &proto.GroupPause{By: "owner", At: 1}
		check("not paused", push(3, nil), map[string]string{})
		check("pause", push(4, paused), map[string]string{runAI1: "paused", runAI2: "paused"})
		check("still paused", push(5, paused), map[string]string{})
		check("resume", push(6, nil), map[string]string{runAI1: "waiting", runAI2: "dormant"})
		surface := func(kind string) func() { return func() { limSurface(t, h, kind) } }
		check("host.offline", surface("host.offline"), map[string]string{runAI1: "offline", runAI2: "offline"})
		check("host.online", surface("host.online"), map[string]string{runAI1: "dormant", runAI2: "dormant"})
	})
}

// A turn that begins while the state is on its way reports its own, which may
// arrive first: the state is reported again.
func TestStateThatChangesWhileItIsReportedIsReportedAgain(t *testing.T) {
	h, rh, _, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
	rh.cmdFn = func(map[string]any) (json.RawMessage, error) {
		h.Sessions.Move(key, Dormant, Working)
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	reportState(t.Context(), h, memberHomeSlot{"home-dev", runGroup1, runAI1}, proto.TurnInterruptedEvent{SystemCode: proto.SystemCode{Code: "turn_interrupted"}, Reason: "stop"})
	var got []any
	for _, c := range rh.snapshotCmds() {
		got = append(got, c.payload["state"], eventOf(c)[0])
	}
	if !slices.Equal(got, []any{"dormant", "turn_interrupted", "working", nil}) {
		t.Fatalf("reported %v", got)
	}
}

func pushLimRows(h *Host, group string, rows ...any) {
	raws := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		raws[i] = json.RawMessage(marshalWire(r))
	}
	frame := proto.Rows{Type: "rows", Group: group, Rows: raws}
	h.Push("home-dev", json.RawMessage(marshalWire(frame)))
}
