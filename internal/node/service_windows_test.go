package node

import "testing"

func TestTaskExecutableIsData(t *testing.T) {
	const executable = `C:\Program Files\test $('x')\solodawn.exe`
	serviceOK(t, scheduledTask(`if ($env:SOLODAWN_TASK_EXECUTABLE -cne 'C:\Program Files\test $(''x'')\solodawn.exe') { exit 7 }`, executable))
}

func TestTaskFailure(t *testing.T) {
	serviceError(t, scheduledTask(`Write-Output 'task failed'; exit 7`, ""), "scheduled task: exit status 7 (task failed)")
}

func TestServiceNeedsTaskSchedulerClient(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	const missing = `scheduled task: exec: "powershell.exe": executable file not found in %PATH% ()`
	serviceError(t, InstallService(`C:\SoloDawn.exe`), missing)
	serviceError(t, RemoveService(), missing)
}
