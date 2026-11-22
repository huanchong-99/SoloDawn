//go:build acceptance

package features

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_pty"] = func(sc *godog.ScenarioContext, f fixtures) {
		hostPTYSteps(sc, f.t)
	}
}

func TestHostPTYHelperProcess(t *testing.T) {
	if t == nil {
		return
	}
	sep := slices.Index(os.Args, "--")
	if sep < 0 || sep+1 >= len(os.Args) {
		return
	}
	switch os.Args[sep+1] {
	case "acc-pty-echo":
		runAccPTYEcho()
	case "acc-pty-flood":
		runAccPTYFlood()
	case "acc-pty-exit":
		fmt.Println("ACC-EXIT")
	default:
		return
	}
	os.Exit(0)
}

func runAccPTYEcho() {
	wd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	fmt.Printf("WD:%s\n", filepath.Base(wd))
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed != "" {
			fmt.Printf("ACK:%s\n", trimmed)
		}
		if err != nil {
			return
		}
	}
}

func runAccPTYFlood() {
	chunk := []byte(strings.Repeat("STREAM-CHUNK-DATA\n", 256))
	for range 512 {
		if _, err := os.Stdout.Write(chunk); err != nil {
			return
		}
	}
}

type accPTYDriver struct {
	build func(l host.Launch) ([]string, string, error)
}

func (d accPTYDriver) Start(context.Context, host.Launch) (host.Process, error) {
	return nil, errors.New("unused structured start")
}

func (d accPTYDriver) Interactive(l host.Launch) ([]string, string, error) {
	return d.build(l)
}

type accTerminal struct {
	label    string
	inR      *io.PipeReader
	inW      *io.PipeWriter
	unread   []byte
	drained  bool
	mu       sync.Mutex
	out      bytes.Buffer
	closed   bool
	blockCh  chan struct{}
	writeCnt atomic.Int64
	notify   chan struct{}
}

func newAccTerminal(label string) *accTerminal {
	r, w := io.Pipe()
	return &accTerminal{label: label, inR: r, inW: w, notify: make(chan struct{}, 1)}
}

func (at *accTerminal) Label() string  { return at.label }
func (at *accTerminal) Device() string { return "owner-device" }

func (at *accTerminal) Read(p []byte) (int, error) {
	if at.unread != nil {
		if len(at.unread) == 0 {
			at.drained = true
			return 0, io.EOF
		}
		n := copy(p, at.unread)
		at.unread = at.unread[n:]
		return n, nil
	}
	return at.inR.Read(p)
}

func (at *accTerminal) Write(p []byte) (int, error) {
	at.writeCnt.Add(1)
	at.mu.Lock()
	block := at.blockCh
	closed := at.closed
	at.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	if block != nil {
		<-block
	}
	at.mu.Lock()
	defer at.mu.Unlock()
	if at.closed {
		return 0, io.ErrClosedPipe
	}
	n, err := at.out.Write(p)
	select {
	case at.notify <- struct{}{}:
	default:
	}
	return n, err
}

func (at *accTerminal) Close() error {
	at.mu.Lock()
	wasClosed := at.closed
	at.closed = true
	block := at.blockCh
	at.blockCh = nil
	at.mu.Unlock()
	if !wasClosed && block != nil {
		close(block)
	}
	if at.inR != nil {
		at.inR.Close()
		at.inW.Close()
	}
	select {
	case at.notify <- struct{}{}:
	default:
	}
	return nil
}

func (at *accTerminal) setBlocked(blocked bool) {
	at.mu.Lock()
	if blocked && at.blockCh == nil && !at.closed {
		at.blockCh = make(chan struct{})
	} else if !blocked && at.blockCh != nil {
		close(at.blockCh)
		at.blockCh = nil
	}
	at.mu.Unlock()
}

func (at *accTerminal) isClosed() bool {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.closed
}

func (at *accTerminal) output() string {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.out.String()
}

func (at *accTerminal) waitOutput(subs ...string) error {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		out := at.output()
		allFound := true
		for _, sub := range subs {
			if !strings.Contains(out, sub) {
				allFound = false
			}
		}
		if allFound {
			return nil
		}
		select {
		case <-at.notify:
		case <-timer.C:
			return fmt.Errorf("timed out waiting for %v in %q", subs, at.output())
		}
	}
}

func hostPTYSteps(sc *godog.ScenarioContext, suiteT *testing.T) {
	cleanup := afterScenario(sc)
	var (
		h             *host.Host
		workDir       string
		activeGroup   string
		activeMember  string
		term          *accTerminal
		serveDone     chan struct{}
		doneCount     atomic.Int32
		doneCh        chan struct{}
		resizeOutcome string
		rejected      []*accTerminal
	)

	startMode := func(group, member, mode string) error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		driverName := "acc-" + mode
		host.Drivers[driverName] = accPTYDriver{
			build: func(host.Launch) ([]string, string, error) {
				return []string{exe, "-test.run=^TestHostPTYHelperProcess$", "--", mode}, "acc-session", nil
			},
		}
		cleanup(func() { delete(host.Drivers, driverName) })
		activeGroup, activeMember = group, member
		doneCount.Store(0)
		doneCh = make(chan struct{})
		onDone := func() {
			if doneCount.Add(1) == 1 {
				close(doneCh)
			}
		}
		p, _, err := h.StartPTY(group, member, driverName, host.Launch{Folder: workDir, Level: 1}, onDone)
		if err != nil {
			return err
		}
		cleanup(func() { p.Close() })
		return nil
	}

	openTerm := func(label string, blocked bool) {
		term = newAccTerminal(label)
		term.setBlocked(blocked)
		serveDone = make(chan struct{})
		go func() {
			defer close(serveDone)
			h.ServeTerminal(term)
		}()
	}

	sc.Step(`^an interactive PTY host with a session folder$`, func() error {
		db, err := store.Open(":memory:")
		if err != nil {
			return err
		}
		cleanup(func() { db.Close() })
		key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
		h, _, err = host.New(db, key, time.Now)
		if err != nil {
			return err
		}
		workDir = filepath.Join(suiteT.TempDir(), "session-folder")
		return os.MkdirAll(workDir, 0o755)
	})

	sc.Step(`^the host starts an interactive PTY session for group "([^"]+)" and member "([^"]+)"$`, func(group, member string) error {
		return startMode(group, member, "acc-pty-echo")
	})

	sc.Step(`^the host starts a flooding interactive PTY session for group "([^"]+)" and member "([^"]+)"$`, func(group, member string) error {
		return startMode(group, member, "acc-pty-flood")
	})

	sc.Step(`^the host starts a self-exiting interactive PTY session for group "([^"]+)" and member "([^"]+)"$`, func(group, member string) error {
		return startMode(group, member, "acc-pty-exit")
	})

	sc.Step(`^the owner opens terminal channel "([^"]+)"$`, func(label string) {
		openTerm(label, false)
	})

	sc.Step(`^the owner opens a back-pressured terminal channel "([^"]+)"$`, func(label string) {
		openTerm(label, true)
	})

	sc.Step(`^the owner writes "([^"]+)" to the terminal channel$`, func(text string) error {
		_, err := term.inW.Write([]byte(text + "\r"))
		return err
	})

	sc.Step(`^the terminal channel receives "([^"]+)" and "([^"]+)"$`, func(first, second string) error {
		return term.waitOutput(first, second)
	})

	sc.Step(`^the owner closes the terminal channel$`, func() {
		term.Close()
	})

	sc.Step(`^the interactive PTY exits and notifies completion once$`, func() error {
		<-serveDone
		<-doneCh
		if got := doneCount.Load(); got != 1 {
			return fmt.Errorf("completion count = %d, want 1", got)
		}
		return nil
	})

	sc.Step(`^pseudo-terminal reads pause while the terminal write buffer is full$`, func() error {
		deadline := time.Now().Add(5 * time.Second)
		for term.writeCnt.Load() == 0 {
			if time.Now().After(deadline) {
				return errors.New("timed out waiting for first blocked terminal write")
			}
			time.Sleep(5 * time.Millisecond)
		}
		pinned := term.writeCnt.Load()
		time.Sleep(80 * time.Millisecond)
		if got := term.writeCnt.Load(); got != pinned {
			return fmt.Errorf("terminal writes grew while blocked: %d -> %d", pinned, got)
		}
		return nil
	})

	sc.Step(`^the terminal write buffer drains$`, func() {
		term.setBlocked(false)
	})

	sc.Step(`^pseudo-terminal reads resume and closing the PTY ends the stream$`, func() error {
		if err := term.waitOutput("STREAM-CHUNK-DATA"); err != nil {
			return err
		}
		if !h.ClosePTY(activeGroup, activeMember) {
			return errors.New("ClosePTY returned false")
		}
		<-serveDone
		<-doneCh
		return nil
	})

	sc.Step(`^the owner sends pty\.resize for group "([^"]*)" and AI "([^"]*)" with cols (\d+) and rows (\d+) as type "([^"]+)"$`, func(group, ai string, cols, rows int, frameType string) error {
		if frameType == "pty.resize" && cols >= 1 && rows >= 1 {
			if err := startMode(group, ai, "acc-pty-echo"); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(map[string]any{
			"t":    frameType,
			"g":    group,
			"ai":   ai,
			"cols": cols,
			"rows": rows,
		})
		if err != nil {
			return err
		}
		_, err = host.Surface["pty.resize"](context.Background(), h, nil, "owner-device", raw)
		resizeOutcome = "ok"
		if hostErr, ok := errors.AsType[*host.Error](err); ok {
			resizeOutcome = hostErr.Code
		} else if err != nil {
			return err
		}
		h.ClosePTY(group, ai)
		return nil
	})

	sc.Step(`^the pty\.resize outcome is "([^"]+)"$`, func(want string) error {
		if resizeOutcome != want {
			return fmt.Errorf("got %q, want %q", resizeOutcome, want)
		}
		return nil
	})

	sc.Step(`^the terminal channel closes and completion is notified once$`, func() error {
		<-serveDone
		<-doneCh
		if !term.isClosed() {
			return errors.New("terminal channel remained open")
		}
		if got := doneCount.Load(); got != 1 {
			return fmt.Errorf("completion count = %d, want 1", got)
		}
		return nil
	})

	sc.Step(`^an unmatched terminal channel "([^"]+)" and a malformed channel "([^"]+)" arrive with unread bytes$`, func(unmatched, malformed string) {
		rejected = []*accTerminal{
			{label: unmatched, unread: []byte("queued-1")},
			{label: malformed, unread: []byte("queued-2")},
		}
		for _, ch := range rejected {
			h.ServeTerminal(ch)
		}
	})

	sc.Step(`^both terminal channels are closed and drained$`, func() error {
		for _, ch := range rejected {
			if !ch.closed || !ch.drained {
				return fmt.Errorf("channel %q: closed=%v drained=%v", ch.label, ch.closed, ch.drained)
			}
		}
		return nil
	})
}
