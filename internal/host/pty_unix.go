//go:build !windows

package host

import (
	"os"
	"syscall"

	gopty "github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/unix"
)

type ptyUnixHandle struct {
	*os.File
}

func (u ptyUnixHandle) Resize(cols, rows int) error {
	var err error
	raw, _ := u.SyscallConn()
	setSize := func(fd uintptr) {
		err = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{
			Row: uint16(rows),
			Col: uint16(cols),
		})
	}
	raw.Control(setSize)
	return err
}

// ptyWrapStarted duplicates the Unix master descriptor in non-blocking mode so
// Go's netpoller owns it on macOS as well as Linux, then closes go-pty's
// blocking master and open slave handles so Read unblocks on Close and reaches
// EOF when the child exits.
func ptyWrapStarted(ptmx gopty.Pty) ptyHandle {
	up := ptmx.(gopty.UnixPty)
	var dupFD int
	dup := func(fd uintptr) {
		dupFD, _ = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
	}
	up.Control(dup)
	syscall.SetNonblock(dupFD, true)
	master := os.NewFile(uintptr(dupFD), up.Master().Name())
	ptmx.Close()
	return ptyUnixHandle{File: master}
}

func ptyBeforeExitStop(p *PTY) {
	if p.hasTerminal() {
		ptyWaitSignal(p.closing, ptyStopTimeout)
	}
}

func ptyKillTree(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL)
}
