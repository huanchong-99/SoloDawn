package host

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// AuditKilled is the audit what value recorded when an idle CLI process does
// not exit within 10 s of closing its input and its process tree is killed
// (flows/agent-turn.md G, contracts/storage.md section 3).
const AuditKilled = "killed"

const (
	defaultIdleMinutes = 15
	defaultMaxOpen     = 4
	defaultTurnMinutes = 60
)

var (
	// idleUnit scales idle_minutes so unit tests can exercise dormancy without minute-long sleeps.
	idleUnit = time.Minute
	// stopTimeout bounds how long Close or Interrupt waits before cancelling Start's context.
	stopTimeout = 10 * time.Second
)

func init() {
	Provide(&Runner, Runtime(sessionRunner{}))
	Register(Attached, "runtime", attachRuntime)
}

type sessionRunner struct{}

type aiSlot struct {
	group  string
	member string
}

type preemptedTurn struct {
	key       SessionKey
	preempted bool
}

type runConfig struct {
	dbFile       string
	idleMinutes  int
	maxOpen      int
	defaultLevel int
	allLevel3    bool
	offline      bool
	hasPerm      bool
	permLevel    int
	repoConfig   bool
}

const queryRunConfigSQL = `SELECT
	coalesce((SELECT file FROM pragma_database_list WHERE name = 'main'), ''),
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'idle_minutes'), 15),
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'max_open'), 4),
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'default_level'), 2),
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'all_level3'), 0),
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'offline'), 0),
	coalesce((SELECT 1 FROM perm WHERE agent = ? AND g = ?), 0),
	coalesce((SELECT level FROM perm WHERE agent = ? AND g = ?), 0),
	coalesce((SELECT repo_config FROM perm WHERE agent = ? AND g = ?), 0)`

// runtimePart holds all per-host session runtime state via Part[runtimePart](h).
//
// Lock order: mu is a leaf lock. Nothing called under mu acquires Sessions,
// Rows, Host, Store, Driver, Process or callback locks.
type runtimePart struct {
	mu          sync.Mutex
	wg          sync.WaitGroup
	procs       map[SessionKey]*liveProc
	closing     map[SessionKey]bool
	starting    map[SessionKey]bool
	waiters     []chan struct{}
	jumping     map[aiSlot]bool
	interrupted map[aiSlot]preemptedTurn
	usage       map[aiSlot]proto.Usage
	limited     map[aiSlot]bool
	pathCancels map[string]context.CancelFunc
	savedCrash  map[string][]Session
}

func stateOf(h *Host) *runtimePart {
	rt := Part[runtimePart](h)
	rt.mu.Lock()
	if rt.procs == nil {
		rt.procs = map[SessionKey]*liveProc{}
		rt.closing = map[SessionKey]bool{}
		rt.starting = map[SessionKey]bool{}
		rt.jumping = map[aiSlot]bool{}
		rt.interrupted = map[aiSlot]preemptedTurn{}
		rt.usage = map[aiSlot]proto.Usage{}
		rt.limited = map[aiSlot]bool{}
		rt.pathCancels = map[string]context.CancelFunc{}
		rt.savedCrash = map[string][]Session{}
	}
	rt.mu.Unlock()
	return rt
}

func (rt *runtimePart) addWaiterLocked() <-chan struct{} {
	// Single reader: the one waitSignal caller that registered this channel.
	ch := make(chan struct{})
	rt.waiters = append(rt.waiters, ch)
	return ch
}

func (rt *runtimePart) notifyLocked() {
	for _, ch := range rt.waiters {
		close(ch)
	}
	rt.waiters = nil
}

func (rt *runtimePart) notify() {
	rt.mu.Lock()
	rt.notifyLocked()
	rt.mu.Unlock()
}

type liveProc struct {
	key          SessionKey
	session      Session
	token        string
	proc         Process
	cancel       context.CancelFunc
	closeOnce    sync.Once
	sentOnce     sync.Once
	idleTimer    *time.Timer
	idleGen      uint64
	turnActive   bool
	resumed      bool
	origText     string
	origTurn     Turn
	noSession    bool
	loginInvalid bool
	eventText    string
	writeErr     string
	replacedOK   bool
	replaceErr   error
	// Single reader: runProcessLoop.
	turnSent chan struct{}
	// Single reader: interruptProcess.
	turnDone chan struct{}
	// Single reader: stopProcess, deliverInitialTurn or writeTurnToWaiting.
	loopDone chan struct{}
}

func (lp *liveProc) triggerClose() {
	lp.closeOnce.Do(func() { lp.proc.Close() })
}

func (lp *liveProc) markTurnSent(err error) {
	lp.sentOnce.Do(func() {
		if err != nil {
			lp.writeErr = err.Error()
		}
		close(lp.turnSent)
	})
}

func readRunConfig(h *Host, agent, group string) (runConfig, error) {
	var cfg runConfig
	var all3, off, hasP, repoCfg int
	scan := func(tx *store.Tx) error {
		return tx.QueryRow(queryRunConfigSQL, agent, group, agent, group, agent, group).Scan(
			&cfg.dbFile, &cfg.idleMinutes, &cfg.maxOpen, &cfg.defaultLevel,
			&all3, &off, &hasP, &cfg.permLevel, &repoCfg,
		)
	}
	err := h.Store.Write(scan)
	cfg.allLevel3 = all3 != 0
	cfg.offline = off != 0
	cfg.hasPerm = hasP != 0
	cfg.repoConfig = repoCfg != 0
	return cfg, err
}

// ReportSaved reports a crash (turn_interrupted with reason crash, plus task
// fail for a task session) for every session row that was working when the
// previous node run stopped (flows/agent-turn.md E). Rows whose home is not
// connected yet are queued and reported once Attach connects that home.
func (h *Host) ReportSaved(ctx context.Context, saved []Session) {
	rt := stateOf(h)
	for _, s := range saved {
		if s.State == Working {
			reportOrQueueSavedCrash(ctx, h, rt, s)
		}
	}
}

func reportOrQueueSavedCrash(ctx context.Context, h *Host, rt *runtimePart, s Session) {
	if sendCrashReport(ctx, h, rt, s, "") {
		return
	}
	rt.mu.Lock()
	rt.savedCrash[s.Home] = append(rt.savedCrash[s.Home], s)
	rt.mu.Unlock()
}

func attachRuntime(h *Host, device string) {
	rt := stateOf(h)
	flushSavedCrashes(h, rt, device)
	home, ok := h.Home(device)
	if !ok {
		return
	}
	startPathWatcher(h, rt, device, home)
}

func flushSavedCrashes(h *Host, rt *runtimePart, device string) {
	rt.mu.Lock()
	pending := rt.savedCrash[device]
	delete(rt.savedCrash, device)
	rt.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		for _, s := range pending {
			reportOrQueueSavedCrash(context.Background(), h, rt, s)
		}
	}()
}

func startPathWatcher(h *Host, rt *runtimePart, device string, home Home) {
	changed := home.PathChanged()
	rt.mu.Lock()
	if prev := rt.pathCancels[device]; prev != nil {
		prev()
		delete(rt.pathCancels, device)
	}
	if changed == nil {
		rt.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt.pathCancels[device] = cancel
	rt.wg.Add(1)
	rt.mu.Unlock()
	go watchHomePath(ctx, h, rt, device, home, changed)
}

// watchHomePath is the single reader of home.PathChanged() for an attached home.
func watchHomePath(ctx context.Context, h *Host, rt *runtimePart, device string, home Home, changed <-chan struct{}) {
	defer rt.wg.Done()
	last := home.Path()
	for {
		if !waitPathSignal(ctx, home.Done(), changed) {
			return
		}
		if next := home.Path(); next != last {
			last = next
			reportHomeStatuses(h, rt, device)
		}
	}
}

func waitPathSignal(ctx context.Context, done, changed <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return false
	case _, ok := <-changed:
		return ok
	}
}

func reportHomeStatuses(h *Host, rt *runtimePart, device string) {
	seen := map[aiSlot]bool{}
	for _, s := range h.Sessions.All() {
		slot := aiSlot{group: s.Group, member: s.Member}
		if s.Home == device && !seen[slot] {
			seen[slot] = true
			sendMemberStatus(h, rt, device, s.Group, s.Member)
		}
	}
}

func (sessionRunner) Answer(h *Host, key SessionKey, ask string, allow bool) error {
	if _, ok := h.Sessions.Get(key); !ok {
		return &Error{"not_found", "no such session"}
	}
	rt := stateOf(h)
	rt.mu.Lock()
	lp := rt.procs[key]
	rt.mu.Unlock()
	if lp == nil {
		return &Error{"not_found", "no open process for session"}
	}
	return lp.proc.Answer(ask, allow)
}

func (sessionRunner) Turn(ctx context.Context, h *Host, key SessionKey, text string, turn Turn) error {
	rt := stateOf(h)
	slot := aiSlot{group: key.Group, member: key.Member}
	if turn.Jump {
		prepareJumpTurn(ctx, h, rt, slot, key, text, turn)
		defer rt.clearJumping(slot)
	}
	return dispatchSessionTurn(ctx, h, rt, key, text, turn)
}

func (rt *runtimePart) clearJumping(slot aiSlot) {
	rt.mu.Lock()
	delete(rt.jumping, slot)
	rt.mu.Unlock()
}

func dispatchSessionTurn(ctx context.Context, h *Host, rt *runtimePart, key SessionKey, text string, turn Turn) error {
	for {
		if err := waitNotClosing(ctx, rt, key); err != nil {
			return err
		}
		s, ok := h.Sessions.Get(key)
		if !ok {
			return &Error{"not_found", "no such session"}
		}
		done, err := stepSessionTurn(ctx, h, rt, s, text, turn)
		if done {
			return err
		}
	}
}

func stepSessionTurn(ctx context.Context, h *Host, rt *runtimePart, s Session, text string, turn Turn) (bool, error) {
	if s.State == Waiting {
		return tryTurnOnWaiting(h, rt, s, text, turn)
	}
	if s.State == Takeover && turn.Kind == HandbackTurn {
		return true, startFromState(h, rt, s, Takeover, text, turn)
	}
	if s.State != Dormant {
		return true, h.Sessions.Move(s.SessionKey, Dormant, Working)
	}
	if err := acquireOpenSlot(ctx, h, rt, s.SessionKey); err != nil {
		return true, err
	}
	return true, startFromState(h, rt, s, Dormant, text, turn)
}

func waitNotClosing(ctx context.Context, rt *runtimePart, key SessionKey) error {
	for {
		rt.mu.Lock()
		closing := rt.closing[key]
		var waitCh <-chan struct{}
		if closing {
			waitCh = rt.addWaiterLocked()
		}
		rt.mu.Unlock()
		if !closing {
			return nil
		}
		if err := waitSignal(ctx, waitCh); err != nil {
			return err
		}
	}
}

func tryTurnOnWaiting(h *Host, rt *runtimePart, s Session, text string, turn Turn) (bool, error) {
	lp, retry := claimWaitingProc(rt, s.SessionKey, text, turn)
	if retry {
		return false, nil
	}
	if lp == nil {
		if err := h.Sessions.Move(s.SessionKey, Waiting, Dormant); err != nil {
			return true, err
		}
		return false, nil
	}
	return true, writeTurnToWaiting(h, rt, s, lp, text, turn)
}

func claimWaitingProc(rt *runtimePart, key SessionKey, text string, turn Turn) (*liveProc, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closing[key] {
		return nil, true
	}
	lp := rt.procs[key]
	if lp == nil || lp.turnActive {
		return nil, false
	}
	stopIdleLocked(lp)
	resetTurnChannelsLocked(lp, text, turn)
	return lp, false
}

func stopIdleLocked(lp *liveProc) {
	lp.idleGen++
	if lp.idleTimer != nil {
		lp.idleTimer.Stop()
		lp.idleTimer = nil
	}
}

func resetTurnChannelsLocked(lp *liveProc, text string, turn Turn) {
	lp.turnActive = true
	lp.resumed = false
	lp.origText = text
	lp.origTurn = turn
	lp.eventText = ""
	lp.writeErr = ""
	lp.sentOnce = sync.Once{}
	lp.turnSent = make(chan struct{})
	lp.turnDone = make(chan struct{})
}

func writeTurnToWaiting(h *Host, rt *runtimePart, s Session, lp *liveProc, text string, turn Turn) error {
	if err := h.Sessions.Move(s.SessionKey, Waiting, Working); err != nil {
		unclaimWaitingProc(h, rt, lp)
		return err
	}
	sendMemberStatus(h, rt, s.Home, s.Group, s.Member)
	h.Sessions.SetTurn(s.SessionKey, turn)
	err := lp.proc.Turn(text)
	if err == nil {
		h.Report(s.SessionKey, Event{Kind: TurnBegan})
		lp.markTurnSent(nil)
		return nil
	}
	lp.markTurnSent(err)
	lp.cancel()
	<-lp.loopDone
	return err
}

func unclaimWaitingProc(h *Host, rt *runtimePart, lp *liveProc) {
	cfg, _ := readRunConfig(h, lp.session.Agent, lp.key.Group)
	idle := idleDuration(cfg.idleMinutes)
	rt.mu.Lock()
	lp.turnActive = false
	armIdleLocked(h, rt, lp, idle)
	rt.mu.Unlock()
}

func startFromState(h *Host, rt *runtimePart, s Session, from State, text string, turn Turn) error {
	err := h.Sessions.Move(s.SessionKey, from, Working)
	rt.releaseStarting(s.SessionKey)
	if err != nil {
		return err
	}
	fresh, _ := h.Sessions.Get(s.SessionKey)
	return startAndWriteTurn(h, rt, fresh, text, turn)
}

func (rt *runtimePart) releaseStarting(key SessionKey) {
	rt.mu.Lock()
	delete(rt.starting, key)
	rt.notifyLocked()
	rt.mu.Unlock()
}

func startAndWriteTurn(h *Host, rt *runtimePart, s Session, text string, turn Turn) error {
	// Single reader: interruptRunningTurn.
	turnDone := make(chan struct{})
	lp, err := launchSessionProcess(h, rt, s, text, turn, turnDone)
	if err != nil {
		abortWorkingStart(h, rt, s, turnDone)
		return err
	}
	sendMemberStatus(h, rt, s.Home, s.Group, s.Member)
	return deliverInitialTurn(h, lp, text, turn)
}

func abortWorkingStart(h *Host, rt *runtimePart, s Session, turnDone chan struct{}) {
	h.Sessions.Move(s.SessionKey, Working, Dormant)
	close(turnDone)
	rt.notify()
	sendMemberStatus(h, rt, s.Home, s.Group, s.Member)
}

func deliverInitialTurn(h *Host, lp *liveProc, text string, turn Turn) error {
	h.Sessions.SetTurn(lp.key, turn)
	err := lp.proc.Turn(text)
	if err == nil {
		h.Report(lp.key, Event{Kind: TurnBegan})
		lp.markTurnSent(nil)
		return nil
	}
	lp.markTurnSent(err)
	lp.triggerClose()
	<-lp.loopDone
	if lp.replacedOK {
		return nil
	}
	return cmp.Or(lp.replaceErr, err)
}

func launchSessionProcess(h *Host, rt *runtimePart, s Session, text string, turn Turn, turnDone chan struct{}) (*liveProc, error) {
	cfg, err := readRunConfig(h, s.Agent, s.Group)
	if err != nil {
		return nil, err
	}
	driver := Drivers[s.Agent]
	if driver == nil {
		return nil, &Error{"bad_request", "unknown agent: " + s.Agent}
	}
	level := effectiveLevel(s, cfg, runtime.GOOS)
	token := h.Sessions.Admit(s.SessionKey, level, false)
	launch := buildLaunch(h, s, cfg, level, token)
	procCtx, procCancel := context.WithCancel(context.Background())
	proc, err := driver.Start(procCtx, launch)
	if err != nil {
		procCancel()
		h.Sessions.Forget(token)
		return nil, err
	}
	return registerLiveProc(h, rt, s, token, proc, procCancel, text, turn, turnDone), nil
}

func registerLiveProc(h *Host, rt *runtimePart, s Session, token string, proc Process, cancel context.CancelFunc, text string, turn Turn, turnDone chan struct{}) *liveProc {
	lp := &liveProc{
		key:        s.SessionKey,
		session:    s,
		token:      token,
		proc:       proc,
		cancel:     cancel,
		turnActive: true,
		resumed:    s.CLISession != "",
		origText:   text,
		origTurn:   turn,
		// Single reader: runProcessLoop.
		turnSent: make(chan struct{}),
		turnDone: turnDone,
		// Single reader: stopProcess or deliverInitialTurn.
		loopDone: make(chan struct{}),
	}
	rt.mu.Lock()
	rt.procs[s.SessionKey] = lp
	rt.wg.Add(1)
	rt.mu.Unlock()
	go runProcessLoop(h, rt, lp)
	return lp
}

func buildLaunch(h *Host, s Session, cfg runConfig, level int, token string) Launch {
	return Launch{
		Folder:       resolveSessionFolder(cfg.dbFile, s.Group, s.Folder),
		Session:      s.CLISession,
		Level:        level,
		Shim:         append(slices.Clone(h.Shim), "--token", token),
		Instructions: sessionInstructions(s),
		RepoConfig:   effectiveRepoConfig(s, cfg),
		ToolTimeout:  groupToolTimeout(h, s.Group),
	}
}

func resolveSessionFolder(dbFile, group, folder string) string {
	if filepath.IsAbs(folder) {
		return folder
	}
	if dbFile != "" {
		return filepath.Join(filepath.Dir(dbFile), "projects", group, folder)
	}
	abs, _ := filepath.Abs(folder)
	return abs
}

func effectiveLevel(s Session, cfg runConfig, goos string) int {
	if s.Kind != TaskSession {
		return 1
	}
	if cfg.allLevel3 {
		return 3
	}
	if cfg.hasPerm {
		return cfg.permLevel
	}
	return fallbackClaudeWindows(cfg.defaultLevel, s.Agent, goos)
}

func fallbackClaudeWindows(level int, agent, goos string) int {
	if level == 2 && agent == "claude" && goos == "windows" {
		return 1
	}
	return level
}

func effectiveRepoConfig(s Session, cfg runConfig) bool {
	return s.Kind != GroupSession && cfg.repoConfig
}

func sessionInstructions(s Session) string {
	if Instructions != nil {
		return Instructions(s)
	}
	return ""
}

func groupToolTimeout(h *Host, group string) time.Duration {
	if raw, ok := h.Rows.Get(group, "group", group); ok {
		var grp proto.GroupRow
		if proto.Decode(raw, &grp) == nil && grp.Limits.TurnMinutes > 0 {
			return time.Duration(grp.Limits.TurnMinutes) * time.Minute
		}
	}
	return defaultTurnMinutes * time.Minute
}

// runProcessLoop is the single reader of lp.proc.Events().
func runProcessLoop(h *Host, rt *runtimePart, lp *liveProc) {
	defer rt.wg.Done()
	defer close(lp.loopDone)
	defer lp.cancel()
	for e := range lp.proc.Events() {
		handleProcessEvent(h, rt, lp, e)
	}
	h.Sessions.Forget(lp.token)
	<-lp.turnSent
	finishExitedProcess(h, rt, lp)
}

func handleProcessEvent(h *Host, rt *runtimePart, lp *liveProc, e Event) {
	slot := aiSlot{group: lp.key.Group, member: lp.key.Member}
	switch e.Kind {
	case Ready:
		onProcessReady(h, rt, lp.key, slot, e)
	case Used:
		rt.recordUsage(slot, e.Usage)
		h.Report(lp.key, e)
	case NoSession:
		onProcessNoSession(h, lp, e)
	case LoginInvalid:
		onProcessLoginInvalid(h, rt, lp, slot, e)
	case TurnEnded:
		onProcessTurnEnded(h, rt, lp, slot, e)
	default:
		noteEventText(lp, e.Text)
		h.Report(lp.key, e)
	}
}

func noteEventText(lp *liveProc, text string) {
	if text != "" {
		lp.eventText = text
	}
}

func onProcessReady(h *Host, rt *runtimePart, key SessionKey, slot aiSlot, e Event) {
	rt.setLimited(slot, false)
	if e.Session != "" {
		setID := func(s *Session) { s.CLISession = e.Session }
		h.Sessions.Update(key, setID)
	}
	h.Report(key, e)
}

func onProcessNoSession(h *Host, lp *liveProc, e Event) {
	h.Report(lp.key, e)
	if lp.resumed {
		lp.noSession = true
		lp.triggerClose()
	}
}

func onProcessLoginInvalid(h *Host, rt *runtimePart, lp *liveProc, slot aiSlot, e Event) {
	lp.loginInvalid = true
	rt.setLimited(slot, true)
	h.Report(lp.key, e)
}

func onProcessTurnEnded(h *Host, rt *runtimePart, lp *liveProc, slot aiSlot, e Event) {
	<-lp.turnSent
	rt.setLimited(slot, false)
	rt.recordUsage(slot, e.Usage)
	h.Report(lp.key, e)
	cfg, _ := readRunConfig(h, lp.session.Agent, lp.key.Group)
	idle := idleDuration(cfg.idleMinutes)
	done := markTurnFinished(h, rt, lp, idle)
	h.Sessions.Move(lp.key, Working, Waiting)
	sendMemberStatus(h, rt, lp.session.Home, lp.key.Group, lp.key.Member)
	close(done)
	rt.notify()
	afterTurnEnded(h, rt, slot)
}

func markTurnFinished(h *Host, rt *runtimePart, lp *liveProc, idle time.Duration) chan struct{} {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	lp.turnActive = false
	lp.loginInvalid = false
	done := lp.turnDone
	armIdleLocked(h, rt, lp, idle)
	return done
}

func idleDuration(minutes int) time.Duration {
	if minutes <= 0 {
		minutes = defaultIdleMinutes
	}
	return time.Duration(minutes) * idleUnit
}

func armIdleLocked(h *Host, rt *runtimePart, lp *liveProc, idle time.Duration) {
	lp.idleGen++
	gen := lp.idleGen
	key := lp.key
	lp.idleTimer = time.AfterFunc(idle, func() { onIdleTimeout(h, rt, key, gen) })
}

func onIdleTimeout(h *Host, rt *runtimePart, key SessionKey, gen uint64) {
	lp := claimIdleClose(rt, key, gen)
	if lp != nil {
		defer rt.wg.Done()
		finishWaitingClose(h, rt, lp)
	}
}

func claimIdleClose(rt *runtimePart, key SessionKey, gen uint64) *liveProc {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	lp := rt.procs[key]
	if lp == nil || lp.idleGen != gen || lp.turnActive || rt.closing[key] {
		return nil
	}
	rt.closing[key] = true
	lp.idleTimer = nil
	rt.wg.Add(1)
	return lp
}

func closeWaitingSession(h *Host, rt *runtimePart, key SessionKey) {
	lp, busyOrClosing := claimWaitingClose(rt, key)
	if lp != nil {
		finishWaitingClose(h, rt, lp)
		return
	}
	if busyOrClosing {
		return
	}
	if s, ok := h.Sessions.Get(key); ok && h.Sessions.Move(key, Waiting, Dormant) == nil {
		rt.notify()
		sendMemberStatus(h, rt, s.Home, s.Group, s.Member)
	}
}

func claimWaitingClose(rt *runtimePart, key SessionKey) (*liveProc, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closing[key] {
		return nil, true
	}
	lp := rt.procs[key]
	if lp == nil {
		return nil, false
	}
	if lp.turnActive {
		return nil, true
	}
	rt.closing[key] = true
	stopIdleLocked(lp)
	return lp, false
}

func finishWaitingClose(h *Host, rt *runtimePart, lp *liveProc) {
	if stopProcess(lp) {
		writeKilledAudit(h, lp.session)
	}
	h.Sessions.Move(lp.key, Waiting, Dormant)
	rt.mu.Lock()
	delete(rt.procs, lp.key)
	delete(rt.closing, lp.key)
	rt.notifyLocked()
	rt.mu.Unlock()
	sendMemberStatus(h, rt, lp.session.Home, lp.key.Group, lp.key.Member)
}

func writeKilledAudit(h *Host, s Session) {
	row := AuditRow{Group: s.Group, Agent: s.Agent, What: AuditKilled, Detail: "idle close timed out"}
	write := func(tx *store.Tx) error {
		_, err := h.Audit(tx, row)
		return err
	}
	h.Store.Write(write)
}

func stopProcess(lp *liveProc) bool {
	lp.triggerClose()
	if !waitTimed(lp.loopDone, stopTimeout) {
		return false
	}
	lp.cancel()
	<-lp.loopDone
	return true
}

func waitTimed(done <-chan struct{}, limit time.Duration) bool {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}

func waitSignal(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func finishExitedProcess(h *Host, rt *runtimePart, lp *liveProc) {
	wasTurn, spontaneousIdle := classifyProcessExit(rt, lp)
	if wasTurn {
		finishTurnProcessExit(h, rt, lp)
		return
	}
	if spontaneousIdle {
		h.Sessions.Move(lp.key, Waiting, Dormant)
		rt.notify()
		sendMemberStatus(h, rt, lp.session.Home, lp.key.Group, lp.key.Member)
	}
}

func classifyProcessExit(rt *runtimePart, lp *liveProc) (bool, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if lp.turnActive {
		return true, false
	}
	if rt.closing[lp.key] {
		return false, false
	}
	stopIdleLocked(lp)
	delete(rt.procs, lp.key)
	return false, true
}

func finishTurnProcessExit(h *Host, rt *runtimePart, lp *liveProc) {
	if lp.noSession {
		handleNoSession(h, rt, lp)
		return
	}
	slot := aiSlot{group: lp.key.Group, member: lp.key.Member}
	detachExitedTurnProc(rt, lp)
	h.Sessions.Move(lp.key, Working, Dormant)
	if lp.loginInvalid {
		sendMemberStatus(h, rt, lp.session.Home, lp.key.Group, lp.key.Member)
	} else {
		sendCrashReport(context.Background(), h, rt, lp.session, cmp.Or(lp.eventText, lp.writeErr))
	}
	close(lp.turnDone)
	rt.notify()
	afterTurnEnded(h, rt, slot)
}

func detachExitedTurnProc(rt *runtimePart, lp *liveProc) {
	rt.mu.Lock()
	lp.turnActive = false
	delete(rt.procs, lp.key)
	rt.mu.Unlock()
}

func handleNoSession(h *Host, rt *runtimePart, lp *liveProc) {
	detachExitedTurnProc(rt, lp)
	var fresh Session
	clearID := func(s *Session) {
		s.CLISession = ""
		fresh = *s
	}
	if err := h.Sessions.Update(lp.key, clearID); err != nil {
		failReplacement(h, rt, lp, err)
		return
	}
	replText, replTurn, err := composeReplacedTurn(h, fresh, lp.origText, lp.origTurn)
	if err != nil {
		failReplacement(h, rt, lp, err)
		return
	}
	startReplacementProcess(h, rt, lp, fresh, replText, replTurn)
}

func composeReplacedTurn(h *Host, s Session, origText string, origTurn Turn) (string, Turn, error) {
	if Compose == nil {
		origTurn.Kind = ReplacedTurn
		return origText, origTurn, nil
	}
	text, turn, err := Compose(h, s, ReplacedTurn, nil)
	if err != nil {
		return "", Turn{}, err
	}
	turn.Kind = ReplacedTurn
	turn.Jump = origTurn.Jump
	turn.Depth = cmp.Or(turn.Depth, origTurn.Depth)
	turn.Thread = cmp.Or(turn.Thread, origTurn.Thread)
	return text, turn, nil
}

func startReplacementProcess(h *Host, rt *runtimePart, old *liveProc, fresh Session, text string, turn Turn) {
	lp2, err := launchSessionProcess(h, rt, fresh, text, turn, old.turnDone)
	if err != nil {
		failReplacement(h, rt, old, err)
		return
	}
	ev := proto.SessionReplacedEvent{
		SystemCode: proto.SystemCode{Code: "session_replaced"},
		Task:       proto.ID(fresh.Task),
	}
	sendMemberStatusEvent(h, rt, fresh.Home, fresh.Group, fresh.Member, ev)
	if err := deliverInitialTurn(h, lp2, text, turn); err != nil {
		old.replaceErr = err
		return
	}
	old.replacedOK = true
}

func failReplacement(h *Host, rt *runtimePart, lp *liveProc, err error) {
	lp.replaceErr = err
	abortWorkingStart(h, rt, lp.session, lp.turnDone)
	afterTurnEnded(h, rt, aiSlot{group: lp.key.Group, member: lp.key.Member})
}

func acquireOpenSlot(ctx context.Context, h *Host, rt *runtimePart, key SessionKey) error {
	for {
		done, err := stepOpenSlot(ctx, h, rt, key)
		if done || err != nil {
			return err
		}
	}
}

func stepOpenSlot(ctx context.Context, h *Host, rt *runtimePart, key SessionKey) (bool, error) {
	if err := checkGroupMemberFree(h, key); err != nil {
		return false, err
	}
	cfg, err := readRunConfig(h, "", key.Group)
	if err != nil {
		return false, err
	}
	victim, waitCh, acquired := tryReserveSlot(h, rt, key, cfg.maxOpen)
	if acquired {
		return true, nil
	}
	if victim != nil {
		closeWaitingSession(h, rt, *victim)
		return false, nil
	}
	return false, waitSignal(ctx, waitCh)
}

func checkGroupMemberFree(h *Host, key SessionKey) error {
	for _, s := range h.Sessions.All() {
		if s.SessionKey != key && s.Group == key.Group && s.Member == key.Member && active(s.State) {
			return &Error{"busy", "another session of this AI in the group is working or taken over"}
		}
	}
	return nil
}

func tryReserveSlot(h *Host, rt *runtimePart, key SessionKey, maxOpen int) (*SessionKey, <-chan struct{}, bool) {
	if maxOpen <= 0 {
		maxOpen = defaultMaxOpen
	}
	sessions := h.Sessions.All()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if countOpenLocked(rt, sessions) < maxOpen {
		rt.starting[key] = true
		return nil, nil, true
	}
	if victim := longestWaitingLocked(rt, sessions); victim != nil {
		return victim, nil, false
	}
	return nil, rt.addWaiterLocked(), false
}

func countOpenLocked(rt *runtimePart, sessions []Session) int {
	open := 0
	for _, s := range sessions {
		if s.State != Dormant || rt.starting[s.SessionKey] {
			open++
		}
	}
	return open
}

func longestWaitingLocked(rt *runtimePart, sessions []Session) *SessionKey {
	var best *Session
	for i := range sessions {
		s := &sessions[i]
		if !eligibleWaitingLocked(rt, *s) {
			continue
		}
		if best == nil || s.LastActive < best.LastActive {
			best = s
		}
	}
	if best == nil {
		return nil
	}
	chosen := best.SessionKey
	return &chosen
}

func eligibleWaitingLocked(rt *runtimePart, s Session) bool {
	if s.State != Waiting || rt.closing[s.SessionKey] {
		return false
	}
	lp := rt.procs[s.SessionKey]
	return lp == nil || !lp.turnActive
}

func prepareJumpTurn(ctx context.Context, h *Host, rt *runtimePart, slot aiSlot, target SessionKey, text string, turn Turn) {
	running, lp, turnDone := claimJumpInterrupt(h, rt, slot)
	if running == nil {
		return
	}
	if lp != nil {
		interruptProcess(lp, turnDone)
	}
	if running.SessionKey == target {
		return
	}
	preempted := reportPreemptedIfTask(ctx, h, *running, turn, text)
	rt.recordInterrupted(slot, running.SessionKey, preempted)
}

func claimJumpInterrupt(h *Host, rt *runtimePart, slot aiSlot) (*Session, *liveProc, <-chan struct{}) {
	var running *Session
	for _, s := range h.Sessions.All() {
		if s.Group == slot.group && s.Member == slot.member && s.State == Working {
			found := s
			running = &found
			break
		}
	}
	if running == nil {
		return nil, nil, nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.jumping[slot] = true
	lp := rt.procs[running.SessionKey]
	if lp == nil || !lp.turnActive {
		return running, nil, nil
	}
	return running, lp, lp.turnDone
}

func interruptProcess(lp *liveProc, turnDone <-chan struct{}) {
	lp.proc.Interrupt()
	if waitTimed(turnDone, stopTimeout) {
		lp.cancel()
		<-turnDone
	}
}

func reportPreemptedIfTask(ctx context.Context, h *Host, interrupted Session, turn Turn, text string) bool {
	if interrupted.Kind != TaskSession {
		return false
	}
	msgID := resolveJumpMsg(ctx, h, interrupted, turn, text)
	if msgID == "" {
		return false
	}
	cmd := proto.TaskPreempted{
		TaskCommand: buildTaskCmd(h, interrupted.Group, "preempted", interrupted.Task),
		Msg:         msgID,
	}
	_, err := h.Command(ctx, interrupted.Home, marshalWire(cmd))
	return err == nil
}

func resolveJumpMsg(ctx context.Context, h *Host, interrupted Session, turn Turn, turnText string) proto.ID {
	home, ok := h.Home(interrupted.Home)
	if !ok {
		return ""
	}
	var thread *string
	if turn.Thread != "" {
		th := turn.Thread
		thread = &th
	}
	req := func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: interrupted.Group, Before: 0, Limit: 20},
			Thread:    thread,
		}
	}
	raw, err := home.Request(ctx, req)
	if err != nil {
		return ""
	}
	return pickJumpMessage(h, interrupted.Group, raw, turnText)
}

func pickJumpMessage(h *Host, group string, raw json.RawMessage, turnText string) proto.ID {
	var fetched proto.Fetched
	if proto.Decode(raw, &fetched) != nil {
		return ""
	}
	return selectJumpMsgID(h, group, fetched.Rows, turnText)
}

func selectJumpMsgID(h *Host, group string, rows []json.RawMessage, turnText string) proto.ID {
	var fallback proto.ID
	for _, elem := range rows {
		msg, ok := decodePersonMsg(h, group, elem)
		if !ok {
			continue
		}
		fallback = cmp.Or(fallback, proto.ID(msg.ID))
		if matchesTurnTail(turnText, msg.Text) {
			return proto.ID(msg.ID)
		}
	}
	return fallback
}

func matchesTurnTail(turnText, msgText string) bool {
	return msgText != "" && strings.HasSuffix(turnText, "\n"+msgText)
}

func decodePersonMsg(h *Host, group string, elem json.RawMessage) (proto.MsgRow, bool) {
	var msg proto.MsgRow
	if proto.Decode(elem, &msg) != nil || msg.By == "system" {
		return msg, false
	}
	if raw, ok := h.Rows.Get(group, "member", msg.By); ok {
		var m proto.MemberRow
		return msg, proto.Decode(raw, &m) == nil && m.Kind == "human"
	}
	return msg, !isAIEnvelope(msg.Env)
}

func isAIEnvelope(env *proto.Envelope) bool {
	if env == nil {
		return false
	}
	var hdr proto.HomeCommand
	return proto.Decode([]byte(env.Payload), &hdr) == nil && hdr.As != ""
}

func (rt *runtimePart) recordInterrupted(slot aiSlot, key SessionKey, preempted bool) {
	rt.mu.Lock()
	if _, exists := rt.interrupted[slot]; !exists {
		rt.interrupted[slot] = preemptedTurn{key: key, preempted: preempted}
	}
	rt.mu.Unlock()
}

func (rt *runtimePart) nextAfterTurn(slot aiSlot) (bool, preemptedTurn, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.jumping[slot] {
		return true, preemptedTurn{}, false
	}
	cont, ok := rt.interrupted[slot]
	delete(rt.interrupted, slot)
	return false, cont, ok
}

func afterTurnEnded(h *Host, rt *runtimePart, slot aiSlot) {
	jumping, cont, ok := rt.nextAfterTurn(slot)
	if jumping {
		return
	}
	if ok && resumeInterruptedSession(h, cont) {
		return
	}
	if Deliver != nil {
		Deliver(h, slot.group, slot.member)
	}
}

func resumeInterruptedSession(h *Host, cont preemptedTurn) bool {
	s, ok := h.Sessions.Get(cont.key)
	if !ok || !prepareInterruptedResume(h, s, cont.preempted) {
		return false
	}
	text, turn, err := composeContinueTurn(h, s)
	if err != nil {
		return false
	}
	return Runner.Turn(context.Background(), h, s.SessionKey, text, turn) == nil
}

func prepareInterruptedResume(h *Host, s Session, preempted bool) bool {
	if isContinueHeld(h, s) {
		return false
	}
	if preempted {
		cmd := buildTaskCmd(h, s.Group, "resume", s.Task)
		_, err := h.Command(context.Background(), s.Home, marshalWire(cmd))
		return err == nil
	}
	return !isTaskInactive(h, s)
}

func isContinueHeld(h *Host, s Session) bool {
	cfg, err := readRunConfig(h, s.Agent, s.Group)
	if err != nil || cfg.offline || isGroupPaused(h, s.Group) {
		return true
	}
	_, held := h.Held(s.SessionKey)
	return held
}

func isTaskInactive(h *Host, s Session) bool {
	if s.Kind != TaskSession {
		return false
	}
	raw, ok := h.Rows.Get(s.Group, "task", s.Task)
	if !ok {
		return false
	}
	var task proto.TaskRow
	return proto.Decode(raw, &task) == nil && task.State != "active"
}

func composeContinueTurn(h *Host, s Session) (string, Turn, error) {
	if Compose != nil {
		return Compose(h, s, ContinueTurn, nil)
	}
	body := "You were interrupted by a direct request from your owner. Continue."
	if s.Kind == TaskSession {
		body = "You were interrupted by a direct request from your owner. Continue the task."
	}
	return body, Turn{Kind: ContinueTurn, Thread: s.Task}, nil
}

func (rt *runtimePart) recordUsage(slot aiSlot, u Usage) {
	rt.mu.Lock()
	cur := rt.usage[slot]
	if u.WindowPct != nil {
		pct := *u.WindowPct
		cur.WindowPct = &pct
	}
	if u.WindowResets != nil {
		resets := *u.WindowResets
		cur.WindowResets = &resets
	}
	rt.usage[slot] = cur
	rt.mu.Unlock()
}

func (rt *runtimePart) getUsage(slot aiSlot) proto.Usage {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	cur := rt.usage[slot]
	var out proto.Usage
	if cur.WindowPct != nil {
		pct := *cur.WindowPct
		out.WindowPct = &pct
	}
	if cur.WindowResets != nil {
		resets := *cur.WindowResets
		out.WindowResets = &resets
	}
	return out
}

func (rt *runtimePart) setLimited(slot aiSlot, v bool) {
	rt.mu.Lock()
	if v {
		rt.limited[slot] = true
	} else {
		delete(rt.limited, slot)
	}
	rt.mu.Unlock()
}

func (rt *runtimePart) isLimited(slot aiSlot) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.limited[slot]
}

func sendCrashReport(ctx context.Context, h *Host, rt *runtimePart, s Session, rawDetail string) bool {
	detail := RedactSecrets(rawDetail)
	ev := proto.TurnInterruptedEvent{
		SystemCode: proto.SystemCode{Code: "turn_interrupted"},
		Reason:     "crash",
		Task:       proto.ID(s.Task),
		Detail:     detail,
	}
	if err := sendMemberStatusWithEvent(ctx, h, rt, s.Home, s.Group, s.Member, &ev); err != nil {
		return !isUnreachable(err)
	}
	if s.Kind != TaskSession {
		return true
	}
	fail := proto.TaskFail{
		TaskCommand: buildTaskCmd(h, s.Group, "fail", s.Task),
		Reason:      "crash",
		Detail:      detail,
	}
	_, err := h.Command(ctx, s.Home, marshalWire(fail))
	return !isUnreachable(err)
}

func isUnreachable(err error) bool {
	if err == nil {
		return false
	}
	he, ok := err.(*Error)
	return ok && he.Code == "home_unreachable"
}

func sendMemberStatus(h *Host, rt *runtimePart, homeDevice, group, member string) {
	sendMemberStatusWithEvent[proto.SystemCode](context.Background(), h, rt, homeDevice, group, member, nil)
}

func sendMemberStatusEvent[E any](h *Host, rt *runtimePart, homeDevice, group, member string, ev E) {
	sendMemberStatusWithEvent(context.Background(), h, rt, homeDevice, group, member, &ev)
}

func sendMemberStatusWithEvent[E any](ctx context.Context, h *Host, rt *runtimePart, homeDevice, group, member string, ev *E) error {
	home, ok := h.Home(homeDevice)
	if !ok {
		return &Error{"home_unreachable", "the home of that group is not connected"}
	}
	cfg, _ := readRunConfig(h, "", group)
	slot := aiSlot{group: group, member: member}
	state, task := deriveMemberStatus(h, rt, cfg, slot)
	status := proto.MemberStatus[E]{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{
				Type:      "member",
				Nonce:     freshNonce(),
				Timestamp: h.Now().UnixMilli(),
				Group:     proto.ID(group),
			},
			Op: "status",
			ID: proto.ID(member),
		},
		State: state,
		Task:  proto.ID(task),
		Path:  home.Path(),
		Usage: rt.getUsage(slot),
		Event: ev,
	}
	_, err := h.Command(ctx, homeDevice, marshalWire(status))
	return err
}

func deriveMemberStatus(h *Host, rt *runtimePart, cfg runConfig, slot aiSlot) (string, string) {
	list := memberSessions(h, slot.group, slot.member)
	if s, ok := findSessionInState(list, Takeover); ok {
		return string(Takeover), s.Task
	}
	if s, ok := findSessionInState(list, Working); ok {
		return string(Working), s.Task
	}
	if over := deriveOverlayState(h, rt, cfg, list, slot); over != "" {
		return over, ""
	}
	if s, ok := latestWaitingSession(list); ok {
		return string(Waiting), s.Task
	}
	return string(Dormant), ""
}

func memberSessions(h *Host, group, member string) []Session {
	var list []Session
	for _, s := range h.Sessions.All() {
		if s.Group == group && s.Member == member {
			list = append(list, s)
		}
	}
	return list
}

func findSessionInState(list []Session, want State) (Session, bool) {
	for _, s := range list {
		if s.State == want {
			return s, true
		}
	}
	return Session{}, false
}

func latestWaitingSession(list []Session) (Session, bool) {
	var best Session
	found := false
	for _, s := range list {
		if s.State == Waiting && (!found || s.LastActive > best.LastActive) {
			best = s
			found = true
		}
	}
	return best, found
}

func deriveOverlayState(h *Host, rt *runtimePart, cfg runConfig, list []Session, slot aiSlot) string {
	reason, held := memberHold(h, list, slot)
	switch {
	case isOfflineOverlay(cfg, reason):
		return "offline"
	case rt.isLimited(slot) || isLimitedHold(held, reason):
		return "limited"
	case isPausedOverlay(h, slot.group, reason):
		return "paused"
	default:
		return ""
	}
}

func isOfflineOverlay(cfg runConfig, reason string) bool {
	return cfg.offline || reason == "offline"
}

func isLimitedHold(held bool, reason string) bool {
	return held && reason != "paused"
}

func isPausedOverlay(h *Host, group, reason string) bool {
	return reason == "paused" || isGroupPaused(h, group)
}

func memberHold(h *Host, list []Session, slot aiSlot) (string, bool) {
	if reason, held := h.Held(SessionKey{Group: slot.group, Member: slot.member}); held {
		return reason, true
	}
	for _, s := range list {
		if reason, held := h.Held(s.SessionKey); held {
			return reason, true
		}
	}
	return "", false
}

func isGroupPaused(h *Host, group string) bool {
	raw, ok := h.Rows.Get(group, "group", group)
	if !ok {
		return false
	}
	var grp proto.GroupRow
	return proto.Decode(raw, &grp) == nil && grp.Paused != nil
}

func buildTaskCmd(h *Host, group, op, taskID string) proto.TaskCommand {
	return proto.TaskCommand{
		HomeCommand: proto.HomeCommand{
			Type:      "task",
			Nonce:     freshNonce(),
			Timestamp: h.Now().UnixMilli(),
			Group:     proto.ID(group),
		},
		Op: op,
		ID: proto.ID(taskID),
	}
}

func freshNonce() proto.ID {
	var random [16]byte
	rand.Read(random[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(random[:]))
}

func marshalWire(v any) string {
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(out.String(), "\n")
}
