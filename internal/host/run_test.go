package host

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
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
	runTask1  = "DrtCn6htSBwmMPrFPbHJHA"
	runTask2  = "6hJ9kL4mP8rT2vX5zB7nQA"
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
	var p *fakeProc
	if maker != nil {
		var mErr error
		p, mErr = maker(ctx, idx, l)
		if mErr != nil {
			return nil, mErr
		}
	} else {
		p = newFakeProc(ctx, l)
	}
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
	return len(d.procs)
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

func TestFakeDriverCountsAStartOnlyWithItsProcess(t *testing.T) {
	entered, release, started := make(chan struct{}), make(chan struct{}), make(chan Process)
	want := &fakeProc{}
	drv := &fakeDriver{makeProc: func(context.Context, int, Launch) (*fakeProc, error) {
		close(entered)
		<-release
		return want, nil
	}}
	go func() {
		p, _ := drv.Start(t.Context(), Launch{})
		started <- p
	}()
	<-entered
	assertEqual(t, drv.count(), 0)
	close(release)
	assertEqual(t, <-started, Process(want))
	assertEqual(t, drv.count(), 1)
	assertEqual(t, drv.lastProc(), want)
}

type recordedAsk struct {
	id    string
	allow bool
}

type fakeProc struct {
	ctx          context.Context
	launch       Launch
	mu           sync.Mutex
	turns        []string
	answers      []recordedAsk
	turnErr      error
	answerErr    error
	ignoreClose  bool
	autoComplete bool
	autoReadyID  string
	interrupts   atomic.Int32
	closedIn     bool
	eventsClosed bool
	events       chan Event
	turnCh       chan string
	closeCh      chan struct{}
	exitedCh     chan struct{}
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
	p.interrupts.Add(1)
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
	done      chan struct{}
	cmds      []recordedCmd
	cmdNotify chan recordedCmd
	cmdFn     func(payload map[string]any) (json.RawMessage, error)
}

func newRunHome(path string) *runHome {
	return &runHome{
		path:      path,
		done:      make(chan struct{}),
		cmdNotify: make(chan recordedCmd, 64),
	}
}

func (rh *runHome) Send(any) error               { return nil }
func (rh *runHome) Path() string                 { return rh.path }
func (rh *runHome) PathChanged() <-chan struct{} { return nil }
func (rh *runHome) Done() <-chan struct{}        { return rh.done }

func (rh *runHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	rh.mu.Lock()
	id := int64(len(rh.cmds) + 1)
	cmdFn := rh.cmdFn
	rh.mu.Unlock()
	cmdFrame := build(id).(proto.Cmd)
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

var activeTestHosts []*Host

func stopAllTestHosts() {
	for _, h := range activeTestHosts {
		stopHostNow(h)
	}
	activeTestHosts = nil
}

func saveRuntimeGlobals(t *testing.T) {
	t.Helper()
	prevDrivers := maps.Clone(Drivers)
	prevListeners := maps.Clone(Listeners)
	prevHolds := maps.Clone(Holds)
	prevDeliver := Deliver
	prevInstructions := Instructions
	prevCompose := Compose
	t.Cleanup(func() {
		stopAllTestHosts()
		Drivers = prevDrivers
		Listeners = prevListeners
		Holds = prevHolds
		Deliver = prevDeliver
		Instructions = prevInstructions
		Compose = prevCompose
	})
}

func installDefaultHooks() {
	Deliver = func(*Host, string, string) {}
	Instructions = func(Session) string { return "" }
	Compose = func(_ *Host, s Session, kind TurnKind, _ []json.RawMessage) (string, Turn, error) {
		return "replaced-default", Turn{Kind: kind, Thread: s.Task}, nil
	}
}

func stopHostNow(h *Host) {
	rt := stateOf(h)
	for {
		rt.mu.Lock()
		busy := len(rt.procs) + len(rt.closing)
		ch := rt.changed
		for _, lp := range rt.procs {
			stopIdleLocked(lp)
			lp.stopKillTimer()
			lp.cancel()
		}
		rt.mu.Unlock()
		if busy == 0 {
			rt.wg.Wait()
			return
		}
		<-ch
	}
}

func openFileStore(t *testing.T, dir string) (*store.Store, string) {
	t.Helper()
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(realDir, "node.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return db, realDir
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
	activeTestHosts = append(activeTestHosts, h)
	t.Cleanup(func() { stopHostNow(h) })
	return h
}

func setupRunEnv(t *testing.T) (*Host, *runHome, *fakeDriver, string) {
	t.Helper()
	stopAllTestHosts()
	saveRuntimeGlobals(t)
	drv := &fakeDriver{}
	Drivers = map[string]Driver{"claude": drv, "codex": drv}
	Listeners = map[string]func(*Host, SessionKey, Event){}
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
	installDefaultHooks()
	db, dataDir := openFileStore(t, t.TempDir())
	t.Cleanup(func() { db.Close() })
	h := newRunHost(t, db)
	h.Shim = []string{"solodawn", "mcp"}
	rh := newRunHome("local")
	h.Attach("home-dev", rh)
	return h, rh, drv, dataDir
}

func setupBubbleEnv(t *testing.T, baseDir string) (*Host, *runHome, *fakeDriver, func()) {
	t.Helper()
	drv := &fakeDriver{}
	Drivers = map[string]Driver{"claude": drv, "codex": drv}
	Listeners = map[string]func(*Host, SessionKey, Event){}
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
	installDefaultHooks()
	dbPath := filepath.Join(baseDir, "node.db")
	db, err := store.Open(dbPath)
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
		stopHostNow(h)
		synctest.Wait()
		db.Close()
	}
	return h, rh, drv, shutdown
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

func waitReportedKind(t *testing.T, reported <-chan EventKind) EventKind {
	t.Helper()
	select {
	case kind := <-reported:
		return kind
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reported event")
		return ""
	}
}

func drainReportedEvents(t *testing.T, reported <-chan EventKind, want []EventKind) {
	t.Helper()
	got := make([]EventKind, len(want))
	for i := range want {
		got[i] = waitReportedKind(t, reported)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("reported events = %v, want %v", got, want)
	}
}

func waitDeliveredSlot(t *testing.T, delivered <-chan aiSlot, want aiSlot) {
	t.Helper()
	select {
	case slot := <-delivered:
		if slot != want {
			t.Fatalf("delivered %+v, want %+v", slot, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Deliver not called")
	}
}

func assertFirstTurnLaunch(t *testing.T, h *Host, drv *fakeDriver, dataDir string) string {
	t.Helper()
	if drv.count() != 1 {
		t.Fatalf("starts = %d, want 1", drv.count())
	}
	l1 := drv.lastLaunch()
	wantDir := filepath.Join(dataDir, "projects", runGroup1, "wt/1-paths")
	got := [7]any{l1.Folder, l1.Session, l1.Level, slices.Equal(l1.Shim[:2], []string{"solodawn", "mcp"}), l1.Instructions, l1.RepoConfig, l1.ToolTimeout}
	want := [7]any{wantDir, "", 3, true, "prompt:task", true, 60 * time.Minute}
	if got != want {
		t.Fatalf("unexpected launch: %+v, want dir %s", l1, wantDir)
	}
	tok1 := tokenFromLaunch(l1)
	caller1, ok := h.Sessions.Caller(tok1)
	gotCaller := [4]any{ok, caller1.Level, caller1.Interactive, caller1.Turn}
	wantCaller := [4]any{true, 3, false, Turn{N: 1, Kind: ItemsTurn, Thread: runTask1, Depth: 1}}
	if gotCaller != wantCaller {
		t.Fatalf("unexpected caller: %+v (ok=%v)", caller1, ok)
	}
	return tok1
}

func assertMemberStatusCmd(t *testing.T, rh *runHome, wantState, wantTask string) {
	t.Helper()
	cmd := rh.waitCmd(t)
	var ms proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(cmd.raw), &ms); err != nil {
		t.Fatalf("decode member status: %v", err)
	}
	got := [4]any{ms.State, string(ms.Task), ms.Path, ms.Usage}
	want := [4]any{wantState, wantTask, "local", proto.Usage{}}
	if got != want {
		t.Fatalf("member status = %+v, want state=%s task=%s", ms, wantState, wantTask)
	}
}

func verifySecondTurnAfterStrayEnd(t *testing.T, h *Host, rh *runHome, drv *fakeDriver, key SessionKey, tok1 string, reported <-chan EventKind, delivered <-chan aiSlot) {
	t.Helper()
	// Second turn while still Waiting reuses the open process without restarting.
	if err := Runner.Turn(t.Context(), h, key, "second message", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	if drv.count() != 1 {
		t.Fatalf("process restarted on waiting turn: %d starts", drv.count())
	}
	assertMemberStatusCmd(t, rh, "working", runTask1)
	caller2, ok := h.Sessions.Caller(tok1)
	if [2]bool{ok, caller2.Turn.N == 2} != [2]bool{true, true} {
		t.Fatalf("caller turn after second turn: %+v (ok=%v)", caller2, ok)
	}
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitDeliveredSlot(t, delivered, aiSlot{runGroup1, runAI1})
	if len(delivered) != 0 {
		t.Fatalf("stray TurnEnded caused extra Deliver call")
	}
	waitForState(t, h, key, Waiting)
	assertMemberStatusCmd(t, rh, "waiting", runTask1)
	drainReportedEvents(t, reported, []EventKind{TurnBegan, TurnEnded})
}

func TestStartResumeEventsStrayTurnEndedAndDeliver(t *testing.T) {
	h, rh, drv, dataDir := setupRunEnv(t)
	Instructions = func(s Session) string { return "prompt:" + string(s.Kind) }

	// Single reader: waitReportedKind on the test goroutine.
	reported := make(chan EventKind, 16)
	Listeners["test"] = func(_ *Host, _ SessionKey, e Event) { reported <- e.Kind }
	// Single reader: waitDeliveredSlot on the test goroutine.
	delivered := make(chan aiSlot, 8)
	Deliver = func(_ *Host, g, m string) { delivered <- aiSlot{g, m} }

	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	if err := h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1-paths")); err != nil {
		t.Fatal(err)
	}
	turn1 := Turn{Kind: ItemsTurn, Thread: runTask1, Depth: 1}
	if err := Runner.Turn(t.Context(), h, key, "first message", turn1); err != nil {
		t.Fatal(err)
	}
	tok1 := assertFirstTurnLaunch(t, h, drv, dataDir)
	assertMemberStatusCmd(t, rh, "working", runTask1)

	p1 := drv.proc(0)
	pct := 42.5
	for _, ev := range []Event{
		{Kind: Ready, Session: "cli-sess-1"},
		{Kind: Ready, Session: ""},
		{Kind: Active},
		{Kind: Used, Usage: Usage{WindowPct: &pct}},
		{Kind: TurnEnded, Status: "completed"},
	} {
		p1.emit(ev)
	}
	waitDeliveredSlot(t, delivered, aiSlot{runGroup1, runAI1})
	if sAfter1 := waitForState(t, h, key, Waiting); sAfter1.CLISession != "cli-sess-1" {
		t.Fatalf("CLISession = %q, want cli-sess-1", sAfter1.CLISession)
	}
	assertMemberStatusCmd(t, rh, "waiting", runTask1)

	// A stray second TurnEnded while Waiting is reported to Listeners without
	// sending another status or calling Deliver again; the trailing Active event
	// ensures runProcessLoop has returned from onProcessTurnEnded before Turn.
	p1.emit(Event{Kind: TurnEnded, Status: "completed"})
	p1.emit(Event{Kind: Active})
	drainReportedEvents(t, reported, []EventKind{TurnBegan, Ready, Ready, Active, Used, TurnEnded, TurnEnded, Active})

	verifySecondTurnAfterStrayEnd(t, h, rh, drv, key, tok1, reported, delivered)
}

func TestDeliverOnTurnEndEvictsSameProcessWithoutDeadlock(t *testing.T) {
	// With max_open set to 1, one AI has a group session and a task session,
	// and a synchronous Deliver calls Runner.Turn for the task session when the
	// group turn ends. The group session's process is evicted without deadlocking.
	h, _, drv, _ := setupRunEnv(t)
	putSetting(t, h.Store, "max_open", "1")

	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1-task"))

	taskTurnErr := make(chan error, 1)
	var once sync.Once
	Deliver = func(h *Host, g, m string) {
		once.Do(func() {
			taskTurnErr <- Runner.Turn(context.Background(), h, taskKey, "held task item", Turn{Kind: ItemsTurn, Thread: runTask1})
		})
	}

	if err := Runner.Turn(t.Context(), h, grpKey, "group msg", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})

	select {
	case err := <-taskTurnErr:
		if err != nil {
			t.Fatalf("synchronous Deliver Turn failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("deadlock: task turn never started; group=%s task=%s", seamState(t, h.Sessions, grpKey), seamState(t, h.Sessions, taskKey))
	}
	if seamState(t, h.Sessions, grpKey) != Dormant || seamState(t, h.Sessions, taskKey) != Working {
		t.Fatalf("states: group=%s task=%s", seamState(t, h.Sessions, grpKey), seamState(t, h.Sessions, taskKey))
	}
}

func TestTwoHostsInOneProcessAreIsolated(t *testing.T) {
	saveRuntimeGlobals(t)
	installDefaultHooks()
	drv := &fakeDriver{makeProc: func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete = true
		return p, nil
	}}
	Drivers = map[string]Driver{"claude": drv}
	db1, _ := openFileStore(t, t.TempDir())
	t.Cleanup(func() { db1.Close() })
	db2, _ := openFileStore(t, t.TempDir())
	t.Cleanup(func() { db2.Close() })
	h1 := newRunHost(t, db1)
	h2 := newRunHost(t, db2)
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h1.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))
	h2.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))

	if err := Runner.Turn(t.Context(), h1, key, "only host 1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, h1, key, Waiting)
	if s2, _ := h2.Sessions.Get(key); s2.State != Dormant {
		t.Fatalf("host 2 state = %s, want dormant", s2.State)
	}
}

func runAndCheckLaunch(t *testing.T, h *Host, drv *fakeDriver, key SessionKey, wantLevel int, wantRepo bool) Launch {
	t.Helper()
	if err := Runner.Turn(t.Context(), h, key, "turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	waitForState(t, h, key, Waiting)
	got := drv.lastLaunch()
	if got.Level != wantLevel {
		t.Fatalf("session %v level = %d, want %d", key, got.Level, wantLevel)
	}
	if got.RepoConfig != wantRepo {
		t.Fatalf("session %v repoConfig = %v, want %v", key, got.RepoConfig, wantRepo)
	}
	closeWaitingSession(h, stateOf(h), key)
	return got
}

func assertKindLaunches(t *testing.T, h *Host, drv *fakeDriver, keys [3]SessionKey, wantLevel int) Launch {
	t.Helper()
	first := runAndCheckLaunch(t, h, drv, keys[0], wantLevel, false)
	runAndCheckLaunch(t, h, drv, keys[1], wantLevel, true)
	runAndCheckLaunch(t, h, drv, keys[2], wantLevel, true)
	return first
}

func TestLaunchConfigLevelsRepoConfigAndTimeout(t *testing.T) {
	h, _, drv, dataDir := setupRunEnv(t)
	drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete = true
		return p, nil
	}

	keys := [3]SessionKey{
		{Group: runGroup1, Member: runAI1},
		{Group: runGroup1, Member: runAI1, Task: runTask2},
		{Group: runGroup1, Member: runAI1, Task: runTask1},
	}
	h.Sessions.Create(makeRunSession(keys[0], GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(keys[1], ReviewSession, "review/12"))
	h.Sessions.Create(makeRunSession(keys[2], TaskSession, "wt/12-paths"))

	// 1. Fresh store defaults every session kind to level 3; task and review enable RepoConfig while group does not.
	grpRow := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: runGroup1, Seq: 1},
		Name:      "Website", Kind: "project", Base: "main", Forge: "none", Home: "home-dev",
		Checks: []string{}, Limits: proto.Limits{TurnMinutes: 17},
	}
	rawGrp, _ := json.Marshal(grpRow)
	h.Rows.Put(runGroup1, "group", runGroup1, rawGrp)

	grpLaunch := assertKindLaunches(t, h, drv, keys, 3)
	wantGrpDir := filepath.Join(dataDir, "projects", runGroup1, "repo")
	if grpLaunch.Folder != wantGrpDir || grpLaunch.ToolTimeout != 17*time.Minute {
		t.Fatalf("group launch: %+v, want dir %s", grpLaunch, wantGrpDir)
	}

	// 2. default_level=2 applies when perm is absent; TurnMinutes=0 falls back to 60m.
	putSetting(t, h.Store, "default_level", "2")
	grpRow.Limits.TurnMinutes = 0
	rawGrp0, _ := json.Marshal(grpRow)
	h.Rows.Put(runGroup1, "group", runGroup1, rawGrp0)
	defLaunch := runAndCheckLaunch(t, h, drv, keys[0], 2, false)
	if defLaunch.ToolTimeout != 60*time.Minute {
		t.Fatalf("default_level=2 launch: %+v", defLaunch)
	}

	// 3. perm(level=1) overrides default_level=2 for every kind; session kind alone controls RepoConfig.
	seamSQL(t, h.Store, `INSERT INTO perm(agent, g, level) VALUES('claude', '`+runGroup1+`', 1)`)
	assertKindLaunches(t, h, drv, keys, 1)

	// 4. all_level3=true overrides perm=1 to 3; malformed group JSON falls back to 60m.
	putSetting(t, h.Store, "all_level3", "true")
	h.Rows.Put(runGroup1, "group", runGroup1, json.RawMessage(`{"t":"group","id":"`+runGroup1+`","seq":1,"name":"W","kind":"project","base":"main","forge":"none","home":"home-dev","checks":[],"limits":{"turn_minutes":17},"extra":1}`))
	all3Launch := runAndCheckLaunch(t, h, drv, keys[1], 3, true)
	if all3Launch.ToolTimeout != 60*time.Minute {
		t.Fatalf("all_level3 launch: %+v", all3Launch)
	}
}

func TestDormancyVirtualClockIdleGuardsAndSpontaneousExit(t *testing.T) {
	if idleDuration(0) != 15*time.Minute || idleDuration(-1) != 15*time.Minute || idleDuration(1) != time.Minute {
		t.Fatalf("idleDuration boundaries wrong: %v %v %v", idleDuration(0), idleDuration(-1), idleDuration(1))
	}

	saveRuntimeGlobals(t)
	base1 := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupBubbleEnv(t, base1)
		defer shutdown()
		drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			p.autoReadyID = "resume-id-1"
			p.autoComplete = true
			return p, nil
		}
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))

		if err := Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		tok1 := tokenFromLaunch(drv.launches[0])
		if seamState(t, h.Sessions, key) != Waiting {
			t.Fatal("expected waiting right after turn")
		}
		// 1 ns before default 15 minutes: still Waiting.
		time.Sleep(15*time.Minute - time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Waiting {
			t.Fatal("went dormant before 15 minutes")
		}
		// At exact 15 minutes: Dormant, token forgotten, dormant status sent.
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Dormant {
			t.Fatal("did not go dormant at 15 minutes")
		}
		if _, ok := h.Sessions.Caller(tok1); ok {
			t.Fatal("token of closed process was not forgotten")
		}
		cmds := rh.snapshotCmds()
		if cmds[len(cmds)-1].payload["state"] != "dormant" {
			t.Fatalf("expected dormant status, got %v", cmds[len(cmds)-1].payload)
		}

		// Custom idle_minutes=1 and resume with stored CLISession.
		putSetting(t, h.Store, "idle_minutes", "1")
		if err := Runner.Turn(t.Context(), h, key, "wake", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if drv.count() != 2 || drv.lastLaunch().Session != "resume-id-1" {
			t.Fatalf("resume launch: count=%d session=%q", drv.count(), drv.lastLaunch().Session)
		}
		time.Sleep(time.Minute - time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Waiting {
			t.Fatal("went dormant before 1 minute")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, key) != Dormant {
			t.Fatal("did not go dormant at 1 minute")
		}

		// A session row ended during a turn still arms its idle timer on TurnEnded and closes when idle expires.
		drv.mu.Lock()
		drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			return newFakeProc(ctx, l), nil
		}
		drv.mu.Unlock()
		kEnded := SessionKey{Group: runGroup2, Member: runAI1}
		h.Sessions.Create(makeRunSession(kEnded, GroupSession, "repo"))
		if err := Runner.Turn(t.Context(), h, kEnded, "end-mid-turn", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		pEnded := drv.lastProc()
		h.Sessions.End(kEnded)
		pEnded.emit(Event{Kind: TurnEnded, Status: "completed"})
		synctest.Wait()
		time.Sleep(time.Minute - time.Nanosecond)
		synctest.Wait()
		select {
		case <-pEnded.exitedCh:
			t.Fatal("ended session process exited before idle timeout")
		default:
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		select {
		case <-pEnded.exitedCh:
		default:
			t.Fatal("ended session process stayed open after idle timeout")
		}
		rt := stateOf(h)
		rt.mu.Lock()
		stillTracked := rt.procs[kEnded] != nil || rt.closing[kEnded]
		rt.mu.Unlock()
		if stillTracked {
			t.Fatal("ended session remained in runtime maps after idle close")
		}
	})

	checkIdleAndArmGuards(t)
}

func checkIdleAndArmGuards(t *testing.T) {
	t.Helper()
	h, rh, drv, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))
	if err := Runner.Turn(t.Context(), h, key, "turn", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	p := drv.proc(0)
	p.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, key, Waiting)
	rh.waitCmd(t)

	verifyIdleGenAndArmGuards(t, h, key)

	// Spontaneous process exit while Waiting moves session to Dormant without crash or AuditKilled.
	p.finishExit()
	waitForState(t, h, key, Dormant)
	assertSpontaneousIdleExit(t, h, rh)
}

func verifyIdleGenAndArmGuards(t *testing.T, h *Host, key SessionKey) {
	t.Helper()
	rt := stateOf(h)
	onIdleTimeout(h, rt, SessionKey{Group: "missing"}, 1)
	rt.mu.Lock()
	lp := rt.procs[key]
	gen1 := lp.idleGen
	stopIdleLocked(lp)
	rt.mu.Unlock()
	// Stopping the idle timer invalidates gen1 and never wraps idleGen back to 0.
	onIdleTimeout(h, rt, key, 0)
	onIdleTimeout(h, rt, key, gen1)
	verifyArmTurnIdleGuards(t, h, rt, lp, key, gen1)
}

func verifyArmTurnIdleGuards(t *testing.T, h *Host, rt *runtimePart, lp *liveProc, key SessionKey, gen1 uint64) {
	t.Helper()
	rt.mu.Lock()
	lp.turnActive = true
	curGen := lp.idleGen
	rt.mu.Unlock()
	armTurnIdle(h, rt, lp, time.Hour)
	onIdleTimeout(h, rt, key, curGen)
	rt.mu.Lock()
	armedActive := lp.idleTimer != nil
	lp.turnActive = false
	rt.closing[key] = true
	rt.mu.Unlock()
	armTurnIdle(h, rt, lp, time.Hour)
	onIdleTimeout(h, rt, key, curGen)
	rt.mu.Lock()
	armedClosing := lp.idleTimer != nil
	delete(rt.closing, key)
	rt.mu.Unlock()
	armTurnIdle(h, rt, lp, time.Hour)
	// Re-arming after stopIdleLocked advances idleGen instead of colliding with gen1.
	onIdleTimeout(h, rt, key, gen1)
	rt.mu.Lock()
	rearmed := lp.idleTimer != nil
	rt.mu.Unlock()
	if [4]bool{armedActive, armedClosing, rearmed, seamState(t, h.Sessions, key) == Waiting} != [4]bool{false, false, true, true} {
		t.Fatalf("idle guard mismatch: active=%v closing=%v rearmed=%v state=%s", armedActive, armedClosing, rearmed, seamState(t, h.Sessions, key))
	}
}

func assertSpontaneousIdleExit(t *testing.T, h *Host, rh *runHome) {
	t.Helper()
	st := rh.waitCmd(t)
	var ms proto.MemberStatus[proto.SystemCode]
	err := proto.Decode([]byte(st.raw), &ms)
	rec := seamRecord(t, h)
	if [4]bool{err == nil, ms.State == "dormant", ms.Event == nil, rec == ""} != [4]bool{true, true, true, true} {
		t.Fatalf("unexpected spontaneous idle exit: err=%v raw=%s audit=%q", err, st.raw, rec)
	}
}

func TestMaxOpenBoundariesLongestWaitingAndBlockingWait(t *testing.T) {
	h, _, drv, _ := setupRunEnv(t)

	// Pin max_open <= 0 default (0 -> 4): 3 waiting sessions stay open when 4th starts; 5th evicts oldest waiting.
	putSetting(t, h.Store, "max_open", "0")
	keys := make([]SessionKey, 5)
	for i := range keys {
		keys[i] = SessionKey{Group: runGroup1, Member: "ai-" + strconv.Itoa(i)}
		h.Sessions.Create(makeRunSession(keys[i], GroupSession, "repo"))
	}
	for i := range 4 {
		if err := Runner.Turn(t.Context(), h, keys[i], "hi", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		drv.proc(i).emit(Event{Kind: TurnEnded, Status: "completed"})
		waitForState(t, h, keys[i], Waiting)
	}
	for i := range 4 {
		if seamState(t, h.Sessions, keys[i]) != Waiting {
			t.Fatalf("session %d evicted below max_open=4", i)
		}
	}
	setLast := func(k SessionKey, ts int64) {
		h.Sessions.Update(k, func(s *Session) { s.LastActive = ts })
	}
	setLast(keys[0], 500)
	setLast(keys[1], 100)
	setLast(keys[2], 100)
	setLast(keys[3], 900)

	// Starting keys[4] at open == 4 evicts keys[1] (tie-broken by key order).
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
		t.Fatal("strictly oldest waiting session keys[2] was not evicted")
	}

	// Test max_open=1 blocking wait & context deadline inside synctest (no fixed sleep).
	stopAllTestHosts()
	base2 := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h2, _, drv2, shutdown := setupBubbleEnv(t, base2)
		defer shutdown()
		putSetting(t, h2.Store, "max_open", "1")
		kA := SessionKey{Group: runGroup1, Member: runAI1}
		kB := SessionKey{Group: runGroup1, Member: runAI2}
		h2.Sessions.Create(makeRunSession(kA, GroupSession, "repo"))
		h2.Sessions.Create(makeRunSession(kB, GroupSession, "repo"))

		if err := Runner.Turn(t.Context(), h2, kA, "a1", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		shortCtx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		if err := Runner.Turn(shortCtx, h2, kB, "b-timeout", Turn{Kind: ItemsTurn}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want DeadlineExceeded", err)
		}

		bDone := make(chan error, 1)
		go func() {
			bDone <- Runner.Turn(t.Context(), h2, kB, "b-run", Turn{Kind: ItemsTurn})
		}()
		synctest.Wait()
		if drv2.count() != 1 {
			t.Fatal("kB started while kA was still working")
		}
		drv2.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
		if err := <-bDone; err != nil {
			t.Fatal(err)
		}
		if seamState(t, h2.Sessions, kA) != Dormant || seamState(t, h2.Sessions, kB) != Working {
			t.Fatalf("states: kA=%s kB=%s", seamState(t, h2.Sessions, kA), seamState(t, h2.Sessions, kB))
		}
	})

	checkSlotGuardOperands(t)
}

func checkSlotGuardOperands(t *testing.T) {
	t.Helper()
	checkProcLessWaitingClose(t)

	h, _, drv, _ := setupRunEnv(t)
	rt := stateOf(h)
	k1 := SessionKey{Group: runGroup1, Member: runAI1}
	k2 := SessionKey{Group: runGroup1, Member: runAI2}
	kOtherGrp := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(k1, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(k2, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(kOtherGrp, GroupSession, "repo"))

	checkReserveSlotStartingAndGroups(t, h, rt, k1, k2, kOtherGrp)
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, k1, Waiting)
	checkWaitingVictimGuards(t, h, rt, k1)
}

func checkProcLessWaitingClose(t *testing.T) {
	t.Helper()
	h, rh, drv, _ := setupRunEnv(t)
	putSetting(t, h.Store, "max_open", "1")
	kA := SessionKey{Group: runGroup1, Member: runAI1}
	kB := SessionKey{Group: runGroup1, Member: runAI2}
	h.Sessions.Create(makeRunSession(kA, GroupSession, "repo"))
	h.Sessions.Move(kA, Dormant, Working)
	h.Sessions.Move(kA, Working, Waiting)
	h.Sessions.Create(makeRunSession(kB, GroupSession, "repo"))
	drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.autoComplete = true
		return p, nil
	}
	if err := Runner.Turn(t.Context(), h, kB, "evict-proc-less", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	evictedCmd := rh.waitCmd(t)
	assertMemberStatusCmd(t, rh, "working", "")
	waitForState(t, h, kB, Waiting)
	assertMemberStatusCmd(t, rh, "waiting", "")
	if [3]any{seamState(t, h.Sessions, kA), evictedCmd.payload["id"], evictedCmd.payload["state"]} != [3]any{Dormant, runAI1, "dormant"} {
		t.Fatalf("proc-less eviction: state=%s cmd=%v", seamState(t, h.Sessions, kA), evictedCmd.payload)
	}
	// Single reader: this check on the test goroutine.
	rt := stateOf(h)
	rt.mu.Lock()
	chAfter := rt.changed
	rt.mu.Unlock()
	closeWaitingSession(h, rt, kA)
	closeWaitingSession(h, rt, SessionKey{Group: "missing"})
	select {
	case <-chAfter:
		t.Fatal("closeWaitingSession notified changed when Move failed or session was missing")
	default:
	}
	if len(rh.snapshotCmds()) != 3 {
		t.Fatalf("unexpected extra member status commands: %d", len(rh.snapshotCmds()))
	}
}

func checkReserveSlotStartingAndGroups(t *testing.T, h *Host, rt *runtimePart, k1, k2, kOtherGrp SessionKey) {
	t.Helper()
	_, _, okNeg := tryReserveSlot(h, rt, k1, -1)
	rt.releaseStarting(k1)
	_, _, okFirst := tryReserveSlot(h, rt, k1, 1)
	_, waitCh, okSecond := tryReserveSlot(h, rt, k2, 1)
	rt.releaseStarting(k1)
	if [4]bool{okNeg, okFirst, okSecond, waitCh != nil} != [4]bool{true, true, false, true} {
		t.Fatalf("slot reservation mismatch: neg=%v first=%v second=%v waitCh=%v", okNeg, okFirst, okSecond, waitCh != nil)
	}
	if err := Runner.Turn(t.Context(), h, k1, "run1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	if err := Runner.Turn(t.Context(), h, kOtherGrp, "run-other-group", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatalf("same AI in different group refused as busy: %v", err)
	}
}

func checkWaitingVictimGuards(t *testing.T, h *Host, rt *runtimePart, k1 SessionKey) {
	t.Helper()
	rt.mu.Lock()
	rt.closing[k1] = true
	closingVictim := longestWaitingLocked(rt, h.Sessions.All())
	rt.mu.Unlock()
	closeWaitingSession(h, rt, k1)
	closingState := seamState(t, h.Sessions, k1)

	rt.mu.Lock()
	delete(rt.closing, k1)
	rt.procs[k1].turnActive = true
	activeVictim := longestWaitingLocked(rt, h.Sessions.All())
	rt.mu.Unlock()
	closeWaitingSession(h, rt, k1)
	activeState := seamState(t, h.Sessions, k1)

	rt.mu.Lock()
	rt.procs[k1].turnActive = false
	rt.mu.Unlock()
	if [4]bool{closingVictim == nil, closingState == Waiting, activeVictim == nil, activeState == Waiting} != [4]bool{true, true, true, true} {
		t.Fatalf("victim guard failed: closingVictim=%v closingState=%s activeVictim=%v activeState=%s", closingVictim, closingState, activeVictim, activeState)
	}
}

func TestCloseTimeoutOnAllPathsAuditAndC6SwitchOrdering(t *testing.T) {
	saveRuntimeGlobals(t)
	base1 := t.TempDir()
	// 1. Idle close 10 s kill on both TaskSession (Detail names member & task) and GroupSession (Detail names member).
	synctest.Test(t, func(t *testing.T) {
		h, rh, drv, shutdown := setupBubbleEnv(t, base1)
		defer shutdown()
		putSetting(t, h.Store, "idle_minutes", "1")
		drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			p.ignoreClose = true
			p.autoComplete = true
			return p, nil
		}

		taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		grpKey := SessionKey{Group: runGroup1, Member: runAI2}
		h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))
		h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))

		if err := Runner.Turn(t.Context(), h, taskKey, "t1", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		if err := Runner.Turn(t.Context(), h, grpKey, "g1", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		// Advance 1m (idle fires) + 10s - 1ns: still Waiting while close waits for 10s kill.
		time.Sleep(time.Minute + 10*time.Second - time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, taskKey) != Waiting || seamState(t, h.Sessions, grpKey) != Waiting {
			t.Fatal("killed before 10s close timeout")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if seamState(t, h.Sessions, taskKey) != Dormant || seamState(t, h.Sessions, grpKey) != Dormant {
			t.Fatal("did not go dormant after 10s kill")
		}
		rec := seamRecord(t, h)
		wantTaskAudit := "|" + runGroup1 + "|claude|killed|-|-|" + runAI1 + " " + runTask1 + "|-|-"
		wantGrpAudit := "|" + runGroup1 + "|claude|killed|-|-|" + runAI2 + "|-|-"
		if !strings.Contains(rec, wantTaskAudit) || !strings.Contains(rec, wantGrpAudit) {
			t.Fatalf("missing AuditKilled rows in:\n%s", rec)
		}
		for _, c := range rh.snapshotCmds() {
			if c.payload["event"] != nil || c.payload["t"] == "task" {
				t.Fatalf("idle kill emitted crash command: %v", c.payload)
			}
		}
		if len(rh.snapshotCmds()) != 6 {
			t.Fatalf("expected 6 statuses (working, waiting, dormant for 2 AIs), got %d", len(rh.snapshotCmds()))
		}
	})

	checkHungInitialWriteAndNoSessionKill(t)
	checkC6SwitchOrdering(t)
}

func checkHungInitialWriteAndNoSessionKill(t *testing.T) {
	t.Helper()
	// A failed initial Turn write on an unresponsive CLI kills the process after 10 s without recording AuditKilled.
	baseA := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupBubbleEnv(t, baseA)
		defer shutdown()
		drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			p.turnErr = errors.New("stdin broken")
			p.ignoreClose = true
			return p, nil
		}
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))

		turnDone := make(chan error, 1)
		go func() {
			turnDone <- Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn})
		}()
		synctest.Wait()
		time.Sleep(10*time.Second - time.Nanosecond)
		synctest.Wait()
		if len(turnDone) != 0 {
			t.Fatal("deliverInitialTurn returned before 10s kill on hung CLI")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if err := <-turnDone; err == nil || err.Error() != "stdin broken" {
			t.Fatalf("got %v, want stdin broken", err)
		}
		if seamState(t, h.Sessions, key) != Dormant || seamRecord(t, h) != "" {
			t.Fatalf("unexpected state=%s or audit=%q", seamState(t, h.Sessions, key), seamRecord(t, h))
		}
	})

	// NoSession on resume with an unresponsive CLI kills the failed process after 10 s and starts the replacement.
	baseB := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupBubbleEnv(t, baseB)
		defer shutdown()
		drv.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			if idx == 0 {
				p.ignoreClose = true
			} else {
				p.autoReadyID = "replaced-after-kill"
				p.autoComplete = true
			}
			return p, nil
		}
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		s := makeRunSession(key, TaskSession, "wt/1")
		s.CLISession = "stale-hung"
		h.Sessions.Create(s)

		if err := Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
			t.Fatal(err)
		}
		drv.proc(0).emit(Event{Kind: NoSession, Text: "gone"})
		synctest.Wait()
		time.Sleep(10*time.Second - time.Nanosecond)
		synctest.Wait()
		if drv.count() != 1 {
			t.Fatal("replacement started before hung NoSession process was killed at 10s")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if drv.count() != 2 || seamState(t, h.Sessions, key) != Waiting {
			t.Fatalf("replacement after 10s kill failed: count=%d state=%s", drv.count(), seamState(t, h.Sessions, key))
		}
	})
}

func checkC6SwitchOrdering(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupBubbleEnv(t, base)
		defer shutdown()
		key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
		h.Sessions.Create(makeRunSession(key, TaskSession, "wt/1"))

		slowExit := make(chan struct{})
		var eventsLog []string
		var logMu sync.Mutex
		record := func(entry string) {
			logMu.Lock()
			eventsLog = append(eventsLog, entry)
			logMu.Unlock()
		}
		drv.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
			record("start-" + strconv.Itoa(idx))
			p := newFakeProc(ctx, l)
			if idx == 0 {
				p.ignoreClose = true
				go func() {
					<-p.closeCh
					<-slowExit
					record("exit-0")
					p.finishExit()
				}()
			} else {
				p.autoComplete = true
			}
			return p, nil
		}
		if err := Runner.Turn(t.Context(), h, key, "turn-before-close", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
		synctest.Wait()

		rt := stateOf(h)
		closedDone := make(chan struct{})
		go func() {
			closeWaitingSession(h, rt, key)
			close(closedDone)
		}()
		<-drv.proc(0).closeCh

		cancelCtx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := Runner.Turn(cancelCtx, h, key, "aborted", Turn{Kind: ItemsTurn}); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want Canceled", err)
		}

		nextTurnDone := make(chan error, 1)
		go func() {
			nextTurnDone <- Runner.Turn(t.Context(), h, key, "after-close", Turn{Kind: ItemsTurn})
		}()
		synctest.Wait()
		logMu.Lock()
		if slices.Contains(eventsLog, "start-1") {
			t.Fatalf("C-6 violated: start-1 began before exit-0: %v", eventsLog)
		}
		logMu.Unlock()
		close(slowExit)
		<-closedDone
		if err := <-nextTurnDone; err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		logMu.Lock()
		if !slices.Equal(eventsLog, []string{"start-0", "exit-0", "start-1"}) {
			t.Fatalf("unexpected lifecycle order: %v", eventsLog)
		}
		logMu.Unlock()
	})
}

func TestCrashRedactionLoginInvalidOnceAndReportSaved(t *testing.T) {
	h, rh, drv, _ := setupRunEnv(t)

	taskKey := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	revKey := SessionKey{Group: runGroup1, Member: runAI2, Task: runTask2}
	grpKey := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(taskKey, TaskSession, "wt/1"))
	h.Sessions.Create(makeRunSession(revKey, ReviewSession, "review/2"))
	h.Sessions.Create(makeRunSession(grpKey, GroupSession, "repo"))

	// 1. TaskSession mid-turn exit with secret -> turn_interrupted(crash) + task fail(crash), both redacted.
	if err := Runner.Turn(t.Context(), h, taskKey, "run", Turn{Kind: ItemsTurn, Thread: runTask1}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
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
	if s, _ := h.Sessions.Get(taskKey); s.CLISession != "keep-me" {
		t.Fatalf("CLISession lost after crash: %q", s.CLISession)
	}

	// 2. ReviewSession and GroupSession mid-turn exits -> turn_interrupted only, no task fail.
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

	checkLoginInvalidLifecycle(t, h, rh, drv, taskKey)
	checkReportSavedBehavior(t)
}

func checkLoginInvalidLifecycle(t *testing.T, h *Host, rh *runHome, drv *fakeDriver, taskKey SessionKey) {
	t.Helper()
	// The first LoginInvalid enters limited and reports a redacted LoginInvalidEvent;
	// a trailing TurnEnded before exit does not clear limited.
	if err := Runner.Turn(t.Context(), h, taskKey, "auth-1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	pFirst := drv.lastProc()
	pFirst.mu.Lock()
	pFirst.ignoreClose = true
	pFirst.mu.Unlock()
	pFirst.emit(Event{Kind: LoginInvalid, Text: "expired key " + runSecret})
	pFirst.emit(Event{Kind: TurnEnded, Status: "failed"})
	pFirst.finishExit()
	waitForState(t, h, taskKey, Dormant)

	limCmd1 := rh.waitCmd(t)
	var msLim1 proto.MemberStatus[proto.LoginInvalidEvent]
	if err := proto.Decode([]byte(limCmd1.raw), &msLim1); err != nil || msLim1.State != "limited" || msLim1.Event == nil || msLim1.Event.Code != "login_invalid" || msLim1.Event.Detail != "expired key [redacted]" {
		t.Fatalf("first login_invalid status: %+v (%v) raw=%s", msLim1, err, limCmd1.raw)
	}

	// Second failed try while still limited (even if Ready fires before LoginInvalid) does NOT report login_invalid again.
	if err := Runner.Turn(t.Context(), h, taskKey, "auth-2", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	pSecond := drv.lastProc()
	pSecond.emit(Event{Kind: Ready, Session: "keep-me"})
	pSecond.emit(Event{Kind: LoginInvalid, Text: "still expired"})
	waitForState(t, h, taskKey, Dormant)
	limCmd2 := rh.waitCmd(t)
	var msLim2 proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode([]byte(limCmd2.raw), &msLim2); err != nil || msLim2.State != "limited" || msLim2.Event != nil {
		t.Fatalf("second login_invalid should omit event: %+v (%v) raw=%s", msLim2, err, limCmd2.raw)
	}

	// Successful turn clears limited; a subsequent LoginInvalid enters limited and reports login_invalid again.
	if err := Runner.Turn(t.Context(), h, taskKey, "auth-ok", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	drv.lastProc().emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, taskKey, Waiting)
	stOK := rh.waitCmd(t)
	if stOK.payload["state"] != "waiting" {
		t.Fatalf("expected waiting after recovery, got %v", stOK.payload)
	}
	// LoginInvalid arriving while the session is waiting outside a turn also enters limited and reports LoginInvalidEvent once.
	drv.lastProc().emit(Event{Kind: LoginInvalid, Text: "idle expired " + runSecret})
	waitForState(t, h, taskKey, Dormant)
	limIdle := rh.waitCmd(t)
	var msLimIdle proto.MemberStatus[proto.LoginInvalidEvent]
	if err := proto.Decode([]byte(limIdle.raw), &msLimIdle); err != nil || msLimIdle.State != "limited" || msLimIdle.Event == nil || msLimIdle.Event.Code != "login_invalid" || msLimIdle.Event.Detail != "idle expired [redacted]" {
		t.Fatalf("waiting login_invalid status: %+v (%v) raw=%s", msLimIdle, err, limIdle.raw)
	}

	// Recover again so a turn-time LoginInvalid with empty detail is also checked.
	if err := Runner.Turn(t.Context(), h, taskKey, "auth-ok-2", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	drv.lastProc().emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, taskKey, Waiting)
	rh.waitCmd(t)
	closeWaitingSession(h, stateOf(h), taskKey)
	rh.waitCmd(t)

	if err := Runner.Turn(t.Context(), h, taskKey, "auth-again", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	drv.lastProc().emit(Event{Kind: LoginInvalid})
	waitForState(t, h, taskKey, Dormant)
	limCmd3 := rh.waitCmd(t)
	var msLim3 proto.MemberStatus[proto.LoginInvalidEvent]
	if err := proto.Decode([]byte(limCmd3.raw), &msLim3); err != nil || msLim3.State != "limited" || msLim3.Event == nil || msLim3.Event.Code != "login_invalid" || msLim3.Event.Detail != "" {
		t.Fatalf("re-entered login_invalid status: %+v (%v) raw=%s", msLim3, err, limCmd3.raw)
	}
}

func checkReportSavedBehavior(t *testing.T) {
	t.Helper()
	stopAllTestHosts()
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, rh, _, shutdown := setupBubbleEnv(t, base)
		defer shutdown()
		close(rh.done)

		saved := []Session{
			{SessionKey: SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}, Kind: TaskSession, State: Working, Home: "home-dev"},
			{SessionKey: SessionKey{Group: runGroup1, Member: runAI2}, Kind: GroupSession, State: Working, Home: "other-home"},
			{SessionKey: SessionKey{Group: runGroup1, Member: runAI1}, Kind: GroupSession, State: Waiting, Home: "home-dev"},
			{SessionKey: SessionKey{Group: runGroup2, Member: runAI1}, Kind: GroupSession, State: Takeover, Home: "home-dev"},
			{SessionKey: SessionKey{Group: runGroup2, Member: runAI2}, Kind: GroupSession, State: Dormant, Home: "home-dev"},
		}
		h.ReportSaved(t.Context(), saved)

		// Attach already-closed home -> re-queues cleanly.
		closedHome := newRunHome("direct")
		close(closedHome.done)
		h.Attach("home-dev", closedHome)
		synctest.Wait()

		// When the home accepts turn_interrupted status and disconnects on task fail, the retry sends only task fail.
		dropOnFailHome := newRunHome("direct")
		dropOnFailHome.cmdFn = func(p map[string]any) (json.RawMessage, error) {
			if p["t"] == "task" && p["op"] == "fail" {
				return nil, errors.New("connection lost")
			}
			return json.RawMessage(`{"re":1,"ok":true}`), nil
		}
		h.Attach("home-dev", dropOnFailHome)
		synctest.Wait()
		if len(dropOnFailHome.snapshotCmds()) != 2 {
			t.Fatalf("expected status + failed task fail attempt, got %d", len(dropOnFailHome.snapshotCmds()))
		}

		liveHome := newRunHome("direct")
		h.Attach("home-dev", liveHome)
		synctest.Wait()
		flushed := liveHome.snapshotCmds()
		if len(flushed) != 1 || flushed[0].payload["t"] != "task" || flushed[0].payload["op"] != "fail" {
			t.Fatalf("expected only task fail on retry without duplicate turn_interrupted, got %+v", flushed)
		}

		// Attach again when nothing is pending -> no commands sent.
		h.Attach("home-dev", liveHome)
		synctest.Wait()
		if len(liveHome.snapshotCmds()) != 1 {
			t.Fatal("empty savedCrash sent unexpected command")
		}

		// Non-unreachable error (conflict) from home does NOT re-queue into savedCrash.
		refuseHome := newRunHome("local")
		refuseHome.cmdFn = func(map[string]any) (json.RawMessage, error) {
			return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"refused"}}`), nil
		}
		h.Attach("other-home", refuseHome)
		synctest.Wait()
		rt := stateOf(h)
		rt.mu.Lock()
		queued := len(rt.savedCrash["other-home"])
		rt.mu.Unlock()
		if queued != 0 {
			t.Fatalf("conflict error was re-queued as unreachable: %d", queued)
		}

		checkAttachMemberStatus(t, h)
	})
}

func checkAttachMemberStatus(t *testing.T, h *Host) {
	t.Helper()
	s1 := makeRunSession(SessionKey{Group: runGroup1, Member: runAI1}, GroupSession, "repo")
	s2 := makeRunSession(SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}, TaskSession, "wt/1")
	s3 := makeRunSession(SessionKey{Group: runGroup1, Member: runAI2}, GroupSession, "repo")
	sOther := makeRunSession(SessionKey{Group: runGroup2, Member: runAI1}, GroupSession, "repo")
	sOther.Home = "other-home"
	for _, s := range []Session{s1, s2, s3, sOther} {
		if err := h.Sessions.Create(s); err != nil {
			t.Fatal(err)
		}
	}

	reconn := newRunHome("direct")
	h.Attach("home-dev", reconn)
	synctest.Wait()
	assertAttachStatusCmds(t, reconn.snapshotCmds())
}

func assertAttachStatusCmds(t *testing.T, cmds []recordedCmd) {
	t.Helper()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 member status commands (ai1 + ai2), got %+v", cmds)
	}
	if cmds[0].payload["id"] != runAI1 || cmds[0].payload["state"] != "dormant" || cmds[0].payload["path"] != "direct" {
		t.Fatalf("expected plain dormant direct status for ai1, got %+v", cmds[0].payload)
	}
	if cmds[1].payload["id"] != runAI2 || cmds[1].payload["state"] != "dormant" || cmds[1].payload["path"] != "direct" {
		t.Fatalf("expected plain dormant direct status for ai2, got %+v", cmds[1].payload)
	}
}

func TestNoSessionFailedResumeSyncAsyncAndFailures(t *testing.T) {
	// 1. Async NoSession (Claude style: Turn succeeds, then NoSession arrives on Events).
	h, rh, drv, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	s := makeRunSession(key, TaskSession, "wt/1")
	s.CLISession = "stale-id"
	h.Sessions.Create(s)

	Compose = func(_ *Host, got Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
		if kind != ReplacedTurn || got.CLISession != "" || items != nil {
			t.Errorf("unexpected Compose args: kind=%s cli=%q items=%v", kind, got.CLISession, items)
		}
		return "replaced prompt with 20 msgs", Turn{Kind: ReplacedTurn}, nil
	}

	if err := Runner.Turn(t.Context(), h, key, "orig text", Turn{Kind: ItemsTurn, Thread: runTask1, Depth: 2}); err != nil {
		t.Fatal(err)
	}
	rh.waitCmd(t)
	pOld := drv.proc(0)
	oldTok := tokenFromLaunch(drv.launches[0])
	pOld.emit(Event{Kind: NoSession, Text: "session not found"})

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
	pNew := drv.proc(1)
	if got := <-pNew.turnCh; got != "replaced prompt with 20 msgs" {
		t.Fatalf("replacement turn text = %q", got)
	}
	newTok := tokenFromLaunch(drv.lastLaunch())
	cNew, ok := h.Sessions.Caller(newTok)
	if !ok || cNew.Turn.Kind != ReplacedTurn || cNew.Turn.Depth != 2 || cNew.Turn.Thread != runTask1 {
		t.Fatalf("replacement Caller.Turn metadata lost: %+v", cNew.Turn)
	}
	pNew.emit(Event{Kind: Ready, Session: "fresh-id"})
	pNew.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, key, Waiting)
	rh.waitCmd(t)
	if cur, _ := h.Sessions.Get(key); cur.CLISession != "fresh-id" {
		t.Fatalf("CLISession = %q, want fresh-id", cur.CLISession)
	}

	checkSyncNoSessionAndFailurePaths(t)
}

func checkSyncNoSessionAndFailurePaths(t *testing.T) {
	t.Helper()
	// 2. Sync NoSession (Codex style: Turn returns error while NoSession is emitted).
	h, _, drv, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	s := makeRunSession(key, GroupSession, "repo")
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
	if err := Runner.Turn(t.Context(), h, key, "hello group", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatalf("sync NoSession failed: %v", err)
	}
	waitForState(t, h, key, Waiting)
	if cur, _ := h.Sessions.Get(key); cur.CLISession != "fresh-codex" {
		t.Fatalf("CLISession = %q, want fresh-codex", cur.CLISession)
	}

	// 3. NoSession on a non-resumed start (CLISession == "") is NOT treated as resume replacement.
	hFresh, _, drvFresh, _ := setupRunEnv(t)
	hFresh.Sessions.Create(makeRunSession(key, GroupSession, "repo"))
	drvFresh.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		p := newFakeProc(ctx, l)
		p.turnErr = errors.New("broken start")
		p.emit(Event{Kind: NoSession, Text: "ignored on fresh"})
		return p, nil
	}
	if err := Runner.Turn(t.Context(), hFresh, key, "hi", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected error when non-resumed start fails")
	}

	checkReplacementErrors(t)
}

func checkReplacementErrors(t *testing.T) {
	t.Helper()
	key := SessionKey{Group: runGroup1, Member: runAI1}

	// (a) Compose returns error
	h1, _, drv1, _ := setupRunEnv(t)
	s := makeRunSession(key, GroupSession, "repo")
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

	// (b) Replacement Start error, (c) replacement Turn error, and (d) readLaunchConfig error during replacement
	for _, mode := range []string{"start-err", "turn-err", "cfg-err"} {
		h2, _, drv2, _ := setupRunEnv(t)
		s2 := makeRunSession(key, GroupSession, "repo")
		s2.CLISession = "bad-2"
		h2.Sessions.Create(s2)
		drv2.makeProc = func(ctx context.Context, idx int, l Launch) (*fakeProc, error) {
			if idx == 1 && mode == "start-err" {
				return nil, errors.New("second start failed")
			}
			p := newFakeProc(ctx, l)
			p.turnErr = errors.New("turn write failed")
			if idx == 0 {
				p.emit(Event{Kind: NoSession})
				if mode == "cfg-err" {
					seamSQL(t, h2.Store, "DROP TABLE perm")
				}
			}
			return p, nil
		}
		if err := Runner.Turn(t.Context(), h2, key, "hi", Turn{Kind: ItemsTurn}); err == nil {
			t.Fatalf("mode %s: expected replacement failure", mode)
		}
		waitForState(t, h2, key, Dormant)
	}

	// (e) Session ended while NoSession was draining, and (f) Update trigger error
	h3, _, drv3, _ := setupRunEnv(t)
	s3 := makeRunSession(key, GroupSession, "repo")
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

	h4, _, drv4, _ := setupRunEnv(t)
	s4 := makeRunSession(key, GroupSession, "repo")
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

func TestMemberStatusPriorityAnswerAndTurnRefusals(t *testing.T) {
	h, _, drv, _ := setupRunEnv(t)
	rt := stateOf(h)
	slot := aiSlot{runGroup1, runAI1}

	kGrp := SessionKey{Group: runGroup1, Member: runAI1}
	kT1 := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	kT2 := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask2}
	h.Sessions.Create(makeRunSession(kGrp, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(kT1, TaskSession, "wt/1"))
	h.Sessions.Create(makeRunSession(kT2, TaskSession, "wt/2"))
	seamIn(t, h.Sessions, SessionKey{Group: runGroup2, Member: runAI1}, Takeover)
	seamIn(t, h.Sessions, SessionKey{Group: runGroup1, Member: runAI2}, Takeover)

	// 5. dormant when all sessions of (runGroup1, runAI1) are dormant.
	if st, tk := h.MemberState(runGroup1, runAI1); st != "dormant" || tk != "" {
		t.Fatalf("got %s/%s, want dormant", st, tk)
	}

	// 4. waiting picks highest LastActive (including LastActive == 0 and tie-breaking).
	h.Sessions.Move(kT1, Dormant, Working)
	h.Sessions.Move(kT1, Working, Waiting)
	h.Sessions.Update(kT1, func(s *Session) { s.LastActive = 0 })
	if st, tk := h.MemberState(runGroup1, runAI1); st != "waiting" || tk != runTask1 {
		t.Fatalf("zero LastActive waiting got %s/%s, want waiting/%s", st, tk, runTask1)
	}
	h.Sessions.Move(kT2, Dormant, Working)
	h.Sessions.Move(kT2, Working, Waiting)
	h.Sessions.Update(kT1, func(s *Session) { s.LastActive = 200 })
	h.Sessions.Update(kT2, func(s *Session) { s.LastActive = 200 })
	if st, tk := h.MemberState(runGroup1, runAI1); st != "waiting" || tk != runTask2 {
		t.Fatalf("tie waiting got %s/%s, want waiting/%s", st, tk, runTask2)
	}
	h.Sessions.Update(kT1, func(s *Session) { s.LastActive = 300 })
	if st, tk := h.MemberState(runGroup1, runAI1); st != "waiting" || tk != runTask1 {
		t.Fatalf("latest waiting got %s/%s, want waiting/%s", st, tk, runTask1)
	}

	// 3. limited (sign-in invalid) overrides waiting.
	rt.enterLimited(slot)
	if st, tk := h.MemberState(runGroup1, runAI1); st != "limited" || tk != "" {
		t.Fatalf("got %s/%s, want limited", st, tk)
	}

	// 2. working overrides limited.
	h.Sessions.Move(kT1, Waiting, Working)
	if st, tk := h.MemberState(runGroup1, runAI1); st != "working" || tk != runTask1 {
		t.Fatalf("got %s/%s, want working/%s", st, tk, runTask1)
	}
	// Calling Turn on a Working session returns conflict.
	seamCode(t, Runner.Turn(t.Context(), h, kT1, "on-working", Turn{Kind: ItemsTurn}), "conflict")
	h.Sessions.Move(kT1, Working, Dormant)

	// 1. takeover overrides everything, and Turn on Takeover returns explicit conflict.
	h.Sessions.Move(kT2, Waiting, Takeover)
	if st, tk := h.MemberState(runGroup1, runAI1); st != "takeover" || tk != runTask2 {
		t.Fatalf("got %s/%s, want takeover/%s", st, tk, runTask2)
	}
	seamCode(t, Runner.Turn(t.Context(), h, kT2, "on-takeover", Turn{Kind: ItemsTurn}), "conflict")
	h.Sessions.Move(kT2, Takeover, Dormant)
	rt.mu.Lock()
	delete(rt.limited, slot)
	rt.mu.Unlock()

	// Answer: forwards allow=true and allow=false, propagates error, refuses missing session or closed proc.
	if err := Runner.Turn(t.Context(), h, kT1, "for-answer", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	p := drv.proc(0)
	if err := Runner.Answer(h, kT1, "ask-1", true); err != nil {
		t.Fatal(err)
	}
	p.answerErr = errors.New("rpc closed")
	if err := Runner.Answer(h, kT1, "ask-2", false); err == nil || err.Error() != "rpc closed" {
		t.Fatalf("got %v, want rpc closed", err)
	}
	if !slices.Equal(p.answers, []recordedAsk{{"ask-1", true}, {"ask-2", false}}) {
		t.Fatalf("recorded answers: %+v", p.answers)
	}
	seamCode(t, Runner.Answer(h, SessionKey{Group: "none"}, "a", true), "not_found")
	seamCode(t, Runner.Answer(h, kGrp, "a", true), "not_found")
	p.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, kT1, Waiting)

	// A concurrent Turn on a Waiting session whose turnActive is already set returns busy without moving to Dormant.
	rt.mu.Lock()
	rt.closing[kT1] = true
	rt.mu.Unlock()
	if done, err := tryTurnOnWaiting(h, rt, Session{SessionKey: kT1, State: Waiting}, "closing-race", Turn{Kind: ItemsTurn}); done || err != nil {
		t.Fatalf("expected (false, nil) on closing race, got (%v, %v)", done, err)
	}
	rt.mu.Lock()
	delete(rt.closing, kT1)
	rt.procs[kT1].turnActive = true
	rt.mu.Unlock()
	done, busyErr := tryTurnOnWaiting(h, rt, Session{SessionKey: kT1, State: Waiting}, "concurrent", Turn{Kind: ItemsTurn})
	if !done {
		t.Fatal("expected done=true on concurrent turnActive waiting session")
	}
	seamCode(t, busyErr, "busy")
	if seamState(t, h.Sessions, kT1) != Waiting || drv.count() != 1 {
		t.Fatalf("concurrent turn changed state=%s or started process=%d", seamState(t, h.Sessions, kT1), drv.count())
	}
	rt.mu.Lock()
	rt.procs[kT1].turnActive = false
	rt.mu.Unlock()

	checkTurnFailurePaths(t)
}

func checkTurnFailurePaths(t *testing.T) {
	t.Helper()
	h, _, drv, _ := setupRunEnv(t)
	kGrp := SessionKey{Group: runGroup1, Member: runAI1}
	kTask := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(kGrp, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(kTask, TaskSession, "wt/1"))

	seamCode(t, Runner.Turn(t.Context(), h, SessionKey{Group: "missing"}, "x", Turn{Kind: ItemsTurn}), "not_found")

	badAgentKey := SessionKey{Group: runGroup2, Member: runAI2}
	badSess := makeRunSession(badAgentKey, GroupSession, "repo")
	badSess.Agent = "unknown-cli"
	h.Sessions.Create(badSess)
	seamCode(t, Runner.Turn(t.Context(), h, badAgentKey, "x", Turn{Kind: ItemsTurn}), "bad_request")

	drv.startErr = errors.New("exec failed")
	if err := Runner.Turn(t.Context(), h, kGrp, "x", Turn{Kind: ItemsTurn}); err == nil || err.Error() != "exec failed" {
		t.Fatalf("got %v, want exec failed", err)
	}
	drv.startErr = nil

	// Busy AI from Dormant and from Waiting.
	if err := Runner.Turn(t.Context(), h, kGrp, "ok", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	seamCode(t, Runner.Turn(t.Context(), h, kTask, "busy", Turn{Kind: ItemsTurn}), "busy")
	drv.proc(0).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, kGrp, Waiting)
	if err := Runner.Turn(t.Context(), h, kTask, "ok-task", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	seamCode(t, Runner.Turn(t.Context(), h, kGrp, "busy-from-waiting", Turn{Kind: ItemsTurn}), "busy")
	rt := stateOf(h)
	rt.mu.Lock()
	rearmed := rt.procs[kGrp].idleTimer != nil
	rt.mu.Unlock()
	if !rearmed {
		t.Fatal("unclaimWaitingProc did not re-arm idle timer")
	}
	drv.proc(1).emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, kTask, Waiting)

	// Waiting process whose Turn() write fails crashes cleanly and returns the write error.
	drv.proc(0).mu.Lock()
	drv.proc(0).turnErr = errors.New("broken pipe")
	drv.proc(0).mu.Unlock()
	if err := Runner.Turn(t.Context(), h, kGrp, "fail-write", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected broken pipe error")
	}
	waitForState(t, h, kGrp, Dormant)

	// Orphan Waiting session: first Move(Waiting, Dormant) fails via trigger, then recovers to Working.
	kOrphan := SessionKey{Group: runGroup2, Member: runAI1}
	h.Sessions.Create(makeRunSession(kOrphan, GroupSession, "repo"))
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

	// startFromDormant Move failure & readLaunchConfig failure when setting table is dropped.
	kDorm := SessionKey{Group: runGroup2, Member: "m-dorm"}
	h.Sessions.Create(makeRunSession(kDorm, GroupSession, "repo"))
	seamSQL(t, h.Store, "CREATE TRIGGER stop_start BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'start move blocked'); END")
	if err := Runner.Turn(t.Context(), h, kDorm, "x", Turn{Kind: ItemsTurn}); err == nil || !strings.Contains(err.Error(), "start move blocked") {
		t.Fatalf("got %v, want start move blocked", err)
	}
	seamSQL(t, h.Store, "DROP TRIGGER stop_start", "DROP TABLE setting")
	if err := Runner.Turn(t.Context(), h, kDorm, "x", Turn{Kind: ItemsTurn}); err == nil {
		t.Fatal("expected error when setting table is dropped")
	}
}

func TestRunnerCloseLifecycleAndConcurrency(t *testing.T) {
	saveRuntimeGlobals(t)
	h, rh, drv, _ := setupRunEnv(t)
	var delivered atomic.Int32
	// Single reader: TestRunnerCloseLifecycleAndConcurrency goroutine.
	deliveredCh := make(chan struct{}, 8)
	Deliver = func(*Host, string, string) {
		delivered.Add(1)
		deliveredCh <- struct{}{}
	}
	checkRunnerCloseNoopAndWaiting(t, h, drv, &delivered, deliveredCh)
	checkRunnerCloseActiveTurn(t, h, rh, drv, deliveredCh)
	checkConcurrentAndKilledClose(t)
}

func checkRunnerCloseNoopAndWaiting(t *testing.T, h *Host, drv *fakeDriver, delivered *atomic.Int32, deliveredCh <-chan struct{}) {
	t.Helper()
	Runner.Close(h, SessionKey{Group: runGroup1, Member: "missing"})
	kStray := SessionKey{Group: runGroup1, Member: "m-stray"}
	h.Sessions.Create(makeRunSession(kStray, GroupSession, "repo"))
	Runner.Close(h, kStray)
	h.Sessions.Move(kStray, Dormant, Working)
	h.Sessions.Move(kStray, Working, Waiting)
	Runner.Close(h, kStray)
	if seamState(t, h.Sessions, kStray) != Dormant {
		t.Fatal("expected stray Waiting session to move Dormant")
	}

	kWait := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	h.Sessions.Create(makeRunSession(kWait, TaskSession, "wt/1-a"))
	if err := Runner.Turn(t.Context(), h, kWait, "w1", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	pWait := drv.lastProc()
	pWait.emit(Event{Kind: TurnEnded, Status: "completed"})
	<-deliveredCh
	waitForState(t, h, kWait, Waiting)
	delivered.Store(0)
	Runner.Close(h, kWait)
	requireExitedNow(t, pWait.exitedCh, "waiting CLI process")
	if seamState(t, h.Sessions, kWait) != Dormant || delivered.Load() != 0 {
		t.Fatalf("unexpected post-close waiting state=%s delivered=%d", seamState(t, h.Sessions, kWait), delivered.Load())
	}
}

func checkRunnerCloseActiveTurn(t *testing.T, h *Host, rh *runHome, drv *fakeDriver, deliveredCh <-chan struct{}) {
	t.Helper()
	kWork := SessionKey{Group: runGroup1, Member: runAI2, Task: runTask2}
	h.Sessions.Create(makeRunSession(kWork, TaskSession, "wt/2-b"))
	if err := Runner.Turn(t.Context(), h, kWork, "w2", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	pWork := drv.lastProc()
	beforeCmds := len(rh.snapshotCmds())
	Runner.Close(h, kWork)
	requireExitedNow(t, pWork.exitedCh, "working CLI process")
	<-deliveredCh
	for _, c := range rh.snapshotCmds()[beforeCmds:] {
		if c.payload["op"] == "fail" || c.payload["event"] != nil {
			t.Fatalf("unexpected crash/fail command on intentional Close: %v", c.payload)
		}
	}
}

func requireExitedNow(t *testing.T, exited <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-exited:
	default:
		t.Fatalf("Runner.Close returned before %s exited", label)
	}
}

func checkConcurrentAndKilledClose(t *testing.T) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, _ := setupRunEnv(t)
		kActive := SessionKey{Group: runGroup1, Member: runAI1}
		h.Sessions.Create(makeRunSession(kActive, GroupSession, "repo"))
		var gate chan struct{}
		drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
			p := newFakeProc(ctx, l)
			p.ignoreClose = true
			gate = p.exitedCh
			return p, nil
		}
		if err := Runner.Turn(t.Context(), h, kActive, "hang", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		// Non-force closeWaitingSession leaves an active turn untouched.
		closeWaitingSession(h, stateOf(h), kActive)
		if seamState(t, h.Sessions, kActive) != Working {
			t.Fatal("closeWaitingSession disturbed active turn")
		}
		h.Sessions.End(kActive)

		// Two concurrent Runner.Close calls wait for the 10s kill timer and record AuditKilled once.
		done1, done2 := make(chan struct{}), make(chan struct{})
		go func() { Runner.Close(h, kActive); close(done1) }()
		synctest.Wait()
		go func() { Runner.Close(h, kActive); close(done2) }()
		synctest.Wait()
		select {
		case <-done2:
			t.Fatal("concurrent Close returned while process was still closing")
		default:
		}
		time.Sleep(stopSeconds * time.Second)
		synctest.Wait()
		<-gate
		<-done1
		<-done2
		if countAuditWhat(t, h.Store, AuditKilled) != 1 {
			t.Fatalf("expected 1 AuditKilled row, got %d", countAuditWhat(t, h.Store, AuditKilled))
		}
	})
}

func countAuditWhat(t *testing.T, db *store.Store, what string) int {
	t.Helper()
	var n int
	seamSQLScan(t, db, &n, "SELECT count(*) FROM audit WHERE what = ?", what)
	return n
}

func seamSQLScan(t *testing.T, db *store.Store, dest any, q string, args ...any) {
	t.Helper()
	read := func(tx *store.Tx) error { return tx.QueryRow(q, args...).Scan(dest) }
	if err := db.Write(read); err != nil {
		t.Fatal(err)
	}
}

// A CLI that exits after the store closed must not reach it: Settle ends
// every CLI and waits for its loop, and member status reads only memory.
func TestSettleEndsEveryCLIBeforeTheStoreCloses(t *testing.T) {
	stopAllTestHosts()
	saveRuntimeGlobals(t)
	drv := &fakeDriver{}
	Drivers = map[string]Driver{"claude": drv}
	Listeners = map[string]func(*Host, SessionKey, Event){}
	installDefaultHooks()
	db, _ := openFileStore(t, t.TempDir())
	h := newRunHost(t, db)
	rh := newRunHome("local")
	h.Attach("home-dev", rh)
	waitKey := SessionKey{Group: runGroup1, Member: runAI1}
	dormKey := SessionKey{Group: runGroup1, Member: runAI2}
	h.Sessions.Create(makeRunSession(waitKey, GroupSession, "repo"))
	h.Sessions.Create(makeRunSession(dormKey, GroupSession, "repo"))
	if err := Runner.Turn(t.Context(), h, waitKey, "hi", Turn{Kind: ItemsTurn}); err != nil {
		t.Fatal(err)
	}
	p := drv.lastProc()
	p.emit(Event{Kind: TurnEnded, Status: "completed"})
	waitForState(t, h, waitKey, Waiting)

	h.Settle()
	select {
	case <-p.exitedCh:
	default:
		t.Fatal("Settle returned while a CLI was still running")
	}
	err := Runner.Turn(t.Context(), h, dormKey, "late", Turn{Kind: ItemsTurn})
	var herr *Error
	if !errors.As(err, &herr) || herr.Code != "busy" || drv.count() != 1 {
		t.Fatalf("a turn after Settle: %v, starts %d", err, drv.count())
	}
	db.Close()
	p.finishExit()

	if s, _ := h.Sessions.Get(waitKey); s.State != Waiting {
		t.Fatalf("Settle changed the row to %s; the next start reports it", s.State)
	}
	if err := sendMemberStatus(t.Context(), h, stateOf(h), "home-dev", runGroup1, runAI1, nil); err != nil {
		t.Fatal(err)
	}
	if got := rh.snapshotCmds(); got[len(got)-1].payload["state"] != "waiting" {
		t.Fatalf("member status after the store closed: %+v", got[len(got)-1].payload)
	}
}

func TestSettleWaitsForRuntimeDelivery(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupBubbleEnv(t, base)
		defer shutdown()
		started, release, settled := make(chan struct{}), make(chan struct{}), make(chan struct{})
		Deliver = func(*Host, string, string) {
			close(started)
			<-release
		}
		key := SessionKey{Group: runGroup1, Member: runAI1}
		if err := h.Sessions.Create(makeRunSession(key, GroupSession, "repo")); err != nil {
			t.Fatal(err)
		}
		if err := Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn}); err != nil {
			t.Fatal(err)
		}
		drv.lastProc().emit(Event{Kind: TurnEnded, Status: "completed"})
		<-started
		go func() {
			h.Settle()
			close(settled)
		}()
		synctest.Wait()
		select {
		case <-settled:
			t.Error("Settle returned before delivery finished")
		default:
		}
		close(release)
		<-settled
	})
}

func TestSettleWaitsForDeliveryAfterAnExitMidTurn(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, drv, shutdown := setupBubbleEnv(t, base)
		defer shutdown()
		started, release, settled := make(chan struct{}), make(chan struct{}), make(chan struct{})
		Deliver = func(*Host, string, string) {
			close(started)
			<-release
		}
		key := SessionKey{Group: runGroup1, Member: runAI1}
		assertEqual(t, h.Sessions.Create(makeRunSession(key, GroupSession, "repo")), nil)
		assertEqual(t, Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn}), nil)
		drv.lastProc().finishExit()
		<-started
		go func() {
			h.Settle()
			close(settled)
		}()
		synctest.Wait()
		select {
		case <-settled:
			t.Error("Settle returned before delivery finished")
		default:
		}
		close(release)
		<-settled
	})
}

func TestSettleUnblocksAWaitingOpenSlot(t *testing.T) {
	saveRuntimeGlobals(t)
	base := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h, _, _, shutdown := setupBubbleEnv(t, base)
		defer shutdown()
		key := SessionKey{Group: runGroup1, Member: runAI1}
		assertEqual(t, h.Sessions.Create(makeRunSession(key, GroupSession, "repo")), nil)
		assertEqual(t, h.Sessions.Move(key, Dormant, Working), nil)
		rt := stateOf(h)
		result := make(chan error, 1)
		rt.startBackground(func() {
			other := SessionKey{Group: runGroup1, Member: runAI2}
			result <- acquireOpenSlot(t.Context(), h, rt, other, 1)
		})
		synctest.Wait()
		h.Settle()
		assertEqual(t, <-result, errSettling)
	})
}

func TestSettleDuringDriverStart(t *testing.T) {
	h, _, drv, _ := setupRunEnv(t)
	key := SessionKey{Group: runGroup1, Member: runAI1}
	assertEqual(t, h.Sessions.Create(makeRunSession(key, GroupSession, "repo")), nil)
	drv.makeProc = func(ctx context.Context, _ int, l Launch) (*fakeProc, error) {
		h.Settle()
		return newFakeProc(ctx, l), nil
	}
	assertEqual(t, Runner.Turn(t.Context(), h, key, "hi", Turn{Kind: ItemsTurn}), errSettling)
	<-drv.lastProc().exitedCh
	_, ok := h.Sessions.Caller(tokenFromLaunch(drv.lastLaunch()))
	assertEqual(t, ok, false)
}
