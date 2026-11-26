package host

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// treeKiller returns what kills a started CLI with every process it left
// behind. Once the CLI has exited, taskkill /t no longer finds its children,
// so the CLI goes into a job object that kills all its processes when the
// job is closed. Without a job nothing is killed after Wait: Go has released
// the process by then, and Cancel (taskkill /t) still stops a running CLI.
func treeKiller(cmd *exec.Cmd) func() {
	job, err := newTreeJob(uint32(cmd.Process.Pid), windows.CreateJobObject)
	if err != nil {
		return func() {}
	}
	return func() { windows.CloseHandle(job) }
}

// newTreeJob puts the process in a new job that kills it, and every
// process it starts, when the job is closed. On an error the job is closed.
func newTreeJob(pid uint32, create func(*windows.SecurityAttributes, *uint16) (windows.Handle, error)) (windows.Handle, error) {
	job, err := create(nil, nil)
	if err != nil {
		return 0, err
	}
	return job, closeJobOnError(job, setupTreeJob(job, pid))
}

func setupTreeJob(job windows.Handle, pid uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	return assignTreeJob(job, pid)
}

func assignTreeJob(job windows.Handle, pid uint32) error {
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
