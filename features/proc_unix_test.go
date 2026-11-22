//go:build acceptance && !windows

package features

import (
	"os"
	"os/exec"
)

func attachManagedProc(cmd *exec.Cmd) (func(os.Signal) error, func()) {
	sigFn := func(sig os.Signal) error {
		return cmd.Process.Signal(sig)
	}
	killFn := func() {
		_ = cmd.Process.Kill()
	}
	return sigFn, killFn
}
