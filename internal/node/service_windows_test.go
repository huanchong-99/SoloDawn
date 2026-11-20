package node

import "testing"

func TestTaskExecutableIsData(t *testing.T) {
	const executable = `C:\Program Files\test $('x')\solodawn.exe`
	serviceOK(t, scheduledTask(`if ($env:SOLODAWN_TASK_EXECUTABLE -cne 'C:\Program Files\test $(''x'')\solodawn.exe') { exit 7 }`, executable))
}

func TestTaskFailure(t *testing.T) {
	serviceError(t, scheduledTask(`Write-Output 'task failed'; exit 7`, ""), "scheduled task: exit status 7 (task failed)")
}
