//go:build !windows

package host

import "os/exec"

// killCheckTree returns what kills a started check with every process it
// left behind: the Cancel that checkShell sets kills the process group.
func killCheckTree(cmd *exec.Cmd) func() {
	return func() { cmd.Cancel() }
}
