//go:build windows

package node

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// addrInUse is the error of a listen on a port that is held; Windows does not
// answer syscall.EADDRINUSE.
const addrInUse = windows.WSAEADDRINUSE

func startDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	return nil
}
