package host

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	gopty "github.com/aymanbagabas/go-pty"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	ptyTestGroupID = "rZNvy-1jH6Z-BcPqA5U5BQ"
	ptyTestAIID    = "1PC8WineBrUQ-apCjx7tug"
)

func TestPTYHelperProcess(t *testing.T) {
	if t == nil {
		return
	}
	sep := slices.Index(os.Args, "--")
	if sep < 0 || sep+1 >= len(os.Args) {
		return
	}
	args := os.Args[sep+1:]
	switch args[0] {
	case "pty-echo":
		runPTYEchoHelper()
	case "pty-selfexit":
		fmt.Println("SELFEXIT-DONE")
	case "pty-flood":
		runPTYFloodHelper()
	case "pty-hangtree":
		runPTYHangTreeHelper(args[1])
	case "pty-hangleaf":
		runPTYHangLeafHelper(args[1])
	default:
		return
	}
	os.Exit(0)
}

func runPTYEchoHelper() {
	wd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	fmt.Printf("WD:%s\n", filepath.Base(wd))
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "quit" {
			fmt.Println("BYE-NOW")
			return
		}
		if trimmed != "" {
			fmt.Printf("ACK:%s\n", trimmed)
		}
		if err != nil {
			return
		}
	}
}

func runPTYFloodHelper() {
	chunk := []byte(strings.Repeat("FLOOD0123456789\n", 256))
	for range 512 {
		if _, err := os.Stdout.Write(chunk); err != nil {
			return
		}
	}
}

func runPTYHangTreeHelper(addrFile string) {
	signal.Ignore(syscall.SIGHUP)
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	child := exec.Command(exe, "-test.run=^TestPTYHelperProcess$", "--", "pty-hangleaf", addrFile)
	if err := child.Start(); err != nil {
		os.Exit(3)
	}
	time.Sleep(30 * time.Second)
}

func runPTYHangLeafHelper(addrFile string) {
	signal.Ignore(syscall.SIGHUP)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(4)
	}
	defer ln.Close()
	if err := os.WriteFile(addrFile, []byte(ln.Addr().String()), 0o600); err != nil {
		os.Exit(5)
	}
	time.Sleep(30 * time.Second)
}

type ptyFakeDriver struct {
	interactive func(l Launch) ([]string, string, error)
}

func (d ptyFakeDriver) Start(context.Context, Launch) (Process, error) {
	return nil, errors.New("unused structured start")
}

func (d ptyFakeDriver) Interactive(l Launch) ([]string, string, error) {
	return d.interactive(l)
}

func newPTYTestHost(t *testing.T) *Host {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := New(db, key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func registerPTYTestDriver(t *testing.T, name string, fn func(l Launch) ([]string, string, error)) {
	t.Helper()
	Drivers[name] = ptyFakeDriver{interactive: fn}
	t.Cleanup(func() { delete(Drivers, name) })
}

func ptyHelperArgv(t *testing.T, mode string, extra ...string) []string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{exe, "-test.run=^TestPTYHelperProcess$", "--", mode}
	return append(argv, extra...)
}

type ptyTestTerminal struct {
	label   string
	device  string
	inR     *io.PipeReader
	inW     *io.PipeWriter
	mu      sync.Mutex
	out     bytes.Buffer
	closed  bool
	blockCh chan struct{}
	writes  atomic.Int64
	notify  chan struct{}
}

func newPTYTestTerminal(label string) *ptyTestTerminal {
	r, w := io.Pipe()
	return &ptyTestTerminal{
		label:  label,
		device: "owner-browser",
		inR:    r,
		inW:    w,
		notify: make(chan struct{}, 1),
	}
}

func (tt *ptyTestTerminal) Label() string  { return tt.label }
func (tt *ptyTestTerminal) Device() string { return tt.device }

func (tt *ptyTestTerminal) Read(p []byte) (int, error) {
	return tt.inR.Read(p)
}

func (tt *ptyTestTerminal) Write(p []byte) (int, error) {
	tt.mu.Lock()
	block := tt.blockCh
	closed := tt.closed
	tt.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	if block != nil {
		<-block
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()
	if tt.closed {
		return 0, io.ErrClosedPipe
	}
	n, err := tt.out.Write(p)
	tt.writes.Add(1)
	select {
	case tt.notify <- struct{}{}:
	default:
	}
	return n, err
}

func (tt *ptyTestTerminal) Close() error {
	tt.mu.Lock()
	wasClosed := tt.closed
	tt.closed = true
	block := tt.blockCh
	tt.blockCh = nil
	tt.mu.Unlock()
	if !wasClosed && block != nil {
		close(block)
	}
	tt.inR.Close()
	tt.inW.Close()
	select {
	case tt.notify <- struct{}{}:
	default:
	}
	return nil
}

func (tt *ptyTestTerminal) setBlocked(blocked bool) {
	tt.mu.Lock()
	if blocked && tt.blockCh == nil && !tt.closed {
		tt.blockCh = make(chan struct{})
	} else if !blocked && tt.blockCh != nil {
		close(tt.blockCh)
		tt.blockCh = nil
	}
	tt.mu.Unlock()
}

func (tt *ptyTestTerminal) isClosed() bool {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return tt.closed
}

func (tt *ptyTestTerminal) output() string {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	return tt.out.String()
}

func (tt *ptyTestTerminal) waitContains(t *testing.T, sub string) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for !strings.Contains(tt.output(), sub) {
		select {
		case <-tt.notify:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %q in %q", sub, tt.output())
		}
	}
}

type ptySpyHandle struct {
	ptyHandle
	reads   atomic.Int64
	mu      sync.Mutex
	resizes []ptySize
}

type ptySize struct {
	cols int
	rows int
}

func (s *ptySpyHandle) Read(p []byte) (int, error) {
	s.reads.Add(1)
	return s.ptyHandle.Read(p)
}

func (s *ptySpyHandle) Resize(cols, rows int) error {
	err := s.ptyHandle.Resize(cols, rows)
	if err == nil {
		s.mu.Lock()
		s.resizes = append(s.resizes, ptySize{cols: cols, rows: rows})
		s.mu.Unlock()
	}
	return err
}

func (s *ptySpyHandle) recordedResizes() []ptySize {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.resizes)
}

func installPTYSpy(t *testing.T) func() *ptySpyHandle {
	t.Helper()
	origWrap := ptyWrap
	var mu sync.Mutex
	var latest *ptySpyHandle
	ptyWrap = func(ptmx gopty.Pty) (ptyHandle, error) {
		handle, err := origWrap(ptmx)
		if err != nil {
			return nil, err
		}
		spy := &ptySpyHandle{ptyHandle: handle}
		mu.Lock()
		latest = spy
		mu.Unlock()
		return spy, nil
	}
	t.Cleanup(func() { ptyWrap = origWrap })
	return func() *ptySpyHandle {
		mu.Lock()
		defer mu.Unlock()
		return latest
	}
}

func TestPTYStopTimeoutDefault(t *testing.T) {
	if ptyStopTimeout != 10*time.Second {
		t.Fatalf("ptyStopTimeout = %v, want 10s", ptyStopTimeout)
	}
}

func TestPTYParseLabelPinsEveryCondition(t *testing.T) {
	valid := []struct {
		label  string
		group  string
		member string
	}{
		{"pty:g1:m1", "g1", "m1"},
		{"pty:" + ptyTestGroupID + ":" + ptyTestAIID, ptyTestGroupID, ptyTestAIID},
	}
	for _, tc := range valid {
		g, m, ok := ptyParseLabel(tc.label)
		if !ok || g != tc.group || m != tc.member {
			t.Fatalf("ptyParseLabel(%q) = (%q, %q, %v), want (%q, %q, true)", tc.label, g, m, ok, tc.group, tc.member)
		}
	}
	invalid := []string{
		"",
		"ctl",
		"other:g1:m1",
		"pty",
		"pty:",
		"pty:g1",
		"pty::m1",
		"pty:g1:",
		"pty:g1:m1:extra",
	}
	for _, label := range invalid {
		g, m, ok := ptyParseLabel(label)
		if ok || g != "" || m != "" {
			t.Fatalf("ptyParseLabel(%q) = (%q, %q, %v), want empty false", label, g, m, ok)
		}
	}
}

func TestPTYInteractiveRoundTripAndChannelCloseTeardown(t *testing.T) {
	h := newPTYTestHost(t)
	workDir := filepath.Join(t.TempDir(), "wt-42")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	registerPTYTestDriver(t, "ptytest", func(l Launch) ([]string, string, error) {
		if l.Folder != workDir || l.Level != 2 {
			return nil, "", fmt.Errorf("unexpected launch: %+v", l)
		}
		return ptyHelperArgv(t, "pty-echo"), "sess-42", nil
	})

	var doneCount atomic.Int32
	doneCh := make(chan struct{})
	onDone := func() {
		if doneCount.Add(1) == 1 {
			close(doneCh)
		}
	}
	p, session, err := h.StartPTY(ptyTestGroupID, ptyTestAIID, "ptytest", Launch{Folder: workDir, Level: 2}, onDone)
	if err != nil || session != "sess-42" || p == nil {
		t.Fatalf("StartPTY = (%v, %q, %v), want sess-42", p, session, err)
	}
	if p.Attached() {
		t.Fatal("Attached reported true before ServeTerminal")
	}

	term := newPTYTestTerminal("pty:" + ptyTestGroupID + ":" + ptyTestAIID)
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		h.ServeTerminal(term)
	}()

	term.waitContains(t, "WD:wt-42")
	if !p.Attached() {
		t.Fatal("Attached reported false after ServeTerminal")
	}
	if _, err := term.inW.Write([]byte("alpha-token\r")); err != nil {
		t.Fatal(err)
	}
	term.waitContains(t, "ACK:alpha-token")

	// A duplicate channel for the same (group, member) is closed without disturbing the active bridge.
	dup := newPTYTestTerminal("pty:" + ptyTestGroupID + ":" + ptyTestAIID)
	h.ServeTerminal(dup)
	if !dup.isClosed() {
		t.Fatal("duplicate terminal was not closed")
	}

	if _, err := term.inW.Write([]byte("beta-token\r")); err != nil {
		t.Fatal(err)
	}
	term.waitContains(t, "ACK:beta-token")

	// Closing the terminal channel ends the bridge, closes the PTY and fires onDone once.
	term.Close()
	<-serveDone
	<-doneCh
	if got := doneCount.Load(); got != 1 {
		t.Fatalf("onDone count = %d, want 1", got)
	}
	// Calling Close and ClosePTY after teardown is idempotent and reports false on ClosePTY.
	if err := p.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	if h.ClosePTY(ptyTestGroupID, ptyTestAIID) {
		t.Fatal("ClosePTY reported true after PTY already closed")
	}
}

func TestPTYProcessSelfExitClosesTerminalAndAllowsReopen(t *testing.T) {
	h := newPTYTestHost(t)
	dir := t.TempDir()
	registerPTYTestDriver(t, "ptyexit", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-selfexit"), "sess-exit", nil
	})

	doneCh := make(chan struct{}, 1)
	_, session, err := h.StartPTY("g1", "m1", "ptyexit", Launch{Folder: dir, Level: 1}, func() {
		doneCh <- struct{}{}
	})
	if err != nil || session != "sess-exit" {
		t.Fatalf("StartPTY = (%q, %v)", session, err)
	}
	term := newPTYTestTerminal("pty:g1:m1")
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		h.ServeTerminal(term)
	}()

	<-serveDone
	<-doneCh
	if !term.isClosed() {
		t.Fatal("terminal was not closed when process exited")
	}

	// Unattached process exit completes quickly without waiting for ptyStopTimeout, and slot can reopen.
	unattachedDone := make(chan struct{}, 1)
	start := time.Now()
	_, _, err = h.StartPTY("g1", "m1", "ptyexit", Launch{Folder: dir, Level: 1}, func() {
		unattachedDone <- struct{}{}
	})
	if err != nil {
		t.Fatalf("reopen StartPTY = %v", err)
	}
	select {
	case <-unattachedDone:
	case <-time.After(3 * time.Second):
		t.Fatal("unattached self-exiting PTY did not finish promptly")
	}
	if elapsed := time.Since(start); elapsed >= ptyStopTimeout {
		t.Fatalf("unattached exit took %v, expected well below %v", elapsed, ptyStopTimeout)
	}
}

func TestPTYBackPressureStopsReadingWhileTerminalWriteBlocks(t *testing.T) {
	h := newPTYTestHost(t)
	getSpy := installPTYSpy(t)
	registerPTYTestDriver(t, "ptyflood", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-flood"), "sess-flood", nil
	})

	p, _, err := h.StartPTY("g1", "m1", "ptyflood", Launch{Folder: t.TempDir(), Level: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	spy := getSpy()
	if got := spy.recordedResizes(); len(got) != 0 {
		p.Close()
		t.Fatalf("unexpected initial resizes without pty.resize: %+v", got)
	}
	term := newPTYTestTerminal("pty:g1:m1")
	term.setBlocked(true)

	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		h.ServeTerminal(term)
	}()

	// Wait until the first Read from the PTY has occurred and blocked inside Terminal.Write.
	deadline := time.Now().Add(5 * time.Second)
	for spy.reads.Load() == 0 {
		if time.Now().After(deadline) {
			p.Close()
			t.Fatal("timed out waiting for first PTY read")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(40 * time.Millisecond)
	pinnedReads := spy.reads.Load()
	if pinnedReads != 1 {
		p.Close()
		t.Fatalf("PTY reads while Terminal.Write blocked = %d, want 1", pinnedReads)
	}
	time.Sleep(60 * time.Millisecond)
	if afterWait := spy.reads.Load(); afterWait != pinnedReads {
		p.Close()
		t.Fatalf("PTY continued reading while Terminal.Write blocked: %d -> %d", pinnedReads, afterWait)
	}

	// Unblocking Terminal.Write resumes reading from the PTY.
	term.setBlocked(false)
	term.waitContains(t, "FLOOD0123456789")
	for spy.reads.Load() <= pinnedReads {
		if time.Now().After(deadline) {
			p.Close()
			t.Fatal("PTY reads did not resume after unblocking Terminal.Write")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Block again and verify ClosePTY cleanly unblocks Terminal.Write and shuts down.
	term.setBlocked(true)
	if !h.ClosePTY("g1", "m1") {
		t.Fatal("ClosePTY returned false for open PTY")
	}
	<-serveDone
}

func TestPTYResizeSurfaceRejectsInvalidFrames(t *testing.T) {
	h := newPTYTestHost(t)
	handle := Surface["pty.resize"]
	if handle == nil {
		t.Fatal("Surface[\"pty.resize\"] is not registered")
	}

	badPayloads := []string{
		`not-json`,
		`{"t":"other","g":"` + ptyTestGroupID + `","ai":"` + ptyTestAIID + `","cols":80,"rows":24}`,
		`{"t":"pty.resize","g":"","ai":"` + ptyTestAIID + `","cols":80,"rows":24}`,
		`{"t":"pty.resize","g":"` + ptyTestGroupID + `","ai":"","cols":80,"rows":24}`,
		`{"t":"pty.resize","g":"` + ptyTestGroupID + `","ai":"` + ptyTestAIID + `","cols":0,"rows":24}`,
		`{"t":"pty.resize","g":"` + ptyTestGroupID + `","ai":"` + ptyTestAIID + `","cols":-1,"rows":24}`,
		`{"t":"pty.resize","g":"` + ptyTestGroupID + `","ai":"` + ptyTestAIID + `","cols":80,"rows":0}`,
		`{"t":"pty.resize","g":"` + ptyTestGroupID + `","ai":"` + ptyTestAIID + `","cols":80,"rows":-1}`,
	}
	for _, raw := range badPayloads {
		res, err := handle(context.Background(), h, nil, "dev1", json.RawMessage(raw))
		assertEqual(t, res, nil)
		assertErrorCode(t, err, "bad_request")
	}
}

func sendTestPTYResize(t *testing.T, h *Host, cols, rows int) {
	t.Helper()
	raw := fmt.Sprintf(`{"t":"pty.resize","g":%q,"ai":%q,"cols":%d,"rows":%d}`, ptyTestGroupID, ptyTestAIID, cols, rows)
	res, err := Surface["pty.resize"](context.Background(), h, nil, "dev1", json.RawMessage(raw))
	assertEqual(t, res, nil)
	assertEqual(t, err, nil)
}

func TestPTYResizeSurfaceAndBoundaries(t *testing.T) {
	h := newPTYTestHost(t)
	getSpy := installPTYSpy(t)
	registerPTYTestDriver(t, "ptyresize", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-echo"), "sess-resize", nil
	})

	sendTestPTYResize(t, h, 100, 30)
	p, _, err := h.StartPTY(ptyTestGroupID, ptyTestAIID, "ptyresize", Launch{Folder: t.TempDir(), Level: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	spy := getSpy()
	assertEqual(t, spy.recordedResizes(), []ptySize(nil))

	sendTestPTYResize(t, h, 1, 1)
	sendTestPTYResize(t, h, 1, 24)
	sendTestPTYResize(t, h, 80, 1)

	p.Close()
	assertEqual(t, p.resize(proto.PTYResize{Cols: 90, Rows: 25}), nil)
	sendTestPTYResize(t, h, 90, 25)
	assertEqual(t, spy.recordedResizes(), []ptySize{{1, 1}, {1, 24}, {80, 1}})
	next, _, err := h.StartPTY(ptyTestGroupID, ptyTestAIID, "ptyresize", Launch{Folder: t.TempDir(), Level: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	assertEqual(t, getSpy().recordedResizes(), []ptySize(nil))
}

func TestPTYStartRefusalsAndErrorCleanup(t *testing.T) {
	h := newPTYTestHost(t)

	// 1. Unknown agent returns bad_request and releases the slot.
	_, _, err := h.StartPTY("g1", "m1", "missing-agent", Launch{}, nil)
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "bad_request" {
		t.Fatalf("unknown agent = %v, want bad_request", err)
	}

	// 2. Driver returning an *Error preserves its code; plain error becomes cli.
	registerPTYTestDriver(t, "conflict-drv", func(Launch) ([]string, string, error) {
		return nil, "", &Error{Code: "conflict", Message: "unsupported level"}
	})
	_, _, err = h.StartPTY("g1", "m1", "conflict-drv", Launch{}, nil)
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "conflict" {
		t.Fatalf("conflict driver = %v, want conflict", err)
	}

	registerPTYTestDriver(t, "plainerr-drv", func(Launch) ([]string, string, error) {
		return nil, "", errors.New("no thread yet")
	})
	_, _, err = h.StartPTY("g1", "m1", "plainerr-drv", Launch{}, nil)
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "cli" || hostErr.Message != "no thread yet" {
		t.Fatalf("plainerr driver = %v, want cli: no thread yet", err)
	}

	// 3. Missing executable on PATH returns cli.
	registerPTYTestDriver(t, "noexe-drv", func(Launch) ([]string, string, error) {
		return []string{"solodawn-definitely-not-on-path-xyz"}, "s", nil
	})
	_, _, err = h.StartPTY("g1", "m1", "noexe-drv", Launch{}, nil)
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "cli" {
		t.Fatalf("missing exe = %v, want cli", err)
	}

	// 4. ptyNew failure returns cli.
	origNew := ptyNew
	ptyNew = func() (gopty.Pty, error) { return nil, errors.New("out of ptys") }
	registerPTYTestDriver(t, "good-drv", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-echo"), "s", nil
	})
	_, _, err = h.StartPTY("g1", "m1", "good-drv", Launch{Folder: t.TempDir()}, nil)
	ptyNew = origNew
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "cli" || hostErr.Message != "out of ptys" {
		t.Fatalf("ptyNew failure = %v, want cli: out of ptys", err)
	}

	// 5. cmd.Start failure (missing working folder) closes ptmx and returns cli.
	registerPTYTestDriver(t, "baddir-drv", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-echo"), "s", nil
	})
	_, _, err = h.StartPTY("g1", "m1", "baddir-drv", Launch{Folder: filepath.Join(t.TempDir(), "absent-folder")}, nil)
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "cli" {
		t.Fatalf("cmd.Start failure = %v, want cli", err)
	}

	// 6. Duplicate StartPTY while open returns busy without calling Driver.Interactive a second time.
	var calls atomic.Int32
	registerPTYTestDriver(t, "once-drv", func(Launch) ([]string, string, error) {
		calls.Add(1)
		return ptyHelperArgv(t, "pty-echo"), "s-once", nil
	})
	p, _, err := h.StartPTY("g1", "m1", "once-drv", Launch{Folder: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	_, _, err = h.StartPTY("g1", "m1", "once-drv", Launch{Folder: t.TempDir()}, nil)
	if hostErr, ok := errors.AsType[*Error](err); !ok || hostErr.Code != "busy" {
		t.Fatalf("duplicate StartPTY = %v, want busy", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("Driver.Interactive calls = %d, want 1", got)
	}
}

func TestPTYRejectsInvalidOrUnmatchedStreams(t *testing.T) {
	h := newPTYTestHost(t)
	for _, label := range []string{"pty:invalid", "pty:g1:absent"} {
		st := newPTYTestTerminal(label)
		h.ServeTerminal(st)
		if !st.isClosed() {
			t.Fatalf("label %q was not closed", label)
		}
	}
}

func TestPTYWrapFailureReleasesSlot(t *testing.T) {
	h := newPTYTestHost(t)
	registerPTYTestDriver(t, "wrap-error", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-selfexit"), "session", nil
	})
	original := ptyWrap
	t.Cleanup(func() { ptyWrap = original })
	ptyWrap = func(gopty.Pty) (ptyHandle, error) { return nil, errors.New("cannot wrap master") }
	p, session, err := h.StartPTY("g1", "m1", "wrap-error", Launch{Folder: t.TempDir()}, nil)
	assertEqual(t, p, (*PTY)(nil))
	assertEqual(t, session, "")
	assertEqual(t, assertErrorCode(t, err, "cli").Message, "cannot wrap master")
	if !ptyState(h).reserve(aiSlot{group: "g1", member: "m1"}) {
		t.Fatal("slot was not released")
	}
}

type ptyResizeErrorHandle struct{ ptyNopHandle }

func (ptyResizeErrorHandle) Resize(int, int) error { return errors.New("cannot resize master") }

func TestPTYResizeFailure(t *testing.T) {
	h := newPTYTestHost(t)
	part := ptyState(h)
	slot := aiSlot{group: ptyTestGroupID, member: ptyTestAIID}
	part.activate(slot, &PTY{part: part, handle: ptyResizeErrorHandle{}})
	raw := fmt.Sprintf(`{"t":"pty.resize","g":%q,"ai":%q,"cols":80,"rows":24}`, slot.group, slot.member)
	res, err := Surface["pty.resize"](context.Background(), h, nil, "dev1", json.RawMessage(raw))
	assertEqual(t, res, nil)
	assertEqual(t, assertErrorCode(t, err, "cli").Message, "cannot resize master")
}

func waitLeafListening(t *testing.T, addrFile string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(addrFile)
		if err == nil && len(data) > 0 {
			addr := strings.TrimSpace(string(data))
			conn, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
			if dialErr == nil {
				conn.Close()
				return addr
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for leaf listener in %s", addrFile)
	return ""
}

func waitLeafStopped(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 40*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("leaf process at %s still listening after tree kill", addr)
}

func TestPTYKillTreeStopsLeaderAndChildSubprocess(t *testing.T) {
	ptmx, err := gopty.New()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()

	addrFile := filepath.Join(t.TempDir(), "leaf.addr")
	argv := ptyHelperArgv(t, "pty-hangtree", addrFile)
	cmd := ptmx.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	addr := waitLeafListening(t, addrFile)

	ptyKillTree(cmd.Process.Pid)
	cmd.Wait()
	waitLeafStopped(t, addr)
}

type ptyNopHandle struct{}

func (ptyNopHandle) Read([]byte) (int, error)    { return 0, io.EOF }
func (ptyNopHandle) Write(b []byte) (int, error) { return len(b), nil }
func (ptyNopHandle) Close() error                { return nil }
func (ptyNopHandle) Resize(int, int) error       { return nil }

func TestPTYCloseTimeoutAndReentrancy(t *testing.T) {
	saved := ptyStopTimeout
	ptyStopTimeout = 30 * time.Millisecond
	t.Cleanup(func() { ptyStopTimeout = saved })

	// 1. End-to-end Close on a real PTY process that ignores SIGHUP and has a child subprocess.
	h := newPTYTestHost(t)
	addrFile := filepath.Join(t.TempDir(), "hang.addr")
	registerPTYTestDriver(t, "ptyhang", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-hangtree", addrFile), "sess-hang", nil
	})
	var reentrantErr error
	var p *PTY
	p, _, err := h.StartPTY("g1", "m1", "ptyhang", Launch{Folder: t.TempDir()}, func() {
		// Re-entrant Close inside onDone does not deadlock.
		reentrantErr = p.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	addr := waitLeafListening(t, addrFile)
	if err := p.Close(); err != nil || reentrantErr != nil {
		t.Fatalf("Close = %v, reentrant Close = %v", err, reentrantErr)
	}
	waitLeafStopped(t, addr)

	// 2. Deterministic check pinning kill-on-timeout vs no-kill-on-prompt-exit on every OS.
	part := ptyState(h)
	for _, slow := range []bool{false, true} {
		slot := aiSlot{group: "g-det", member: fmt.Sprintf("m-%v", slow)}
		part.reserve(slot)
		procDone := make(chan struct{})
		if !slow {
			close(procDone)
		}
		var killed atomic.Int32
		killFn := func() {
			killed.Add(1)
			if slow {
				close(procDone)
			}
		}
		entry := &PTY{
			part:     part,
			slot:     slot,
			handle:   ptyNopHandle{},
			kill:     killFn,
			closing:  make(chan struct{}),
			procDone: procDone,
			outDone:  make(chan struct{}),
			done:     make(chan struct{}),
		}
		part.activate(slot, entry)
		entry.Close()
		wantKills := int32(0)
		if slow {
			wantKills = 1
		}
		if got := killed.Load(); got != wantKills {
			t.Fatalf("slow=%v: kill count = %d, want %d", slow, got, wantKills)
		}
	}
}
