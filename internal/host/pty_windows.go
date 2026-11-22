//go:build windows

package host

import (
	"os/exec"
	"strconv"

	gopty "github.com/aymanbagabas/go-pty"
)

func ptyWrapStarted(ptmx gopty.Pty) ptyHandle {
	return ptmx
}

func ptyBeforeExitStop(_ *PTY) {}

func ptyKillTree(pid int) {
	exec.Command("taskkill.exe", "/f", "/t", "/pid", strconv.Itoa(pid)).Run()
}
