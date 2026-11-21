# Run in elevated Windows PowerShell. The temporary account is not an administrator.
$ErrorActionPreference = 'Stop'
$id = [guid]::NewGuid().ToString('N').Substring(0,12)
$name = "s5-$id"
$dir = Join-Path $env:ProgramData $name
$password = ConvertTo-SecureString ([guid]::NewGuid().ToString('N') + 'aA1!') -AsPlainText -Force
$user = New-LocalUser -Name $name -Password $password
try {
    $owner = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = [Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true, $false)
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
        $owner, 'FullControl', 'ContainerInherit,ObjectInherit', 'None', 'Allow'))
    # The client can traverse the directory, but cannot replace its contents.
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
        $user.SID, 'ReadAndExecute', 'Allow'))
    [void][IO.Directory]::CreateDirectory($dir, $acl)
    $exe = Join-Path $dir 'probe.exe'
    go -C $PSScriptRoot build -o $exe .
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    $acl = Get-Acl $exe
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
        $user.SID, 'ReadAndExecute', 'Allow'))
    Set-Acl $exe $acl

    $log = Join-Path $dir 'server.log'
    $errLog = Join-Path $dir 'server.err'
    $server = Start-Process $exe -ArgumentList 'serve',$user.SID.Value -WorkingDirectory $dir -PassThru -RedirectStandardOutput $log -RedirectStandardError $errLog
    $null = $server.Handle
    $timer = [Diagnostics.Stopwatch]::StartNew()
    while ((Get-Content $log) -notcontains 'READY') {
        if ($server.HasExited -or $timer.Elapsed.TotalSeconds -gt 15) { throw 'server not ready' }
        Start-Sleep -Milliseconds 100
    }
    $cred = [pscredential]::new("$env:COMPUTERNAME\$name", $password)
    $client = Start-Process $exe -ArgumentList 'client',$user.SID.Value -Credential $cred -WorkingDirectory $dir -PassThru -Wait
    if (-not $server.WaitForExit(15000)) { throw 'server timed out' }
    Get-Content $log
    Write-Output "exit: server=$($server.ExitCode) client=$($client.ExitCode)"
    if ($client.ExitCode -ne 0 -or $server.ExitCode -ne 0) { throw 'socket access check failed' }
} finally {
    foreach ($proc in @($client, $server)) {
        if ($proc -and -not $proc.HasExited) { $proc.Kill(); $proc.WaitForExit() }
    }
    try {
        if ($errLog -and (Test-Path $errLog)) { Get-Content $errLog }
        if (Test-Path $dir) { Remove-Item -LiteralPath $dir -Recurse -Force }
    } finally {
        Remove-LocalUser -SID $user.SID
        $password.Dispose()
    }
}
