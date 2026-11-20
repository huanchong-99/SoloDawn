//go:build !windows

package node

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"syscall"
)

// currentUID is the user that must own the data directory.
var currentUID = os.Getuid

// privateDir creates dir, mode 0700, if it does not exist. An existing one
// must be a directory owned by the current user; its mode is set to 0700.
func privateDir(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if int(info.Sys().(*syscall.Stat_t).Uid) != currentUID() {
		return fmt.Errorf("%s is not owned by the current user", dir)
	}
	return os.Chmod(dir, 0o700)
}

// lockHeld is flock's answer when another process holds the lock (EAGAIN).
const lockHeld = syscall.EWOULDBLOCK

func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// listenPrivate binds under umask 0077, so the socket is never open to
// others, not even before ownerOnly. The umask is the process's: 0077 keeps
// the owner's bits for whatever another goroutine creates meanwhile.
func listenPrivate(path string) (*net.UnixListener, error) {
	defer syscall.Umask(syscall.Umask(0o077))
	return net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
}

func ownerOnly(path string) error {
	return os.Chmod(path, 0o600)
}
