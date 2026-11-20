//go:build acceptance

package features

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	stepSets["host_run"] = func(sc *godog.ScenarioContext, f fixtures) {
		hostRunSteps(sc, f.t)
	}
}

const (
	hrGroup = "rZNvy-1jH6Z-BcPqA5U5BQ"
	hrAI1   = "1PC8WineBrUQ-apCjx7tug"
	hrAI2   = "9mL3vB7xQ2kP5wZ8nH4jCA"
	hrHuman = "3xK8pM2vN7qR4wY9bL5tGA"
	hrTask1 = "DrtCn6htSBwmMPrFPbHJHA"
	hrTask2 = "6hJ9kL4mP8rT2vX5zB7nQA"
	hrMsg1  = "mC2ePrmW9VnmM_TRlN7zdg"
)

type hrHome struct {
	mu      sync.Mutex
	done    chan struct{}
	cmds    []map[string]any
	rawCmds []string
	notify  chan map[string]any
}

func newHRHome() *hrHome {
	return &hrHome{done: make(chan struct{}), notify: make(chan map[string]any, 64)}
}

func (h *hrHome) Send(any) error               { return nil }
func (h *hrHome) Path() string                 { return "local" }
func (h *hrHome) PathChanged() <-chan struct{} { return nil }
func (h *hrHome) Done() <-chan struct{}        { return h.done }

func (h *hrHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	h.mu.Lock()
	id := int64(len(h.cmds) + 1)
	h.mu.Unlock()
	frame := build(id)
	cmd, ok := frame.(proto.Cmd)
	if !ok {
		msgRow, _ := json.Marshal(proto.MessageRow[proto.EmptyBody, proto.NoCard]{
			RowHeader: proto.RowHeader{Type: "msg", ID: hrMsg1, Seq: 1},
			Pos:       1, Group: hrGroup, Main: true, By: hrHuman, Kind: "text",
			Timestamp: 1763600000000, Text: "jump now", Mentions: []string{},
		})
		fetched, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: id, OK: true}, Rows: []json.RawMessage{msgRow}})
		return fetched, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(cmd.Env.Payload), &payload); err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.cmds = append(h.cmds, payload)
	h.rawCmds = append(h.rawCmds, cmd.Env.Payload)
	h.mu.Unlock()
	h.notify <- payload
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (h *hrHome) snapshot() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.cmds)
}

type hrMemDriver struct {
	parentCtx context.Context
	mu        sync.Mutex
	launches  []host.Launch
	procs     []*hrMemProc
	noSess    map[int]bool
	autoEnd   bool
	readyID   string
}

func (d *hrMemDriver) Start(ctx context.Context, l host.Launch) (host.Process, error) {
	d.mu.Lock()
	idx := len(d.launches)
	d.launches = append(d.launches, l)
	failResume := d.noSess[idx]
	autoEnd := d.autoEnd
	readyID := d.readyID
	parentCtx := d.parentCtx
	p := &hrMemProc{ctx: ctx, events: make(chan host.Event, 16), turns: make(chan struct {
		text string
		kind host.TurnKind
	}, 8)}
	d.procs = append(d.procs, p)
	d.mu.Unlock()
	go func() {
		if parentCtx != nil {
			select {
			case <-ctx.Done():
			case <-parentCtx.Done():
			}
		} else {
			<-ctx.Done()
		}
		p.closeEvents()
	}()
	p.onTurn = func(text string) error {
		if failResume {
			p.emit(host.Event{Kind: host.NoSession, Text: "session missing"})
			return errors.New("session missing")
		}
		id := l.Session
		if id == "" {
			id = readyID
			if id == "" {
				id = fmt.Sprintf("fresh-%d", idx)
			}
		}
		p.emit(host.Event{Kind: host.Ready, Session: id})
		if autoEnd {
			p.emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
		}
		return nil
	}
	return p, nil
}

func (d *hrMemDriver) Interactive(l host.Launch) ([]string, string, error) {
	return []string{"mem", l.Session}, l.Session, nil
}

type hrMemProc struct {
	ctx    context.Context
	mu     sync.Mutex
	closed bool
	events chan host.Event
	onTurn func(string) error
	turns  chan struct {
		text string
		kind host.TurnKind
	}
	interrupted int
}

func (p *hrMemProc) Events() <-chan host.Event { return p.events }

func (p *hrMemProc) Turn(text string) error {
	p.turns <- struct {
		text string
		kind host.TurnKind
	}{text: text}
	return p.onTurn(text)
}

func (p *hrMemProc) Interrupt() error {
	p.mu.Lock()
	p.interrupted++
	p.mu.Unlock()
	p.emit(host.Event{Kind: host.TurnEnded, Status: "interrupted"})
	return nil
}

func (p *hrMemProc) Answer(string, bool) error { return nil }

func (p *hrMemProc) Close() error {
	p.closeEvents()
	return nil
}

func (p *hrMemProc) emit(e host.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.events <- e
	}
}

func (p *hrMemProc) closeEvents() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.events)
	}
	p.mu.Unlock()
}

// hrCLIDriver drives the real built fakecli/claude binary on PATH so scenarios
// exercise actual OS subprocess start, stdin/stdout stream-json, mid-turn exit
// and SOLODAWN_FAKECLI_LOG ordering.
type hrCLIDriver struct {
	parentCtx context.Context
	mu        sync.Mutex
	wg        sync.WaitGroup
	scripts   []string
	logFile   string
}

func newHRCLIDriver(cleanup func(func()), logFile string, scripts ...string) *hrCLIDriver {
	ctx, cancel := context.WithCancel(context.Background())
	d := &hrCLIDriver{parentCtx: ctx, scripts: scripts, logFile: logFile}
	cleanup(func() {
		cancel()
		d.wg.Wait()
		time.Sleep(15 * time.Millisecond)
	})
	return d
}

func (d *hrCLIDriver) Start(ctx context.Context, l host.Launch) (host.Process, error) {
	d.mu.Lock()
	script := d.scripts[0]
	if len(d.scripts) > 1 {
		d.scripts = d.scripts[1:]
	}
	logFile := d.logFile
	parentCtx := d.parentCtx
	d.mu.Unlock()

	sessionID := l.Session
	flag := "--resume"
	if sessionID == "" {
		sessionID = "generated-id"
		flag = "--session-id"
	}
	runCtx, cancel := context.WithCancel(ctx)
	if parentCtx != nil {
		go func() {
			select {
			case <-runCtx.Done():
			case <-parentCtx.Done():
				cancel()
			}
		}()
	}
	cmd := exec.CommandContext(runCtx, "claude", "-p", flag, sessionID)
	cmd.Dir = l.Folder
	cmd.Env = append(os.Environ(), "SOLODAWN_FAKE_CLAUDE_SCRIPT="+script, "SOLODAWN_FAKECLI_LOG="+logFile)
	inR, inW, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		inR.Close()
		inW.Close()
		return nil, err
	}
	cmd.Stdin = inR
	if err := cmd.Start(); err != nil {
		cancel()
		inR.Close()
		inW.Close()
		return nil, err
	}
	inR.Close()
	events := make(chan host.Event, 16)
	p := &hrCLIProc{in: inW, events: events}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var line struct {
				Type      string `json:"type"`
				Subtype   string `json:"subtype"`
				SessionID string `json:"session_id"`
			}
			if json.Unmarshal(scanner.Bytes(), &line) != nil {
				continue
			}
			if line.Type == "system" && line.Subtype == "init" {
				events <- host.Event{Kind: host.Ready, Session: line.SessionID}
			} else if line.Type == "result" {
				events <- host.Event{Kind: host.TurnEnded, Status: "completed"}
			}
		}
		cmd.Wait()
		cancel()
		p.Close()
		close(events)
	}()
	return p, nil
}

func (d *hrCLIDriver) Interactive(l host.Launch) ([]string, string, error) {
	return []string{"claude", "--resume", l.Session}, l.Session, nil
}

type hrCLIProc struct {
	once   sync.Once
	in     *os.File
	events <-chan host.Event
}

func (p *hrCLIProc) Events() <-chan host.Event { return p.events }
func (p *hrCLIProc) Interrupt() error          { return nil }
func (p *hrCLIProc) Answer(string, bool) error { return nil }
func (p *hrCLIProc) Close() error {
	var err error
	p.once.Do(func() { err = p.in.Close() })
	return err
}
func (p *hrCLIProc) Turn(text string) error {
	msg, _ := json.Marshal(map[string]any{"type": "user", "text": text})
	_, err := p.in.Write(append(msg, '\n'))
	return err
}

func hostRunSteps(sc *godog.ScenarioContext, suiteT *testing.T) {
	cleanup := afterScenario(sc)
	origDrivers := maps.Clone(host.Drivers)
	origCompose := host.Compose
	cleanup(func() {
		host.Drivers = origDrivers
		host.Compose = origCompose
	})

	registerIdleAndMaxOpenSteps(sc, suiteT, cleanup)
	registerCLICrashAndSwitchSteps(sc, suiteT, cleanup)
	registerReplaceAndJumpSteps(sc, suiteT, cleanup)
}

func newTestHost(cleanup func(func()), maxOpen, idleMin int) (*host.Host, *hrHome, context.Context, error) {
	procCtx, cancelProcs := context.WithCancel(context.Background())
	db, err := store.Open(":memory:")
	if err != nil {
		cancelProcs()
		return nil, nil, nil, err
	}
	cleanup(func() { db.Close() })
	cleanup(func() {
		cancelProcs()
		time.Sleep(10 * time.Millisecond)
	})
	err = db.Write(func(tx *store.Tx) error {
		if maxOpen > 0 {
			if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('max_open', ?)", fmt.Sprint(maxOpen)); err != nil {
				return err
			}
		}
		if idleMin > 0 {
			if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('idle_minutes', ?)", fmt.Sprint(idleMin)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, time.Now)
	if err != nil {
		return nil, nil, nil, err
	}
	rh := newHRHome()
	h.Attach("home-dev", rh)
	return h, rh, procCtx, nil
}

func registerIdleAndMaxOpenSteps(sc *godog.ScenarioContext, suiteT *testing.T, cleanup func(func())) {
	// Scenario 1 (A3.4): run inside synctest.Test so 1 minute of virtual idle time runs deterministically in <5ms.
	var idleResumedSession string
	var idleStates []string
	var idleStatusReported bool

	sc.Step(`^a host session runtime with idle_minutes set to 1$`, func() {
		idleResumedSession = ""
		idleStates = nil
		idleStatusReported = false
	})
	sc.Step(`^a task session completes a turn with CLI session "([^"]+)"$`, func(cliID string) error {
		var stepErr error
		wtDir := suiteT.TempDir()
		synctest.Test(suiteT, func(t *testing.T) {
			db, err := store.Open(":memory:")
			if err != nil {
				stepErr = err
				return
			}
			defer db.Close()
			if err := db.Write(func(tx *store.Tx) error {
				_, e := tx.Exec("INSERT INTO setting(key, value) VALUES('idle_minutes', '1')")
				return e
			}); err != nil {
				stepErr = err
				return
			}
			drv := &hrMemDriver{autoEnd: true, readyID: cliID}
			host.Drivers = map[string]host.Driver{"claude": drv}
			h, _, err := host.New(db, identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))), time.Now)
			if err != nil {
				stepErr = err
				return
			}
			rh := newHRHome()
			h.Attach("home-dev", rh)
			tKey := host.SessionKey{Group: hrGroup, Member: hrAI1, Task: hrTask1}
			if err := h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: wtDir, Agent: "claude", Home: "home-dev", Name: "Claude"}); err != nil {
				stepErr = err
				return
			}
			if err := host.Runner.Turn(t.Context(), h, tKey, "first", host.Turn{Kind: host.ItemsTurn}); err != nil {
				stepErr = err
				return
			}
			synctest.Wait()
			time.Sleep(time.Minute)
			synctest.Wait()
			s, _ := h.Sessions.Get(tKey)
			idleStates = append(idleStates, string(s.State))
			for _, cmd := range rh.snapshot() {
				if cmd["t"] == "member" && cmd["state"] == "dormant" {
					idleStatusReported = true
				}
			}
			if err := host.Runner.Turn(t.Context(), h, tKey, "second", host.Turn{Kind: host.ItemsTurn}); err != nil {
				stepErr = err
				return
			}
			synctest.Wait()
			idleResumedSession = drv.launches[1].Session
			time.Sleep(time.Minute)
			synctest.Wait()
		})
		return stepErr
	})
	sc.Step(`^1 minute passes with no turn$`, func() {})
	sc.Step(`^the task session is "dormant" and member status reported "dormant"$`, func() error {
		if !slices.Equal(idleStates, []string{"dormant"}) || !idleStatusReported {
			return fmt.Errorf("states=%v dormantReported=%v", idleStates, idleStatusReported)
		}
		return nil
	})
	sc.Step(`^the task session receives another turn$`, func() {})
	sc.Step(`^the driver resumes CLI session "([^"]+)"$`, func(want string) error {
		if idleResumedSession != want {
			return fmt.Errorf("resumed %q, want %q", idleResumedSession, want)
		}
		return nil
	})

	// Scenario 2 (A3.5): max_open = 1
	var h2 *host.Host
	var drv2 *hrMemDriver
	k1 := host.SessionKey{Group: hrGroup, Member: hrAI1}
	k2 := host.SessionKey{Group: hrGroup, Member: hrAI2}
	var firstWasDormantBeforeSecond bool
	var waitDone chan error

	sc.Step(`^a host session runtime with max_open set to 1 and two AIs$`, func() (err error) {
		var procCtx context.Context
		h2, _, procCtx, err = newTestHost(cleanup, 1, 15)
		if err != nil {
			return err
		}
		drv2 = &hrMemDriver{parentCtx: procCtx}
		host.Drivers = map[string]host.Driver{"claude": drv2}
		if err := h2.Sessions.Create(host.Session{SessionKey: k1, Kind: host.GroupSession, Folder: suiteT.TempDir(), Agent: "claude", Home: "home-dev", Name: "AI1"}); err != nil {
			return err
		}
		return h2.Sessions.Create(host.Session{SessionKey: k2, Kind: host.GroupSession, Folder: suiteT.TempDir(), Agent: "claude", Home: "home-dev", Name: "AI2"})
	})
	sc.Step(`^the first AI finishes a turn and is "waiting"$`, func() error {
		if err := host.Runner.Turn(suiteT.Context(), h2, k1, "hello", host.Turn{Kind: host.ItemsTurn}); err != nil {
			return err
		}
		drv2.procs[0].emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
		return waitSessionState(h2, k1, host.Waiting)
	})
	sc.Step(`^the second AI receives a turn$`, func() error {
		if err := host.Runner.Turn(suiteT.Context(), h2, k2, "world", host.Turn{Kind: host.ItemsTurn}); err != nil {
			return err
		}
		s1, _ := h2.Sessions.Get(k1)
		firstWasDormantBeforeSecond = s1.State == host.Dormant
		return nil
	})
	sc.Step(`^the first AI becomes "dormant" before the second AI starts$`, func() error {
		if !firstWasDormantBeforeSecond {
			return errors.New("first AI was not dormant when second started")
		}
		return nil
	})
	sc.Step(`^the second AI is still "working" and the first AI receives a turn$`, func() {
		waitDone = make(chan error, 1)
		go func() {
			waitDone <- host.Runner.Turn(suiteT.Context(), h2, k1, "again", host.Turn{Kind: host.ItemsTurn})
		}()
	})
	sc.Step(`^the first AI waits until the second AI finishes its turn and then starts$`, func() error {
		time.Sleep(20 * time.Millisecond)
		if s1, _ := h2.Sessions.Get(k1); s1.State != host.Dormant {
			return fmt.Errorf("first AI started early: %s", s1.State)
		}
		drv2.procs[1].emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
		select {
		case err := <-waitDone:
			if err != nil {
				return err
			}
		case <-time.After(5 * time.Second):
			return errors.New("first AI did not start after second finished")
		}
		return waitSessionState(h2, k1, host.Working)
	})
}

func registerCLICrashAndSwitchSteps(sc *godog.ScenarioContext, suiteT *testing.T, cleanup func(func())) {
	var h *host.Host
	var rh *hrHome
	var cliDrv *hrCLIDriver
	var dir, logFile string
	tKey := host.SessionKey{Group: hrGroup, Member: hrAI1, Task: hrTask1}

	sc.Step(`^a task session backed by the scripted Claude CLI$`, func() (err error) {
		dir = suiteT.TempDir()
		logFile = filepath.Join(dir, "lifecycle.log")
		h, rh, _, err = newTestHost(cleanup, 4, 15)
		if err != nil {
			return err
		}
		cliDrv = newHRCLIDriver(cleanup, logFile)
		host.Drivers = map[string]host.Driver{"claude": cliDrv}
		return h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: dir, Agent: "claude", Home: "home-dev", Name: "Claude"})
	})
	sc.Step(`^the scripted Claude CLI exits mid-turn after reporting session "([^"]+)"$`, func(id string) error {
		crashScript := filepath.Join(dir, "crash.json")
		resumeScript := filepath.Join(dir, "resume.json")
		s1 := fmt.Sprintf(`{"session":%q,"steps":[{"dir":"stdin","msg":{"type":"user"}},{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}}],"exit":1}`, id)
		s2 := fmt.Sprintf(`{"session":%q,"steps":[{"dir":"stdin","msg":{"type":"user"}},{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},{"dir":"stdout","msg":{"type":"result","subtype":"success"}},{"dir":"eof"}]}`, id)
		if err := os.WriteFile(crashScript, []byte(s1), 0600); err != nil {
			return err
		}
		if err := os.WriteFile(resumeScript, []byte(s2), 0600); err != nil {
			return err
		}
		cliDrv.scripts = []string{crashScript, resumeScript}
		if err := h.Sessions.Update(tKey, func(row *host.Session) { row.CLISession = id }); err != nil {
			return err
		}
		if err := host.Runner.Turn(suiteT.Context(), h, tKey, "crash turn", host.Turn{Kind: host.ItemsTurn, Thread: hrTask1}); err != nil {
			return err
		}
		return waitSessionState(h, tKey, host.Dormant)
	})
	sc.Step(`^the host reports turn_interrupted "crash" and task fail "crash"$`, func() error {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var sawInt, sawFail bool
			for _, cmd := range rh.snapshot() {
				if ev, ok := cmd["event"].(map[string]any); ok && ev["code"] == "turn_interrupted" && ev["reason"] == "crash" {
					sawInt = true
				}
				if cmd["t"] == "task" && cmd["op"] == "fail" && cmd["reason"] == "crash" {
					sawFail = true
				}
			}
			if sawInt && sawFail {
				return nil
			}
			time.Sleep(5 * time.Millisecond)
		}
		return fmt.Errorf("missing crash commands in %+v", rh.snapshot())
	})
	sc.Step(`^the next turn on the task session resumes "([^"]+)" and completes$`, func(id string) error {
		if err := host.Runner.Turn(suiteT.Context(), h, tKey, "resume turn", host.Turn{Kind: host.ItemsTurn, Thread: hrTask1}); err != nil {
			return err
		}
		return waitSessionState(h, tKey, host.Waiting)
	})

	// Scenario 4 (A7.4 ordering): 10 switches under max_open = 1 with fake CLI
	kA := host.SessionKey{Group: hrGroup, Member: hrAI1}
	kB := host.SessionKey{Group: hrGroup, Member: hrAI2}
	sc.Step(`^two scripted Claude sessions under max_open 1 sharing a lifecycle log$`, func() (err error) {
		dir = suiteT.TempDir()
		logFile = filepath.Join(dir, "switch.log")
		script := filepath.Join(dir, "turn-eof.json")
		body := `{"steps":[{"dir":"stdin","msg":{"type":"user"}},{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},{"dir":"stdout","msg":{"type":"result","subtype":"success"}},{"dir":"eof"}]}`
		if err := os.WriteFile(script, []byte(body), 0600); err != nil {
			return err
		}
		h, rh, _, err = newTestHost(cleanup, 1, 15)
		if err != nil {
			return err
		}
		cliDrv = newHRCLIDriver(cleanup, logFile, script)
		host.Drivers = map[string]host.Driver{"claude": cliDrv}
		if err := h.Sessions.Create(host.Session{SessionKey: kA, Kind: host.GroupSession, Folder: dir, Agent: "claude", Home: "home-dev", Name: "A", CLISession: "sess-a"}); err != nil {
			return err
		}
		return h.Sessions.Create(host.Session{SessionKey: kB, Kind: host.GroupSession, Folder: dir, Agent: "claude", Home: "home-dev", Name: "B", CLISession: "sess-b"})
	})
	sc.Step(`^turns alternate between the two sessions (\d+) times$`, func(switches int) error {
		pairs := []host.SessionKey{kA, kB}
		for i := 0; i <= switches; i++ {
			target := pairs[i%2]
			if err := host.Runner.Turn(suiteT.Context(), h, target, "ping", host.Turn{Kind: host.ItemsTurn}); err != nil {
				return err
			}
			if err := waitSessionState(h, target, host.Waiting); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the lifecycle log records (\d+) non-overlapping start-exit pairs$`, func(total int) error {
		// Trigger one switch to a third dormant session so the 11th process exits cleanly before inspecting the log.
		kC := host.SessionKey{Group: hrGroup, Member: "end-slot"}
		if err := h.Sessions.Create(host.Session{SessionKey: kC, Kind: host.GroupSession, Folder: dir, Agent: "claude", Home: "home-dev", Name: "C", CLISession: "sess-c"}); err != nil {
			return err
		}
		if err := host.Runner.Turn(suiteT.Context(), h, kC, "done", host.Turn{Kind: host.ItemsTurn}); err != nil {
			return err
		}
		if err := waitSessionState(h, kC, host.Waiting); err != nil {
			return err
		}
		raw, err := os.ReadFile(logFile)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		// First 2*total lines strictly alternate start / exit with matching session ids.
		if len(lines) < 2*total {
			return fmt.Errorf("got %d lines (%q), want at least %d", len(lines), raw, 2*total)
		}
		for i := 0; i < total; i++ {
			startParts := strings.Fields(lines[2*i])
			exitParts := strings.Fields(lines[2*i+1])
			if len(startParts) != 3 || len(exitParts) != 3 || startParts[0] != "start" || exitParts[0] != "exit" || startParts[2] != exitParts[2] {
				return fmt.Errorf("overlapping or mismatched pair %d: %q then %q", i, lines[2*i], lines[2*i+1])
			}
		}
		return nil
	})
}

func registerReplaceAndJumpSteps(sc *godog.ScenarioContext, suiteT *testing.T, cleanup func(func())) {
	var h *host.Host
	var rh *hrHome
	var drv *hrMemDriver
	tKey := host.SessionKey{Group: hrGroup, Member: hrAI1, Task: hrTask1}
	gKey := host.SessionKey{Group: hrGroup, Member: hrAI1}

	sc.Step(`^a task session whose stored CLI session id is no longer resumable$`, func() (err error) {
		var procCtx context.Context
		h, rh, procCtx, err = newTestHost(cleanup, 4, 15)
		if err != nil {
			return err
		}
		drv = &hrMemDriver{parentCtx: procCtx, noSess: map[int]bool{0: true}, autoEnd: true}
		host.Drivers = map[string]host.Driver{"claude": drv}
		host.Compose = func(_ *host.Host, _ host.Session, kind host.TurnKind, _ []json.RawMessage) (string, host.Turn, error) {
			return "composed:" + string(kind), host.Turn{Kind: kind, Thread: hrTask1}, nil
		}
		return h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: suiteT.TempDir(), Agent: "claude", Home: "home-dev", Name: "Claude", CLISession: "gone-id"})
	})
	sc.Step(`^the task session receives a turn$`, func() error {
		return host.Runner.Turn(suiteT.Context(), h, tKey, "work", host.Turn{Kind: host.ItemsTurn, Thread: hrTask1})
	})
	sc.Step(`^the host reports session_replaced and delivers a "([^"]+)" turn on a fresh session$`, func(kind string) error {
		if err := waitSessionState(h, tKey, host.Waiting); err != nil {
			return err
		}
		if len(drv.launches) != 2 || drv.launches[1].Session != "" {
			return fmt.Errorf("launches: %+v", drv.launches)
		}
		gotTurn := <-drv.procs[1].turns
		if gotTurn.text != "composed:"+kind {
			return fmt.Errorf("got turn %q, want composed:%s", gotTurn.text, kind)
		}
		for _, cmd := range rh.snapshot() {
			if ev, ok := cmd["event"].(map[string]any); ok && ev["code"] == "session_replaced" {
				return nil
			}
		}
		return fmt.Errorf("session_replaced not reported in %+v", rh.snapshot())
	})

	// Scenario 6: Jump
	sc.Step(`^an AI running a turn in its task session and also holding a group session$`, func() (err error) {
		var procCtx context.Context
		h, rh, procCtx, err = newTestHost(cleanup, 4, 15)
		if err != nil {
			return err
		}
		drv = &hrMemDriver{parentCtx: procCtx}
		host.Drivers = map[string]host.Driver{"claude": drv}
		host.Compose = func(_ *host.Host, s host.Session, kind host.TurnKind, _ []json.RawMessage) (string, host.Turn, error) {
			return "composed:" + string(kind), host.Turn{Kind: kind, Thread: s.Task}, nil
		}
		if err := h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: suiteT.TempDir(), Agent: "claude", Home: "home-dev", Name: "Claude"}); err != nil {
			return err
		}
		if err := h.Sessions.Create(host.Session{SessionKey: gKey, Kind: host.GroupSession, Folder: suiteT.TempDir(), Agent: "claude", Home: "home-dev", Name: "Claude"}); err != nil {
			return err
		}
		return host.Runner.Turn(suiteT.Context(), h, tKey, "initial work", host.Turn{Kind: host.ItemsTurn, Thread: hrTask1})
	})
	sc.Step(`^a jump turn arrives for the group session$`, func() error {
		return host.Runner.Turn(suiteT.Context(), h, gKey, "header\njump now", host.Turn{Kind: host.ItemsTurn, Jump: true})
	})
	sc.Step(`^the task session is interrupted and reported preempted while the jump turn runs$`, func() error {
		if drv.procs[0].interrupted != 1 {
			return fmt.Errorf("interrupted = %d, want 1", drv.procs[0].interrupted)
		}
		for _, cmd := range rh.snapshot() {
			if cmd["t"] == "task" && cmd["op"] == "preempted" && cmd["id"] == hrTask1 && cmd["msg"] == hrMsg1 {
				return nil
			}
		}
		return fmt.Errorf("task preempted not found in %+v", rh.snapshot())
	})
	sc.Step(`^when the jump turn ends the host sends task resume and a "([^"]+)" turn to the task session$`, func(kind string) error {
		drv.procs[1].emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
		if err := waitSessionState(h, tKey, host.Working); err != nil {
			return err
		}
		<-drv.procs[0].turns // initial turn
		cont := <-drv.procs[0].turns
		if cont.text != "composed:"+kind {
			return fmt.Errorf("got %q, want composed:%s", cont.text, kind)
		}
		for _, cmd := range rh.snapshot() {
			if cmd["t"] == "task" && cmd["op"] == "resume" && cmd["id"] == hrTask1 {
				return nil
			}
		}
		return fmt.Errorf("task resume not found in %+v", rh.snapshot())
	})
	var cmdCountBeforeSameJump int
	sc.Step(`^a jump turn arrives for the running task session itself$`, func() error {
		cmdCountBeforeSameJump = len(rh.snapshot())
		return host.Runner.Turn(suiteT.Context(), h, tKey, "same-session jump", host.Turn{Kind: host.ItemsTurn, Thread: hrTask1, Jump: true})
	})
	sc.Step(`^the task session is interrupted and runs the jump turn without pausing the task$`, func() error {
		if drv.procs[0].interrupted != 2 {
			return fmt.Errorf("interrupted = %d, want 2", drv.procs[0].interrupted)
		}
		for _, cmd := range rh.snapshot()[cmdCountBeforeSameJump:] {
			if cmd["t"] == "task" && cmd["op"] == "preempted" {
				return fmt.Errorf("same-session jump sent task preempted: %+v", cmd)
			}
		}
		got := <-drv.procs[0].turns
		if got.text != "same-session jump" {
			return fmt.Errorf("got %q, want same-session jump", got.text)
		}
		return nil
	})
}

func waitSessionState(h *host.Host, key host.SessionKey, want host.State) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := h.Sessions.Get(key); ok && s.State == want {
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
	s, _ := h.Sessions.Get(key)
	return fmt.Errorf("session %v state = %s, want %s", key, s.State, want)
}
