//go:build !windows

package host

import (
	"os/exec"
	"syscall"
)

// detachTerminal starts cmd in a new session, which has no controlling
// terminal, so nothing it starts can prompt on /dev/tty. A stop kills the
// whole session.
func detachTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
