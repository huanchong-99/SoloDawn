//go:build !windows

package node

import (
	"os"
	"syscall"
)

const connRefused = syscall.ECONNREFUSED

// privateDir creates dir, mode 0700, if it does not exist.
func privateDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

func ownerOnly(path string) error {
	return os.Chmod(path, 0o600)
}
