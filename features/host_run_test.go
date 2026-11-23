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
	hrTask1 = "DrtCn6htSBwmMPrFPbHJHA"
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
	cmd := build(id).(proto.Cmd)
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
	onStart   func(idx int, l host.Launch)
}

func (d *hrMemDriver) Start(ctx context.Context, l host.Launch) (host.Process, error) {
	d.mu.Lock()
	idx := len(d.launches)
	d.launches = append(d.launches, l)
	failResume := d.noSess[idx]
	autoEnd := d.autoEnd
	readyID := d.readyID
	parentCtx := d.parentCtx
	onStart := d.onStart
	p := &hrMemProc{ctx: ctx, events: make(chan host.Event, 16), turns: make(chan string, 8)}
	d.procs = append(d.procs, p)
	d.mu.Unlock()
	if onStart != nil {
		onStart(idx, l)
	}
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
	turns  chan string
}

func (p *hrMemProc) Events() <-chan host.Event { return p.events }

func (p *hrMemProc) Turn(text string) error {
	p.turns <- text
	return p.onTurn(text)
}

func (p *hrMemProc) Interrupt() error          { return nil }
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

// hrCLIDriver drives the built fakecli/claude binary on PATH so scenarios
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

	if err := os.MkdirAll(l.Folder, 0o755); err != nil {
		return nil, err
	}
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
	origDeliver := host.Deliver
	origInstructions := host.Instructions
	cleanup(func() {
		host.Drivers = origDrivers
		host.Compose = origCompose
		host.Deliver = origDeliver
		host.Instructions = origInstructions
	})

	registerIdleAndMaxOpenSteps(sc, suiteT)
	registerCLICrashAndSwitchSteps(sc, suiteT, cleanup)
	registerReplaceSteps(sc, suiteT, cleanup)
}

func installHRDefaultHooks() {
	host.Deliver = func(*host.Host, string, string) {}
	host.Instructions = func(host.Session) string { return "" }
	host.Compose = func(_ *host.Host, s host.Session, kind host.TurnKind, _ []json.RawMessage) (string, host.Turn, error) {
		return "composed:" + string(kind), host.Turn{Kind: kind, Thread: s.Task}, nil
	}
}

func newTestHost(suiteT *testing.T, cleanup func(func()), maxOpen, idleMin int) (*host.Host, *hrHome, context.Context, error) {
	installHRDefaultHooks()
	procCtx, cancelProcs := context.WithCancel(context.Background())
	dbPath := filepath.Join(suiteT.TempDir(), "node.db")
	db, err := store.Open(dbPath)
	if err != nil {
		cancelProcs()
		return nil, nil, nil, err
	}
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
		cancelProcs()
		db.Close()
		return nil, nil, nil, err
	}
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, time.Now)
	if err != nil {
		cancelProcs()
		db.Close()
		return nil, nil, nil, err
	}
	cleanup(func() { db.Close() })
	cleanup(func() {
		cancelProcs()
		h.Settle()
	})
	rh := newHRHome()
	h.Attach("home-dev", rh)
	return h, rh, procCtx, nil
}

func registerIdleAndMaxOpenSteps(sc *godog.ScenarioContext, suiteT *testing.T) {
	// Scenario 1 (A3.4): honest steps inside synctest.Test.
	var idleMinutes int
	var idleStateBeforeSecond host.State
	var idleStatusReported bool
	var idleResumedSession string

	sc.Step(`^a host session runtime with idle_minutes set to (\d+)$`, func(mins int) {
		idleMinutes = mins
		idleStateBeforeSecond = ""
		idleStatusReported = false
		idleResumedSession = ""
	})
	sc.Step(`^a task session with CLI session "([^"]+)" completes a turn, stays idle for 1 minute and receives another turn$`, func(cliID string) error {
		var stepErr error
		baseDir := suiteT.TempDir()
		synctest.Test(suiteT, func(t *testing.T) {
			installHRDefaultHooks()
			procCtx, cancelProcs := context.WithCancel(context.Background())
			db, err := store.Open(filepath.Join(baseDir, "node.db"))
			if err != nil {
				cancelProcs()
				stepErr = err
				return
			}
			defer func() {
				cancelProcs()
				synctest.Wait()
				db.Close()
			}()
			if err := db.Write(func(tx *store.Tx) error {
				_, e := tx.Exec("INSERT INTO setting(key, value) VALUES('idle_minutes', ?)", fmt.Sprint(idleMinutes))
				return e
			}); err != nil {
				stepErr = err
				return
			}
			drv := &hrMemDriver{parentCtx: procCtx, autoEnd: true, readyID: cliID}
			host.Drivers = map[string]host.Driver{"claude": drv}
			h, _, err := host.New(db, identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))), time.Now)
			if err != nil {
				stepErr = err
				return
			}
			rh := newHRHome()
			h.Attach("home-dev", rh)
			tKey := host.SessionKey{Group: hrGroup, Member: hrAI1, Task: hrTask1}
			if err := h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: "wt/1-task", Agent: "claude", Home: "home-dev", Name: "Claude"}); err != nil {
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
			idleStateBeforeSecond = s.State
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
		})
		return stepErr
	})
	sc.Step(`^the task session became "dormant" and member status reported "dormant" before the second turn$`, func() error {
		if idleStateBeforeSecond != host.Dormant || !idleStatusReported {
			return fmt.Errorf("state=%q dormantReported=%v", idleStateBeforeSecond, idleStatusReported)
		}
		return nil
	})
	sc.Step(`^the driver resumed CLI session "([^"]+)"$`, func(want string) error {
		if idleResumedSession != want {
			return fmt.Errorf("resumed %q, want %q", idleResumedSession, want)
		}
		return nil
	})

	// Scenario 2 (A3.5): max_open = 1 inside synctest.Test, recording first AI state inside hrMemDriver.Start.
	var maxOpenCfg int
	var firstStateAtSecondStart host.State
	var firstStateWhileSecondWorked host.State
	var launchesWhileSecondWorked int
	var firstStateAfterSecondFinished host.State

	sc.Step(`^a host session runtime with max_open set to (\d+) and two AIs$`, func(maxOpen int) {
		maxOpenCfg = maxOpen
		firstStateAtSecondStart = ""
		firstStateWhileSecondWorked = ""
		launchesWhileSecondWorked = 0
		firstStateAfterSecondFinished = ""
	})
	sc.Step(`^the first AI finishes a turn, the second AI starts a turn, and the first AI requests a turn before the second finishes$`, func() error {
		var stepErr error
		baseDir := suiteT.TempDir()
		synctest.Test(suiteT, func(t *testing.T) {
			installHRDefaultHooks()
			procCtx, cancelProcs := context.WithCancel(context.Background())
			db, err := store.Open(filepath.Join(baseDir, "node.db"))
			if err != nil {
				cancelProcs()
				stepErr = err
				return
			}
			defer func() {
				cancelProcs()
				synctest.Wait()
				db.Close()
			}()
			if err := db.Write(func(tx *store.Tx) error {
				_, e := tx.Exec("INSERT INTO setting(key, value) VALUES('max_open', ?)", fmt.Sprint(maxOpenCfg))
				return e
			}); err != nil {
				stepErr = err
				return
			}
			h2, _, err := host.New(db, identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))), time.Now)
			if err != nil {
				stepErr = err
				return
			}
			h2.Attach("home-dev", newHRHome())
			k1 := host.SessionKey{Group: hrGroup, Member: hrAI1}
			k2 := host.SessionKey{Group: hrGroup, Member: hrAI2}
			drv2 := &hrMemDriver{
				parentCtx: procCtx,
				onStart: func(idx int, _ host.Launch) {
					if idx == 1 {
						s1, _ := h2.Sessions.Get(k1)
						firstStateAtSecondStart = s1.State
					}
				},
			}
			host.Drivers = map[string]host.Driver{"claude": drv2}
			if err := h2.Sessions.Create(host.Session{SessionKey: k1, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: "home-dev", Name: "AI1"}); err != nil {
				stepErr = err
				return
			}
			if err := h2.Sessions.Create(host.Session{SessionKey: k2, Kind: host.GroupSession, Folder: "repo", Agent: "claude", Home: "home-dev", Name: "AI2"}); err != nil {
				stepErr = err
				return
			}
			if err := host.Runner.Turn(t.Context(), h2, k1, "hello", host.Turn{Kind: host.ItemsTurn}); err != nil {
				stepErr = err
				return
			}
			drv2.procs[0].emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
			synctest.Wait()

			if err := host.Runner.Turn(t.Context(), h2, k2, "world", host.Turn{Kind: host.ItemsTurn}); err != nil {
				stepErr = err
				return
			}
			waitDone := make(chan error, 1)
			go func() {
				waitDone <- host.Runner.Turn(t.Context(), h2, k1, "again", host.Turn{Kind: host.ItemsTurn})
			}()
			synctest.Wait()
			s1Mid, _ := h2.Sessions.Get(k1)
			firstStateWhileSecondWorked = s1Mid.State
			launchesWhileSecondWorked = len(drv2.launches)

			drv2.procs[1].emit(host.Event{Kind: host.TurnEnded, Status: "completed"})
			if err := <-waitDone; err != nil {
				stepErr = err
				return
			}
			s1End, _ := h2.Sessions.Get(k1)
			firstStateAfterSecondFinished = s1End.State
		})
		return stepErr
	})
	sc.Step(`^the first AI became "dormant" before the second AI started$`, func() error {
		if firstStateAtSecondStart != host.Dormant {
			return fmt.Errorf("first AI state at second AI Start = %q, want dormant", firstStateAtSecondStart)
		}
		return nil
	})
	sc.Step(`^the first AI waited while the second AI was working and started once it finished$`, func() error {
		if firstStateWhileSecondWorked != host.Dormant || launchesWhileSecondWorked != 2 {
			return fmt.Errorf("first AI did not wait: state=%q launches=%d", firstStateWhileSecondWorked, launchesWhileSecondWorked)
		}
		if firstStateAfterSecondFinished != host.Working {
			return fmt.Errorf("first AI state after second finished = %q, want working", firstStateAfterSecondFinished)
		}
		return nil
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
		h, rh, _, err = newTestHost(suiteT, cleanup, 4, 15)
		if err != nil {
			return err
		}
		cliDrv = newHRCLIDriver(cleanup, logFile)
		host.Drivers = map[string]host.Driver{"claude": cliDrv}
		return h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: "wt/1-task", Agent: "claude", Home: "home-dev", Name: "Claude"})
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
		h, rh, _, err = newTestHost(suiteT, cleanup, 1, 15)
		if err != nil {
			return err
		}
		cliDrv = newHRCLIDriver(cleanup, logFile, script)
		host.Drivers = map[string]host.Driver{"claude": cliDrv}
		if err := h.Sessions.Create(host.Session{SessionKey: kA, Kind: host.GroupSession, Folder: "repo-a", Agent: "claude", Home: "home-dev", Name: "A", CLISession: "sess-a"}); err != nil {
			return err
		}
		return h.Sessions.Create(host.Session{SessionKey: kB, Kind: host.GroupSession, Folder: "repo-b", Agent: "claude", Home: "home-dev", Name: "B", CLISession: "sess-b"})
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
		kC := host.SessionKey{Group: hrGroup, Member: "end-slot"}
		if err := h.Sessions.Create(host.Session{SessionKey: kC, Kind: host.GroupSession, Folder: "repo-c", Agent: "claude", Home: "home-dev", Name: "C", CLISession: "sess-c"}); err != nil {
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

func registerReplaceSteps(sc *godog.ScenarioContext, suiteT *testing.T, cleanup func(func())) {
	var h *host.Host
	var rh *hrHome
	var drv *hrMemDriver
	tKey := host.SessionKey{Group: hrGroup, Member: hrAI1, Task: hrTask1}

	sc.Step(`^a task session whose stored CLI session id is no longer resumable$`, func() (err error) {
		var procCtx context.Context
		h, rh, procCtx, err = newTestHost(suiteT, cleanup, 4, 15)
		if err != nil {
			return err
		}
		drv = &hrMemDriver{parentCtx: procCtx, noSess: map[int]bool{0: true}, autoEnd: true}
		host.Drivers = map[string]host.Driver{"claude": drv}
		return h.Sessions.Create(host.Session{SessionKey: tKey, Kind: host.TaskSession, Folder: "wt/1-task", Agent: "claude", Home: "home-dev", Name: "Claude", CLISession: "gone-id"})
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
		if gotTurn != "composed:"+kind {
			return fmt.Errorf("got turn %q, want composed:%s", gotTurn, kind)
		}
		for _, cmd := range rh.snapshot() {
			if ev, ok := cmd["event"].(map[string]any); ok && ev["code"] == "session_replaced" {
				return nil
			}
		}
		return fmt.Errorf("session_replaced not reported in %+v", rh.snapshot())
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
