//go:build !windows

package node

import (
	"os"
	"syscall"
)

const connRefused = syscall.ECONNREFUSED

func ownerOnly(path string) error {
	return os.Chmod(path, 0o600)
}
