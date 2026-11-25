//go:build windows

package host

import (
	"strings"
	"testing"
	"time"
)

func TestPTYWindowsResizeAfterConsoleCloses(t *testing.T) {
	w := ptyWinHandle{closed: true}
	if err := w.Resize(90, 25); err != nil {
		t.Fatalf("resize closed console: %v", err)
	}
}

// TestPTYWindowsSelfExitDeliversLastOutput starts a CLI in a ConPTY with a
// terminal attached, makes it print BYE-NOW and exit at once, and expects
// BYE-NOW in the terminal before the terminal closes. ptyStopTimeout is an
// hour, so the stop can only come from the end of the output.
func TestPTYWindowsSelfExitDeliversLastOutput(t *testing.T) {
	saved := ptyStopTimeout
	ptyStopTimeout = time.Hour
	t.Cleanup(func() { ptyStopTimeout = saved })
	h := newPTYTestHost(t)
	registerPTYTestDriver(t, "ptywinexit", func(Launch) ([]string, string, error) {
		return ptyHelperArgv(t, "pty-echo"), "sess-win-exit", nil
	})
	done := make(chan struct{})
	onDone := func() { close(done) }
	if _, _, err := h.StartPTY("g1", "m1", "ptywinexit", Launch{Folder: t.TempDir(), Level: 1}, onDone); err != nil {
		t.Fatal(err)
	}
	term := newPTYTestTerminal("pty:g1:m1")
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		h.ServeTerminal(term)
	}()
	term.waitContains(t, "WD:")
	if _, err := term.inW.Write([]byte("quit\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		term.Close()
		<-serveDone
		t.Fatal(timedOut(30*time.Second, "the PTY to stop at the end of its output", []string{term.output()}))
	}
	// ServeTerminal returns only once the terminal has been closed, and the
	// test terminal refuses writes after Close.
	<-serveDone
	if !strings.Contains(term.output(), "BYE-NOW") {
		t.Fatalf("the last output was lost; terminal got %q", term.output())
	}
}
