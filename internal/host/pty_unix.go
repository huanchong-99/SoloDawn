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
	raw, err := u.SyscallConn()
	if err != nil {
		return err
	}
	setSize := func(fd uintptr) {
		err = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{
			Row: uint16(rows),
			Col: uint16(cols),
		})
	}
	if controlErr := raw.Control(setSize); controlErr != nil {
		return controlErr
	}
	return err
}

// ptyWrapStarted duplicates the Unix master descriptor in non-blocking mode so
// Go's netpoller owns it on macOS as well as Linux, then closes go-pty's
// blocking master and open slave handles so Read unblocks on Close and reaches
// EOF when the child exits.
func ptyWrapStarted(ptmx gopty.Pty) (ptyHandle, error) {
	up := ptmx.(gopty.UnixPty)
	var master *os.File
	var err error
	dup := func(fd uintptr) {
		master, err = ptyDupMaster(fd, up.Master().Name())
	}
	if controlErr := up.Control(dup); controlErr != nil {
		return nil, controlErr
	}
	if err != nil {
		return nil, err
	}
	ptmx.Close()
	return ptyUnixHandle{File: master}, nil
}

func ptyDupMaster(fd uintptr, name string) (*os.File, error) {
	dup, err := unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return ptyNonblockFile(dup, name)
}

func ptyNonblockFile(fd int, name string) (*os.File, error) {
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func ptyBeforeExitStop(p *PTY) {
	if p.Attached() {
		ptyWaitSignal(p.closing, ptyStopTimeout)
	}
}

func ptyKillTree(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL)
}
