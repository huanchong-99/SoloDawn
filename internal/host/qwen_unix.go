//go:build !windows

package host

import (
	"os"
	"syscall"
)

func signalQwenStop(proc *os.Process) {
	_ = proc.Signal(syscall.SIGTERM)
}
