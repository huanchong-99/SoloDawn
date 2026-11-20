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

// InstallService registers the absolute executable path for the next user login.
func InstallService(executable string) error {
	return scheduledTask(`$action = New-ScheduledTaskAction -Execute $env:SOLODAWN_TASK_EXECUTABLE -Argument 'node'
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
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
