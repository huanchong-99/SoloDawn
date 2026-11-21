//go:build !windows

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandsNeverWaitForAPerson(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_SSH_COMMAND", "owner-ssh")
	dir := t.TempDir()
	script := `printf '%s|%s|%s|%s|%s|%s|' "$GIT_TERMINAL_PROMPT" "$GH_PROMPT_DISABLED" "$SSH_ASKPASS_REQUIRE" "$GCM_INTERACTIVE" "$GIT_SSH_COMMAND" "$PWD"; ` +
		`[ "$(ps -o pgid= -p $$ | tr -d ' ')" = "$$" ] && echo leader`
	out, stderr, err := runIn(t.Context(), dir, "sh", "-c", script)
	workOK(t, err)
	workEqual(t, stderr, "")
	// A new session: the command leads its own process group and has no
	// controlling terminal. The owner's ssh command is left alone.
	workEqual(t, out, "0|1|never|never|owner-ssh|"+dir+"|leader\n")
}

// fakeGit puts a git first on PATH that runs script.
func fakeGit(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	workOK(t, os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"+script+"\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A command that hangs is stopped at its limit, with every process it
// started: the sleep here would hold the output open until WaitDelay.
func TestCommandStopsAtItsLimit(t *testing.T) {
	fakeGit(t, "sleep 30; echo late")
	saved := commandUnit
	commandUnit = 30 * time.Millisecond
	t.Cleanup(func() { commandUnit = saved })
	for _, c := range []struct {
		args  []string
		limit time.Duration
	}{
		{[]string{"fetch", "origin"}, 300 * time.Millisecond},
		{[]string{"push", "origin"}, 1800 * time.Millisecond},
	} {
		start := time.Now()
		out, _, err := runIn(t.Context(), t.TempDir(), "git", c.args...)
		elapsed := time.Since(start)
		workEqual(t, err.Error(), "signal: killed")
		workEqual(t, out, "")
		if elapsed < c.limit || elapsed > c.limit+4*time.Second {
			t.Fatalf("git %s stopped after %v, limit %v", strings.Join(c.args, " "), elapsed, c.limit)
		}
	}
}

// The child sees the folder it runs in as PWD; without a folder it keeps the
// node's own PWD. printenv shows the variable as passed, which a shell's own
// $PWD would not.
func TestCommandPWD(t *testing.T) {
	t.Setenv("PWD", "/node/folder")
	dir := t.TempDir()
	out, _, err := runIn(t.Context(), dir, "printenv", "PWD")
	workOK(t, err)
	workEqual(t, out, dir+"\n")
	out, _, err = runIn(t.Context(), "", "printenv", "PWD")
	workOK(t, err)
	workEqual(t, out, "/node/folder\n")
}

// A grandchild that left the command's session survives the kill of the
// session and keeps the output open; the command still returns once the wait
// delay after its limit has passed.
func TestCommandReturnsWithinTheWaitDelay(t *testing.T) {
	savedUnit, savedDelay := commandUnit, commandWaitDelay
	commandUnit, commandWaitDelay = 120*time.Millisecond, 600*time.Millisecond
	t.Cleanup(func() { commandUnit, commandWaitDelay = savedUnit, savedDelay })
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "pid.tmp")
	pidFile := filepath.Join(dir, "pid")
	// perl writes pidFile only after setsid() has detached the grandchild from
	// the command's session, and the limit gives perl 1.2s to start under load.
	script := `perl -MPOSIX=setsid -e 'setsid() != -1 or die; open my $f, ">", $ARGV[0] or die; print $f $$ or die; close $f or die; rename $ARGV[0], $ARGV[1] or die; exec "sleep", "30"' '` + tmpFile + `' '` + pidFile + `' & while [ ! -f '` + pidFile + `' ]; do sleep 0.01; done; sleep 30`
	t.Cleanup(func() {
		pid, err := os.ReadFile(pidFile)
		workOK(t, err)
		workOK(t, exec.Command("kill", strings.TrimSpace(string(pid))).Run())
	})
	start := time.Now()
	_, _, err := runIn(t.Context(), t.TempDir(), "sh", "-c", script)
	elapsed := time.Since(start)
	workEqual(t, err.Error(), "signal: killed")
	if limit := 10*commandUnit + commandWaitDelay; elapsed < limit || elapsed > limit+3*time.Second {
		t.Fatalf("returned after %v, want %v plus a little", elapsed, limit)
	}
}
