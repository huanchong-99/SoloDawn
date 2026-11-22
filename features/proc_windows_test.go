//go:build acceptance && windows

package features

import (
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareManagedCmd(_ *exec.Cmd) {}

func attachManagedGroup(cmd *exec.Cmd) (func(os.Signal) error, func()) {
	job, err := openKillOnCloseJob(uint32(cmd.Process.Pid))
	sigFn := func(sig os.Signal) error {
		return cmd.Process.Signal(sig)
	}
	killFn := func() {
		_ = cmd.Process.Kill()
		if err == nil {
			_ = windows.CloseHandle(job)
		}
	}
	return sigFn, killFn
}

func openKillOnCloseJob(pid uint32) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	if err := configureKillJob(job, pid); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func configureKillJob(job windows.Handle, pid uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(proc)
	return windows.AssignProcessToJobObject(job, proc)
}
