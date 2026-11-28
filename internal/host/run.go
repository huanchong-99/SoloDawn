package host

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	defaultIdleMinutes = 15
	defaultMaxOpen     = 4
	defaultTurnMinutes = 60
	stopSeconds        = 10
)

func init() {
	Provide(&Runner, Runtime(sessionRunner{}))
	Register(Attached, "runtime", attachRuntime)
	Register(Settled, "runtime", settleRuntime)
}

type sessionRunner struct{}

type aiSlot struct{ group, member string }

type launchConfig struct {
	maxOpen int
	level   int
}

const queryLaunchConfigSQL = `SELECT
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'max_open'), 4),
	iif(coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'all_level3'), 0), 3,
		coalesce((SELECT level FROM perm WHERE agent = ? AND g = ?), (SELECT json_extract(value, '$') FROM setting WHERE key = 'default_level'), 3))`

const queryIdleMinutesSQL = `SELECT coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'idle_minutes'), 15)`

type pendingCrash struct {
	session    Session
	reason     string
	detail     string
	keepTask   bool
	statusSent bool
}

// runtimePart holds all per-host session runtime state via Part[runtimePart](h).
//
// Lock order: mu may call Sessions.Get and Sessions.All, which take only the
// leaf in-memory Sessions lock. Nothing called under mu acquires Store, Rows,
// Host, Driver, Process or hook/listener locks.
type runtimePart struct {
	mu         sync.Mutex
	wg         sync.WaitGroup // the process loops, the asks and background callbacks
	settling   bool           // Settle has begun: no process starts, and a loop that ends leaves the rows as they are
	procs      map[SessionKey]*liveProc
	closing    map[SessionKey]bool
	starting   map[SessionKey]bool
	limited    map[aiSlot]bool
	savedCrash map[string][]pendingCrash
	changed    chan struct{} // closed and replaced under mu to wake waitSignal callers
}

var errSettling = &Error{"busy", "the host is shutting down"}

// settleRuntime ends every CLI process, then every open ask, whose answer
// could otherwise block on a live CLI, and waits for the loops, the asks and
// the runtime's callbacks. The session rows keep their state:
// the next start reports a turn that was running (flows/agent-turn.md E).
func settleRuntime(h *Host) {
	rt := stateOf(h)
	rt.mu.Lock()
	rt.settling = true
	rt.notifyLocked()
	for _, lp := range rt.procs {
		stopIdleLocked(lp)
		lp.cancel()
	}
	rt.mu.Unlock()
	cancelAsks(h)
	rt.wg.Wait()
}

func stateOf(h *Host) *runtimePart {
	rt := Part[runtimePart](h)
	rt.mu.Lock()
	if rt.procs == nil {
		rt.procs, rt.closing, rt.starting = map[SessionKey]*liveProc{}, map[SessionKey]bool{}, map[SessionKey]bool{}
		rt.limited, rt.savedCrash, rt.changed = map[aiSlot]bool{}, map[string][]pendingCrash{}, make(chan struct{})
	}
	rt.mu.Unlock()
	return rt
}

func (rt *runtimePart) notifyLocked() {
	close(rt.changed)
	rt.changed = make(chan struct{})
}

func (rt *runtimePart) notify() {
	rt.mu.Lock()
	rt.notifyLocked()
	rt.mu.Unlock()
}

func (rt *runtimePart) startBackground(h *Host, name string, work func(), more ...any) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.settling {
		h.bg(rt.wg.Go, name, work, more...)
	}
}

type liveProc struct {
	key             SessionKey
	session         Session
	token           string
	level           int // the level it was started with
	proc            Process
	cancel          context.CancelFunc
	closeOnce       sync.Once
	killTimer       atomic.Pointer[time.Timer]
	interruptTimer  atomic.Pointer[time.Timer]
	killed          atomic.Bool
	idleTimer       *time.Timer
	idleGen         uint64
	turnActive      bool
	resumed         bool
	origTurn        Turn
	interruptWhy    string
	interruptDetail string
	exitWhy         EventKind
	eventText       string
	turnErr         atomic.Pointer[string]
	replacedOK      bool
	replaceErr      error
	// Single reader: stopProcess or writeTurnToWaiting.
	loopDone chan struct{}
}

func (lp *liveProc) setTurnErr(err error) {
	s := err.Error()
	lp.turnErr.Store(&s)
}

func (lp *liveProc) turnErrText() string {
	if p := lp.turnErr.Load(); p != nil {
		return *p
	}
	return ""
}

func (lp *liveProc) triggerClose() {
	lp.closeOnce.Do(func() {
		kill := func() {
			lp.killed.Store(true)
			lp.cancel()
		}
		t := time.AfterFunc(stopSeconds*time.Second, kill)
		lp.killTimer.Store(t)
		lp.proc.Close()
	})
}

func (lp *liveProc) stopKillTimer() {
	if t := lp.killTimer.Load(); t != nil {
		t.Stop()
	}
}

func (lp *liveProc) stopInterruptTimer() {
	if t := lp.interruptTimer.Load(); t != nil {
		t.Stop()
	}
}

func readLaunchConfig(h *Host, agent, group string) (launchConfig, error) {
	var cfg launchConfig
	scan := func(tx *store.Tx) error {
		return tx.QueryRow(queryLaunchConfigSQL, agent, group).Scan(&cfg.maxOpen, &cfg.level)
	}
	return cfg, h.Store.Write(scan)
}

func readIdleDuration(h *Host) time.Duration {
	minutes := defaultIdleMinutes
	scan := func(tx *store.Tx) error {
		return tx.QueryRow(queryIdleMinutesSQL).Scan(&minutes)
	}
	h.Store.Write(scan)
	return idleDuration(minutes)
}

// ReportSaved reports a crash (turn_interrupted with reason crash, plus task
// fail for a task session) for every session row that was working when the
// previous node run stopped (flows/agent-turn.md E). Rows whose home is not
// connected yet are queued and reported once Attach connects that home.
func (h *Host) ReportSaved(ctx context.Context, saved []Session) {
	rt := stateOf(h)
	for _, s := range saved {
		if s.State == Working {
			reportOrQueueCrash(ctx, h, rt, pendingCrash{session: s})
		}
	}
}

func reportOrQueueCrash(ctx context.Context, h *Host, rt *runtimePart, c pendingCrash) {
	if sendCrashStep(ctx, h, rt, &c) {
		return
	}
	rt.mu.Lock()
	rt.savedCrash[c.session.Home] = append(rt.savedCrash[c.session.Home], c)
	rt.mu.Unlock()
}

func attachRuntime(h *Host, device string) {
	rt := stateOf(h)
	rt.mu.Lock()
	pending := rt.savedCrash[device]
	delete(rt.savedCrash, device)
	rt.mu.Unlock()
	slots := homeAISlots(h, device)
	rt.startBackground(h, "home_flush", func() { flushHomeRuntime(h, rt, device, pending, slots) })
}

func homeAISlots(h *Host, device string) []aiSlot {
	seen := map[aiSlot]bool{}
	var slots []aiSlot
	for _, s := range h.Sessions.All() {
		slot := aiSlot{group: s.Group, member: s.Member}
		if s.Home == device && !seen[slot] {
			seen[slot] = true
			slots = append(slots, slot)
		}
	}
	return slots
}

func flushHomeRuntime(h *Host, rt *runtimePart, device string, pending []pendingCrash, slots []aiSlot) {
	for _, c := range pending {
		reportOrQueueCrash(context.Background(), h, rt, c)
	}
	for _, slot := range slots {
		sendMemberStatus(context.Background(), h, rt, device, slot.group, slot.member, nil)
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

func (sessionRunner) Close(h *Host, key SessionKey) {
	closeSession(h, stateOf(h), key)
}

// processLevel is the level that the session's open CLI was started with, 0
// when none is open.
func processLevel(h *Host, key SessionKey) int {
	rt := stateOf(h)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if lp := rt.procs[key]; lp != nil {
		return lp.level
	}
	return 0
}

// closeSession closes the session's CLI process, also during a turn, and
// waits for it to exit. It reports whether a turn was running.
func closeSession(h *Host, rt *runtimePart, key SessionKey) bool {
	for {
		lp, wasTurn, waitCh := claimSessionClose(rt, key, true)
		if waitCh == nil {
			finishClaimedClose(h, rt, key, lp, wasTurn)
			return wasTurn
		}
		<-waitCh
	}
}

func (sessionRunner) Turn(ctx context.Context, h *Host, key SessionKey, text string, turn Turn) error {
	rt := stateOf(h)
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
	if s.State != Dormant {
		return true, startHandBack(h, rt, s, text, turn)
	}
	cfg, err := readLaunchConfig(h, s.Agent, s.Group)
	if err != nil {
		return true, err
	}
	if err := acquireOpenSlot(ctx, h, rt, s.SessionKey, cfg.maxOpen); err != nil {
		return true, err
	}
	return true, startFromDormant(h, rt, s, cfg, text, turn)
}

// startHandBack starts the structured CLI of a session in takeover for its
// hand-back turn (flows/takeover.md step 16). The session is open already, so
// it takes no slot of max_open.
func startHandBack(h *Host, rt *runtimePart, s Session, text string, turn Turn) error {
	if s.State != Takeover || turn.Kind != HandbackTurn {
		return &Error{"conflict", "the session is " + string(s.State)}
	}
	cfg, err := readLaunchConfig(h, s.Agent, s.Group)
	if err != nil {
		return err
	}
	return startFromDormant(h, rt, s, cfg, text, turn)
}

func waitNotClosing(ctx context.Context, rt *runtimePart, key SessionKey) error {
	for {
		rt.mu.Lock()
		closing := rt.closing[key]
		waitCh := rt.changed
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
	lp, retry, err := claimWaitingProc(rt, s.SessionKey)
	if retry || err != nil {
		return err != nil, err
	}
	if lp == nil {
		err := h.Sessions.Move(s.SessionKey, Waiting, Dormant)
		return err != nil, err
	}
	return true, writeTurnToWaiting(h, rt, s, lp, text, turn)
}

func claimWaitingProc(rt *runtimePart, key SessionKey) (*liveProc, bool, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closing[key] {
		return nil, true, nil
	}
	lp := rt.procs[key]
	if lp == nil {
		return nil, false, nil
	}
	if lp.turnActive {
		return nil, false, &Error{"busy", "the session is already running a turn"}
	}
	stopIdleLocked(lp)
	lp.turnActive = true
	lp.interruptWhy, lp.interruptDetail = "", ""
	return lp, false, nil
}

func stopIdleLocked(lp *liveProc) {
	lp.idleGen++
	if lp.idleTimer != nil {
		lp.idleTimer.Stop()
		lp.idleTimer = nil
	}
}

func writeTurnToWaiting(h *Host, rt *runtimePart, s Session, lp *liveProc, text string, turn Turn) error {
	if err := h.Sessions.Move(s.SessionKey, Waiting, Working); err != nil {
		unclaimWaitingProc(h, rt, lp)
		return err
	}
	sendMemberStatus(context.Background(), h, rt, s.Home, s.Group, s.Member, nil)
	h.Sessions.SetTurn(s.SessionKey, turn)
	h.Report(s.SessionKey, Event{Kind: TurnBegan})
	if err := lp.proc.Turn(text); err != nil {
		lp.setTurnErr(err)
		lp.cancel()
		<-lp.loopDone
		return err
	}
	return nil
}

func unclaimWaitingProc(h *Host, rt *runtimePart, lp *liveProc) {
	idle := readIdleDuration(h)
	rt.mu.Lock()
	lp.turnActive = false
	armIdleLocked(h, rt, lp, idle)
	rt.mu.Unlock()
}

// startFromDormant starts the CLI of a dormant session, or of one handed back
// from takeover, for a turn. Nothing starts in a folder that is gone.
func startFromDormant(h *Host, rt *runtimePart, s Session, cfg launchConfig, text string, turn Turn) error {
	if gone := missingFolder(s); gone != "" {
		rt.releaseStarting(s.SessionKey)
		return failStart(h, rt, s, gone)
	}
	err := h.Sessions.Move(s.SessionKey, s.State, Working)
	rt.releaseStarting(s.SessionKey)
	if err != nil {
		return err
	}
	fresh, _ := h.Sessions.Get(s.SessionKey)
	lp, err := launchSessionProcess(h, rt, fresh, cfg, turn)
	if err != nil {
		h.Sessions.Move(fresh.SessionKey, Working, Dormant)
		rt.notify()
		reportLaunchFailure(h, rt, fresh, err)
		return err
	}
	sendMemberStatus(context.Background(), h, rt, fresh.Home, fresh.Group, fresh.Member, nil)
	return deliverInitialTurn(h, lp, text, turn)
}

func reportLaunchFailure(h *Host, rt *runtimePart, s Session, err error) {
	if errors.Is(err, errSettling) {
		sendMemberStatus(context.Background(), h, rt, s.Home, s.Group, s.Member, nil)
		return
	}
	reportOrQueueCrash(context.Background(), h, rt, pendingCrash{session: s, detail: eventDetail(err.Error())})
}

// missingFolder is empty while the session's folder exists, and otherwise
// says that it does not (contracts/local.md section 7).
func missingFolder(s Session) string {
	if isFolder(s.Folder) {
		return ""
	}
	return "the folder " + s.Folder + " does not exist"
}

// failStart reports a start in a folder that is gone as a crash: the session
// goes dormant, the member reports turn_interrupted with the path, and a task
// session fails its task (flows/agent-turn.md E). A dormant session stays so.
func failStart(h *Host, rt *runtimePart, s Session, gone string) error {
	h.Sessions.Move(s.SessionKey, s.State, Dormant)
	reportOrQueueCrash(context.Background(), h, rt, pendingCrash{session: s, detail: eventDetail(gone)})
	return &Error{"cli", gone}
}

func (rt *runtimePart) releaseStarting(key SessionKey) {
	rt.mu.Lock()
	delete(rt.starting, key)
	rt.notifyLocked()
	rt.mu.Unlock()
}

func deliverInitialTurn(h *Host, lp *liveProc, text string, turn Turn) error {
	h.Sessions.SetTurn(lp.key, turn)
	h.Report(lp.key, Event{Kind: TurnBegan})
	err := lp.proc.Turn(text)
	if err == nil {
		return nil
	}
	lp.setTurnErr(err)
	stopProcess(lp)
	if lp.replacedOK {
		return nil
	}
	return cmp.Or(lp.replaceErr, err)
}

func launchSessionProcess(h *Host, rt *runtimePart, s Session, cfg launchConfig, turn Turn) (*liveProc, error) {
	driver := Drivers[s.Agent]
	if driver == nil {
		return nil, &Error{"bad_request", "unknown agent: " + s.Agent}
	}
	token := h.Sessions.Admit(s.SessionKey, cfg.level, false)
	launch := Launch{
		Folder: s.Folder, Session: s.CLISession,
		Level: cfg.level, Shim: append(slices.Clone(h.Shim), "--token", token), Instructions: Instructions(s),
		RepoConfig: s.Kind != GroupSession, ToolTimeout: groupToolTimeout(h, s.Group),
	}
	procCtx, procCancel := context.WithCancel(context.Background())
	proc, err := driver.Start(procCtx, memberLaunch(h, s, launch))
	if err != nil {
		procCancel()
		h.Sessions.Forget(token)
		return nil, err
	}
	lp := &liveProc{
		key: s.SessionKey, session: s, token: token, level: cfg.level, proc: proc, cancel: procCancel,
		turnActive: true, resumed: s.CLISession != "", origTurn: turn,
		loopDone: make(chan struct{}), // single reader: stopProcess or writeTurnToWaiting
	}
	rt.mu.Lock()
	settling := rt.settling
	if !settling {
		rt.procs[s.SessionKey] = lp
		rt.wg.Add(1)
	}
	rt.mu.Unlock()
	if settling {
		procCancel()
		h.Sessions.Forget(token)
		return nil, errSettling
	}
	h.diag.Info("proc.start", diagSession(s.SessionKey, "agent", s.Agent, "resume", lp.resumed)...)
	go runProcessLoop(h, rt, lp)
	return lp, nil
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
	defer lp.stopInterruptTimer()
	defer lp.stopKillTimer()
	defer lp.cancel()
	for e := range lp.proc.Events() {
		handleProcessEvent(h, rt, lp, e)
	}
	h.diag.Info("proc.exit", diagSession(lp.key, "agent", lp.session.Agent, "why", diagExitWhy(rt, lp))...)
	h.Sessions.Forget(lp.token)
	if !rt.dropIfSettling(lp) {
		finishExitedProcess(h, rt, lp)
	}
}

func (rt *runtimePart) dropIfSettling(lp *liveProc) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.settling {
		return false
	}
	delete(rt.procs, lp.key)
	delete(rt.closing, lp.key)
	rt.notifyLocked()
	return true
}

func handleProcessEvent(h *Host, rt *runtimePart, lp *liveProc, e Event) {
	slot := aiSlot{group: lp.key.Group, member: lp.key.Member}
	switch e.Kind {
	case Ready:
		onProcessReady(h, lp.key, e)
	case NoSession:
		onProcessNoSession(h, rt, lp, e)
	case LoginInvalid:
		onProcessLoginInvalid(h, rt, lp, e)
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

func (rt *runtimePart) markProcessExit(lp *liveProc, why EventKind, text string) {
	rt.mu.Lock()
	lp.exitWhy = why
	rt.mu.Unlock()
	noteEventText(lp, text)
	lp.triggerClose()
}

func (rt *runtimePart) hasExitWhy(lp *liveProc) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return lp.exitWhy != ""
}

func onProcessReady(h *Host, key SessionKey, e Event) {
	if e.Session != "" {
		setID := func(s *Session) { s.CLISession = e.Session }
		h.Sessions.Update(key, setID)
	}
	h.Report(key, e)
}

func onProcessNoSession(h *Host, rt *runtimePart, lp *liveProc, e Event) {
	h.Report(lp.key, e)
	if lp.resumed {
		rt.markProcessExit(lp, NoSession, "")
	}
}

func onProcessLoginInvalid(h *Host, rt *runtimePart, lp *liveProc, e Event) {
	rt.markProcessExit(lp, LoginInvalid, e.Text)
	h.Report(lp.key, e)
}

func onProcessTurnEnded(h *Host, rt *runtimePart, lp *liveProc, slot aiSlot, e Event) {
	h.Report(lp.key, e)
	if rt.hasExitWhy(lp) {
		return
	}
	why, detail, ok := finishActiveTurn(rt, lp, slot, e.Status != "failed")
	if !ok {
		return
	}
	failedText := cmp.Or(e.Text, lp.eventText)
	lp.resumed = false
	lp.eventText = ""
	idle := readIdleDuration(h)
	h.Sessions.Move(lp.key, Working, Waiting)
	armTurnIdle(h, rt, lp, idle)
	reportTurnResult(h, rt, lp.session, reportedWhy(why), detail, e.Status == "failed", failedText)
	rt.notify()
	rt.startBackground(h, "deliver", func() { Deliver(h, slot.group, slot.member) }, "g", slot.group)
}

func finishActiveTurn(rt *runtimePart, lp *liveProc, slot aiSlot, clearLimit bool) (string, string, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !lp.turnActive {
		return "", "", false
	}
	lp.turnActive = false
	lp.stopInterruptTimer()
	why, detail := lp.interruptWhy, lp.interruptDetail
	lp.interruptWhy, lp.interruptDetail = "", ""
	if clearLimit {
		delete(rt.limited, slot)
	}
	return why, detail, true
}

// reportFinishedTurn sends the member status that ends a turn: with the
// rate_limited event that came during it, or as an interrupt (why: stop or
// timeout, which also fails a task).
func reportFinishedTurn(h *Host, rt *runtimePart, s Session, why, detail string) {
	reportTurnResult(h, rt, s, why, detail, false, "")
}

// reportTurnResult handles member status at turn end, adding a visible
// turn_interrupted notice without closing the CLI or failing the task when a
// CLI stays open after returning status "failed" (without rate_limited).
func reportTurnResult(h *Host, rt *runtimePart, s Session, why, detail string, failed bool, failedText string) {
	ev := takePendingLimitEvent(h, s.SessionKey)
	if why == "" && ev == nil && failed {
		c := pendingCrash{session: s, reason: "crash", detail: eventDetail(failedText), keepTask: true}
		reportOrQueueCrash(context.Background(), h, rt, c)
		return
	}
	if why == "" {
		sendMemberStatus(context.Background(), h, rt, s.Home, s.Group, s.Member, ev)
		return
	}
	c := pendingCrash{session: s, reason: why, detail: detail}
	sendCrashStep(context.Background(), h, rt, &c)
}

// interruptSessionTurn interrupts the turn a session is running, with the
// reason its end is reported with: stop, timeout, or jump, which reports
// nothing. It reports whether the turn is now ending for that reason, and not
// for an earlier one.
func interruptSessionTurn(h *Host, key SessionKey, reason, detail string) bool {
	lp := claimTurnInterrupt(stateOf(h), key, reason, detail)
	if lp == nil {
		return false
	}
	h.diag.Info("turn.interrupt", diagSession(key, "why", reason)...)
	lp.proc.Interrupt()
	return true
}

// reportedWhy is the reason the end of an interrupted turn is reported with.
// A queue jump's interrupt is none (flows/agent-turn.md step 12): a turn it
// ended is a turn that ended, and a CLI killed for not answering it crashed.
func reportedWhy(why string) string {
	if why == "jump" {
		return ""
	}
	return why
}

func claimTurnInterrupt(rt *runtimePart, key SessionKey, reason, detail string) *liveProc {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	lp := rt.procs[key]
	if lp == nil || !lp.turnActive || rt.closing[key] || lp.interruptWhy != "" || lp.exitWhy != "" {
		return nil
	}
	lp.interruptWhy, lp.interruptDetail = reason, detail
	lp.interruptTimer.Store(time.AfterFunc(stopSeconds*time.Second, lp.cancel))
	return lp
}

// interruptTurn interrupts the turn the AI is running in its group, if any,
// and waits for its end (flows/takeover.md step 3). A CLI that has not ended
// it within 10 s is closed (flows/agent-turn.md G).
func interruptTurn(ctx context.Context, h *Host, slot aiSlot) {
	rt := stateOf(h)
	for {
		s, working := findSessionInState(memberSessions(h, slot.group, slot.member), Working)
		if !working {
			return
		}
		rt.mu.Lock()
		lp := rt.procs[s.SessionKey]
		rt.mu.Unlock()
		if lp != nil {
			lp.proc.Interrupt()
		}
		wait, cancel := context.WithTimeout(ctx, stopSeconds*time.Second)
		if waitTurnOver(wait, h, s.SessionKey) != nil {
			Runner.Close(h, s.SessionKey)
		}
		cancel()
	}
}

// waitTurnOver waits until a session is no longer working: its turn has
// ended, or its CLI has exited.
func waitTurnOver(ctx context.Context, h *Host, key SessionKey) error {
	rt := stateOf(h)
	for {
		rt.mu.Lock()
		changed := rt.changed
		rt.mu.Unlock()
		if s, _ := h.Sessions.Get(key); s.State != Working {
			return nil
		}
		if err := waitSignal(ctx, changed); err != nil {
			return err
		}
	}
}

func armTurnIdle(h *Host, rt *runtimePart, lp *liveProc, idle time.Duration) {
	rt.mu.Lock()
	if !lp.turnActive && !rt.closing[lp.key] {
		armIdleLocked(h, rt, lp, idle)
	}
	rt.mu.Unlock()
}

func idleDuration(minutes int) time.Duration {
	if minutes <= 0 {
		minutes = defaultIdleMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func armIdleLocked(h *Host, rt *runtimePart, lp *liveProc, idle time.Duration) {
	lp.idleGen++
	gen := lp.idleGen
	key := lp.key
	fire := func() { onIdleTimeout(h, rt, key, gen) }
	lp.idleTimer = time.AfterFunc(idle, fire)
}

func onIdleTimeout(h *Host, rt *runtimePart, key SessionKey, gen uint64) {
	if lp := claimIdleClose(rt, key, gen); lp != nil {
		finishSessionClose(h, rt, lp, false)
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
	return lp
}

func closeWaitingSession(h *Host, rt *runtimePart, key SessionKey) {
	if lp, _, waitCh := claimSessionClose(rt, key, false); waitCh == nil {
		finishClaimedClose(h, rt, key, lp, false)
	}
}

func finishClaimedClose(h *Host, rt *runtimePart, key SessionKey, lp *liveProc, wasTurn bool) {
	if lp != nil {
		finishSessionClose(h, rt, lp, wasTurn)
	} else if s, ok := h.Sessions.Get(key); ok && h.Sessions.Move(key, Waiting, Dormant) == nil {
		rt.notify()
		sendMemberStatus(context.Background(), h, rt, s.Home, s.Group, s.Member, nil)
	}
}

func claimSessionClose(rt *runtimePart, key SessionKey, force bool) (*liveProc, bool, <-chan struct{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closing[key] {
		return nil, false, rt.changed
	}
	lp := rt.procs[key]
	if lp == nil {
		return nil, false, nil
	}
	if lp.turnActive && !force {
		return nil, false, rt.changed
	}
	wasTurn := lp.turnActive
	lp.turnActive = false
	rt.closing[key] = true
	stopIdleLocked(lp)
	return lp, wasTurn, nil
}

func finishSessionClose(h *Host, rt *runtimePart, lp *liveProc, wasTurn bool) {
	if stopProcess(lp) {
		writeKilledAudit(h, lp.session)
	}
	completeWaitingDormant(h, rt, lp)
	if wasTurn {
		rt.startBackground(h, "deliver", func() { Deliver(h, lp.key.Group, lp.key.Member) }, "g", lp.key.Group)
	}
}

func completeWaitingDormant(h *Host, rt *runtimePart, lp *liveProc) {
	if s, ok := h.Sessions.Get(lp.key); ok {
		h.Sessions.Move(lp.key, s.State, Dormant)
	}
	if lp.exitWhy == LoginInvalid {
		slot := aiSlot{group: lp.key.Group, member: lp.key.Member}
		finishLoginInvalid(h, rt, lp.session, slot, lp.eventText)
	} else {
		sendMemberStatus(context.Background(), h, rt, lp.session.Home, lp.key.Group, lp.key.Member, nil)
	}
	rt.mu.Lock()
	delete(rt.procs, lp.key)
	delete(rt.closing, lp.key)
	rt.notifyLocked()
	rt.mu.Unlock()
}

func writeKilledAudit(h *Host, s Session) {
	detail := strings.TrimSpace(s.Member + " " + s.Task)
	row := AuditRow{Group: s.Group, Agent: s.Agent, What: AuditKilled, Detail: detail}
	write := func(tx *store.Tx) error {
		_, err := h.Audit(tx, row)
		return err
	}
	h.Store.Write(write)
}

func stopProcess(lp *liveProc) bool {
	lp.triggerClose()
	<-lp.loopDone
	lp.stopKillTimer()
	return lp.killed.Load()
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
		completeWaitingDormant(h, rt, lp)
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
	rt.closing[lp.key] = true
	stopIdleLocked(lp)
	return false, true
}

func finishTurnProcessExit(h *Host, rt *runtimePart, lp *liveProc) {
	stopTurnWatch(h, lp.key)
	if lp.exitWhy == NoSession {
		handleNoSession(h, rt, lp)
		return
	}
	slot := aiSlot{group: lp.key.Group, member: lp.key.Member}
	why, intrDetail := beginTurnExitClose(rt, lp)
	h.Sessions.Move(lp.key, Working, Dormant)
	if lp.exitWhy == LoginInvalid {
		finishLoginInvalid(h, rt, lp.session, slot, lp.eventText)
	} else {
		detail := eventDetail(cmp.Or(lp.eventText, intrDetail, lp.turnErrText()))
		c := pendingCrash{session: lp.session, reason: reportedWhy(why), detail: detail}
		sendCrashStep(context.Background(), h, rt, &c)
	}
	rt.startBackground(h, "deliver", func() { Deliver(h, slot.group, slot.member) }, "g", slot.group)
	endTurnExitClose(rt, lp.key)
}

func beginTurnExitClose(rt *runtimePart, lp *liveProc) (string, string) {
	rt.mu.Lock()
	lp.turnActive = false
	lp.stopInterruptTimer()
	why, detail := lp.interruptWhy, lp.interruptDetail
	lp.interruptWhy, lp.interruptDetail = "", ""
	delete(rt.procs, lp.key)
	rt.closing[lp.key] = true
	rt.mu.Unlock()
	return why, detail
}

func endTurnExitClose(rt *runtimePart, key SessionKey) {
	rt.mu.Lock()
	delete(rt.closing, key)
	rt.notifyLocked()
	rt.mu.Unlock()
}

func finishLoginInvalid(h *Host, rt *runtimePart, s Session, slot aiSlot, text string) {
	var ev any
	if rt.enterLimited(slot) {
		ev = proto.LoginInvalidEvent{SystemCode: proto.SystemCode{Code: "login_invalid"}, Detail: eventDetail(text)}
	}
	sendMemberStatus(context.Background(), h, rt, s.Home, s.Group, s.Member, ev)
	notifyLoginChanged(h)
}

// eventDetail redacts secrets and keeps the first non-empty line for brief
// failure notices.
func eventDetail(text string) string {
	for _, line := range strings.Split(RedactSecrets(text), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (rt *runtimePart) enterLimited(slot aiSlot) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.limited[slot] {
		return false
	}
	rt.limited[slot] = true
	return true
}

func (rt *runtimePart) isLimited(slot aiSlot) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.limited[slot]
}

func handleNoSession(h *Host, rt *runtimePart, lp *liveProc) {
	beginTurnExitClose(rt, lp)
	var fresh Session
	clearID := func(s *Session) {
		s.CLISession = ""
		fresh = *s
	}
	if err := h.Sessions.Update(lp.key, clearID); err != nil {
		failReplacement(h, rt, lp, err)
		return
	}
	replText, replTurn, err := composeReplacedTurn(h, fresh, lp.origTurn)
	if err != nil {
		failReplacement(h, rt, lp, err)
		return
	}
	startReplacementProcess(h, rt, lp, fresh, replText, replTurn)
}

func composeReplacedTurn(h *Host, s Session, origTurn Turn) (string, Turn, error) {
	text, turn, err := Compose(h, s, ReplacedTurn, nil)
	if err != nil {
		return "", Turn{}, err
	}
	turn.Kind = ReplacedTurn
	turn.Depth = cmp.Or(turn.Depth, origTurn.Depth)
	turn.Thread = cmp.Or(turn.Thread, origTurn.Thread)
	return text, turn, nil
}

func startReplacementProcess(h *Host, rt *runtimePart, old *liveProc, fresh Session, text string, turn Turn) {
	cfg, err := readLaunchConfig(h, fresh.Agent, fresh.Group)
	if err != nil {
		failReplacement(h, rt, old, err)
		return
	}
	lp2, err := launchSessionProcess(h, rt, fresh, cfg, turn)
	if err != nil {
		failReplacement(h, rt, old, err)
		return
	}
	endTurnExitClose(rt, fresh.SessionKey)
	ev := proto.SessionReplacedEvent{SystemCode: proto.SystemCode{Code: "session_replaced"}, Task: proto.ID(fresh.Task)}
	sendMemberStatus(context.Background(), h, rt, fresh.Home, fresh.Group, fresh.Member, ev)
	if err := deliverInitialTurn(h, lp2, text, turn); err != nil {
		old.replaceErr = err
		return
	}
	old.replacedOK = true
}

func failReplacement(h *Host, rt *runtimePart, lp *liveProc, err error) {
	lp.replaceErr = err
	h.Sessions.Move(lp.key, Working, Dormant)
	sendMemberStatus(context.Background(), h, rt, lp.session.Home, lp.key.Group, lp.key.Member, nil)
	rt.startBackground(h, "deliver", func() { Deliver(h, lp.key.Group, lp.key.Member) }, "g", lp.key.Group)
	endTurnExitClose(rt, lp.key)
}

func acquireOpenSlot(ctx context.Context, h *Host, rt *runtimePart, key SessionKey, maxOpen int) error {
	for {
		victim, waitCh, acquired := tryReserveSlot(h, rt, key, maxOpen)
		if acquired {
			return nil
		}
		if victim != nil {
			closeWaitingSession(h, rt, *victim)
			continue
		}
		if waitCh == nil {
			return errSettling
		}
		if err := waitSignal(ctx, waitCh); err != nil {
			return err
		}
	}
}

func tryReserveSlot(h *Host, rt *runtimePart, key SessionKey, maxOpen int) (*SessionKey, <-chan struct{}, bool) {
	if maxOpen <= 0 {
		maxOpen = defaultMaxOpen
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.settling {
		return nil, nil, false
	}
	sessions := h.Sessions.All()
	if countOpenLocked(rt, sessions) < maxOpen {
		rt.starting[key] = true
		return nil, nil, true
	}
	if victim := longestWaitingLocked(rt, sessions); victim != nil {
		return victim, nil, false
	}
	return nil, rt.changed, false
}

func countOpenLocked(rt *runtimePart, sessions []Session) int {
	open := 0
	for _, s := range sessions {
		if (s.State != Dormant && s.Agent != externalAgent) || rt.starting[s.SessionKey] {
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
	if s.State != Waiting || s.Agent == externalAgent || rt.closing[s.SessionKey] {
		return false
	}
	lp := rt.procs[s.SessionKey]
	return lp == nil || !lp.turnActive
}

func sendCrashStep(ctx context.Context, h *Host, rt *runtimePart, c *pendingCrash) bool {
	s := c.session
	reason := cmp.Or(c.reason, "crash")
	if !c.statusSent {
		ev := proto.TurnInterruptedEvent{SystemCode: proto.SystemCode{Code: "turn_interrupted"}, Reason: reason, Task: proto.ID(s.Task), Detail: c.detail}
		if err := sendMemberStatus(ctx, h, rt, s.Home, s.Group, s.Member, ev); err != nil {
			return !isUnreachable(err)
		}
		c.statusSent = true
	}
	if shouldSkipTaskFail(c, reason) {
		return true
	}
	hdr := homeHeader(h, s.Group, "", "task")
	fail := proto.TaskFail{TaskCommand: proto.TaskCommand{HomeCommand: hdr, Op: "fail", ID: proto.ID(s.Task)}, Reason: reason, Detail: c.detail}
	_, err := h.Command(ctx, s.Home, marshalWire(fail))
	return !isUnreachable(err)
}

func shouldSkipTaskFail(c *pendingCrash, reason string) bool {
	return c.keepTask || c.session.Kind != TaskSession || reason == "stop"
}

func isUnreachable(err error) bool {
	var he *Error
	return errors.As(err, &he) && he.Code == "home_unreachable"
}

func sendMemberStatus(ctx context.Context, h *Host, _ *runtimePart, homeDevice, group, member string, ev any) error {
	home, ok := h.Home(homeDevice)
	if !ok {
		return errHomeUnreachable
	}
	state, task := h.MemberState(group, member)
	var evPtr *any
	if ev != nil {
		evPtr = &ev
	}
	hdr := homeHeader(h, group, "", "member")
	status := proto.MemberStatus[any]{
		MemberCommand: proto.MemberCommand{HomeCommand: hdr, Op: "status", ID: proto.ID(member)},
		State:         state, Task: proto.ID(task), Path: home.Path(),
		Usage: memberUsage(h, group, member), Event: evPtr,
	}
	_, err := h.Command(ctx, homeDevice, marshalWire(status))
	return err
}

// MemberState returns the derived state and active task id of member in group
// (planning/sessions.md section 3).
func (h *Host) MemberState(group, member string) (state, task string) {
	list := memberSessions(h, group, member)
	if s, ok := findSessionInState(list, Takeover); ok {
		return string(Takeover), s.Task
	}
	if s, ok := findSessionInState(list, Working); ok {
		return string(Working), s.Task
	}
	if ov := memberOverlayState(h, group, member); ov != "" {
		return ov, ""
	}
	if s, ok := latestWaitingSession(list); ok {
		return string(Waiting), s.Task
	}
	if memberAgent(h, group, member) == externalAgent {
		return string(Waiting), ""
	}
	return string(Dormant), ""
}

// memberOverlayState maps the holds of the limits unit to a state. A
// takeover's hold is no overlay: the session's own state shows it.
func memberOverlayState(h *Host, group, member string) string {
	switch reason, _ := h.Held(SessionKey{Group: group, Member: member}); reason {
	case "offline":
		return "offline"
	case "rate_limit":
		return "limited"
	}
	if stateOf(h).isLimited(aiSlot{group: group, member: member}) {
		return "limited"
	}
	if extAbsent(h, group, member) {
		return "offline"
	}
	if groupIsPaused(h, group) {
		return "paused"
	}
	return ""
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

func homeHeader(h *Host, group, as, kind string) proto.HomeCommand {
	return proto.HomeCommand{
		Type:      kind,
		Nonce:     proto.NewNonce(),
		Timestamp: h.Now().UnixMilli(),
		Group:     proto.ID(group),
		As:        proto.ID(as),
	}
}

func marshalWire(v any) string {
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(out.String(), "\n")
}
