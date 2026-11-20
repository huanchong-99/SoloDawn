package host

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	runGroup1 = "rZNvy-1jH6Z-BcPqA5U5BQ"
	runGroup2 = "7pX2mN9qK4vW8yB1cR6tFA"
	runAI1    = "1PC8WineBrUQ-apCjx7tug"
	runAI2    = "9mL3vB7xQ2kP5wZ8nH4jCA"
	runHuman1 = "3xK8pM2vN7qR4wY9bL5tGA"
	runTask1  = "DrtCn6htSBwmMPrFPbHJHA"
	runTask2  = "6hJ9kL4mP8rT2vX5zB7nQA"
	runMsg1   = "mC2ePrmW9VnmM_TRlN7zdg"
	runMsg2   = "4vN8qR2wY6bL9tG3xK7pMA"
	runSysMsg = "8zB4nQ7hJ2kL5mP9rT3vXA"
	runAIMsg  = "2wY6bL9tG3xK7pM4vN8qRA"
	runSecret = "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
)

type fakeDriver struct {
	mu       sync.Mutex
	launches []Launch
	procs    []*fakeProc
	startErr error
	makeProc func(ctx context.Context, idx int, l Launch) (*fakeProc, error)
}

func (d *fakeDriver) Start(ctx context.Context, l Launch) (Process, error) {
	d.mu.Lock()
	idx := len(d.launches)
	d.launches = append(d.launches, l)
	err := d.startErr
	maker := d.makeProc
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if maker != nil {
		return maker(ctx, idx, l)
	}
	p := newFakeProc(ctx, l)
	d.mu.Lock()
	d.procs = append(d.procs, p)
	d.mu.Unlock()
	return p, nil
}

func (d *fakeDriver) Interactive(l Launch) ([]string, string, error) {
	return []string{"fake", l.Session}, l.Session, nil
}

func (d *fakeDriver) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.launches)
}

func (d *fakeDriver) proc(i int) *fakeProc {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.procs[i]
}

func (d *fakeDriver) lastProc() *fakeProc {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.procs[len(d.procs)-1]
}

func (d *fakeDriver) lastLaunch() Launch {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.launches[len(d.launches)-1]
}

type recordedAsk struct {
	id    string
	allow bool
}

type fakeProc struct {
	ctx             context.Context
	launch          Launch
	mu              sync.Mutex
	turns           []string
	interrupts      int
	answers         []recordedAsk
	turnErr         error
	answerErr       error
	ignoreClose     bool
	ignoreInterrupt bool
	autoComplete    bool
	autoReadyID     string
	closedIn        bool
	eventsClosed    bool
	events          chan Event
	turnCh          chan string
	closeCh         chan struct{}
	exitedCh        chan struct{}
}

func newFakeProc(ctx context.Context, l Launch) *fakeProc {
	p := &fakeProc{
		ctx:      ctx,
		launch:   l,
		events:   make(chan Event, 32),
		turnCh:   make(chan string, 16),
		closeCh:  make(chan struct{}),
		exitedCh: make(chan struct{}),
	}
	go p.watchContext()
	return p
}

func (p *fakeProc) watchContext() {
	select {
	case <-p.ctx.Done():
		p.finishExit()
	case <-p.exitedCh:
		return
	}
}

func (p *fakeProc) Events() <-chan Event { return p.events }

func (p *fakeProc) Turn(text string) error {
	p.mu.Lock()
	err := p.turnErr
	autoReady := p.autoReadyID
	autoDone := p.autoComplete
	first := len(p.turns) == 0
	if err == nil {
		p.turns = append(p.turns, text)
	}
	p.mu.Unlock()
	if err != nil {
		return err
	}
	p.turnCh <- text
	if first && autoReady != "" {
		p.emit(Event{Kind: Ready, Session: autoReady})
	}
	if autoDone {
		p.emit(Event{Kind: TurnEnded, Status: "completed"})
	}
	return nil
}

func (p *fakeProc) Interrupt() error {
	p.mu.Lock()
	p.interrupts++
	ignore := p.ignoreInterrupt
	p.mu.Unlock()
	if !ignore {
		p.emit(Event{Kind: TurnEnded, Status: "interrupted"})
	}
	return nil
}

func (p *fakeProc) Answer(ask string, allow bool) error {
	p.mu.Lock()
	err := p.answerErr
	p.answers = append(p.answers, recordedAsk{id: ask, allow: allow})
	p.mu.Unlock()
	return err
}

func (p *fakeProc) Close() error {
	p.mu.Lock()
	already := p.closedIn
	p.closedIn = true
	ignore := p.ignoreClose
	p.mu.Unlock()
	if !already {
		close(p.closeCh)
	}
	if !ignore {
		p.finishExit()
	}
	return nil
}

func (p *fakeProc) emit(e Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.eventsClosed {
		p.events <- e
	}
}

func (p *fakeProc) finishExit() {
	p.mu.Lock()
	if p.eventsClosed {
		p.mu.Unlock()
		return
	}
	p.eventsClosed = true
	close(p.events)
	close(p.exitedCh)
	p.mu.Unlock()
}

type recordedCmd struct {
	raw     string
	payload map[string]any
}

type runHome struct {
	mu        sync.Mutex
	path      string
	pathCh    chan struct{}
	done      chan struct{}
	cmds      []recordedCmd
	cmdNotify chan recordedCmd
	fetchFn   func(req any) (json.RawMessage, error)
	cmdFn     func(payload map[string]any) (json.RawMessage, error)
}

func newRunHome(path string) *runHome {
	return &runHome{
		path:      path,
		done:      make(chan struct{}),
		cmdNotify: make(chan recordedCmd, 64),
	}
}

func (rh *runHome) Send(any) error { return nil }

func (rh *runHome) Path() string {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	return rh.path
}

func (rh *runHome) setPath(next string) {
	rh.mu.Lock()
	rh.path = next
	ch := rh.pathCh
	rh.mu.Unlock()
	if ch != nil {
		ch <- struct{}{}
	}
}

func (rh *runHome) PathChanged() <-chan struct{} {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	return rh.pathCh
}

func (rh *runHome) Done() <-chan struct{} { return rh.done }

func (rh *runHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	rh.mu.Lock()
	id := int64(len(rh.cmds) + 1)
	fetchFn := rh.fetchFn
	cmdFn := rh.cmdFn
	rh.mu.Unlock()
	frame := build(id)
	cmdFrame, ok := frame.(proto.Cmd)
	if !ok {
		if fetchFn != nil {
			return fetchFn(frame)
		}
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(cmdFrame.Env.Payload), &fields); err != nil {
		return nil, err
	}
	rec := recordedCmd{raw: cmdFrame.Env.Payload, payload: fields}
	rh.mu.Lock()
	rh.cmds = append(rh.cmds, rec)
	rh.mu.Unlock()
	rh.cmdNotify <- rec
	if cmdFn != nil {
		return cmdFn(fields)
	}
	return json.RawMessage(`{"re":` + strconv.FormatInt(id, 10) + `,"ok":true}`), nil
}

func (rh *runHome) waitCmd(t *testing.T) recordedCmd {
	t.Helper()
	select {
	case c := <-rh.cmdNotify:
		if nulls := proto.UnexpectedNulls([]byte(c.raw)); len(nulls) != 0 {
			t.Fatalf("unexpected nulls %v in %s", nulls, c.raw)
		}
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for home command")
		return recordedCmd{}
	}
}

func (rh *runHome) snapshotCmds() []recordedCmd {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	return slices.Clone(rh.cmds)
}

func saveRuntimeGlobals(t *testing.T) {
	t.Helper()
	prevDrivers := maps.Clone(Drivers)
	prevListeners := maps.Clone(Listeners)
	prevHolds := maps.Clone(Holds)
	prevDeliver := Deliver
	prevInstructions := Instructions
	prevCompose := Compose
	prevIdle := idleUnit
	prevStop := stopTimeout
	t.Cleanup(func() {
		Drivers = prevDrivers
		Listeners = prevListeners
		Holds = prevHolds
		Deliver = prevDeliver
		Instructions = prevInstructions
		Compose = prevCompose
		idleUnit = prevIdle
		stopTimeout = prevStop
	})
}

var activeTestHosts []*Host

func stopHostNow(h *Host) {
	rt := stateOf(h)
	rt.mu.Lock()
	clear(rt.interrupted)
	for k, cancel := range rt.pathCancels {
		cancel()
		delete(rt.pathCancels, k)
	}
	for _, lp := range rt.procs {
		stopIdleLocked(lp)
		lp.cancel()
	}
	rt.mu.Unlock()
	rt.wg.Wait()
}

func stopAllTestHosts() {
	for _, h := range activeTestHosts {
		stopHostNow(h)
	}
	activeTestHosts = nil
}

func cleanupRuntime(t *testing.T, h *Host) {
	t.Helper()
	activeTestHosts = append(activeTestHosts, h)
	t.Cleanup(func() {
		stopHostNow(h)
	})
}

func newRunHost(t *testing.T, db *store.Store) *Host {
	t.Helper()
	var at atomic.Int64
	at.Store(seamStart - 1000)
	clock := func() time.Time {
		return time.UnixMilli(at.Add(1000))
	}
	h, _, err := New(db, seamKey(), clock)
	if err != nil {
		t.Fatal(err)
	}
	cleanupRuntime(t, h)
	return h
}

func setupRunEnv(t *testing.T) (*Host, *runHome, *fakeDriver) {
	t.Helper()
	stopAllTestHosts()
	saveRuntimeGlobals(t)
	drv := &fakeDriver{}
	Drivers = map[string]Driver{"claude": drv, "codex": drv}
	Listeners = map[string]func(*Host, SessionKey, Event){}
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
	Deliver = nil
	Instructions = nil
	Compose = nil
	h := newRunHost(t, seamStore(t))
	h.Shim = []string{"solodawn", "mcp"}
	rh := newRunHome("local")
	h.Attach("home-dev", rh)
	return h, rh, drv
}

func makeRunSession(key SessionKey, kind Kind, folder string) Session {
	return Session{
		SessionKey: key,
		Kind:       kind,
		Folder:     folder,
		Agent:      "claude",
		Home:       "home-dev",
		Name:       "Claude Code",
	}
}

func putSetting(t *testing.T, db *store.Store, key, jsonVal string) {
	t.Helper()
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)", key, jsonVal)
		return err
	}
	if err := db.Write(write); err != nil {
		t.Fatal(err)
	}
}

func waitForState(t *testing.T, h *Host, key SessionKey, want State) Session {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := h.Sessions.Get(key); ok && s.State == want {
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	s, _ := h.Sessions.Get(key)
	t.Fatalf("session %v stayed %s, want %s", key, s.State, want)
	return Session{}
}

func tokenFromLaunch(l Launch) string {
	idx := slices.Index(l.Shim, "--token")
	if idx < 0 || idx+1 >= len(l.Shim) {
		return ""
	}
	return l.Shim[idx+1]
}

func TestStartResumeEventsUsageAndDeliver(t *testing.T) {
	h, rh, drv := setupRunEnv(t)
	idleUnit = 30 * time.Millisecond
	Instructions = func(s Session) string { return "prompt:" + string(s.Kind) }

	var events []Event
	var mu sync.Mutex
	Listeners["test"] = func(_ *Host, _ SessionKey, e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}
	delivered := make(chan aiSlot, 8)
	Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }

	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	dir := t.TempDir()
	if err := h.Sessions.Create(makeRunSession(key, TaskSession, dir)); err != nil {
		t.Fatal(err)
	}

	turn1 := Turn{Kind: ItemsTurn, Thread: runTask1, Depth: 1}
	if err := Runner.Turn(t.Context(), h, key, "first message", turn1); err != nil {
		t.Fatal(err)
	}
	if drv.count() != 1 {
		t.Fatalf("starts = %d, want 1", drv.count())
	}
	l1 := drv.lastLaunch()
	tok1 := tokenFromLaunch(l1)
	wantLevel := fallbackClaudeWindows(2, "claude", runtime.GOOS)
	if l1.Folder != dir || l1.Session != "" || l1.Level != wantLevel || !slices.Equal(l1.Shim[:2], []string{"solodawn", "mcp"}) || l1.Instructions != "prompt:task" || l1.RepoConfig || l1.ToolTimeout != 60*time.Minute {
		t.Fatalf("unexpected launch: %+v", l1)
	}
	caller1, ok := h.Sessions.Caller(tok1)
	if !ok || caller1.Level != wantLevel || caller1.Interactive || caller1.Turn != (Turn{N: 1, Kind: ItemsTurn, Thread: runTask1, Depth: 1}) {
		t.Fatalf("unexpected caller: %+v (ok=%v)", caller1, ok)
	}

	stWorking := rh.waitCmd(t)
	var ms1 proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(stWorking.raw), &ms1); err != nil || ms1.State != "working" || string(ms1.Task) != runTask1 || ms1.Path != "local" {
		t.Fatalf("working status: %+v (%v)", ms1, err)
	}

	p1 := drv.proc(0)
	// Empty Ready.Session does not overwrite CLISession; non-empty stores it.
	p1.emit(Event{Kind: Ready, Session: "cli-sess-1"})
	p1.emit(Event{Kind: Ready, Session: ""})
	p1.emit(Event{Kind: Active})
	pct := 42.5
	resets := int64(1763609999000)
	p1.emit(Event{Kind: Used, Usage: Usage{WindowPct: &pct}})
	p1.emit(Event{Kind: TurnEnded, Status: "completed", Usage: Usage{WindowResets: &resets}})

	select {
	case slot := <-delivered:
		if slot != (aiSlot{runGroup1, runAI1}) {
			t.Fatalf("delivered %+v", slot)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Deliver not called")
	}

	sAfter1 := waitForState(t, h, key, Waiting)
	if sAfter1.CLISession != "cli-sess-1" {
		t.Fatalf("CLISession = %q, want cli-sess-1", sAfter1.CLISession)
	}
	stWaiting := rh.waitCmd(t)
	var ms2 proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(stWaiting.raw), &ms2); err != nil || ms2.State != "waiting" || string(ms2.Task) != runTask1 || ms2.Usage.WindowPct == nil || *ms2.Usage.WindowPct != 42.5 || ms2.Usage.WindowResets == nil || *ms2.Usage.WindowResets != resets {
		t.Fatalf("waiting status: %+v (%v)", ms2, err)
	}

	// Second turn while still Waiting reuses the open process without restarting.
	putSetting(t, h.Store, "idle_minutes", "1")
	if err := Runner.Turn(t.Context(), h, key, "second message", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	if drv.count() != 1 {
		t.Fatalf("process restarted on waiting turn: %d starts", drv.count())
	}
	rh.waitCmd(t)
	caller2, ok := h.Sessions.Caller(tok1)
	if !ok || caller2.Turn.N != 2 {
		t.Fatalf("caller turn after second turn: %+v", caller2)
	}
	p1.emit(Event{Kind: TurnEnded, Status: "completed"})
	<-delivered
	stWaiting2 := rh.waitCmd(t)
	var ms2b proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(stWaiting2.raw), &ms2b); err != nil || ms2b.Usage.WindowPct == nil || *ms2b.Usage.WindowPct != 42.5 || ms2b.Usage.WindowResets == nil || *ms2b.Usage.WindowResets != resets {
		t.Fatalf("empty usage wiped previous window usage: %+v (%v)", ms2b, err)
	}

	// Wait for idle dormancy (1 * 30ms): process exits, token forgotten, state Dormant.
	waitForState(t, h, key, Dormant)
	stDormant := rh.waitCmd(t)
	var ms3 proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(stDormant.raw), &ms3); err != nil || ms3.State != "dormant" || ms3.Task != "" {
		t.Fatalf("dormant status: %+v (%v)", ms3, err)
	}
	if _, ok := h.Sessions.Caller(tok1); ok {
		t.Fatal("token of closed process was not forgotten")
	}

	// Third turn wakes from Dormant as a resume with the stored CLISession.
	putSetting(t, h.Store, "idle_minutes", "15")
	if err := Runner.Turn(t.Context(), h, key, "third message", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	if drv.count() != 2 || drv.lastLaunch().Session != "cli-sess-1" {
		t.Fatalf("resume launch: count=%d, session=%q", drv.count(), drv.lastLaunch().Session)
	}
	drv.proc(1).Close()

	mu.Lock()
	var kinds []EventKind
	for _, ev := range events {
		kinds = append(kinds, ev.Kind)
	}
	mu.Unlock()
	wantPrefix := []EventKind{TurnBegan, Ready, Ready, Active, Used, TurnEnded, TurnBegan, TurnEnded}
	if len(kinds) < len(wantPrefix) || !slices.Equal(kinds[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("reported events = %v, want prefix %v", kinds, wantPrefix)
	}
}

func TestTwoHostsInOneProcessAreIsolated(t *testing.T) {
	saveRuntimeGlobals(t)
	drv := &fakeDriver{makeProc: func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete = true
		return p, nil
	}}
	Drivers = map[string]Driver{"claude": drv}
	h1 := newRunHost(t, seamStore(t))
	h2 := newRunHost(t, seamStore(t))
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	dir := t.TempDir()
	h1.Sessions.Create(makeRunSession(key, TaskSession, dir))
	h2.Sessions.Create(makeRunSession(key, TaskSession, dir))

	if err := Runner.Turn(t.Context(), h1, key, "only host 1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, h1, key, Waiting)
	if s2, _ := h2.Sessions.Get(key); s2.State != Dormant {
		t.Fatalf("host 2 state = %s, want dormant", s2.State)
	}
}

func TestLaunchConfigFoldersLevelsRepoConfigAndTimeout(t *testing.T) {
	// Verify file-backed store resolves projects/<group>/<folder>, and :memory: resolves filepath.Abs.
	realTmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(realTmp, "node.db")
	fileDB, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fileDB.Close() })

	saveRuntimeGlobals(t)
	drv := &fakeDriver{makeProc: func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete = true
		return p, nil
	}}
	Drivers = map[string]Driver{"claude": drv, "codex": drv}
	hFile := newRunHost(t, fileDB)

	keyTask := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	keyRev := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask2}
	keyGrp := SessionKey{Group: runGroup1, Member: runAI1}

	hFile.Sessions.Create(makeRunSession(keyTask, TaskSession, "wt/12-paths"))
	hFile.Sessions.Create(makeRunSession(keyRev, ReviewSession, "review/12"))
	hFile.Sessions.Create(makeRunSession(keyGrp, GroupSession, "repo"))

	// Enable perm(level=2, repo_config=1) and all_level3=true: Group and Review stay level 1; Group never loads RepoConfig.
	seamSQL(t, fileDB,
		`INSERT INTO perm(agent, g, level, repo_config) VALUES('claude', '`+runGroup1+`', 2, 1)`,
		`INSERT INTO setting(key, value) VALUES('all_level3', 'true')`,
	)

	// Group row with TurnMinutes = 17, then TurnMinutes = 0 (boundary -> 60m), then malformed JSON (-> 60m).
	grpRow := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 1},
		Name:      "Website", Kind: "project", Base: "main", Forge: "none", Home: "home-dev",
		Checks: []string{}, Limits: proto.Limits{TurnMinutes: 17},
	}
	rawGrp, _ := json.Marshal(grpRow)
	hFile.Rows.Put(runGroup1, "group", runGroup1, rawGrp)

	if err := Runner.Turn(t.Context(), hFile, keyGrp, "group turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, hFile, keyGrp, Waiting)
	lg := drv.lastLaunch()
	wantGrpDir := filepath.Join(filepath.Dir(dbPath), "projects", runGroup1, "repo")
	if lg.Folder != wantGrpDir || lg.Level != 1 || lg.RepoConfig || lg.ToolTimeout != 17*time.Minute {
		t.Fatalf("group launch: %+v, want dir %s", lg, wantGrpDir)
	}

	// Review session: Level=1, RepoConfig=true; TurnMinutes=0 falls back to 60m.
	grpRow.Limits.TurnMinutes = 0
	rawGrp0, _ := json.Marshal(grpRow)
	hFile.Rows.Put(runGroup1, "group", runGroup1, rawGrp0)
	if err := Runner.Turn(t.Context(), hFile, keyRev, "review turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, hFile, keyRev, Waiting)
	lr := drv.lastLaunch()
	if lr.Level != 1 || !lr.RepoConfig || lr.ToolTimeout != 60*time.Minute {
		t.Fatalf("review launch: %+v", lr)
	}

	// Task session with all_level3=true -> Level 3 (overrides perm=2); unknown field in group JSON fails proto.Decode -> 60m even with turn_minutes=17.
	hFile.Rows.Put(runGroup1, "group", runGroup1, json.RawMessage(`{"t":"group","id":"`+runGroup1+`","seq":1,"name":"W","kind":"project","base":"main","forge":"none","home":"home-dev","checks":[],"limits":{"turn_minutes":17},"extra":1}`))
	if err := Runner.Turn(t.Context(), hFile, keyTask, "task turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, hFile, keyTask, Waiting)
	lt := drv.lastLaunch()
	if lt.Level != 3 || !lt.RepoConfig || lt.ToolTimeout != 60*time.Minute {
		t.Fatalf("task all_level3 launch: %+v", lt)
	}

	// Pin effectiveLevel & fallbackClaudeWindows directly across every operand and boundary.
	checkEffectiveLevelMatrix(t)

	// Pin :memory: relative folder -> filepath.Abs.
	hMem, _, drvMem := setupRunEnv(t)
	drvMem.makeProc = drv.makeProc
	memKey := SessionKey{Group: runGroup1, Member: runAI1}
	hMem.Sessions.Create(makeRunSession(memKey, GroupSession, "rel-repo"))
	if err := Runner.Turn(t.Context(), hMem, memKey, "mem turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, hMem, memKey, Waiting)
	wantAbs, _ := filepath.Abs("rel-repo")
	if drvMem.lastLaunch().Folder != wantAbs {
		t.Fatalf("memory relative folder = %q, want %q", drvMem.lastLaunch().Folder, wantAbs)
	}
}

func checkEffectiveLevelMatrix(t *testing.T) {
	t.Helper()
	taskClaude := Session{Kind: TaskSession, Agent: "claude"}
	taskCodex := Session{Kind: TaskSession, Agent: "codex"}
	cases := []struct {
		s    Session
		cfg  runConfig
		goos string
		want int
	}{
		{Session{Kind: GroupSession, Agent: "claude"}, runConfig{allLevel3: true, hasPerm: true, permLevel: 3, defaultLevel: 3}, "linux", 1},
		{Session{Kind: ReviewSession, Agent: "claude"}, runConfig{allLevel3: true, hasPerm: true, permLevel: 3, defaultLevel: 3}, "linux", 1},
		{taskClaude, runConfig{allLevel3: true, hasPerm: true, permLevel: 1, defaultLevel: 1}, "windows", 3},
		{taskClaude, runConfig{hasPerm: true, permLevel: 2, defaultLevel: 1}, "windows", 2},
		{taskClaude, runConfig{defaultLevel: 2}, "windows", 1},
		{taskClaude, runConfig{defaultLevel: 0}, "windows", 0},
		{taskClaude, runConfig{defaultLevel: 3}, "windows", 3},
		{taskCodex, runConfig{defaultLevel: 2}, "windows", 2},
		{taskClaude, runConfig{defaultLevel: 2}, "linux", 2},
	}
	for i, tc := range cases {
		if got := effectiveLevel(tc.s, tc.cfg, tc.goos); got != tc.want {
			t.Fatalf("case %d: got %d, want %d", i, got, tc.want)
		}
	}
}

func TestDormancyVirtualClockBoundariesAndSpontaneousIdleExit(t *testing.T) {
	// Pin idleDuration <= 0 boundary (0 and -1 -> 15m, 1 -> 1m).
	saveRuntimeGlobals(t)
	idleUnit = time.Minute
	if idleDuration(0) != 15*time.Minute || idleDuration(-1) != 15*time.Minute || idleDuration(1) != time.Minute {
		t.Fatalf("idleDuration boundaries wrong: %v %v %v", idleDuration(0), idleDuration(-1), idleDuration(1))
	}

	wtDir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		db, err := store.Open(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		h, _, err := New(db, seamKey(), time.Now)
		if err != nil {
			t.Fatal(err)
		}
		drv := &fakeDriver{makeProc: func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			p.autoComplete = true
			return p, nil
		}}
		Drivers = map[string]Driver{"claude": drv}
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		if err := h.Sessions.Create(makeRunSession(key, TaskSession, wtDir)); err != nil {
			t.Fatal(err)
		}
		if err := Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Waiting {
			t.Fatal("expected waiting right after turn")
		}
		// 1 ns before default 15 minutes: still Waiting.
		time.Sleep(15*time.Minute - time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Waiting {
			t.Fatal("went dormant before 15 minutes")
		}
		// At exact 15 minutes: Dormant.
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Dormant {
			t.Fatal("did not go dormant at 15 minutes")
		}
	})

	// Test stale idle generation & spontaneous exit while Waiting.
	h, rh, drv := setupRunEnv(t)
	idleUnit = time.Hour
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(key, TaskSession, t.TempDir()))
	if err := Runner.Turn(t.Context(), h, key, "turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	p := drv.proc(0)
	p.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, key, Waiting)
	rh.waitCmd(t)

	// Stale generation, missing session key, turnActive, or closing on onIdleTimeout does nothing.
	rt := stateOf(h)
	onIdleTimeout(h, rt, SessionKey{Group: "missing"}, 1)
	onIdleTimeout(h, rt, key, 9999)
	rt.mu.Lock()
	curGen := rt.procs[key].idleGen
	rt.procs[key].turnActive = true
	rt.mu.Unlock()
	onIdleTimeout(h, rt, key, curGen)
	rt.mu.Lock()
	rt.procs[key].turnActive = false
	rt.closing[key] = true
	rt.mu.Unlock()
	onIdleTimeout(h, rt, key, curGen)
	rt.mu.Lock()
	delete(rt.closing, key)
	rt.mu.Unlock()
	if seamState(t, h.Sessions, key) != Waiting {
		t.Fatal("guarded idle timeout closed session")
	}

	// Spontaneous process exit while Waiting moves session to Dormant without crash or AuditKilled.
	p.finishExit()
	waitForState(t, h, key, Dormant)
	st := rh.waitCmd(t)
	var ms proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(st.raw), &ms); err != nil || ms.State != "dormant" || ms.Event != nil {
		t.Fatalf("unexpected status on spontaneous idle exit: %s", st.raw)
	}
	if rec := seamRecord(t, h); rec != "" {
		t.Fatalf("unexpected audit row on spontaneous idle exit: %q", rec)
	}
}

func TestMaxOpenBoundariesLongestWaitingAndBlockingWait(t *testing.T) {
	h, _, drv := setupRunEnv(t)
	idleUnit = time.Hour

	// Pin max_open <= 0 default (0 -> 4): 3 waiting sessions stay open when 4th starts; 5th evicts oldest waiting.
	putSetting(t, h.Store, "max_open", "0")
	keys := make([]SessionKey, 5)
	for i := range keys {
		keys[i] = SessionKey{Group: runGroup1, Member: "ai-" + strconv.Itoa(i)}
		h.Sessions.Create(makeRunSession(keys[i], GroupSession, t.TempDir()))
	}
	for i := range 4 {
		if err := Runner.Turn(t.Context(), h, keys[i], "hi", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		drv.proc(i).emit(Event{Kind: TurnEnded, Status: "completed"})
		waitForState(t, h, keys[i], Waiting)
	}
	// All 4 are still Waiting (open == 3 < 4 did NOT evict when 4th started).
	for i := range 4 {
		if seamState(t, h.Sessions, keys[i]) != Waiting {
			t.Fatalf("session %d evicted below max_open=4", i)
		}
	}
	// Make keys[2] have the earliest LastActive, and keys[1] & keys[3] tie later.
	setLast := func(k SessionKey, ts int64) {
		h.Sessions.Update(k, func(s *Session) { s.LastActive = ts })
	}
	setLast(keys[0], 500)
	setLast(keys[1], 100)
	setLast(keys[2], 100) // ties with keys[1]; keys[1] is earlier in byKey order.
	setLast(keys[3], 900)

	// Starting keys[4] at open == 4 evicts keys[1] (tie-broken by key order: < vs <= pinned).
	if err := Runner.Turn(t.Context(), h, keys[4], "fifth", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	if seamState(t, h.Sessions, keys[1]) != Dormant || seamState(t, h.Sessions, keys[2]) != Waiting {
		t.Fatalf("tie-break wrong: k1=%s, k2=%s", seamState(t, h.Sessions, keys[1]), seamState(t, h.Sessions, keys[2]))
	}
	drv.proc(4).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, keys[4], Waiting)

	// Now keys[2] has strictly smallest LastActive (100 < 500): waking keys[1] evicts keys[2].
	if err := Runner.Turn(t.Context(), h, keys[1], "wake k1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	if seamState(t, h.Sessions, keys[2]) != Dormant {
		t.Fatalf("strictly oldest waiting session keys[2] was not evicted")
	}

	// Test max_open=1 when open session is Working: second AI waits until first finishes, or cancels on ctx.
	h2, _, drv2 := setupRunEnv(t)
	idleUnit = time.Hour
	putSetting(t, h2.Store, "max_open", "1")
	kA := SessionKey{Group: runGroup1, Member: runAI1}
	kB := SessionKey{Group: runGroup1, Member: runAI2}
	h2.Sessions.Create(makeRunSession(kA, GroupSession, t.TempDir()))
	h2.Sessions.Create(makeRunSession(kB, GroupSession, t.TempDir()))

	if err := Runner.Turn(t.Context(), h2, kA, "a1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	// Context cancellation while waiting for a slot returns context.DeadlineExceeded.
	shortCtx, cancel := context.WithTimeout(t.Context(), 15*time.Millisecond)
	defer cancel()
	if err := Runner.Turn(shortCtx, h2, kB, "b-timeout", Turn{Kind: ItemsTurn}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want DeadlineExceeded", err)
	}

	// Start kB in background; finishing kA moves kA to Waiting, then kB wakes, closes kA to Dormant, and starts kB.
	bDone := make(chan error, 1)
	go func() {
		bDone <- Runner.Turn(t.Context(), h2, kB, "b-run", Turn{Kind: ItemsTurn})
	}()
	time.Sleep(15 * time.Millisecond)
	if drv2.count() != 1 {
		t.Fatal("kB started while kA was still working")
	}
	drv2.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	select {
	case err := <-bDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("kB did not start after kA finished")
	}
	if seamState(t, h2.Sessions, kA) != Dormant || seamState(t, h2.Sessions, kB) != Working {
		t.Fatalf("states: kA=%s kB=%s", seamState(t, h2.Sessions, kA), seamState(t, h2.Sessions, kB))
	}

	// Also test closeWaitingSession when a Waiting row has no liveProc (e.g. seeded directly in session table).
	h3, _, drv3 := setupRunEnv(t)
	putSetting(t, h3.Store, "max_open", "1")
	seamIn(t, h3.Sessions, kA, Waiting)
	h3.Sessions.Create(makeRunSession(kB, GroupSession, t.TempDir()))
	drv3.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete = true
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h3, kB, "evict-proc-less", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	if seamState(t, h3.Sessions, kA) != Dormant {
		t.Fatal("proc-less Waiting session was not moved to Dormant")
	}
	checkSlotGuardOperands(t)
}

func checkSlotGuardOperands(t *testing.T) {
	t.Helper()
	h, _, drv := setupRunEnv(t)
	idleUnit = time.Hour
	rt := stateOf(h)
	k1 := SessionKey{Group: runGroup1, Member: runAI1}
	k2 := SessionKey{Group: runGroup1, Member: runAI2}
	kOtherGrp := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(k1, GroupSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(k2, GroupSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(kOtherGrp, GroupSession, t.TempDir()))

	// Pin maxOpen = -1 -> defaultMaxOpen (4), and rt.starting[k1] counted in countOpenLocked under maxOpen = 1.
	if _, _, ok := tryReserveSlot(h, rt, k1, -1); !ok {
		t.Fatal("maxOpen=-1 should default to 4 and acquire")
	}
	rt.releaseStarting(k1)
	if _, _, ok := tryReserveSlot(h, rt, k1, 1); !ok {
		t.Fatal("expected first slot reservation to succeed")
	}
	if _, waitCh, ok := tryReserveSlot(h, rt, k2, 1); ok || waitCh == nil {
		t.Fatal("rt.starting[k1] did not count as open in countOpenLocked")
	}
	rt.releaseStarting(k1)

	// Start k1 -> Working: checkGroupMemberFree(h, k1) ignores self, and kOtherGrp (different group, same member) succeeds.
	if err := Runner.Turn(t.Context(), h, k1, "run1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	if err := checkGroupMemberFree(h, k1); err != nil {
		t.Fatalf("checkGroupMemberFree matched self: %v", err)
	}
	if err := Runner.Turn(t.Context(), h, kOtherGrp, "run-other-group", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatalf("same AI in different group refused as busy: %v", err)
	}

	// Move k1 -> Waiting, then pin eligibleWaitingLocked & closeWaitingSession when closing or turnActive.
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, k1, Waiting)
	rt.mu.Lock()
	rt.closing[k1] = true
	if longestWaitingLocked(rt, h.Sessions.All()) != nil {
		t.Fatal("closing session selected as waiting victim")
	}
	rt.mu.Unlock()
	closeWaitingSession(h, rt, k1)
	if seamState(t, h.Sessions, k1) != Waiting {
		t.Fatal("closeWaitingSession moved an already-closing session")
	}
	rt.mu.Lock()
	delete(rt.closing, k1)
	rt.procs[k1].turnActive = true
	if longestWaitingLocked(rt, h.Sessions.All()) != nil {
		t.Fatal("turnActive session selected as waiting victim")
	}
	rt.mu.Unlock()
	closeWaitingSession(h, rt, k1)
	if seamState(t, h.Sessions, k1) != Waiting {
		t.Fatal("closeWaitingSession moved a turnActive session")
	}
	rt.mu.Lock()
	rt.procs[k1].turnActive = false
	rt.mu.Unlock()
}

func TestCloseTimeoutAuditAndC6SwitchOrdering(t *testing.T) {
	h, rh, drv := setupRunEnv(t)
	idleUnit = 20 * time.Millisecond
	stopTimeout = 30 * time.Millisecond

	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(key, TaskSession, t.TempDir()))
	putSetting(t, h.Store, "idle_minutes", "1")

	// Process ignores Close(): after stopTimeout, context is cancelled and AuditKilled is written.
	drv.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		if idx == 0 {
			p.ignoreClose = true
		}
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h, key, "hang-on-close", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	rt := stateOf(h)
	rt.mu.Lock()
	p0 := rt.procs[key].proc.(*fakeProc)
	rt.mu.Unlock()
	p0.emit(Event{Kind: Ready, Session: "c6-id"})
	p0.emit(Event{Kind: TurnEnded, Status: "completed"})
	rh.waitCmd(t)

	// Wait for idle close + 30ms kill timeout -> Dormant, AuditKilled recorded, no turn_interrupted or task fail.
	waitForState(t, h, key, Dormant)
	st := rh.waitCmd(t)
	var ms proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(st.raw), &ms); err != nil || ms.State != "dormant" || ms.Event != nil {
		t.Fatalf("idle kill status had unexpected event: %s", st.raw)
	}
	rec := seamRecord(t, h)
	if !strings.Contains(rec, "|"+runGroup1+"|claude|killed|-|-|idle close timed out|-|-") {
		t.Fatalf("missing AuditKilled row, got %q", rec)
	}
	// Planned idle close sends exactly 1 dormant status (classifyProcessExit closing guard).
	rt.wg.Wait()
	if len(rh.snapshotCmds()) != 3 {
		t.Fatalf("expected 3 statuses (working, waiting, dormant), got %d", len(rh.snapshotCmds()))
	}

	// Test C-6 race: Runner.Turn called WHILE session is mid-close waits for old process to exit first (or ctx cancel).
	slowExit := make(chan struct{})
	var eventsLog []string
	var logMu sync.Mutex
	record := func(entry string) {
		logMu.Lock()
		eventsLog = append(eventsLog, entry)
		logMu.Unlock()
	}
	idleUnit = time.Hour
	stopTimeout = 5 * time.Second
	drv.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
		record("start-" + strconv.Itoa(idx))
		p := newFakeProc(ctx, l)
		if idx == 1 {
			p.ignoreClose = true
			go func() {
				<-p.closeCh
				<-slowExit
				record("exit-1")
				p.finishExit()
			}()
		}
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h, key, "turn-before-close", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	p1 := rt.procs[key].proc.(*fakeProc)
	rt.mu.Unlock()
	p1.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, key, Waiting)

	// Begin closing key in background; while closing, test both ctx-cancelled Turn and waiting Turn.
	closedDone := make(chan struct{})
	go func() {
		closeWaitingSession(h, rt, key)
		close(closedDone)
	}()
	<-p1.closeCh

	cancelCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Runner.Turn(cancelCtx, h, key, "aborted", Turn{Kind: ItemsTurn}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want Canceled", err)
	}

	nextTurnDone := make(chan error, 1)
	go func() {
		nextTurnDone <- Runner.Turn(t.Context(), h, key, "after-close", Turn{Kind: ItemsTurn})
	}()
	time.Sleep(15 * time.Millisecond)
	logMu.Lock()
	if slices.Contains(eventsLog, "start-2") {
		t.Fatalf("C-6 violated: start-2 began before exit-1: %v", eventsLog)
	}
	logMu.Unlock()
	close(slowExit)
	<-closedDone
	if err := <-nextTurnDone; err != nil {
		t.Fatal(err)
	}
	logMu.Lock()
	if !slices.Equal(eventsLog, []string{"start-1", "exit-1", "start-2"}) {
		t.Fatalf("unexpected lifecycle order: %v", eventsLog)
	}
	logMu.Unlock()

	// Also pin claimWaitingProc retry when closing races, and (nil, false) when turnActive is already true.
	rt.mu.Lock()
	rt.closing[key] = true
	rt.mu.Unlock()
	if done, err := tryTurnOnWaiting(h, rt, Session{SessionKey: key, State: Waiting}, "race", Turn{Kind: ItemsTurn}); done || err != nil {
		t.Fatalf("expected retry (false, nil) on closing race, got (%v, %v)", done, err)
	}
	rt.mu.Lock()
	delete(rt.closing, key)
	rt.mu.Unlock()
	if gotLP, retry := claimWaitingProc(rt, key, "active", Turn{}); gotLP != nil || retry {
		t.Fatalf("claimWaitingProc claimed active proc: lp=%v retry=%v", gotLP, retry)
	}
}

func TestCrashSecretRedactionLoginInvalidAndReportSaved(t *testing.T) {
	h, rh, drv := setupRunEnv(t)
	idleUnit = time.Hour

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	revKey := SessionKey{Group: runGroup1, Member: runAI2, Task: runTask2}
	grpKey := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(revKey, ReviewSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, t.TempDir()))

	// 1. TaskSession mid-turn exit with secret in event text -> turn_interrupted(crash) + task fail(crash), both redacted.
	if err := Runner.Turn(t.Context(), h, taskKey, "run", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t) // working status
	p0 := drv.proc(0)
	p0.emit(Event{Kind: Ready, Session: "keep-me"})
	p0.emit(Event{Kind: Active, Text: "fatal token " + runSecret})
	p0.emit(Event{Kind: Active, Text: ""})
	p0.finishExit()
	waitForState(t, h, taskKey, Dormant)

	crashStatus := rh.waitCmd(t)
	var msCrash proto.MemberStatus[proto.TurnInterruptedEvent]
	if err := proto.Decode([]byte(crashStatus.raw), &msCrash); err != nil || msCrash.Event == nil {
		t.Fatalf("decode crash status: %v (%s)", err, crashStatus.raw)
	}
	if msCrash.State != "dormant" || msCrash.Event.Reason != "crash" || string(msCrash.Event.Task) != runTask1 || msCrash.Event.Detail != "fatal token [redacted]" {
		t.Fatalf("unexpected crash event: %+v", msCrash)
	}
	failCmd := rh.waitCmd(t)
	var tf proto.TaskFail
	if err := proto.Decode([]byte(failCmd.raw), &tf); err != nil || tf.Op != "fail" || string(tf.ID) != runTask1 || tf.Reason != "crash" || tf.Detail != "fatal token [redacted]" {
		t.Fatalf("unexpected task fail: %+v (%v)", tf, err)
	}
	// Stored CLISession survives a crash so the next turn resumes it.
	if s, _ := h.Sessions.Get(taskKey); s.CLISession != "keep-me" {
		t.Fatalf("CLISession lost after crash: %q", s.CLISession)
	}

	// 2. ReviewSession mid-turn exit -> turn_interrupted with task, no task fail.
	if err := Runner.Turn(t.Context(), h, revKey, "rev", Turn{Kind: ItemsTurn, Thread: runTask2}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	drv.proc(1).finishExit()
	waitForState(t, h, revKey, Dormant)
	revCrash := rh.waitCmd(t)
	var msRev proto.MemberStatus[proto.TurnInterruptedEvent]
	if err := proto.Decode([]byte(revCrash.raw), &msRev); err != nil || msRev.Event == nil || string(msRev.Event.Task) != runTask2 || msRev.Event.Detail != "" {
		t.Fatalf("review crash: %+v (%v)", msRev, err)
	}

	// 3. GroupSession mid-turn exit -> turn_interrupted without task, no task fail.
	if err := Runner.Turn(t.Context(), h, grpKey, "grp", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	drv.proc(2).finishExit()
	waitForState(t, h, grpKey, Dormant)
	grpCrash := rh.waitCmd(t)
	var msGrp proto.MemberStatus[proto.TurnInterruptedEvent]
	if err := proto.Decode([]byte(grpCrash.raw), &msGrp); err != nil || msGrp.Event == nil || msGrp.Event.Task != "" {
		t.Fatalf("group crash: %+v (%v)", msGrp, err)
	}

	// 4. LoginInvalid before exit -> sets limited state in member status, no turn_interrupted crash, no task fail.
	if err := Runner.Turn(t.Context(), h, taskKey, "auth-check", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	p3 := drv.proc(3)
	p3.emit(Event{Kind: LoginInvalid, Text: "not logged in"})
	p3.finishExit()
	waitForState(t, h, taskKey, Dormant)
	limCmd := rh.waitCmd(t)
	var msLim proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(limCmd.raw), &msLim); err != nil || msLim.State != "limited" || msLim.Event != nil {
		t.Fatalf("login_invalid exit status: %+v (%v) raw=%s", msLim, err, limCmd.raw)
	}

	// 5. ReportSaved: queues when home is disconnected, flushes on Attach; ignores non-Working; does not requeue non-unreachable errors.
	checkReportSavedBehavior(t)
}

func checkReportSavedBehavior(t *testing.T) {
	t.Helper()
	h, rh, _ := setupRunEnv(t)
	close(rh.done) // disconnect home-dev.

	saved := []Session{
		{SessionKey: SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}, Kind: TaskSession, State: Working, Home: "home-dev"},
		{SessionKey: SessionKey{Group: runGroup1, Member: runAI2}, Kind: GroupSession, State: Working, Home: "other-home"},
		{SessionKey: SessionKey{Group: runGroup1, Member: runAI1}, Kind: GroupSession, State: Waiting, Home: "home-dev"},
		{SessionKey: SessionKey{Group: runGroup2, Member: runAI1}, Kind: GroupSession, State: Takeover, Home: "home-dev"},
		{SessionKey: SessionKey{Group: runGroup2, Member: runAI2}, Kind: GroupSession, State: Dormant, Home: "home-dev"},
	}
	h.ReportSaved(t.Context(), saved)

	// Attach a home that is already closed -> attachRuntime returns cleanly and re-queues crash report.
	closedHome := newRunHome("direct")
	close(closedHome.done)
	h.Attach("home-dev", closedHome)
	time.Sleep(15 * time.Millisecond)

	// Attach live home-dev: queued TaskSession crash flushes (member status + task fail).
	liveHome := newRunHome("direct")
	h.Attach("home-dev", liveHome)
	c1 := liveHome.waitCmd(t)
	c2 := liveHome.waitCmd(t)
	if c1.payload["t"] != "member" || c2.payload["t"] != "task" || c2.payload["op"] != "fail" {
		t.Fatalf("unexpected flushed saved crash cmds: %v, %v", c1.payload, c2.payload)
	}
	time.Sleep(15 * time.Millisecond)
	if len(liveHome.snapshotCmds()) != 2 {
		t.Fatalf("non-working saved sessions sent unexpected commands: %d", len(liveHome.snapshotCmds()))
	}

	// Non-unreachable error (e.g. conflict) from home does NOT queue into savedCrash.
	refuseHome := newRunHome("local")
	refuseHome.cmdFn = func(map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"refused"}}`), nil
	}
	h.Attach("other-home", refuseHome)
	rt := stateOf(h)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(refuseHome.snapshotCmds()) >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	rt.mu.Lock()
	queued := len(rt.savedCrash["other-home"])
	rt.mu.Unlock()
	if queued != 0 {
		t.Fatalf("conflict error was re-queued as unreachable: %d", queued)
	}
}

func TestNoSessionFailedResumeSyncAsyncAndFailures(t *testing.T) {
	// 1. Async NoSession (Claude style: Turn succeeds, then NoSession arrives on Events).
	h, rh, drv := setupRunEnv(t)
	idleUnit = time.Hour
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	s := makeRunSession(key, TaskSession, t.TempDir())
	s.CLISession = "stale-id"
	h.Sessions.Create(s)

	Compose = func(_ *Host, got Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
		if kind != ReplacedTurn || got.CLISession != "" || items != nil {
			t.Errorf("unexpected Compose args: kind=%s cli=%q items=%v", kind, got.CLISession, items)
		}
		return "replaced prompt with 20 msgs", Turn{Kind: ReplacedTurn}, nil
	}

	if err := Runner.Turn(t.Context(), h, key, "orig text", Turn{Kind: ItemsTurn, Thread: runTask1, Depth: 2, Jump: true}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t) // initial working status
	pOld := drv.proc(0)
	oldTok := tokenFromLaunch(drv.launches[0])
	pOld.emit(Event{Kind: NoSession, Text: "session not found"})

	// Wait for session_replaced status from replacement process.
	replStatus := rh.waitCmd(t)
	var msRepl proto.MemberStatus[proto.SessionReplacedEvent]
	if err := proto.Decode([]byte(replStatus.raw), &msRepl); err != nil || msRepl.Event == nil || msRepl.Event.Code != "session_replaced" || string(msRepl.Event.Task) != runTask1 {
		t.Fatalf("unexpected session_replaced status: %+v (%v)", msRepl, err)
	}
	if _, ok := h.Sessions.Caller(oldTok); ok {
		t.Fatal("old token still valid after NoSession")
	}
	if drv.count() != 2 || drv.lastLaunch().Session != "" {
		t.Fatalf("replacement launch wrong: count=%d session=%q", drv.count(), drv.lastLaunch().Session)
	}
	newTok := tokenFromLaunch(drv.lastLaunch())
	cNew, ok := h.Sessions.Caller(newTok)
	if !ok || cNew.Turn.Kind != ReplacedTurn || !cNew.Turn.Jump || cNew.Turn.Depth != 2 || cNew.Turn.Thread != runTask1 {
		t.Fatalf("replacement Caller.Turn metadata lost: %+v", cNew.Turn)
	}
	pNew := drv.proc(1)
	if got := <-pNew.turnCh; got != "replaced prompt with 20 msgs" {
		t.Fatalf("replacement turn text = %q", got)
	}
	pNew.emit(Event{Kind: Ready, Session: "fresh-id"})
	pNew.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, key, Waiting)
	rh.waitCmd(t)
	if cur, _ := h.Sessions.Get(key); cur.CLISession != "fresh-id" {
		t.Fatalf("CLISession = %q, want fresh-id", cur.CLISession)
	}

	// 2. Sync NoSession (Codex style: Turn returns error while NoSession is emitted; Compose == nil fallback).
	checkSyncNoSessionAndFailurePaths(t)
}

func checkSyncNoSessionAndFailurePaths(t *testing.T) {
	t.Helper()
	h, _, drv := setupRunEnv(t)
	idleUnit = time.Hour
	key := SessionKey{Group: runGroup1, Member: runAI1}
	s := makeRunSession(key, GroupSession, t.TempDir())
	s.CLISession = "stale-codex"
	h.Sessions.Create(s)

	drv.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		if idx == 0 {
			p.turnErr = errors.New("thread not found")
			p.emit(Event{Kind: NoSession, Text: "thread not found"})
		} else {
			p.autoReadyID = "fresh-codex"
			p.autoComplete = true
		}
		return p, nil
	}
	// Compose is nil here -> composeReplacedTurn keeps origText with ReplacedTurn.
	if err := Runner.Turn(t.Context(), h, key, "hello group", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatalf("sync NoSession should succeed via replacement: %v", err)
	}
	waitForState(t, h, key, Waiting)
	if cur, _ := h.Sessions.Get(key); cur.CLISession != "fresh-codex" {
		t.Fatalf("CLISession = %q, want fresh-codex", cur.CLISession)
	}

	// 3. NoSession on a non-resumed start (CLISession == "") is NOT treated as resume replacement.
	hFresh, _, drvFresh := setupRunEnv(t)
	hFresh.Sessions.Create(makeRunSession(key, GroupSession, t.TempDir()))
	drvFresh.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.turnErr = errors.New("broken start")
		p.emit(Event{Kind: NoSession, Text: "ignored on fresh"})
		return p, nil
	}
	if err := Runner.Turn(t.Context(), hFresh, key, "hi", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected error when non-resumed start fails")
	}

	// 4. Replacement failure paths: Compose error, replacement Start error, replacement Turn error, missing session, Update error.
	checkReplacementErrors(t)
}

func checkReplacementErrors(t *testing.T) {
	t.Helper()
	// (a) Compose returns error
	h1, _, drv1 := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	s := makeRunSession(key, GroupSession, t.TempDir())
	s.CLISession = "bad-1"
	h1.Sessions.Create(s)
	Compose = func(*Host, Session, TurnKind, []json.RawMessage) (string, Turn, error) {
		return "", Turn{}, errors.New("compose failed")
	}
	drv1.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.turnErr = errors.New("no thread")
		p.emit(Event{Kind: NoSession})
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h1, key, "hi", Turn{Kind: ItemsTurn}); err == nil || err.Error() != "compose failed" {
		t.Fatalf("got %v, want compose failed", err)
	}
	waitForState(t, h1, key, Dormant)

	// (b) Replacement Start returns error, and (c) replacement Turn returns error
	for _, failSecondStart := range []bool{true, false} {
		h2, _, drv2 := setupRunEnv(t)
		s2 := makeRunSession(key, GroupSession, t.TempDir())
		s2.CLISession = "bad-2"
		h2.Sessions.Create(s2)
		drv2.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
			if idx == 1 && failSecondStart {
				return nil, errors.New("second start failed")
			}
			p := newFakeProc(ctx, l)
			p.turnErr = errors.New("turn write failed")
			if idx == 0 {
				p.emit(Event{Kind: NoSession})
			}
			return p, nil
		}
		if err := Runner.Turn(t.Context(), h2, key, "hi", Turn{Kind: ItemsTurn}); err == nil {
			t.Fatal("expected replacement failure")
		}
		waitForState(t, h2, key, Dormant)
	}

	// (d) Session ended while NoSession was draining, and (e) Update trigger error
	h3, _, drv3 := setupRunEnv(t)
	s3 := makeRunSession(key, GroupSession, t.TempDir())
	s3.CLISession = "bad-3"
	h3.Sessions.Create(s3)
	drv3.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.turnErr = errors.New("no session")
		p.emit(Event{Kind: NoSession})
		h3.Sessions.End(key)
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h3, key, "hi", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected not_found when session ended during NoSession")
	}

	h4, _, drv4 := setupRunEnv(t)
	s4 := makeRunSession(key, GroupSession, t.TempDir())
	s4.CLISession = "bad-4"
	h4.Sessions.Create(s4)
	drv4.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.turnErr = errors.New("no session")
		p.emit(Event{Kind: NoSession})
		seamSQL(t, h4.Store, "CREATE TRIGGER block_upd BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'upd blocked'); END")
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h4, key, "hi", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected Update error during NoSession")
	}
}

func TestJumpPreemptResumeSameSessionAndInterruptTimeout(t *testing.T) {
	h, rh, drv := setupRunEnv(t)
	idleUnit = time.Hour

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, t.TempDir()))

	// Seed human & AI member rows + FetchThread handler so resolveJumpMsg exercises every filter branch.
	humanRaw, _ := json.Marshal(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: runHuman1, Seq: 1}, Group: runGroup1, Kind: "human", User: "u1", Name: "Owner"},
		Role:      "owner",
	})
	aiRaw, _ := json.Marshal(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: runAI1, Seq: 2}, Group: runGroup1, Kind: "ai", User: "u1", Name: "Claude"},
		CLI:       "claude", Host: "h1", Allow: []string{}, Approvers: []string{},
	})
	h.Rows.Put(runGroup1, "member", runHuman1, humanRaw)
	h.Rows.Put(runGroup1, "member", runAI1, aiRaw)

	rh.fetchFn = func(req any) (json.RawMessage, error) {
		ft, ok := req.(proto.FetchThread)
		if !ok || ft.Thread != nil {
			t.Errorf("expected main-line FetchThread (Thread == nil), got %+v", req)
		}
		rows := []json.RawMessage{
			json.RawMessage(`{"t":"not-a-msg"}`),
			mustMarshalMsg(runSysMsg, "system", "sys", nil),
			mustMarshalMsg(runAIMsg, runAI1, "ai msg", nil),
			mustMarshalMsg(runMsg2, runHuman1, "newer unrelated msg", nil),
			mustMarshalMsg(runMsg1, runHuman1, "urgent question", nil),
		}
		return mustMarshalFetched(rows), nil
	}

	// 1. Start task turn, then jump from group session -> interrupts task, sends task preempted (Msg=runMsg1), runs jump, then sends task resume + ContinueTurn.
	if err := Runner.Turn(t.Context(), h, taskKey, "work on task", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t) // task working

	jumpText := "[SoloDawn] header\n== 1 of 1 · mention\nurgent question"
	if err := Runner.Turn(t.Context(), h, grpKey, jumpText, Turn{Kind: ItemsTurn, Jump: true}); err != nil {
		t.Fatal(err)
	}
	if drv.proc(0).interrupts != 1 {
		t.Fatalf("task interrupts = %d, want 1", drv.proc(0).interrupts)
	}

	// Drain until we see task preempted with Msg == runMsg1.
	sawPreempt := false
	for !sawPreempt {
		c := rh.waitCmd(t)
		if c.payload["t"] == "task" && c.payload["op"] == "preempted" {
			var tp proto.TaskPreempted
			if err := proto.Decode([]byte(c.raw), &tp); err != nil || string(tp.ID) != runTask1 || string(tp.Msg) != runMsg1 {
				t.Fatalf("unexpected TaskPreempted: %+v (%v)", tp, err)
			}
			sawPreempt = true
		}
	}
	rh.waitCmd(t) // group working

	// Second jump on the same running session (grpKey) while grpKey is working: interrupts grpKey, no task preempted, preserves taskKey as the interrupted session.
	if err := Runner.Turn(t.Context(), h, grpKey, "follow-up jump", Turn{Kind: ItemsTurn, Jump: true}); err != nil {
		t.Fatal(err)
	}
	if drv.proc(1).interrupts != 1 {
		t.Fatalf("same-session jump did not interrupt grpKey: %d", drv.proc(1).interrupts)
	}

	// Complete the jump turn in grpKey -> triggers task resume + ContinueTurn on taskKey.
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})

	sawResume := false
	for !sawResume {
		c := rh.waitCmd(t)
		if c.payload["t"] == "task" && c.payload["op"] == "resume" {
			var tr proto.TaskCommand
			if err := proto.Decode([]byte(c.raw), &tr); err != nil || string(tr.ID) != runTask1 {
				t.Fatalf("unexpected task resume: %+v (%v)", tr, err)
			}
			sawResume = true
		}
	}
	waitForState(t, h, taskKey, Working)
	<-drv.proc(0).turnCh // first turn
	contGot := <-drv.proc(0).turnCh
	if contGot != "You were interrupted by a direct request from your owner. Continue the task." {
		t.Fatalf("unexpected ContinueTurn text: %q", contGot)
	}
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, taskKey, Waiting)

	checkJumpNonTaskRefusedAndTimeout(t)
}

func checkJumpNonTaskRefusedAndTimeout(t *testing.T) {
	t.Helper()
	// (a) Jump interrupting a GroupSession -> no task preempted, runs ContinueTurn ("... Continue.").
	h, rh, drv := setupRunEnv(t)
	idleUnit = time.Hour
	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	revKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask2}
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(revKey, ReviewSession, t.TempDir()))

	if err := Runner.Turn(t.Context(), h, grpKey, "chatting", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	<-drv.proc(0).turnCh

	if err := Runner.Turn(t.Context(), h, revKey, "urgent review", Turn{Kind: ItemsTurn, Thread: runTask2, Jump: true}); err != nil {
		t.Fatal(err)
	}
	<-drv.proc(1).turnCh
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})
	contGrp := <-drv.proc(0).turnCh
	if contGrp != "You were interrupted by a direct request from your owner. Continue." {
		t.Fatalf("group continue text = %q", contGrp)
	}
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, grpKey, Waiting)

	for _, c := range rh.snapshotCmds() {
		if c.payload["t"] == "task" {
			t.Fatalf("unexpected task command for non-task jump: %v", c.payload)
		}
	}

	// (b) Jump when AI has no running turn -> runs directly; 10 s interrupt timeout kills unresponsive process.
	h2, rh2, drv2 := setupRunEnv(t)
	stopTimeout = 25 * time.Millisecond
	idleUnit = time.Hour
	delivered := make(chan struct{}, 4)
	Deliver = func(*Host, string, string) { delivered <- struct{}{} }
	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h2.Sessions.Create(makeRunSession(taskKey, TaskSession, t.TempDir()))
	h2.Sessions.Create(makeRunSession(grpKey, GroupSession, t.TempDir()))

	// Jump when idle:
	if err := Runner.Turn(t.Context(), h2, grpKey, "idle jump", Turn{Kind: ItemsTurn, Jump: true}); err != nil {
		t.Fatal(err)
	}
	drv2.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h2, grpKey, Waiting)
	<-delivered

	// Start unresponsive task turn (ignoreInterrupt = true); home refuses task preempted; task row becomes cancelled -> skips ContinueTurn and calls Deliver.
	drv2.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.ignoreInterrupt = true
		return p, nil
	}
	rh2.fetchFn = func(any) (json.RawMessage, error) {
		return mustMarshalFetched([]json.RawMessage{
			mustMarshalMsg(runMsg1, runHuman1, "kill stuck turn", nil),
		}), nil
	}
	rh2.cmdFn = func(p map[string]any) (json.RawMessage, error) {
		if p["t"] == "task" && p["op"] == "preempted" {
			return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"at merge"}}`), nil
		}
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}

	if err := Runner.Turn(t.Context(), h2, taskKey, "stuck turn", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	// Mark task cancelled in Rows so prepareInterruptedResume sees inactive task after jump ends.
	cancelledTask, _ := json.Marshal(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: runTask1, Seq: 5},
		Group:     runGroup1, Num: 1, Title: "T", Slug: "t", Origin: "direct", State: "cancelled", Step: "merge", Assignee: runAI1, Depends: []string{},
	})
	h2.Rows.Put(runGroup1, "task", runTask1, cancelledTask)

	drv2.makeProc = nil
	if err := Runner.Turn(t.Context(), h2, grpKey, "kill stuck turn", Turn{Kind: ItemsTurn, Thread: runTask1, Jump: true}); err != nil {
		t.Fatal(err)
	}
	// Stuck task process was killed -> Dormant.
	if seamState(t, h2.Sessions, taskKey) != Dormant {
		t.Fatalf("stuck task session = %s, want dormant", seamState(t, h2.Sessions, taskKey))
	}
	drv2.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("Deliver not called when ContinueTurn skipped")
	}

	checkJumpEdgeHelpers(t)
}

func checkJumpEdgeHelpers(t *testing.T) {
	t.Helper()
	h, rh, _ := setupRunEnv(t)
	interrupted := makeRunSession(SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}, TaskSession, t.TempDir())

	// Envelope-based AI vs human detection when member rows are absent:
	aiCmd, _ := json.Marshal(proto.HomeCommand{Type: "post", Nonce: runMsg1, Timestamp: seamStart, Group: runGroup1, As: runAI1})
	aiEnv := seamKey().Sign(string(aiCmd))
	humanCmd, _ := json.Marshal(proto.HomeCommand{Type: "post", Nonce: runMsg2, Timestamp: seamStart, Group: runGroup1})
	humanEnv := seamKey().Sign(string(humanCmd))
	rh.fetchFn = func(req any) (json.RawMessage, error) {
		ft := req.(proto.FetchThread)
		if ft.Thread == nil || *ft.Thread != runTask1 {
			t.Fatalf("expected task thread %s, got %+v", runTask1, ft.Thread)
		}
		return mustMarshalFetched([]json.RawMessage{
			mustMarshalMsg(runAIMsg, runAI1, "from ai", &aiEnv),
			mustMarshalMsg(runMsg2, runHuman1, "fallback human", &humanEnv),
		}), nil
	}
	if got := resolveJumpMsg(t.Context(), h, interrupted, Turn{Thread: runTask1}, "no exact match"); string(got) != runMsg2 {
		t.Fatalf("fallback jump msg = %q, want %q", got, runMsg2)
	}

	// Fetch error & non-Fetched JSON & disconnected home:
	rh.fetchFn = func(any) (json.RawMessage, error) { return json.RawMessage(`{"re":1,"ok":true}`), nil }
	if got := resolveJumpMsg(t.Context(), h, interrupted, Turn{}, "x"); got != "" {
		t.Fatalf("got %q on non-fetched response", got)
	}
	rh.fetchFn = func(any) (json.RawMessage, error) { return nil, errors.New("boom") }
	if got := resolveJumpMsg(t.Context(), h, interrupted, Turn{}, "x"); got != "" {
		t.Fatalf("got %q on fetch error", got)
	}
	close(rh.done)
	if got := resolveJumpMsg(t.Context(), h, interrupted, Turn{}, "x"); got != "" {
		t.Fatalf("got %q on closed home", got)
	}

	// claimJumpInterrupt when session row is Working without a liveProc; and resumeInterruptedSession failure cases:
	h2, rh2, _ := setupRunEnv(t)
	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	seamIn(t, h2.Sessions, taskKey, Working)
	rt2 := stateOf(h2)
	prepareJumpTurn(t.Context(), h2, rt2, aiSlot{runGroup1, runAI1}, SessionKey{Group: runGroup1, Member: runAI1}, "j", Turn{Jump: true})
	rt2.clearJumping(aiSlot{runGroup1, runAI1})

	// Hold / offline / paused / DB error / Compose error / task resume refused in resumeInterruptedSession:
	Holds["cap"] = func(*Host, SessionKey) (string, bool) { return "cap", true }
	if resumeInterruptedSession(h2, preemptedTurn{key: taskKey, preempted: true}) {
		t.Fatal("expected held session not to resume")
	}
	delete(Holds, "cap")
	putSetting(t, h2.Store, "offline", "true")
	if !isContinueHeld(h2, Session{SessionKey: taskKey, Agent: "claude"}) {
		t.Fatal("expected offline setting to hold continue")
	}
	putSetting(t, h2.Store, "offline", "false")
	pausedGrp, _ := json.Marshal(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 1},
		Name:      "W", Kind: "project", Base: "main", Forge: "none", Home: "home-dev",
		Checks: []string{}, Paused: &proto.GroupPause{By: runHuman1, At: 1},
	})
	h2.Rows.Put(runGroup1, "group", runGroup1, pausedGrp)
	if !isContinueHeld(h2, Session{SessionKey: taskKey, Agent: "claude"}) {
		t.Fatal("expected paused group to hold continue")
	}
	h2.Rows.Reset(runGroup1)
	if isContinueHeld(h2, Session{SessionKey: taskKey, Agent: "claude"}) {
		t.Fatal("expected unheld continue to return false")
	}

	rh2.cmdFn = func(map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"paused"}}`), nil
	}
	if resumeInterruptedSession(h2, preemptedTurn{key: taskKey, preempted: true}) {
		t.Fatal("expected refused task resume to stop ContinueTurn")
	}
	rh2.cmdFn = nil
	Compose = func(*Host, Session, TurnKind, []json.RawMessage) (string, Turn, error) {
		return "", Turn{}, errors.New("bad compose")
	}
	if resumeInterruptedSession(h2, preemptedTurn{key: taskKey, preempted: false}) {
		t.Fatal("expected Compose error to return false")
	}
	Compose = nil

	// Pin matchesTurnTail empty msgText, decodePersonMsg malformed member row, isAIEnvelope malformed JSON with As, recordInterrupted !exists, and isTaskInactive operands:
	if matchesTurnTail("header\n", "") {
		t.Fatal("empty msgText matched trailing newline")
	}
	badMemberID := "5wZ8nH4jCA9mL3vB7xQ2kP"
	h2.Rows.Put(runGroup1, "member", badMemberID, json.RawMessage(`{"t":"member","id":"`+badMemberID+`","seq":1,"g":"`+runGroup1+`","kind":"human","extra":1}`))
	if _, ok := decodePersonMsg(h2, runGroup1, mustMarshalMsg(runMsg1, badMemberID, "hi", nil)); ok {
		t.Fatal("malformed member row accepted as human")
	}
	badEnv := seamKey().Sign(`{"t":"post","as":"` + runAI1 + `","n":"bad-id","ts":1,"g":"` + runGroup1 + `"}`)
	if isAIEnvelope(&badEnv) {
		t.Fatal("malformed envelope payload accepted as AI")
	}
	slot := aiSlot{runGroup1, runAI1}
	rt2.nextAfterTurn(slot)
	rt2.recordInterrupted(slot, taskKey, true)
	rt2.recordInterrupted(slot, SessionKey{Group: runGroup1, Member: runAI1}, false)
	if _, got, ok := rt2.nextAfterTurn(slot); !ok || got.key != taskKey || !got.preempted {
		t.Fatalf("recordInterrupted overwrote original interrupted session: %+v (ok=%v)", got, ok)
	}

	// Non-existent, active, ReviewSession-with-cancelled-row, and malformed-cancelled-row in isTaskInactive:
	if isTaskInactive(h2, Session{SessionKey: taskKey, Kind: TaskSession}) {
		t.Fatal("missing task row treated as inactive")
	}
	activeTask, _ := json.Marshal(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: runTask1, Seq: 1},
		Group:     runGroup1, Num: 1, Title: "T", Slug: "t", Origin: "direct", State: "active", Step: "implement", Assignee: runAI1, Depends: []string{},
	})
	h2.Rows.Put(runGroup1, "task", runTask1, activeTask)
	if isTaskInactive(h2, Session{SessionKey: taskKey, Kind: TaskSession}) {
		t.Fatal("active task row treated as inactive")
	}
	h2.Rows.Put(runGroup1, "task", runTask1, json.RawMessage(`{"t":"task","id":"`+runTask1+`","seq":2,"g":"`+runGroup1+`","num":1,"title":"T","slug":"t","origin":"direct","state":"cancelled","step":"implement","assignee":"`+runAI1+`","depends":[],"extra":1}`))
	if isTaskInactive(h2, Session{SessionKey: taskKey, Kind: TaskSession}) {
		t.Fatal("malformed task row treated as inactive")
	}
	cancelledTask, _ := json.Marshal(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: runTask1, Seq: 3},
		Group:     runGroup1, Num: 1, Title: "T", Slug: "t", Origin: "direct", State: "cancelled", Step: "implement", Assignee: runAI1, Depends: []string{},
	})
	h2.Rows.Put(runGroup1, "task", runTask1, cancelledTask)
	if isTaskInactive(h2, Session{SessionKey: taskKey, Kind: ReviewSession}) {
		t.Fatal("ReviewSession treated as inactive task")
	}
	seamSQL(t, h2.Store, "DROP TABLE setting")
	if !isContinueHeld(h2, Session{SessionKey: taskKey, Agent: "claude"}) {
		t.Fatal("expected DB error in isContinueHeld to return true")
	}
}

func TestMemberStatusPriorityPathWatcherAnswerAndRefusals(t *testing.T) {
	h, _, drv := setupRunEnv(t)
	rt := stateOf(h)
	slot := aiSlot{runGroup1, runAI1}

	kGrp := SessionKey{Group: runGroup1, Member: runAI1}
	kT1 := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	kT2 := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask2}
	h.Sessions.Create(makeRunSession(kGrp, GroupSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(kT1, TaskSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(kT2, TaskSession, t.TempDir()))
	// Seed Takeover sessions in another group and another member to pin memberSessions filter.
	seamIn(t, h.Sessions, SessionKey{Group: runGroup2, Member: runAI1}, Takeover)
	seamIn(t, h.Sessions, SessionKey{Group: runGroup1, Member: runAI2}, Takeover)

	// 7. dormant when all sessions of (runGroup1, runAI1) are dormant
	cfg, _ := readRunConfig(h, "", runGroup1)
	if st, tk := deriveMemberStatus(h, rt, cfg, slot); st != "dormant" || tk != "" {
		t.Fatalf("got %s/%s, want dormant", st, tk)
	}

	// 6. waiting picks highest LastActive (including LastActive == 0 for !found, and > vs >= pinned on tie).
	h.Sessions.Move(kT1, Dormant, Working)
	h.Sessions.Move(kT1, Working, Waiting)
	h.Sessions.Update(kT1, func(s *Session) { s.LastActive = 0 })
	if st, tk := deriveMemberStatus(h, rt, cfg, slot); st != "waiting" || tk != runTask1 {
		t.Fatalf("zero LastActive waiting got %s/%s, want waiting/%s", st, tk, runTask1)
	}
	h.Sessions.Move(kT2, Dormant, Working)
	h.Sessions.Move(kT2, Working, Waiting)
	h.Sessions.Update(kT1, func(s *Session) { s.LastActive = 200 })
	h.Sessions.Update(kT2, func(s *Session) { s.LastActive = 200 })
	// Tie: first in byKey order (runTask2 '6h...' < runTask1 'Dr...') wins.
	if st, tk := deriveMemberStatus(h, rt, cfg, slot); st != "waiting" || tk != runTask2 {
		t.Fatalf("tie waiting got %s/%s, want waiting/%s", st, tk, runTask2)
	}
	h.Sessions.Update(kT1, func(s *Session) { s.LastActive = 300 })
	if st, tk := deriveMemberStatus(h, rt, cfg, slot); st != "waiting" || tk != runTask1 {
		t.Fatalf("latest waiting got %s/%s, want waiting/%s", st, tk, runTask1)
	}

	// 5. paused (via GroupRow.Paused and via Holds "paused") overrides waiting.
	pausedGrp, _ := json.Marshal(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 1},
		Name:      "W", Kind: "project", Base: "main", Forge: "none", Home: "home-dev",
		Checks: []string{}, Paused: &proto.GroupPause{By: runHuman1, At: 1},
	})
	h.Rows.Put(runGroup1, "group", runGroup1, pausedGrp)
	if st, tk := deriveMemberStatus(h, rt, cfg, slot); st != "paused" || tk != "" {
		t.Fatalf("got %s/%s, want paused", st, tk)
	}
	h.Rows.Reset(runGroup1)
	Holds["p"] = func(_ *Host, k SessionKey) (string, bool) {
		return "paused", k == kT1 // session-level hold
	}
	if st, _ := deriveMemberStatus(h, rt, cfg, slot); st != "paused" {
		t.Fatalf("got %s, want paused from session hold", st)
	}

	// 4. limited overrides paused.
	Holds["a_lim"] = func(*Host, SessionKey) (string, bool) { return "cap_reached", true }
	if st, tk := deriveMemberStatus(h, rt, cfg, slot); st != "limited" || tk != "" {
		t.Fatalf("got %s/%s, want limited", st, tk)
	}

	// 3. offline (both setting and Holds) overrides limited.
	if st, _ := deriveMemberStatus(h, rt, runConfig{offline: true}, slot); st != "offline" {
		t.Fatalf("got %s, want offline from setting", st)
	}
	Holds["0_off"] = func(*Host, SessionKey) (string, bool) { return "offline", true }
	if st, _ := deriveMemberStatus(h, rt, cfg, slot); st != "offline" {
		t.Fatalf("got %s, want offline from hold", st)
	}

	// 2. working overrides offline.
	h.Sessions.Move(kT1, Waiting, Working)
	if st, tk := deriveMemberStatus(h, rt, runConfig{offline: true}, slot); st != "working" || tk != runTask1 {
		t.Fatalf("got %s/%s, want working/%s", st, tk, runTask1)
	}
	h.Sessions.Move(kT1, Working, Dormant)

	// 1. takeover overrides everything.
	h.Sessions.Move(kT2, Waiting, Takeover)
	if st, tk := deriveMemberStatus(h, rt, runConfig{offline: true}, slot); st != "takeover" || tk != runTask2 {
		t.Fatalf("got %s/%s, want takeover/%s", st, tk, runTask2)
	}
	delete(Holds, "p")
	delete(Holds, "a_lim")
	delete(Holds, "0_off")

	// HandbackTurn on Takeover session starts structured CLI (Takeover -> Working); non-handback on Takeover is conflict.
	seamCode(t, Runner.Turn(t.Context(), h, kT2, "normal", Turn{Kind: ItemsTurn}), "conflict")
	if err := Runner.Turn(t.Context(), h, kT2, "handback summary", Turn{Kind: HandbackTurn, Thread: runTask2}); err != nil {
		t.Fatalf("HandbackTurn from Takeover failed: %v", err)
	}

	// Answer: forwards allow=true and allow=false, propagates error, refuses missing session or closed proc.
	p := drv.proc(0)
	if err := Runner.Answer(h, kT2, "ask-1", true); err != nil {
		t.Fatal(err)
	}
	p.answerErr = errors.New("rpc closed")
	if err := Runner.Answer(h, kT2, "ask-2", false); err == nil || err.Error() != "rpc closed" {
		t.Fatalf("got %v, want rpc closed", err)
	}
	if !slices.Equal(p.answers, []recordedAsk{{"ask-1", true}, {"ask-2", false}}) {
		t.Fatalf("recorded answers: %+v", p.answers)
	}
	seamCode(t, Runner.Answer(h, SessionKey{Group: "none"}, "a", true), "not_found")
	seamCode(t, Runner.Answer(h, kGrp, "a", true), "not_found")

	checkPathWatcherAndTurnErrors(t)
}

func checkPathWatcherAndTurnErrors(t *testing.T) {
	t.Helper()
	h, _, drv := setupRunEnv(t)
	kGrp := SessionKey{Group: runGroup1, Member: runAI1}
	kTask := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(kGrp, GroupSession, t.TempDir()))
	h.Sessions.Create(makeRunSession(kTask, TaskSession, t.TempDir()))

	// PathChanged watcher: ignores unchanged path, reports once per AI on changed path (and ignores other homes), stops on replacement, Done(), and closed channel.
	otherHomeSess := makeRunSession(SessionKey{Group: runGroup2, Member: "m-other-home"}, GroupSession, t.TempDir())
	otherHomeSess.Home = "other-dev"
	h.Sessions.Create(otherHomeSess)
	ph1 := newRunHome("relay")
	ph1.pathCh = make(chan struct{}, 4)
	h.Attach("home-dev", ph1)

	ph1.setPath("relay") // unchanged -> no status sent.
	time.Sleep(15 * time.Millisecond)
	if len(ph1.snapshotCmds()) != 0 {
		t.Fatal("unchanged path triggered status")
	}
	ph1.setPath("direct") // changed -> 1 status for (runGroup1, runAI1) across its 2 sessions.
	stCmd := ph1.waitCmd(t)
	var ms proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(stCmd.raw), &ms); err != nil || ms.Path != "direct" {
		t.Fatalf("path watcher status: %+v (%v)", ms, err)
	}

	// Replace with ph2 (cancels ph1's watcher), verify ph1 path change no longer fires, then close ph2.pathCh; attach ph3 and close ph3.done.
	time.Sleep(10 * time.Millisecond)
	if len(ph1.snapshotCmds()) != 1 {
		t.Fatalf("expected exactly 1 status on ph1, got %d", len(ph1.snapshotCmds()))
	}
	ph2 := newRunHome("local")
	ph2.pathCh = make(chan struct{}, 2)
	h.Attach("home-dev", ph2)
	time.Sleep(10 * time.Millisecond)
	ph1.setPath("stale-path")
	time.Sleep(10 * time.Millisecond)
	if len(ph2.snapshotCmds()) != 0 {
		t.Fatal("cancelled ph1 watcher triggered status on ph2")
	}
	close(ph2.pathCh)
	time.Sleep(10 * time.Millisecond)

	ph3 := newRunHome("local")
	ph3.pathCh = make(chan struct{}, 2)
	h.Attach("home-dev", ph3)
	close(ph3.done)
	time.Sleep(10 * time.Millisecond)
	h.Attach("home-dev", newRunHome("local"))

	// Turn error cases: missing session, unknown agent, Start error, busy AI, Waiting Move failure, Waiting Turn write failure, DB read error.
	seamCode(t, Runner.Turn(t.Context(), h, SessionKey{Group: "missing"}, "x", Turn{Kind: ItemsTurn}), "not_found")

	badAgentKey := SessionKey{Group: runGroup2, Member: runAI2}
	badSess := makeRunSession(badAgentKey, GroupSession, t.TempDir())
	badSess.Agent = "unknown-cli"
	h.Sessions.Create(badSess)
	seamCode(t, Runner.Turn(t.Context(), h, badAgentKey, "x", Turn{Kind: ItemsTurn}), "bad_request")

	drv.startErr = errors.New("exec failed")
	if err := Runner.Turn(t.Context(), h, kGrp, "x", Turn{Kind: ItemsTurn}); err == nil || err.Error() != "exec failed" {
		t.Fatalf("got %v, want exec failed", err)
	}
	drv.startErr = nil

	// Start kGrp -> Working; starting kTask of same AI in same group returns busy.
	if err := Runner.Turn(t.Context(), h, kGrp, "ok", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	seamCode(t, Runner.Turn(t.Context(), h, kTask, "busy", Turn{Kind: ItemsTurn}), "busy")
	// Finish kGrp -> Waiting; start kTask -> Working; trying kGrp (Waiting -> Working) while kTask is Working hits writeTurnToWaiting Move error.
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, kGrp, Waiting)
	if err := Runner.Turn(t.Context(), h, kTask, "ok-task", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	seamCode(t, Runner.Turn(t.Context(), h, kGrp, "busy-from-waiting", Turn{Kind: ItemsTurn}), "busy")
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, kTask, Waiting)

	// Waiting process whose Turn() write fails crashes cleanly and returns the write error.
	drv.proc(0).mu.Lock()
	drv.proc(0).turnErr = errors.New("broken pipe " + runSecret)
	drv.proc(0).mu.Unlock()
	if err := Runner.Turn(t.Context(), h, kGrp, "fail-write", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected broken pipe error")
	}
	waitForState(t, h, kGrp, Dormant)

	// Waiting session with no liveProc: first Move(Waiting, Dormant) fails due to trigger, then after dropping trigger transitions Waiting -> Dormant -> Working; also Dormant + HandbackTurn succeeds.
	kOrphan := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(kOrphan, GroupSession, t.TempDir()))
	h.Sessions.Move(kOrphan, Dormant, Working)
	h.Sessions.Move(kOrphan, Working, Waiting)
	seamSQL(t, h.Store, "CREATE TRIGGER stop_move BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'move blocked'); END")
	if err := Runner.Turn(t.Context(), h, kOrphan, "x", Turn{Kind: ItemsTurn}); err == nil || !strings.Contains(err.Error(), "move blocked") {
		t.Fatalf("got %v, want move blocked", err)
	}
	seamSQL(t, h.Store, "DROP TRIGGER stop_move")
	if err := Runner.Turn(t.Context(), h, kOrphan, "recover", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatalf("orphan Waiting session recovery failed: %v", err)
	}
	drv.lastProc().emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, kOrphan, Waiting)

	kHbDorm := SessionKey{Group: runGroup2, Member: "m-hb-dorm", Task: runTask1}
	h.Sessions.Create(makeRunSession(kHbDorm, TaskSession, t.TempDir()))
	if err := Runner.Turn(t.Context(), h, kHbDorm, "hb-from-dormant", Turn{Kind: HandbackTurn, Thread: runTask1}); err != nil {
		t.Fatalf("HandbackTurn from Dormant failed: %v", err)
	}

	// StartFromState Move failure & DB setting drop error in acquireOpenSlot / launchSessionProcess:
	kDorm := SessionKey{Group: runGroup2, Member: "m-dorm"}
	h.Sessions.Create(makeRunSession(kDorm, GroupSession, t.TempDir()))
	seamSQL(t, h.Store, "CREATE TRIGGER stop_start BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'start move blocked'); END")
	if err := Runner.Turn(t.Context(), h, kDorm, "x", Turn{Kind: ItemsTurn}); err == nil || !strings.Contains(err.Error(), "start move blocked") {
		t.Fatalf("got %v, want start move blocked", err)
	}
	seamSQL(t, h.Store, "DROP TRIGGER stop_start", "DROP TABLE setting")
	if err := Runner.Turn(t.Context(), h, kDorm, "x", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected error when setting table is dropped")
	}
	// Cover launchSessionProcess readRunConfig error on HandbackTurn (skips acquireOpenSlot).
	kTake := SessionKey{Group: "g-take", Member: "m-take", Task: runTask1}
	hTake, _, _ := setupRunEnv(t)
	seamIn(t, hTake.Sessions, kTake, Takeover)
	seamSQL(t, hTake.Store, "DROP TABLE perm")
	if err := Runner.Turn(t.Context(), hTake, kTake, "hb", Turn{Kind: HandbackTurn}); err == nil {
		t.Fatal("expected readRunConfig error on dropped perm table")
	}
}

func mustMarshalMsg(id, by, text string, env *proto.Envelope) json.RawMessage {
	row := proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: id, Seq: 1},
		Pos:       1, Group: runGroup1, Main: true, By: by, Kind: "text",
		Timestamp: seamStart, Text: text, Mentions: []string{}, Body: proto.EmptyBody{}, Env: env,
	}
	b, _ := json.Marshal(row)
	return b
}

func mustMarshalFetched(rows []json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: 1, OK: true},
		Rows:    rows,
	})
	return b
}
