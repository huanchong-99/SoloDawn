//go:build darwin

package node

import (
	"os"
	"path/filepath"
)

// startBackground starts the node through launchd when exe (following symbolic
// links) lives inside a .app bundle so macOS attributes permission prompts to
// the app, and starts the unresolved exe detached otherwise
// (contracts/local.md section 11). Launch preserves any existing agent plist so
// Finder openings keep the PATH written from a shell; pair refreshes it.
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

func agentPlist(resolved string, rewrite bool) (string, error) {
	if !rewrite {
		if plist, err := agentPath(); err == nil {
			if _, err := os.Stat(plist); err == nil {
				return plist, nil
			}
		}
	}
	return writeAgentPlist(resolved, os.Getenv("PATH"))
}
