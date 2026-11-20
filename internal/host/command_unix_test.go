//go:build !windows

package host

import (
	"os"
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
