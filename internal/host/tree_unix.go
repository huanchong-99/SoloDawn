//go:build !windows

package host

import "os/exec"

// treeKiller returns what kills a started CLI with every process it left
// behind: the Cancel that detachTerminal sets kills its session.
func treeKiller(cmd *exec.Cmd) func() {
	return func() { cmd.Cancel() }
}
