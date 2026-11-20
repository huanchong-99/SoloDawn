//go:build windows

package host

import (
	"os/exec"
	"strconv"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW of the process creation flags.
const createNoWindow = 0x08000000

// detachTerminal starts cmd without a console window, so nothing can prompt
// on one. A stop kills the whole process tree, ssh included.
func detachTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	cmd.Cancel = func() error {
		return exec.Command("taskkill.exe", "/f", "/t", "/pid", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
