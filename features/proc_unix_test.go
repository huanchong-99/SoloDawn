//go:build acceptance && !windows

package features

import (
	"os"
	"os/exec"
	"syscall"
)

func prepareManagedCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachManagedGroup(cmd *exec.Cmd) (func(os.Signal) error, func()) {
	pid := cmd.Process.Pid
	sigFn := func(sig os.Signal) error {
		if sig == os.Interrupt {
			return syscall.Kill(-pid, syscall.SIGINT)
		}
		return syscall.Kill(-pid, syscall.SIGKILL)
	}
	killFn := func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	return sigFn, killFn
}
