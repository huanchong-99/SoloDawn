//go:build !windows

package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fake taskkill written as a shell script needs a POSIX system.
func TestTaskkillArgs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	t.Setenv("TASKKILL_ARGS", out)
	if err := os.WriteFile(filepath.Join(dir, "taskkill.exe"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TASKKILL_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := taskkill(12345); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(out)
	if err != nil || string(args) != "/f\n/t\n/pid\n12345\n" {
		t.Fatalf("taskkill arguments: %q (%v)", args, err)
	}
}

// A command started with detachTerminal leads a session of its own, which has
// no controlling terminal: nothing it starts can wait for a person at
// /dev/tty.
func TestCommandsNeverWaitForAPerson(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `[ "$(ps -o pgid= -p $$ | tr -d ' ')" = "$$" ] && echo leader; (: </dev/tty) 2>/dev/null || echo no terminal`)
	detachTerminal(cmd)
	if out, err := cmd.Output(); err != nil || string(out) != "leader\nno terminal\n" {
		t.Fatalf("output %q (%v), want a session leader without a terminal", out, err)
	}
}

// A grandchild that left the command's session survives the kill of the
// session and keeps the output open; the command still returns once the wait
// delay after its stop has passed.
func TestCommandReturnsWithinTheWaitDelay(t *testing.T) {
	saved := commandWaitDelay
	commandWaitDelay = 600 * time.Millisecond
	t.Cleanup(func() { commandWaitDelay = saved })
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "pid.tmp")
	pidFile := filepath.Join(dir, "pid")
	// perl writes pidFile only after setsid() has detached the grandchild from
	// the command's session, and the limit gives perl 1.2s to start under load.
	script := `perl -MPOSIX=setsid -e 'setsid() != -1 or die; open my $f, ">", $ARGV[0] or die; print $f $$ or die; close $f or die; rename $ARGV[0], $ARGV[1] or die; exec "sleep", "30"' '` + tmpFile + `' '` + pidFile + `' & while [ ! -f '` + pidFile + `' ]; do sleep 0.01; done; sleep 30`
	t.Cleanup(func() {
		pid, _ := os.ReadFile(pidFile)
		if err := exec.Command("kill", strings.TrimSpace(string(pid))).Run(); err != nil {
			t.Errorf("kill the grandchild %q: %v", pid, err)
		}
	})
	const limit = 1200 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	detachTerminal(cmd)
	cmd.WaitDelay = commandWaitDelay
	start := time.Now()
	_, err := cmd.Output()
	elapsed := time.Since(start)
	if want := limit + commandWaitDelay; err == nil || err.Error() != "signal: killed" || elapsed < want || elapsed > want+3*time.Second {
		t.Fatalf("returned %v after %v, want killed after %v plus a little", err, elapsed, want)
	}
}
