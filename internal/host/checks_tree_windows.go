package host

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// killCheckTree returns what kills a started check with every process it
// left behind. Once cmd.exe has exited, taskkill /t no longer finds its
// children, so the shell goes into a job object that kills all its
// processes when the job is closed. Without a job, Cancel (taskkill /t) is
// the fallback.
func killCheckTree(cmd *exec.Cmd) func() {
	job, err := newCheckJob(uint32(cmd.Process.Pid))
	if err != nil {
		return func() { cmd.Cancel() }
	}
	return func() { windows.CloseHandle(job) }
}

// newCheckJob puts the process in a new job that kills it, and every
// process it starts, when the job is closed. On an error the job is closed.
func newCheckJob(pid uint32) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	return job, closeJobOnError(job, setupCheckJob(job, pid))
}

func setupCheckJob(job windows.Handle, pid uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	return assignCheckJob(job, pid)
}

func assignCheckJob(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
}

func closeJobOnError(job windows.Handle, err error) error {
	if err != nil {
		windows.CloseHandle(job)
	}
	return err
}
