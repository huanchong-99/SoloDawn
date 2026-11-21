//go:build windows

package node

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

func startDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	return nil
}

func replaceProcess(binary string, args, env []string) error {
	attr := &os.ProcAttr{
		Env:   env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   &syscall.SysProcAttr{CreationFlags: createNoWindow | windows.CREATE_NEW_PROCESS_GROUP},
	}
	proc, err := os.StartProcess(binary, args, attr)
	if err != nil {
		return err
	}
	proc.Release()
	return nil
}
