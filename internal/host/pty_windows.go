//go:build windows

package host

import (
	"errors"
	"sync"

	gopty "github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/windows"
)

// ptyWinHandle closes the pseudo console apart from its pipes, so the output
// conhost writes while it closes can still be read to the end. go-pty's own
// Close closes the pseudo console and both pipes in one call.
//
// Lock order: PTY.mu -> ptyWinHandle.mu. ClosePseudoConsole runs under mu and
// can wait until the output pipe has been read.
type ptyWinHandle struct {
	gopty.ConPty
	mu     sync.Mutex
	closed bool // the pseudo console has been closed
}

func ptyWrapStarted(ptmx gopty.Pty) (ptyHandle, error) {
	return &ptyWinHandle{ConPty: ptmx.(gopty.ConPty)}, nil
}

func (w *ptyWinHandle) closeConsole() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		windows.ClosePseudoConsole(windows.Handle(w.Fd()))
	}
}

func (w *ptyWinHandle) Resize(cols, rows int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	return w.ConPty.Resize(cols, rows)
}

func (w *ptyWinHandle) Close() error {
	w.closeConsole()
	return errors.Join(w.InputPipe().Close(), w.OutputPipe().Close())
}

// ptyBeforeExitStop closes the pseudo console of an attached PTY whose process
// has exited. conhost then writes its last frame and closes the output pipe,
// and pumpOut forwards that output and starts the stop at the end of it.
func ptyBeforeExitStop(p *PTY) {
	if p.Attached() {
		ptyCloseConsoleAndWait(p)
	}
}

// ptyCloseConsoleAndWait leaves handles that are not a ConPTY, the test
// doubles, alone. The close runs in its own goroutine because it can wait for
// the output to be read, and the wait is bounded by ptyStopTimeout.
func ptyCloseConsoleAndWait(p *PTY) {
	if w, ok := p.handle.(*ptyWinHandle); ok {
		go w.closeConsole()
		ptyWaitSignal(p.closing, ptyStopTimeout)
	}
}

func ptyKillTree(pid int) {
	taskkill(pid)
}
