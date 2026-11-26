//go:build !windows

package node

import (
	"os/exec"
	"syscall"
)

// addrInUse is the error of a listen on a port that is held.
const addrInUse = syscall.EADDRINUSE

func startDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	return nil
}
