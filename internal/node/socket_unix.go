//go:build !windows

package node

import "os"

func ownerOnly(path string) error {
	return os.Chmod(path, 0o600)
}
