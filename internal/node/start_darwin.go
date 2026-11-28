//go:build darwin

package node

import (
	"cmp"
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var shellPathWait = 5 * time.Second

// startBackground starts the node through launchd when exe (following symbolic
// links) lives inside a .app bundle so macOS attributes permission prompts to
// the app, and starts the unresolved exe detached otherwise
// (contracts/local.md section 11).
func startBackground(exe string, rewrite bool) error {
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil || bundleOf(resolved) == "" {
		return detachNode(exe, "node")
	}
	plist, err := agentPlist(resolved, rewrite)
	if err != nil {
		return err
	}
	return startAgent(plist)
}

// agentPlist returns the agent's plist. pair writes it with the PATH of the
// shell that runs it. Launch keeps one that exists, and writes a missing one
// with the PATH of the user's shell: an app opened from the Finder has only
// the system's directories, where the node would find no CLI.
func agentPlist(resolved string, rewrite bool) (string, error) {
	if rewrite {
		return writeAgentPlist(resolved, os.Getenv("PATH"))
	}
	plist, err := agentPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(plist); err == nil {
		return plist, nil
	}
	return writeAgentPlist(resolved, userShellPath())
}

// userShellPath asks the user's shell, started as a terminal starts it, for its
// PATH. The empty echo ends a line that the shell's start files left open. A
// shell that fails, or whose start files take longer than shellPathWait,
// leaves the PATH of this process, and one log line says so.
func userShellPath() string {
	shell := os.Getenv("SHELL")
	ctx, cancel := context.WithTimeout(context.Background(), shellPathWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ilc", "echo; printenv PATH")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		log.Printf("node: read PATH from %s: %v; the login agent gets the PATH of the app", shell, cmp.Or(ctx.Err(), err))
		return os.Getenv("PATH")
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}
