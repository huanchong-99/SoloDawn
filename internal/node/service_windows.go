package node

import (
	"fmt"
	"os/exec"
	"strings"
)

const currentTask = `$ErrorActionPreference = 'Stop'
$user = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$name = "SoloDawn-$user"
`

// taskAction runs the node through a console host without a window: a
// console program started by the task would open one, and closing it would
// stop the node.
const taskAction = `$action = New-ScheduledTaskAction -Execute 'conhost.exe' -Argument ('--headless "' + $env:SOLODAWN_TASK_EXECUTABLE + '" node')
`

// InstallService registers the absolute executable path for the next user
// login. The task starts with the user's own environment, PATH included, so
// path is not needed here.
func InstallService(executable, _ string) error {
	return scheduledTask(taskAction+`$trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskPath '\' -TaskName $name -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null`, executable)
}

// RemoveService removes start-at-login without stopping a running node.
func RemoveService() error {
	return scheduledTask(`Unregister-ScheduledTask -TaskPath '\' -TaskName $name -Confirm:$false`, "")
}

func scheduledTask(script, executable string) error {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", currentTask+script)
	cmd.Env = append(cmd.Environ(), "SOLODAWN_TASK_EXECUTABLE="+executable)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scheduled task: %w (%s)", err, strings.Join(strings.Fields(string(out)), " "))
	}
	return nil
}
