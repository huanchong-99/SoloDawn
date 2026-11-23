//go:build !windows

package host

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func checkShell(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		group, _ := os.FindProcess(-cmd.Process.Pid) // FindProcess always succeeds on Unix.
		return group.Kill()
	}
	return cmd
}

// killCheckTree returns what kills a started check with every process it
// left behind: the Cancel that checkShell sets kills the process group.
func killCheckTree(cmd *exec.Cmd) func() {
	return func() { cmd.Cancel() }
}
