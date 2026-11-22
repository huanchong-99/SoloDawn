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
	db                 *store.Store
	h                  *host.Host
	hostKey            identity.Key
	homeKey            identity.Key
	home               *hlHome
	drv                *hlDriver
	turnsAtOffline     int
	offlineStoppedErr  error
	turnMinutes        int
	cliInterrupted     bool
	recordedCmds       []map[string]any
	capStatusCmd       map[string]any
	acksBeforeRelease  int
	turnsBeforeRelease int
	acksAfterRelease   int
	turnsAfterRelease  int
}

func hostLimitsSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	prevDrivers := maps.Clone(host.Drivers)
	prevCompose := host.Compose
	prevInstr := host.Instructions
	cleanup(func() {
		host.Drivers = prevDrivers
		host.Compose = prevCompose
		host.Instructions = prevInstr
	})

	s := &hlScenario{}
	registerOfflineScenarioSteps(sc, f.t, cleanup, s)
	registerTimeoutScenarioSteps(sc, f.t, s)
	registerDayCapScenarioSteps(sc, f.t, s)
	registerRateLimitScenarioSteps(sc, f.t, s)
}

func installHLHooks(drv *hlDriver) {
	host.Drivers = map[string]host.Driver{"claude": drv, "codex": drv}
	host.Instructions = func(host.Session) string { return "" }
	host.Compose = func(_ *host.Host, s host.Session, kind host.TurnKind, items []json.RawMessage) (string, host.Turn, error) {
		return fmt.Sprintf("items=%d", len(items)), host.Turn{Kind: kind, Thread: s.Task}, nil
	}
}

func openHLHost(baseDir string, clock func() time.Time, turnMinutes int, caps *proto.Caps) (*store.Store, *host.Host, identity.Key, identity.Key, error) {
	hostKey := makeAccKey(11)
	homeKey := makeAccKey(12)
	db, err := store.Open(filepath.Join(baseDir, "node.db"))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := seedHLSettings(db, homeKey.ID(), caps); err != nil {
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

func seedHLSettings(db *store.Store, homeID string, caps *proto.Caps) error {
	homes := accJSON([]proto.Home{{Group: hlGroup, Home: homeID, Name: "Website"}})
	write := func(tx *store.Tx) error {
		if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('homes', ?)", homes); err != nil {
			return err
		}
		if caps == nil {
			return nil
		}
		_, err := tx.Exec("INSERT INTO agent(agent, cli_version, account, caps) VALUES('claude', '1.0.0', 'personal', ?)", accJSON(caps))
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

func waitHL(desc string, ok func() bool) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", desc)
}

func registerOfflineScenarioSteps(sc *godog.ScenarioContext, suiteT *testing.T, cleanup func(func()), s *hlScenario) {
	sc.Step(`^a host with an active task session and a waiting group session connected to its home$`, func() error {
		return setupOfflineScenario(suiteT, cleanup, s)
	})
	sc.Step(`^the owner sends host\.offline$`, func() error {
		_, err := host.Surface["host.offline"](suiteT.Context(), s.h, nil, "owner", hlCmdPayload(s.h, "host.offline"))
		s.turnsAtOffline = s.drv.turnCount()
		return err
	})
	sc.Step(`^every session on the host is dormant and the home received turn_interrupted "stop" and state "offline"$`, func() error {
		return checkOfflineAfterFirstCall(s)
	})
	sc.Step(`^the home pushes a mention item while the host is offline$`, func() {
		pushHLMention(s.h, s.homeKey.ID(), hlAI2, 1)
	})
	sc.Step(`^the item remains unacknowledged on the home and starts no turn$`, func() error {
		time.Sleep(30 * time.Millisecond)
		if s.home.ackCount(1) != 0 || s.drv.turnCount() != s.turnsAtOffline {
			return fmt.Errorf("held item delivered while offline: acks=%d turns=%d", s.home.ackCount(1), s.drv.turnCount())
		}
		return nil
	})
	sc.Step(`^the home stops, the owner sends host\.online and host\.offline, the home reconnects, and the owner sends host\.online$`, func() error {
		return toggleWhileHomeStoppedAndReconnect(suiteT.Context(), s)
	})
	sc.Step(`^host\.offline succeeded while the home was stopped and host\.online delivered and acknowledged the held item$`, func() error {
		if s.offlineStoppedErr != nil {
			return fmt.Errorf("host.offline with stopped home failed: %w", s.offlineStoppedErr)
		}
		grpKey := host.SessionKey{Group: hlGroup, Member: hlAI2}
		return waitHL("held item ack, turn, and waiting state", func() bool {
			sess, ok := s.h.Sessions.Get(grpKey)
			return [4]any{ok, sess.State, s.home.ackCount(1), s.drv.turnCount()} == [4]any{true, host.Waiting, 1, s.turnsAtOffline + 1}
		})
	})
}

func closeHLSessions(h *host.Host) {
	for _, sess := range h.Sessions.All() {
		host.Runner.Close(h, sess.SessionKey)
	}
}

func setupOfflineScenario(suiteT *testing.T, cleanup func(func()), s *hlScenario) error {
	s.drv = &hlDriver{}
	installHLHooks(s.drv)
	var err error
	s.db, s.h, s.hostKey, s.homeKey, err = openHLHost(suiteT.TempDir(), time.Now, 60, nil)
	if err != nil {
		return err
	}
	cleanup(func() {
		if s.home != nil {
			s.home.stop()
		}
		closeHLSessions(s.h)
		s.h.Settle()
		s.db.Close()
	})
	s.home = newHLHome(3)
	s.h.Attach(s.homeKey.ID(), s.home)

	taskKey := host.SessionKey{Group: hlGroup, Member: hlAI1, Task: hlTask1}
	grpKey := host.SessionKey{Group: hlGroup, Member: hlAI2}
	s.h.Sessions.Create(host.Session{SessionKey: taskKey, Kind: host.TaskSession, Folder: "wt/1", Agent: "claude", Home: s.homeKey.ID(), Name: "Claude Code"})
	s.h.Sessions.Create(host.Session{SessionKey: grpKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: s.homeKey.ID(), Name: "Sec", Secretary: true})

	if err := host.Runner.Turn(suiteT.Context(), s.h, taskKey, "task work", host.Turn{Kind: host.ItemsTurn, Thread: hlTask1}); err != nil {
		return err
	}
	if err := host.Runner.Turn(suiteT.Context(), s.h, grpKey, "group msg", host.Turn{Kind: host.ItemsTurn}); err != nil {
		return err
	}
	s.drv.proc(1).emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
	return waitSessionState(s.h, grpKey, host.Waiting)
}

func checkOfflineAfterFirstCall(s *hlScenario) error {
	for _, sess := range s.h.Sessions.All() {
		if sess.State != host.Dormant {
			return fmt.Errorf("session %v state = %s, want dormant", sess.SessionKey, sess.State)
		}
	}
	return checkOfflineHomeCmds(s.home.snapshotCmds())
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

func toggleWhileHomeStoppedAndReconnect(ctx context.Context, s *hlScenario) error {
	s.home.stop()
	if _, err := host.Surface["host.online"](ctx, s.h, nil, "owner", hlCmdPayload(s.h, "host.online")); err != nil {
		return err
	}
	_, s.offlineStoppedErr = host.Surface["host.offline"](ctx, s.h, nil, "owner", hlCmdPayload(s.h, "host.offline"))
	s.drv.autoEnd = true
	s.home = newHLHome(3)
	s.h.Attach(s.homeKey.ID(), s.home)
	time.Sleep(20 * time.Millisecond)
	_, err := host.Surface["host.online"](ctx, s.h, nil, "owner", hlCmdPayload(s.h, "host.online"))
	return err
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
	baseDir := suiteT.TempDir()
	synctest.Test(suiteT, func(t *testing.T) {
		drv := &hlDriver{endOnInterrupt: true}
		installHLHooks(drv)
		db, h, _, homeKey, err := openHLHost(baseDir, time.Now, s.turnMinutes, nil)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			closeHLSessions(h)
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

func registerDayCapScenarioSteps(sc *godog.ScenarioContext, suiteT *testing.T, s *hlScenario) {
	sc.Step(`^a host AI with a daily cap of 2 turns and a held follow-up mention$`, func() {
		s.capStatusCmd = nil
	})
	sc.Step(`^the AI finishes 2 turns reporting usage$`, func() error {
		return runDayCapScenarioBubble(suiteT, s)
	})
	sc.Step(`^member status reports turns_today 2 and day_cap_pct 100 with cap_reached "day" and state "limited"$`, func() error {
		return checkDayCapStatus(s.capStatusCmd)
	})
	sc.Step(`^the follow-up mention stays held until local midnight and is delivered once midnight passes$`, func() error {
		got := [4]int{s.acksBeforeRelease, s.turnsBeforeRelease, s.acksAfterRelease, s.turnsAfterRelease}
		if got != [4]int{0, 2, 1, 3} {
			return fmt.Errorf("midnight release counts = %v, want [0 2 1 3]", got)
		}
		return nil
	})
}

func runDayCapScenarioBubble(suiteT *testing.T, s *hlScenario) error {
	var stepErr error
	baseDir := suiteT.TempDir()
	synctest.Test(suiteT, func(t *testing.T) {
		drv := &hlDriver{autoEnd: true}
		installHLHooks(drv)
		caps := &proto.Caps{Day: &proto.Cap{Unit: "turns", Max: 2}}
		db, h, _, homeKey, err := openHLHost(baseDir, time.Now, 60, caps)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			closeHLSessions(h)
			h.Settle()
			synctest.Wait()
			db.Close()
		}()
		h.Attach(homeKey.ID(), home)
		synctest.Wait()

		grpKey := host.SessionKey{Group: hlGroup, Member: hlAI2}
		h.Sessions.Create(host.Session{SessionKey: grpKey, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: homeKey.ID(), Name: "Sec", Secretary: true})
		for range 2 {
			if err := host.Runner.Turn(t.Context(), h, grpKey, "turn", host.Turn{Kind: host.ItemsTurn}); err != nil {
				stepErr = err
				return
			}
			synctest.Wait()
		}
		cmds := home.snapshotCmds()
		s.capStatusCmd = cmds[len(cmds)-1]

		pushHLMention(h, homeKey.ID(), hlAI2, 1)
		synctest.Wait()
		s.acksBeforeRelease = home.ackCount(1)
		s.turnsBeforeRelease = drv.turnCount()

		now := time.Now()
		y, m, d := now.Date()
		midnight := time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
		time.Sleep(midnight.Sub(now))
		synctest.Wait()
		s.acksAfterRelease = home.ackCount(1)
		s.turnsAfterRelease = drv.turnCount()
	})
	return stepErr
}

func checkDayCapStatus(cmd map[string]any) error {
	usage, _ := cmd["usage"].(map[string]any)
	ev, _ := cmd["event"].(map[string]any)
	got := [5]any{cmd["state"], ev["code"], ev["cap"], usage["turns_today"], usage["day_cap_pct"]}
	if got != [5]any{"limited", "cap_reached", "day", float64(2), float64(100)} {
		return fmt.Errorf("unexpected cap status: %+v", cmd)
	}
	return nil
}

func registerRateLimitScenarioSteps(sc *godog.ScenarioContext, suiteT *testing.T, s *hlScenario) {
	sc.Step(`^a host AI with a held follow-up mention$`, func() {
		s.capStatusCmd = nil
	})
	sc.Step(`^a turn reports vendor rate_limited until 30 seconds later$`, func() error {
		return runRateLimitScenarioBubble(suiteT, s)
	})
	sc.Step(`^member status reports rate_limited and state "limited" and holds the mention until the reset time passes$`, func() error {
		return checkRateLimitScenarioResult(s)
	})
}

func checkRateLimitScenarioResult(s *hlScenario) error {
	ev, _ := s.capStatusCmd["event"].(map[string]any)
	got := [6]any{s.capStatusCmd["state"], ev["code"], s.acksBeforeRelease, s.turnsBeforeRelease, s.acksAfterRelease, s.turnsAfterRelease}
	if got != [6]any{"limited", "rate_limited", 0, 1, 1, 2} {
		return fmt.Errorf("rate limit scenario mismatch: %v (cmd=%+v)", got, s.capStatusCmd)
	}
	return nil
}

func runRateLimitScenarioBubble(suiteT *testing.T, s *hlScenario) error {
	var stepErr error
	baseDir := suiteT.TempDir()
	synctest.Test(suiteT, func(t *testing.T) {
		drv := &hlDriver{}
		installHLHooks(drv)
		db, h, _, homeKey, err := openHLHost(baseDir, time.Now, 60, nil)
		if err != nil {
			stepErr = err
			return
		}
		home := newHLHome(3)
		defer func() {
			home.stop()
			closeHLSessions(h)
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
		s.capStatusCmd = cmds[len(cmds)-1]

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
