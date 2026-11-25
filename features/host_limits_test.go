//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_limits"] = hostLimitsSteps
}

const (
	hlGroup = "AAAAAAAAAAAAAAAAAAAAAQ"
	hlAI1   = "BBBBBBBBBBBBBBBBBBBBBQ"
	hlAI2   = "CCCCCCCCCCCCCCCCCCCCCQ"
	hlTask1 = "DDDDDDDDDDDDDDDDDDDDDQ"
)

type hlHome struct {
	mu      sync.Mutex
	done    chan struct{}
	acks    []proto.Ack
	cmds    []map[string]any
	syncSeq int64
}

func newHLHome(syncSeq int64) *hlHome {
	return &hlHome{done: make(chan struct{}), syncSeq: syncSeq}
}

func (f *hlHome) Send(frame any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ack, ok := frame.(proto.Ack); ok {
		f.acks = append(f.acks, ack)
	}
	return nil
}

func (f *hlHome) Path() string                 { return "local" }
func (f *hlHome) PathChanged() <-chan struct{} { return nil }
func (f *hlHome) Done() <-chan struct{}        { return f.done }

func (f *hlHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	built := build(1)
	if s, ok := built.(proto.Sync); ok {
		ans := accJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: s.ID, OK: true}, Group: hlGroup, Seq: f.syncSeq})
		return json.RawMessage(ans), nil
	}
	cmd := built.(proto.Cmd)
	var payload map[string]any
	if err := json.Unmarshal([]byte(cmd.Env.Payload), &payload); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.cmds = append(f.cmds, payload)
	id := len(f.cmds)
	f.mu.Unlock()
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (f *hlHome) snapshotCmds() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.cmds)
}

func (f *hlHome) ackCount(item int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, a := range f.acks {
		if a.Item == item {
			n++
		}
	}
	return n
}

func (f *hlHome) stop() {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
}

type hlDriver struct {
	mu             sync.Mutex
	procs          []*hlProc
	turns          int
	autoEnd        bool
	endOnInterrupt bool
}

func (d *hlDriver) Start(ctx context.Context, _ host.Launch) (host.Process, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := &hlProc{
		ctx:            ctx,
		drv:            d,
		events:         make(chan host.Event, 32),
		autoEnd:        d.autoEnd,
		endOnInterrupt: d.endOnInterrupt,
		exited:         make(chan struct{}),
	}
	go p.watchCtx()
	d.procs = append(d.procs, p)
	return p, nil
}

func (d *hlDriver) Interactive(l host.Launch) ([]string, string, error) {
	return []string{"hl", l.Session}, l.Session, nil
}

func (d *hlDriver) turnCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.turns
}

func (d *hlDriver) openProcs() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	var open []int
	for i, p := range d.procs {
		if !p.isClosed() {
			open = append(open, i)
		}
	}
	return open
}

func (d *hlDriver) proc(i int) *hlProc {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.procs[i]
}

type hlProc struct {
	ctx            context.Context
	drv            *hlDriver
	mu             sync.Mutex
	events         chan host.Event
	closed         bool
	autoEnd        bool
	endOnInterrupt bool
	interrupted    atomic.Bool
	exited         chan struct{}
}

func (p *hlProc) watchCtx() {
	select {
	case <-p.ctx.Done():
		p.finish()
	case <-p.exited:
	}
}

func (p *hlProc) Events() <-chan host.Event { return p.events }

func (p *hlProc) Turn(string) error {
	p.drv.mu.Lock()
	p.drv.turns++
	p.drv.mu.Unlock()
	p.emit(host.Event{Kind: host.Ready, Session: "hl-sess"})
	if p.autoEnd {
		p.emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
	}
	return nil
}

func (p *hlProc) Interrupt() error {
	p.interrupted.Store(true)
	if p.endOnInterrupt {
		p.emit(host.Event{Kind: host.TurnEnded, Status: "interrupted"})
	}
	return nil
}

func (p *hlProc) Answer(string, bool) error { return nil }

func (p *hlProc) Close() error {
	p.finish()
	return nil
}

func (p *hlProc) emit(e host.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.events <- e
	}
}

func (p *hlProc) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *hlProc) finish() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.events)
		close(p.exited)
	}
	p.mu.Unlock()
}

type hlScenario struct {
	offlineStates      []host.State
	offlineCmds        []map[string]any
	heldAcks           int
	heldTurns          int
	turnsAtOffline     int
	offlineStoppedErr  error
	cliOpen            []int // the started CLI processes that host.offline left open
	onlineState        host.State
	onlineAcks         int
	onlineTurns        int
	turnMinutes        int
	cliInterrupted     bool
	recordedCmds       []map[string]any
	rateStatusCmd      map[string]any
	acksBeforeRelease  int
	turnsBeforeRelease int
	acksAfterRelease   int
	turnsAfterRelease  int
}

func hostLimitsSteps(sc *godog.ScenarioContext, f fixtures) {
	s := &hlScenario{}
	registerOfflineScenarioSteps(sc, f.t, s)
	registerStoppedHomeSteps(sc, f.t, s)
	registerTimeoutScenarioSteps(sc, f.t, s)
	registerRateLimitScenarioSteps(sc, f.t, s)
}

// saveHLHooks returns what puts back the hooks that installHLHooks replaces.
func saveHLHooks() func() {
	drivers, compose, instr := maps.Clone(host.Drivers), host.Compose, host.Instructions
	return func() { host.Drivers, host.Compose, host.Instructions = drivers, compose, instr }
}

func installHLHooks(drv *hlDriver) {
	host.Drivers = map[string]host.Driver{"claude": drv, "codex": drv}
	host.Instructions = func(host.Session) string { return "" }
	host.Compose = func(_ *host.Host, s host.Session, kind host.TurnKind, items []json.RawMessage) (string, host.Turn, error) {
		return fmt.Sprintf("items=%d", len(items)), host.Turn{Kind: kind, Thread: s.Task}, nil
	}
}

func openHLHost(baseDir string, clock func() time.Time, turnMinutes int) (*store.Store, *host.Host, identity.Key, identity.Key, error) {
	hostKey := makeAccKey(11)
	homeKey := makeAccKey(12)
	db, err := store.Open(filepath.Join(baseDir, "node.db"))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := seedHLSettings(db, homeKey.ID()); err != nil {
		db.Close()
		return nil, nil, nil, nil, err
	}
	h, _, err := host.New(db, hostKey, clock)
	if err != nil {
		db.Close()
		return nil, nil, nil, nil, err
	}
	h.Shim = []string{"solodawn", "mcp"}
	seedHLRows(h, hostKey.ID(), homeKey.ID(), turnMinutes)
	return db, h, hostKey, homeKey, nil
}

func seedHLSettings(db *store.Store, homeID string) error {
	homes := accJSON([]proto.Home{{Group: hlGroup, Home: homeID, Name: "Website"}})
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('homes', ?)", homes)
		return err
	}
	return db.Write(write)
}

func seedHLRows(h *host.Host, hostID, homeID string, turnMinutes int) {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: hlGroup, Seq: 1},
		Name:      "Website", Kind: "project", Base: "main", Forge: "none",
		Home: homeID, Secretary: hlAI2, Checks: []string{},
		Limits: proto.Limits{TurnMinutes: turnMinutes},
	}
	h.Rows.Put(hlGroup, "group", hlGroup, json.RawMessage(accJSON(grp)))
	h.Rows.Put(hlGroup, "member", hlAI1, json.RawMessage(accMemberRowJSON(hlAI1, "Claude Code", hostID, false, 2)))
	h.Rows.Put(hlGroup, "member", hlAI2, json.RawMessage(accMemberRowJSON(hlAI2, "Sec", hostID, true, 3)))
}

func hlCmdPayload(h *host.Host, kind string) json.RawMessage {
	cmd := proto.HostCommand{
		Type:      kind,
		Nonce:     proto.NewNonce(),
		Timestamp: h.Now().UnixMilli(),
		To:        h.Key.ID(),
	}
	return json.RawMessage(accJSON(cmd))
}

func pushHLMention(h *host.Host, homeID, aiID string, itemNum int64) {
	msgID := makeAccID(byte(60 + itemNum))
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(msgID), Seq: 20 + itemNum},
		Pos:       20 + itemNum, Group: hlGroup, Main: true, By: hlAI2, Kind: "text",
		Timestamp: h.Now().UnixMilli(), Text: "ping", Mentions: []string{aiID}, Body: json.RawMessage(`{}`),
	}
	wire := proto.InboxItem{
		Item: proto.Item{Item: itemNum, AI: proto.ID(aiID), Why: "mention", Msg: msgID},
		Msg:  json.RawMessage(accJSON(msg)),
	}
	frame := accJSON(proto.Inbox{Type: "inbox", Group: hlGroup, Items: []proto.InboxItem{wire}})
	h.Push(homeID, json.RawMessage(frame))
}

func registerOfflineScenarioSteps(sc *godog.ScenarioContext, suiteT *testing.T, s *hlScenario) {
	sc.Step(`^a host with an active task session and a waiting group session connected to its home$`, func() {
		*s = hlScenario{}
	})
	sc.Step(`^the owner sends host\.offline$`, func() error {
		return runOfflineScenarioBubble(suiteT, s)
	})
	sc.Step(`^every session on the host is dormant and the home received turn_interrupted "stop" and state "offline"$`, func() error {
		if err := checkDormant(s.offlineStates); err != nil {
			return err
		}
		return checkOfflineHomeCmds(s.offlineCmds)
	})
	sc.Step(`^the home pushes a mention item while the host is offline$`, func() {})
	sc.Step(`^the item remains unacknowledged on the home and starts no turn$`, func() error {
		if s.heldAcks != 0 || s.heldTurns != s.turnsAtOffline {
			return fmt.Errorf("held item delivered while offline: acks=%d turns=%d", s.heldAcks, s.heldTurns)
		}
		return nil
	})
	sc.Step(`^the home stops, the owner sends host\.offline again, the home reconnects, and the owner sends host\.online$`, func() {})
	sc.Step(`^host\.offline succeeded while the home was stopped and host\.online delivered and acknowledged the held item$`, func() error {
		if s.offlineStoppedErr != nil {
			return fmt.Errorf("host.offline with stopped home failed: %w", s.offlineStoppedErr)
		}
		return checkOnline(s)
	})
}

func checkOnline(s *hlScenario) error {
	got := [3]any{s.onlineState, s.onlineAcks, s.onlineTurns}
	if got != [3]any{host.Waiting, 1, s.turnsAtOffline + 1} {
		return fmt.Errorf("after online: state, acks, turns = %v", got)
	}
	return nil
}

func checkDormant(states []host.State) error {
	if !slices.Equal(states, []host.State{host.Dormant, host.Dormant}) {
		return fmt.Errorf("session states after offline: %v", states)
	}
	return nil
}

func registerStoppedHomeSteps(sc *godog.ScenarioContext, suiteT *testing.T, s *hlScenario) {
	sc.Step(`^the owner sends host\.offline while the home is stopped$`, func() error {
		return runOfflineWhileStoppedBubble(suiteT, s)
	})
	sc.Step(`^host\.offline succeeded, every session on the host is dormant and its CLI closed$`, func() error {
		if s.offlineStoppedErr != nil {
			return fmt.Errorf("host.offline with stopped home failed: %w", s.offlineStoppedErr)
		}
		if len(s.cliOpen) != 0 {
			return fmt.Errorf("CLI processes %v still open after host.offline", s.cliOpen)
		}
		return checkDormant(s.offlineStates)
	})
	sc.Step(`^the home starts again and pushes a mention item$`, func() {})
	sc.Step(`^the returning home received state "offline"$`, func() error {
		if !slices.ContainsFunc(s.offlineCmds, func(cmd map[string]any) bool { return cmd["state"] == "offline" }) {
			return fmt.Errorf("no state offline in %+v", s.offlineCmds)
		}
		return nil
	})
	sc.Step(`^the owner sends host\.online to the host whose home returned$`, func() {})
	sc.Step(`^host\.online delivered and acknowledged the held item$`, func() error { return checkOnline(s) })
}

// runOfflineWhileStoppedBubble sends the first host.offline, with a turn
// running, after the home has stopped.
func runOfflineWhileStoppedBubble(suiteT *testing.T, s *hlScenario) error {
	return runHLTurns(suiteT, func(t *testing.T, drv *hlDriver, h *host.Host, homeID string, home *hlHome) error {
		home.stop()
		synctest.Wait()
		s.offlineStoppedErr = hlSurface(t.Context(), h, "host.offline")
		s.turnsAtOffline = drv.turnCount()
		s.cliOpen = drv.openProcs()
		s.offlineStates = hlStates(h)

		home = newHLHome(3)
		defer home.stop()
		h.Attach(homeID, home)
		synctest.Wait()
		s.offlineCmds = home.snapshotCmds()
		pushHLMention(h, homeID, hlAI2, 1)
		synctest.Wait()
		s.heldAcks, s.heldTurns = home.ackCount(1), drv.turnCount()
		return s.recordOnline(t, drv, h, home)
	})
}

func hlSurface(ctx context.Context, h *host.Host, kind string) error {
	_, err := host.Surface[kind](ctx, h, nil, "owner", hlCmdPayload(h, kind))
	return err
}

// runOfflineScenarioBubble runs the whole offline scenario in one bubble, so
// that synctest.Wait orders its steps, and records what the later steps check.
func runOfflineScenarioBubble(suiteT *testing.T, s *hlScenario) error {
	return runHLTurns(suiteT, func(t *testing.T, drv *hlDriver, h *host.Host, homeID string, home *hlHome) error {
		if err := hlSurface(t.Context(), h, "host.offline"); err != nil {
			return err
		}
		s.turnsAtOffline = drv.turnCount()
		s.offlineStates = hlStates(h)
		s.offlineCmds = home.snapshotCmds()

		pushHLMention(h, homeID, hlAI2, 1)
		synctest.Wait()
		s.heldAcks, s.heldTurns = home.ackCount(1), drv.turnCount()

		home.stop()
		s.offlineStoppedErr = hlSurface(t.Context(), h, "host.offline")
		home = newHLHome(3)
		defer home.stop()
		h.Attach(homeID, home)
		synctest.Wait()
		return s.recordOnline(t, drv, h, home)
	})
}

func (s *hlScenario) recordOnline(t *testing.T, drv *hlDriver, h *host.Host, home *hlHome) error {
	drv.autoEnd = true
	if err := hlSurface(t.Context(), h, "host.online"); err != nil {
		return err
	}
	synctest.Wait()
	sess, _ := h.Sessions.Get(host.SessionKey{Group: hlGroup, Member: hlAI2})
	s.onlineState, s.onlineAcks, s.onlineTurns = sess.State, home.ackCount(1), drv.turnCount()
	return nil
}

func hlStates(h *host.Host) []host.State {
	var states []host.State
	for _, sess := range h.Sessions.All() {
		states = append(states, sess.State)
	}
	return states
}

// runHLTurns runs body, in one bubble, on a host that is attached to its home
// and has a working task session and a waiting group session.
func runHLTurns(suiteT *testing.T, body func(t *testing.T, drv *hlDriver, h *host.Host, homeID string, home *hlHome) error) error {
	var stepErr error
	defer saveHLHooks()()
	baseDir := suiteT.TempDir()
	synctest.Test(suiteT, func(t *testing.T) {
		drv := &hlDriver{}
		installHLHooks(drv)
		db, h, _, homeKey, err := openHLHost(baseDir, time.Now, 60)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			h.Settle()
			synctest.Wait()
			db.Close()
		}()
		h.Attach(homeKey.ID(), home)
		if stepErr = startHLTurns(t.Context(), h, homeKey.ID()); stepErr != nil {
			return
		}
		drv.proc(1).emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
		synctest.Wait()
		stepErr = body(t, drv, h, homeKey.ID(), home)
	})
	return stepErr
}

func startHLTurns(ctx context.Context, h *host.Host, homeID string) error {
	taskKey := host.SessionKey{Group: hlGroup, Member: hlAI1, Task: hlTask1}
	grpKey := host.SessionKey{Group: hlGroup, Member: hlAI2}
	h.Sessions.Create(host.Session{SessionKey: taskKey, Kind: host.TaskSession, Folder: "wt/1", Agent: "claude", Home: homeID, Name: "Claude Code"})
	h.Sessions.Create(host.Session{SessionKey: grpKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: homeID, Name: "Sec", Secretary: true})
	if err := host.Runner.Turn(ctx, h, taskKey, "task work", host.Turn{Kind: host.ItemsTurn, Thread: hlTask1}); err != nil {
		return err
	}
	return host.Runner.Turn(ctx, h, grpKey, "group msg", host.Turn{Kind: host.ItemsTurn})
}

func checkOfflineHomeCmds(cmds []map[string]any) error {
	var sawStop, sawOffline bool
	for _, cmd := range cmds {
		if cmd["state"] == "offline" {
			sawOffline = true
		}
		ev, _ := cmd["event"].(map[string]any)
		if [2]any{ev["code"], ev["reason"]} == [2]any{"turn_interrupted", "stop"} {
			sawStop = true
		}
	}
	if [2]bool{sawStop, sawOffline} != [2]bool{true, true} {
		return fmt.Errorf("missing stop=%v or offline=%v in %+v", sawStop, sawOffline, cmds)
	}
	return nil
}

func registerTimeoutScenarioSteps(sc *godog.ScenarioContext, suiteT *testing.T, s *hlScenario) {
	sc.Step(`^a host task session in a group whose turn_minutes is (\d+)$`, func(mins int) {
		s.turnMinutes = mins
	})
	sc.Step(`^the task session starts a turn that does not finish within 1 minute$`, func() error {
		return runTimeoutScenarioBubble(suiteT, s)
	})
	sc.Step(`^the CLI receives an interrupt and the home receives turn_interrupted "timeout" and task fail "timeout"$`, func() error {
		return checkTimeoutScenarioResult(s)
	})
}

func runTimeoutScenarioBubble(suiteT *testing.T, s *hlScenario) error {
	var stepErr error
	defer saveHLHooks()()
	baseDir := suiteT.TempDir()
	synctest.Test(suiteT, func(t *testing.T) {
		drv := &hlDriver{endOnInterrupt: true}
		installHLHooks(drv)
		db, h, _, homeKey, err := openHLHost(baseDir, time.Now, s.turnMinutes)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			h.Settle()
			synctest.Wait()
			db.Close()
		}()
		h.Attach(homeKey.ID(), home)
		synctest.Wait()

		tKey := host.SessionKey{Group: hlGroup, Member: hlAI1, Task: hlTask1}
		h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: "wt/1", Agent: "claude", Home: homeKey.ID(), Name: "Claude Code"})
		if err := host.Runner.Turn(t.Context(), h, tKey, "long turn", host.Turn{Kind: host.ItemsTurn, Thread: hlTask1}); err != nil {
			stepErr = err
			return
		}
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()

		s.cliInterrupted = drv.proc(0).interrupted.Load()
		s.recordedCmds = home.snapshotCmds()
	})
	return stepErr
}

func checkTimeoutScenarioResult(s *hlScenario) error {
	var sawIntr, sawFail bool
	for _, cmd := range s.recordedCmds {
		ev, _ := cmd["event"].(map[string]any)
		if [2]any{ev["code"], ev["reason"]} == [2]any{"turn_interrupted", "timeout"} {
			sawIntr = true
		}
		if [3]any{cmd["t"], cmd["op"], cmd["reason"]} == [3]any{"task", "fail", "timeout"} {
			sawFail = true
		}
	}
	if [3]bool{s.cliInterrupted, sawIntr, sawFail} != [3]bool{true, true, true} {
		return fmt.Errorf("timeout mismatch: cli=%v intr=%v fail=%v cmds=%+v", s.cliInterrupted, sawIntr, sawFail, s.recordedCmds)
	}
	return nil
}

func registerRateLimitScenarioSteps(sc *godog.ScenarioContext, suiteT *testing.T, s *hlScenario) {
	sc.Step(`^a host AI with a held follow-up mention$`, func() {
		s.rateStatusCmd = nil
	})
	sc.Step(`^a turn reports vendor rate_limited until 30 seconds later$`, func() error {
		return runRateLimitScenarioBubble(suiteT, s)
	})
	sc.Step(`^member status reports rate_limited and state "limited" and holds the mention until the reset time passes$`, func() error {
		return checkRateLimitScenarioResult(s)
	})
}

func checkRateLimitScenarioResult(s *hlScenario) error {
	ev, _ := s.rateStatusCmd["event"].(map[string]any)
	got := [6]any{s.rateStatusCmd["state"], ev["code"], s.acksBeforeRelease, s.turnsBeforeRelease, s.acksAfterRelease, s.turnsAfterRelease}
	if got != [6]any{"limited", "rate_limited", 0, 1, 1, 2} {
		return fmt.Errorf("rate limit scenario mismatch: %v (cmd=%+v)", got, s.rateStatusCmd)
	}
	return nil
}

func runRateLimitScenarioBubble(suiteT *testing.T, s *hlScenario) error {
	var stepErr error
	defer saveHLHooks()()
	baseDir := suiteT.TempDir()
	synctest.Test(suiteT, func(t *testing.T) {
		drv := &hlDriver{}
		installHLHooks(drv)
		db, h, _, homeKey, err := openHLHost(baseDir, time.Now, 60)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			h.Settle()
			synctest.Wait()
			db.Close()
		}()
		h.Attach(homeKey.ID(), home)
		synctest.Wait()

		grpKey := host.SessionKey{Group: hlGroup, Member: hlAI2}
		h.Sessions.Create(host.Session{SessionKey: grpKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: homeKey.ID(), Name: "Sec", Secretary: true})
		if err := host.Runner.Turn(t.Context(), h, grpKey, "turn", host.Turn{Kind: host.ItemsTurn}); err != nil {
			stepErr = err
			return
		}
		synctest.Wait()

		resets := time.Now().Add(30 * time.Second).UnixMilli()
		drv.proc(0).emit(host.Event{Kind: host.RateLimited, Resets: resets})
		drv.proc(0).emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
		synctest.Wait()

		cmds := home.snapshotCmds()
		s.rateStatusCmd = cmds[len(cmds)-1]

		drv.mu.Lock()
		drv.autoEnd = true
		drv.mu.Unlock()
		pushHLMention(h, homeKey.ID(), hlAI2, 1)
		time.Sleep(29 * time.Second)
		synctest.Wait()
		s.acksBeforeRelease = home.ackCount(1)
		s.turnsBeforeRelease = drv.turnCount()

		time.Sleep(time.Second)
		synctest.Wait()
		s.acksAfterRelease = home.ackCount(1)
		s.turnsAfterRelease = drv.turnCount()
	})
	return stepErr
}
