package host

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gopty "github.com/aymanbagabas/go-pty"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// ptyStopTimeout bounds how long Close waits after closing the pseudo-terminal
// before killing the process tree (flows/takeover.md step 13).
var ptyStopTimeout = 10 * time.Second

// ptyNew and ptyWrap are package-internal seams over go-pty creation.
var (
	ptyNew  = gopty.New
	ptyWrap = ptyWrapStarted
)

func init() {
	Register(Surface, "pty.resize", ptyResize)
}

type ptyHandle interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

type ptySlot struct {
	group  string
	member string
}

type ptySize struct {
	cols int
	rows int
}

// ptyPart keeps the open interactive pseudo-terminals and remembered terminal
// sizes of one Host, stored via Part[ptyPart](h).
//
// Lock order: PTY.mu -> ptyPart.mu. Neither lock waits on I/O, process exit
// or callbacks.
type ptyPart struct {
	mu       sync.Mutex
	reserved map[ptySlot]bool
	open     map[ptySlot]*PTY
	sizes    map[ptySlot]ptySize
}

func ptyState(h *Host) *ptyPart {
	part := Part[ptyPart](h)
	part.mu.Lock()
	if part.open == nil {
		part.reserved = map[ptySlot]bool{}
		part.open = map[ptySlot]*PTY{}
		part.sizes = map[ptySlot]ptySize{}
	}
	part.mu.Unlock()
	return part
}

func (part *ptyPart) reserve(slot ptySlot) bool {
	part.mu.Lock()
	defer part.mu.Unlock()
	if part.reserved[slot] {
		return false
	}
	part.reserved[slot] = true
	return true
}

func (part *ptyPart) release(slot ptySlot) {
	part.mu.Lock()
	defer part.mu.Unlock()
	delete(part.reserved, slot)
	delete(part.open, slot)
}

func (part *ptyPart) activate(slot ptySlot, p *PTY) {
	part.mu.Lock()
	defer part.mu.Unlock()
	part.open[slot] = p
}

func (part *ptyPart) lookup(slot ptySlot) *PTY {
	part.mu.Lock()
	defer part.mu.Unlock()
	return part.open[slot]
}

func (part *ptyPart) noteSize(slot ptySlot, cols, rows int) *PTY {
	part.mu.Lock()
	defer part.mu.Unlock()
	part.sizes[slot] = ptySize{cols: cols, rows: rows}
	return part.open[slot]
}

func (part *ptyPart) sizeOf(slot ptySlot) (ptySize, bool) {
	part.mu.Lock()
	defer part.mu.Unlock()
	sz, ok := part.sizes[slot]
	return sz, ok
}

type ptyReaderOnly struct {
	r io.Reader
}

func (ro ptyReaderOnly) Read(p []byte) (int, error) {
	return ro.r.Read(p)
}

type ptyWriterOnly struct {
	w io.Writer
}

func (wo ptyWriterOnly) Write(p []byte) (int, error) {
	return wo.w.Write(p)
}

// PTY is one running interactive CLI attached to a pseudo-terminal
// (contracts/drivers.md section 1.3, flows/takeover.md steps 6 to 13).
//
// Lock order: PTY.mu -> ptyPart.mu. Neither lock waits on I/O, process exit
// or callbacks.
type PTY struct {
	part    *ptyPart
	slot    ptySlot
	handle  ptyHandle
	mcpPath string
	kill    func()
	onDone  func()

	mu          sync.Mutex
	term        Terminal
	readerTaken bool
	stopping    bool
	closed      bool

	// Single reader: waitLoop.
	closing chan struct{}
	// Single reader: runStop.
	procDone chan struct{}
	// Single reader: runStop.
	outDone chan struct{}
	// Single reader: Close callers waiting for runStop to finish.
	done chan struct{}
}

// StartPTY starts the interactive CLI of agent for (group, member) in a
// pseudo-terminal in launch.Folder (contracts/drivers.md section 1.3). It
// returns the open PTY and the CLI session id from Driver.Interactive. When
// the PTY ends for any reason, onDone (if not nil) runs once after the
// process has exited.
func (h *Host) StartPTY(group, member, agent string, launch Launch, onDone func()) (*PTY, string, error) {
	part := ptyState(h)
	slot := ptySlot{group: group, member: member}
	if !part.reserve(slot) {
		return nil, "", &Error{Code: "busy", Message: "interactive terminal already open"}
	}
	argv, session, err := ptyInteractiveArgs(agent, launch)
	if err != nil {
		part.release(slot)
		return nil, "", err
	}
	p, err := ptySpawn(part, slot, argv, launch.Folder, onDone)
	if err != nil {
		part.release(slot)
		return nil, "", err
	}
	return p, session, nil
}

func ptyInteractiveArgs(agent string, launch Launch) ([]string, string, error) {
	driver := Drivers[agent]
	if driver == nil {
		return nil, "", &Error{Code: "bad_request", Message: "unknown agent: " + agent}
	}
	argv, session, err := driver.Interactive(launch)
	if err != nil {
		return nil, "", ptyWrapCLIError(err)
	}
	if len(argv) == 0 || argv[0] == "" {
		os.Remove(ptyMCPConfigPath(argv))
		return nil, "", &Error{Code: "bad_request", Message: "empty interactive command"}
	}
	return argv, session, nil
}

func ptyWrapCLIError(err error) error {
	if _, ok := errors.AsType[*Error](err); ok {
		return err
	}
	return &Error{Code: "cli", Message: err.Error()}
}

func ptyMCPConfigPath(argv []string) string {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--mcp-config" {
			return argv[i+1]
		}
	}
	return ""
}

func ptyResolveExe(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func ptyOpenHandle(name string) (string, gopty.Pty, error) {
	exePath, err := ptyResolveExe(name)
	if err != nil {
		return "", nil, ptyWrapCLIError(err)
	}
	ptmx, err := ptyNew()
	if err != nil {
		return "", nil, ptyWrapCLIError(err)
	}
	return exePath, ptmx, nil
}

func ptySpawn(part *ptyPart, slot ptySlot, argv []string, folder string, onDone func()) (*PTY, error) {
	mcpPath := ptyMCPConfigPath(argv)
	exePath, ptmx, err := ptyOpenHandle(argv[0])
	if err != nil {
		os.Remove(mcpPath)
		return nil, err
	}
	cmd := ptmx.Command(exePath, argv[1:]...)
	cmd.Dir = folder
	if err := cmd.Start(); err != nil {
		ptmx.Close()
		os.Remove(mcpPath)
		return nil, ptyWrapCLIError(err)
	}
	handle := ptyWrap(ptmx)
	return ptyActivate(part, slot, handle, cmd, mcpPath, onDone), nil
}

func ptyActivate(part *ptyPart, slot ptySlot, handle ptyHandle, cmd *gopty.Cmd, mcpPath string, onDone func()) *PTY {
	pid := cmd.Process.Pid
	killTree := func() { ptyKillTree(pid) }
	p := &PTY{
		part:     part,
		slot:     slot,
		handle:   handle,
		mcpPath:  mcpPath,
		kill:     killTree,
		onDone:   onDone,
		closing:  make(chan struct{}),
		procDone: make(chan struct{}),
		outDone:  make(chan struct{}),
		done:     make(chan struct{}),
	}
	part.activate(slot, p)
	p.applyStoredSize()
	go p.waitLoop(cmd)
	return p
}

// Resize sets the terminal dimensions of the pseudo-terminal.
func (p *PTY) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return &Error{Code: "bad_request", Message: "cols and rows must be at least 1"}
	}
	p.part.noteSize(p.slot, cols, rows)
	return p.applyStoredSize()
}

func (p *PTY) applyStoredSize() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	sz, ok := p.part.sizeOf(p.slot)
	if !ok {
		return nil
	}
	return p.handle.Resize(sz.cols, sz.rows)
}

// Close ends the interactive CLI by closing its pseudo-terminal, waiting up to
// 10 s for exit, and killing its process tree if it is still running
// (flows/takeover.md step 13, flows/agent-turn.md G).
func (p *PTY) Close() error {
	p.triggerStop()
	<-p.done
	return nil
}

// ClosePTY closes the open interactive pseudo-terminal of (group, member), if
// one is open, and reports whether one was found.
func (h *Host) ClosePTY(group, member string) bool {
	p := ptyState(h).lookup(ptySlot{group: group, member: member})
	if p == nil {
		return false
	}
	p.Close()
	return true
}

// ServeTerminal bridges t, a "pty:<g>:<member>" stream, to the open
// interactive pseudo-terminal of (g, member) (contracts/datachannel.md
// sections 1 and 7). An unrecognised, unmatched or duplicate stream is closed
// and drained so it never holds back the connection's receive window.
func (h *Host) ServeTerminal(t Terminal) {
	group, member, ok := ptyParseLabel(t.Label())
	if !ok {
		ptyRejectTerminal(t)
		return
	}
	p := ptyState(h).lookup(ptySlot{group: group, member: member})
	if p == nil || !p.attach(t) {
		ptyRejectTerminal(t)
		return
	}
	go p.pumpOut(t)
	io.Copy(ptyWriterOnly{p.handle}, ptyReaderOnly{t})
	p.Close()
	io.Copy(io.Discard, ptyReaderOnly{t})
}

func (p *PTY) attach(t Terminal) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readerTaken {
		return false
	}
	p.readerTaken = true
	p.term = t
	return true
}

func (p *PTY) hasTerminal() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term != nil
}

func (p *PTY) pumpOut(t Terminal) {
	defer close(p.outDone)
	io.Copy(ptyWriterOnly{t}, ptyReaderOnly{p.handle})
	go p.triggerStop()
	io.Copy(io.Discard, ptyReaderOnly{p.handle})
}

func (p *PTY) drainUnattached() {
	defer close(p.outDone)
	io.Copy(io.Discard, ptyReaderOnly{p.handle})
}

// waitLoop is the single caller of cmd.Wait and the single reader of p.closing.
func (p *PTY) waitLoop(cmd *gopty.Cmd) {
	cmd.Wait()
	os.Remove(p.mcpPath)
	close(p.procDone)
	ptyBeforeExitStop(p)
	p.triggerStop()
}

func (p *PTY) beginStop() (Terminal, bool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopping {
		return nil, false, false
	}
	p.stopping = true
	p.closed = true
	needDrain := !p.readerTaken
	p.readerTaken = true
	return p.term, needDrain, true
}

func (p *PTY) triggerStop() {
	term, needDrain, first := p.beginStop()
	if !first {
		return
	}
	p.runStop(term, needDrain)
}

// runStop is the single reader of p.outDone and p.procDone.
func (p *PTY) runStop(term Terminal, needDrain bool) {
	close(p.closing)
	if term != nil {
		term.Close()
	}
	if needDrain {
		go p.drainUnattached()
	}
	p.handle.Close()
	<-p.outDone
	if !ptyWaitSignal(p.procDone, ptyStopTimeout) {
		p.kill()
		<-p.procDone
	}
	p.part.release(p.slot)
	close(p.done)
	if p.onDone != nil {
		p.onDone()
	}
}

func ptyParseLabel(label string) (group, member string, ok bool) {
	rest, hasPrefix := strings.CutPrefix(label, "pty:")
	if !hasPrefix {
		return "", "", false
	}
	group, member, _ = strings.Cut(rest, ":")
	if group == "" || member == "" || strings.Contains(member, ":") {
		return "", "", false
	}
	return group, member, true
}

func ptyRejectTerminal(t Terminal) {
	t.Close()
	io.Copy(io.Discard, ptyReaderOnly{t})
}

func ptyResize(_ context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	var req proto.PTYResize
	if err := proto.Decode(data, &req); err != nil {
		return nil, &Error{Code: "bad_request", Message: "invalid pty.resize frame"}
	}
	if req.Type != "pty.resize" || req.Cols < 1 || req.Rows < 1 {
		return nil, &Error{Code: "bad_request", Message: "invalid pty.resize frame"}
	}
	part := ptyState(h)
	slot := ptySlot{group: string(req.Group), member: string(req.AI)}
	if p := part.noteSize(slot, req.Cols, req.Rows); p != nil {
		p.applyStoredSize()
	}
	return nil, nil
}

func ptyWaitSignal(sig <-chan struct{}, d time.Duration) bool {
	ptyTimer := time.NewTimer(d)
	defer ptyTimer.Stop()
	select {
	case <-sig:
		return true
	case <-ptyTimer.C:
		return false
	}
}
