package host

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
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

// ptyPart keeps the open interactive pseudo-terminals of one Host, stored via Part[ptyPart](h).
//
// Lock order: PTY.mu -> ptyPart.mu. Neither lock waits on I/O, process exit
// or callbacks.
type ptyPart struct {
	watch    *watchPart
	mu       sync.Mutex
	reserved map[aiSlot]bool
	open     map[aiSlot]*PTY
}

func ptyState(h *Host) *ptyPart {
	part := Part[ptyPart](h)
	part.mu.Lock()
	if part.open == nil {
		part.watch = watchState(h)
		part.reserved = map[aiSlot]bool{}
		part.open = map[aiSlot]*PTY{}
	}
	part.mu.Unlock()
	return part
}

func (part *ptyPart) reserve(slot aiSlot) bool {
	part.mu.Lock()
	defer part.mu.Unlock()
	if part.reserved[slot] {
		return false
	}
	part.reserved[slot] = true
	return true
}

func (part *ptyPart) release(slot aiSlot) {
	part.mu.Lock()
	defer part.mu.Unlock()
	delete(part.reserved, slot)
	delete(part.open, slot)
}

func (part *ptyPart) activate(slot aiSlot, p *PTY) {
	part.mu.Lock()
	defer part.mu.Unlock()
	part.open[slot] = p
}

func (part *ptyPart) lookup(slot aiSlot) *PTY {
	part.mu.Lock()
	defer part.mu.Unlock()
	return part.open[slot]
}

// PTY is one running interactive CLI attached to a pseudo-terminal
// (contracts/drivers.md section 1.3, flows/takeover.md steps 6 to 13).
//
// Lock order: PTY.mu -> ptyPart.mu. Neither lock waits on I/O, process exit
// or callbacks.
type PTY struct {
	part   *ptyPart
	slot   aiSlot
	handle ptyHandle
	kill   func()
	onDone func()

	mu          sync.Mutex
	term        Terminal
	readerTaken bool
	closed      bool
	watchSize   proto.PTYResize

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
// the PTY ends for any reason, onDone (if non-nil) runs once after the process
// has exited.
func (h *Host) StartPTY(group, member, agent string, launch Launch, onDone func()) (*PTY, string, error) {
	part := ptyState(h)
	slot := aiSlot{group: group, member: member}
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
	return argv, session, nil
}

func ptyWrapCLIError(err error) error {
	if _, ok := errors.AsType[*Error](err); ok {
		return err
	}
	return &Error{Code: "cli", Message: err.Error()}
}

func ptyOpenHandle(name string) (string, gopty.Pty, error) {
	exePath, err := exec.LookPath(name)
	if err != nil {
		return "", nil, ptyWrapCLIError(err)
	}
	ptmx, err := ptyNew()
	if err != nil {
		return "", nil, ptyWrapCLIError(err)
	}
	return exePath, ptmx, nil
}

const ptyDefaultTerm = "xterm-256color"

func ptySpawn(part *ptyPart, slot aiSlot, argv []string, folder string, onDone func()) (*PTY, error) {
	exePath, ptmx, err := ptyOpenHandle(argv[0])
	if err != nil {
		return nil, err
	}
	cmd := ptmx.Command(exePath, argv[1:]...)
	cmd.Dir = folder
	cmd.Env = ptyEnv()
	if err := cmd.Start(); err != nil {
		ptmx.Close()
		return nil, ptyWrapCLIError(err)
	}
	handle, err := ptyWrap(ptmx)
	if err != nil {
		ptmx.Close()
		ptyKillTree(cmd.Process.Pid)
		cmd.Wait()
		return nil, ptyWrapCLIError(err)
	}
	return ptyActivate(part, slot, handle, cmd, onDone), nil
}

// ptyEnv inherits the node's environment and sets TERM to xterm-256color when
// TERM is unset, empty, or "dumb" (such as under launchd or non-interactive
// shells) so interactive CLIs do not fall back to dumb-terminal prompts.
func ptyEnv() []string {
	env := os.Environ()
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return append(env, "TERM="+ptyDefaultTerm)
	}
	return env
}

func ptyActivate(part *ptyPart, slot aiSlot, handle ptyHandle, cmd *gopty.Cmd, onDone func()) *PTY {
	pid := cmd.Process.Pid
	killTree := func() { ptyKillTree(pid) }
	p := &PTY{
		part:     part,
		slot:     slot,
		handle:   handle,
		kill:     killTree,
		onDone:   onDone,
		closing:  make(chan struct{}),
		procDone: make(chan struct{}),
		outDone:  make(chan struct{}),
		done:     make(chan struct{}),
	}
	part.activate(slot, p)
	go p.waitLoop(cmd)
	return p
}

// Attached reports whether a Terminal stream has attached to p.
func (p *PTY) Attached() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term != nil
}

func (p *PTY) resize(req proto.PTYResize) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	if err := p.handle.Resize(req.Cols, req.Rows); err != nil {
		p.mu.Unlock()
		return ptyWrapCLIError(err)
	}
	p.watchSize = req
	p.mu.Unlock()
	p.part.watch.resize(p.slot, req.Cols, req.Rows)
	return nil
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
	p := ptyState(h).lookup(aiSlot{group: group, member: member})
	if p == nil {
		return false
	}
	p.Close()
	return true
}

// ServeTerminal bridges t, a "pty:<g>:<member>" stream, to the open
// interactive pseudo-terminal of (g, member) (contracts/datachannel.md
// sections 1 and 7). An unrecognised, unmatched or duplicate stream is closed.
func (h *Host) ServeTerminal(t Terminal) {
	group, member, ok := ptyParseLabel(t.Label())
	if !ok {
		t.Close()
		return
	}
	p := ptyState(h).lookup(aiSlot{group: group, member: member})
	if p == nil || !p.attach(t) {
		t.Close()
		return
	}
	go p.pumpOut(watchFanout{t, p.part.watch, p.slot})
	io.Copy(p.handle, t)
	p.Close()
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

func (p *PTY) pumpOut(t io.Writer) {
	defer close(p.outDone)
	io.Copy(t, p.handle)
	go p.triggerStop()
	io.Copy(io.Discard, p.handle)
}

func (p *PTY) drainUnattached() {
	defer close(p.outDone)
	io.Copy(io.Discard, p.handle)
}

// waitLoop is the single caller of cmd.Wait and the single reader of p.closing.
func (p *PTY) waitLoop(cmd *gopty.Cmd) {
	cmd.Wait()
	close(p.procDone)
	ptyBeforeExitStop(p)
	p.triggerStop()
}

func (p *PTY) beginStop() (Terminal, bool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, false, false
	}
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
	p.part.watch.stop(p.slot)
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

func ptyParseLabel(label string) (string, string, bool) {
	rest, hasPrefix := strings.CutPrefix(label, "pty:")
	if !hasPrefix {
		return "", "", false
	}
	group, member, _ := strings.Cut(rest, ":")
	if group == "" || member == "" || strings.Contains(member, ":") {
		return "", "", false
	}
	return group, member, true
}

func ptyResize(_ context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	var req proto.PTYResize
	if err := proto.Decode(data, &req); err != nil {
		return nil, &Error{Code: "bad_request", Message: "invalid pty.resize frame"}
	}
	if req.Type != "pty.resize" || req.Cols < 1 || req.Rows < 1 {
		return nil, &Error{Code: "bad_request", Message: "invalid pty.resize frame"}
	}
	slot := aiSlot{group: string(req.Group), member: string(req.AI)}
	p := ptyState(h).lookup(slot)
	if p == nil {
		return nil, nil
	}
	return nil, p.resize(req)
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
