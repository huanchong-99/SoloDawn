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
	serviceError(t, InstallService(`C:\SoloDawn.exe`, ""), missing)
	serviceError(t, RemoveService(), missing)
}

// The action is built, not registered: the task runs conhost.exe, which
// hosts the node without a console window.
func TestTaskActionIsHeadless(t *testing.T) {
	const executable = `C:\Program Files\SoloDawn test\solodawn.exe`
	serviceOK(t, scheduledTask(taskAction+`if ($action.Execute -cne 'conhost.exe' -or $action.Arguments -cne '--headless "C:\Program Files\SoloDawn test\solodawn.exe" node') { Write-Output ($action.Execute + ' ' + $action.Arguments); exit 7 }`, executable))
}
