# ============================================================================
# GitCortex Post-Installation Self-Check Script (Lightweight)
# Checks installation files, .env config, system dependencies, and AI CLIs.
# Usage: powershell -ExecutionPolicy Bypass -File post-install-check.ps1
# ============================================================================
param(
    [string]$InstallDir,
    [switch]$SkipServerTest,
    [switch]$Verbose
)

$ErrorActionPreference = "Continue"

# --- Auto-detect install directory ---
if (-not $InstallDir) {
    $RegPath = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{7B8C4D2E-3F1A-4E5B-9C6D-8A7B3E2F1D4C}_is1"
    $RegVal = (Get-ItemProperty -Path $RegPath -Name "InstallLocation" -ErrorAction SilentlyContinue).InstallLocation
    if ($RegVal -and (Test-Path $RegVal)) {
        $InstallDir = $RegVal.TrimEnd('\')
    }
    elseif ($MyInvocation.MyCommand.Path) {
        $ScriptsDir = Split-Path -Parent $MyInvocation.MyCommand.Path
        $Candidate = Split-Path -Parent $ScriptsDir
        if (Test-Path (Join-Path $Candidate "gitcortex-server.exe")) {
            $InstallDir = $Candidate
        }
    }
    if (-not $InstallDir) {
        $InstallDir = "C:\Program Files\GitCortex"
    }
}

# --- Result tracking ---
$script:Results = @()
$script:PassCount = 0
$script:FailCount = 0
$script:WarnCount = 0

function Add-Check {
    param(
        [string]$Group,
        [string]$Name,
        [ValidateSet("PASS","FAIL","WARN","SKIP")]
        [string]$Status,
        [string]$Detail = ""
    )
    $color = switch ($Status) {
        "PASS" { "Green" }
        "FAIL" { "Red" }
        "WARN" { "Yellow" }
        "SKIP" { "DarkGray" }
    }
    $msg = "[$Status] $Name"
    if ($Detail) { $msg += " -- $Detail" }
    Write-Host "  $msg" -ForegroundColor $color

    $script:Results += [PSCustomObject]@{
        Group  = $Group
        Name   = $Name
        Status = $Status
        Detail = $Detail
    }
    switch ($Status) {
        "PASS" { $script:PassCount++ }
        "FAIL" { $script:FailCount++ }
        "WARN" { $script:WarnCount++ }
    }
}

function Test-FilePath {
    param([string]$Group, [string]$Name, [string]$RelPath, [string]$Level = "FAIL")
    $FullPath = Join-Path $InstallDir $RelPath
    if (Test-Path $FullPath) {
        Add-Check -Group $Group -Name $Name -Status "PASS"
    } else {
        Add-Check -Group $Group -Name $Name -Status $Level -Detail "$RelPath not found"
    }
}

# ====================================
Write-Host ""
Write-Host "====================================" -ForegroundColor Cyan
Write-Host "  GitCortex Post-Installation Check" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan
Write-Host "  Install Dir: $InstallDir"
Write-Host ""

# ============================================================================
# Group 1: File System (core files only)
# ============================================================================
Write-Host "--- 1. File System ---" -ForegroundColor White

Test-FilePath "File System" "gitcortex-server.exe" "gitcortex-server.exe"
Test-FilePath "File System" "gitcortex-tray.exe" "gitcortex-tray.exe"
Test-FilePath "File System" ".env" ".env"
Test-FilePath "File System" "scripts\" "scripts"
Test-FilePath "File System" "assets\GitCortex.ico" "assets\GitCortex.ico"

Write-Host ""

# ============================================================================
# Group 2: Environment Variables (.env)
# ============================================================================
Write-Host "--- 2. Environment Variables (.env) ---" -ForegroundColor White

$EnvFile = Join-Path $InstallDir ".env"
$EnvVars = @{}
$EnvParseOk = $false

if (Test-Path $EnvFile) {
    # BOM check
    $RawBytes = [System.IO.File]::ReadAllBytes($EnvFile)
    if ($RawBytes.Length -ge 3 -and $RawBytes[0] -eq 0xEF -and $RawBytes[1] -eq 0xBB -and $RawBytes[2] -eq 0xBF) {
        Add-Check -Group "Environment" -Name ".env no UTF-8 BOM" -Status "WARN" -Detail "File has UTF-8 BOM, may cause parsing issues"
    } else {
        Add-Check -Group "Environment" -Name ".env no UTF-8 BOM" -Status "PASS"
    }

    try {
        $Lines = Get-Content $EnvFile -Encoding UTF8
        foreach ($line in $Lines) {
            $line = $line.Trim()
            if ($line -and -not $line.StartsWith("#")) {
                $eqIdx = $line.IndexOf("=")
                if ($eqIdx -gt 0) {
                    $key = $line.Substring(0, $eqIdx).Trim()
                    $val = $line.Substring($eqIdx + 1).Trim()
                    $EnvVars[$key] = $val
                }
            }
        }
        $EnvParseOk = $true
        Add-Check -Group "Environment" -Name ".env parseable" -Status "PASS"
    } catch {
        Add-Check -Group "Environment" -Name ".env parseable" -Status "FAIL" -Detail $_.Exception.Message
    }
} else {
    Add-Check -Group "Environment" -Name ".env parseable" -Status "FAIL" -Detail ".env file not found"
}

if ($EnvParseOk) {
    # Encryption key
    if ($EnvVars.ContainsKey("GITCORTEX_ENCRYPTION_KEY")) {
        Add-Check -Group "Environment" -Name "GITCORTEX_ENCRYPTION_KEY exists" -Status "PASS"
        $EncKey = $EnvVars["GITCORTEX_ENCRYPTION_KEY"]

        if ($EncKey.Length -eq 32) {
            Add-Check -Group "Environment" -Name "ENCRYPTION_KEY length=32" -Status "PASS"
        } else {
            Add-Check -Group "Environment" -Name "ENCRYPTION_KEY length=32" -Status "FAIL" -Detail "Length is $($EncKey.Length), expected 32"
        }

        $ByteCount = [System.Text.Encoding]::UTF8.GetByteCount($EncKey)
        if ($ByteCount -eq $EncKey.Length) {
            Add-Check -Group "Environment" -Name "ENCRYPTION_KEY pure ASCII" -Status "PASS"
        } else {
            Add-Check -Group "Environment" -Name "ENCRYPTION_KEY pure ASCII" -Status "FAIL" -Detail "Contains non-ASCII characters"
        }
    } else {
        Add-Check -Group "Environment" -Name "GITCORTEX_ENCRYPTION_KEY exists" -Status "FAIL" -Detail "Key not found in .env"
        Add-Check -Group "Environment" -Name "ENCRYPTION_KEY length=32" -Status "FAIL" -Detail "Key missing"
        Add-Check -Group "Environment" -Name "ENCRYPTION_KEY pure ASCII" -Status "FAIL" -Detail "Key missing"
    }

    # Local mode
    if ($EnvVars["GITCORTEX_LOCAL_MODE"] -eq "1") {
        Add-Check -Group "Environment" -Name "GITCORTEX_LOCAL_MODE=1" -Status "PASS"
    } else {
        Add-Check -Group "Environment" -Name "GITCORTEX_LOCAL_MODE=1" -Status "WARN" -Detail "Not set to 1"
    }

    # Backend port
    if ($EnvVars.ContainsKey("BACKEND_PORT")) {
        $portVal = 0
        if ([int]::TryParse($EnvVars["BACKEND_PORT"], [ref]$portVal)) {
            Add-Check -Group "Environment" -Name "BACKEND_PORT is numeric" -Status "PASS"
        } else {
            Add-Check -Group "Environment" -Name "BACKEND_PORT is numeric" -Status "WARN" -Detail "Value '$($EnvVars["BACKEND_PORT"])' is not a number"
        }
    } else {
        Add-Check -Group "Environment" -Name "BACKEND_PORT is numeric" -Status "WARN" -Detail "Not set"
    }

    # Bash path
    if ($EnvVars.ContainsKey("CLAUDE_CODE_GIT_BASH_PATH")) {
        $bashPath = $EnvVars["CLAUDE_CODE_GIT_BASH_PATH"]
        if (Test-Path $bashPath) {
            Add-Check -Group "Environment" -Name "CLAUDE_CODE_GIT_BASH_PATH valid" -Status "PASS"
        } else {
            Add-Check -Group "Environment" -Name "CLAUDE_CODE_GIT_BASH_PATH valid" -Status "FAIL" -Detail "Points to '$bashPath' which does not exist"
        }
    } else {
        Add-Check -Group "Environment" -Name "CLAUDE_CODE_GIT_BASH_PATH valid" -Status "WARN" -Detail "Not set (install Git for Windows and update .env)"
    }

    # npm update suppression
    if ($EnvVars["NO_UPDATE_NOTIFIER"] -eq "1") {
        Add-Check -Group "Environment" -Name "NO_UPDATE_NOTIFIER=1" -Status "PASS"
    } else {
        Add-Check -Group "Environment" -Name "NO_UPDATE_NOTIFIER=1" -Status "WARN" -Detail "Not set to 1"
    }

    if ($EnvVars["NPM_CONFIG_UPDATE_NOTIFIER"] -eq "false") {
        Add-Check -Group "Environment" -Name "NPM_CONFIG_UPDATE_NOTIFIER=false" -Status "PASS"
    } else {
        Add-Check -Group "Environment" -Name "NPM_CONFIG_UPDATE_NOTIFIER=false" -Status "WARN" -Detail "Not set"
    }
}

Write-Host ""

# ============================================================================
# Group 3: System Dependencies
# ============================================================================
Write-Host "--- 3. System Dependencies ---" -ForegroundColor White

$SysDeps = @(
    @{ Name = "Node.js (node)"; Cmd = "node" },
    @{ Name = "Git (git)"; Cmd = "git" },
    @{ Name = "npm"; Cmd = "npm" },
    @{ Name = "GitHub CLI (gh)"; Cmd = "gh"; Optional = $true }
)

foreach ($dep in $SysDeps) {
    $found = $null
    try {
        $found = & where.exe $dep.Cmd 2>$null | Select-Object -First 1
    } catch {}
    if ($found) {
        Add-Check -Group "System Deps" -Name $dep.Name -Status "PASS" -Detail $found
    } else {
        $level = if ($dep.Optional) { "WARN" } else { "FAIL" }
        $suffix = if ($dep.Optional) { " (optional)" } else { " -- install and add to PATH" }
        Add-Check -Group "System Deps" -Name $dep.Name -Status $level -Detail "Not found in PATH$suffix"
    }
}

Write-Host ""

# ============================================================================
# Group 4: Runtime Tool Versions
# ============================================================================
Write-Host "--- 4. Runtime Tool Versions ---" -ForegroundColor White

function Test-RuntimeTool {
    param([string]$Name, [string]$Cmd, [string]$Args, [string]$ExpectPattern)
    try {
        $tmpOut = [System.IO.Path]::GetTempFileName()
        $tmpErr = [System.IO.Path]::GetTempFileName()
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = "cmd.exe"
        $psi.Arguments = "/C `"$Cmd $Args > `"$tmpOut`" 2> `"$tmpErr`"`""
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.EnvironmentVariables["NPM_CONFIG_UPDATE_NOTIFIER"] = "false"
        $psi.EnvironmentVariables["NO_UPDATE_NOTIFIER"] = "1"
        $proc = [System.Diagnostics.Process]::Start($psi)
        $exited = $proc.WaitForExit(15000)
        if (-not $exited) {
            try { $proc.Kill() } catch {}
            Add-Check -Group "Runtime" -Name $Name -Status "FAIL" -Detail "Timed out after 15s"
            return
        }
        $stdout = if (Test-Path $tmpOut) { (Get-Content $tmpOut -Raw -ErrorAction SilentlyContinue) } else { "" }
        $stderr = if (Test-Path $tmpErr) { (Get-Content $tmpErr -Raw -ErrorAction SilentlyContinue) } else { "" }
        Remove-Item $tmpOut, $tmpErr -Force -ErrorAction SilentlyContinue
        $output = "$stdout $stderr".Trim()
        if ($proc.ExitCode -eq 0 -and $output -match $ExpectPattern) {
            $ver = ($output -split "`n")[0].Trim()
            Add-Check -Group "Runtime" -Name $Name -Status "PASS" -Detail $ver
        } else {
            $detail = "Exit=$($proc.ExitCode)"
            if ($output) { $detail += ", output: $($output.Substring(0, [Math]::Min($output.Length, 200)))" }
            Add-Check -Group "Runtime" -Name $Name -Status "WARN" -Detail $detail
        }
    } catch {
        Add-Check -Group "Runtime" -Name $Name -Status "WARN" -Detail $_.Exception.Message
    }
}

Test-RuntimeTool "Node.js" "node" "--version" "^v"
Test-RuntimeTool "Git" "git" "--version" "git version"
Test-RuntimeTool "npm" "npm" "--version" "^\d"
Test-RuntimeTool "GitHub CLI" "gh" "--version" "gh version"

Write-Host ""

# ============================================================================
# Group 5: AI CLIs
# ============================================================================
Write-Host "--- 5. AI CLIs ---" -ForegroundColor White

$CLIs = @(
    @{ Name = "Claude Code"; Cmd = "claude"; Args = "--version" },
    @{ Name = "Codex"; Cmd = "codex"; Args = "--version" },
    @{ Name = "Gemini CLI"; Cmd = "gemini"; Args = "--version" },
    @{ Name = "Amp"; Cmd = "amp"; Args = "--version" },
    @{ Name = "Qwen Code"; Cmd = "qwen"; Args = "--version" },
    @{ Name = "Opencode"; Cmd = "opencode"; Args = "--version" },
    @{ Name = "Droid"; Cmd = "droid"; Args = "--version" },
    @{ Name = "GitHub Copilot"; Cmd = "gh copilot"; Args = "--version" }
)

$DetectedCount = 0
foreach ($cli in $CLIs) {
    try {
        $tmpOut = [System.IO.Path]::GetTempFileName()
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = "cmd.exe"
        $psi.Arguments = "/C $($cli.Cmd) $($cli.Args) > `"$tmpOut`" 2>&1"
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.EnvironmentVariables["NPM_CONFIG_UPDATE_NOTIFIER"] = "false"
        $psi.EnvironmentVariables["NO_UPDATE_NOTIFIER"] = "1"
        $proc = [System.Diagnostics.Process]::Start($psi)
        $exited = $proc.WaitForExit(15000)
        if (-not $exited) { try { $proc.Kill() } catch {} }
        $stdout = if (Test-Path $tmpOut) { (Get-Content $tmpOut -Raw -ErrorAction SilentlyContinue) } else { "" }
        Remove-Item $tmpOut -Force -ErrorAction SilentlyContinue
        if ($exited -and $proc.ExitCode -eq 0 -and $stdout) {
            $ver = ($stdout -split "`n")[0].Trim()
            Add-Check -Group "AI CLIs" -Name $cli.Name -Status "PASS" -Detail $ver
            $DetectedCount++
        } else {
            Add-Check -Group "AI CLIs" -Name $cli.Name -Status "WARN" -Detail "Not installed or not runnable"
        }
    } catch {
        Add-Check -Group "AI CLIs" -Name $cli.Name -Status "WARN" -Detail "Not installed"
    }
}

if ($DetectedCount -eq 0) {
    Add-Check -Group "AI CLIs" -Name "At least 1 CLI detected" -Status "FAIL" -Detail "0 AI CLIs detected -- install via: npm install -g @anthropic-ai/claude-code"
} else {
    Add-Check -Group "AI CLIs" -Name "At least 1 CLI detected" -Status "PASS" -Detail "$DetectedCount CLI(s) found"
}

# Claude Code auth check
$ClaudeConfigDir = Join-Path $env:USERPROFILE ".claude"
$ClaudeCredentials = Join-Path $ClaudeConfigDir "credentials.json"
$ClaudeHasAuth = $false
if (Test-Path $ClaudeCredentials) {
    try {
        $credContent = Get-Content $ClaudeCredentials -Raw -ErrorAction SilentlyContinue
        if ($credContent -and $credContent.Length -gt 10) {
            $ClaudeHasAuth = $true
        }
    } catch {}
}
if ($ClaudeHasAuth) {
    Add-Check -Group "AI CLIs" -Name "Claude Code auth" -Status "PASS" -Detail "credentials.json found"
} else {
    Add-Check -Group "AI CLIs" -Name "Claude Code auth" -Status "WARN" -Detail "No ~/.claude/credentials.json -- run 'claude login' or configure API key in GitCortex settings"
}

Write-Host ""

# ============================================================================
# Group 6: Server Startup
# ============================================================================
Write-Host "--- 6. Server Startup ---" -ForegroundColor White

$BackendPort = if ($EnvVars.ContainsKey("BACKEND_PORT")) { $EnvVars["BACKEND_PORT"] } else { "23456" }
$ServerExe = Join-Path $InstallDir "gitcortex-server.exe"
$ServerStartedByUs = $false
$ServerRunning = $false

if ($SkipServerTest) {
    Add-Check -Group "Server" -Name "Server startup" -Status "SKIP" -Detail "-SkipServerTest specified"
    Add-Check -Group "Server" -Name "GET /healthz" -Status "SKIP" -Detail "-SkipServerTest specified"
    Add-Check -Group "Server" -Name "GET /readyz" -Status "SKIP" -Detail "-SkipServerTest specified"
} else {
    # Check if server is already running
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/healthz" -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop
        if ($resp.StatusCode -eq 200) {
            $ServerRunning = $true
            Add-Check -Group "Server" -Name "Server already running" -Status "PASS" -Detail "Reusing existing server on port $BackendPort"
        }
    } catch {
        $ServerRunning = $false
    }

    if (-not $ServerRunning) {
        $PortInUse = $false
        try {
            $netstat = netstat -ano | Select-String ":$BackendPort\s"
            if ($netstat) {
                $PortInUse = $true
                Add-Check -Group "Server" -Name "Port $BackendPort available" -Status "FAIL" -Detail "Port occupied by another process"
            } else {
                Add-Check -Group "Server" -Name "Port $BackendPort available" -Status "PASS"
            }
        } catch {
            Add-Check -Group "Server" -Name "Port $BackendPort available" -Status "PASS"
        }

        if (-not $PortInUse -and (Test-Path $ServerExe)) {
            try {
                $psi = New-Object System.Diagnostics.ProcessStartInfo
                $psi.FileName = $ServerExe
                $psi.WorkingDirectory = $InstallDir
                $psi.UseShellExecute = $false
                $psi.CreateNoWindow = $true
                foreach ($kv in $EnvVars.GetEnumerator()) {
                    $psi.EnvironmentVariables[$kv.Key] = $kv.Value
                }
                $ServerProc = [System.Diagnostics.Process]::Start($psi)
                $ServerStartedByUs = $true

                $Healthy = $false
                for ($i = 0; $i -lt 20; $i++) {
                    Start-Sleep -Seconds 1
                    try {
                        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/healthz" -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
                        if ($resp.StatusCode -eq 200) {
                            $Healthy = $true
                            $ServerRunning = $true
                            break
                        }
                    } catch {}
                }

                if ($Healthy) {
                    Add-Check -Group "Server" -Name "Server startup" -Status "PASS" -Detail "Started in $($i+1)s"
                } else {
                    Add-Check -Group "Server" -Name "Server startup" -Status "FAIL" -Detail "Failed to respond within 20s"
                }
            } catch {
                Add-Check -Group "Server" -Name "Server startup" -Status "FAIL" -Detail $_.Exception.Message
            }
        } elseif (-not (Test-Path $ServerExe)) {
            Add-Check -Group "Server" -Name "Server startup" -Status "FAIL" -Detail "gitcortex-server.exe not found"
        }
    }

    # Health checks
    if ($ServerRunning) {
        try {
            $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/healthz" -UseBasicParsing -TimeoutSec 5
            $body = $resp.Content
            if ($body -match '"ok"\s*:\s*true') {
                Add-Check -Group "Server" -Name "GET /healthz" -Status "PASS"
            } else {
                Add-Check -Group "Server" -Name "GET /healthz" -Status "FAIL" -Detail "Unexpected response: $body"
            }
        } catch {
            Add-Check -Group "Server" -Name "GET /healthz" -Status "FAIL" -Detail $_.Exception.Message
        }

        try {
            $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/readyz" -UseBasicParsing -TimeoutSec 5
            $body = $resp.Content
            if ($body -match '"ready"\s*:\s*true') {
                Add-Check -Group "Server" -Name "GET /readyz" -Status "PASS"
            } else {
                Add-Check -Group "Server" -Name "GET /readyz" -Status "WARN" -Detail "Not fully ready: $body"
            }
        } catch {
            Add-Check -Group "Server" -Name "GET /readyz" -Status "WARN" -Detail $_.Exception.Message
        }
    } elseif (-not $SkipServerTest) {
        if (-not ($script:Results | Where-Object { $_.Name -eq "GET /healthz" })) {
            Add-Check -Group "Server" -Name "GET /healthz" -Status "FAIL" -Detail "Server not running"
            Add-Check -Group "Server" -Name "GET /readyz" -Status "FAIL" -Detail "Server not running"
        }
    }
}

Write-Host ""

# ============================================================================
# Group 7: Frontend
# ============================================================================
Write-Host "--- 7. Frontend ---" -ForegroundColor White

if ($SkipServerTest -or -not $ServerRunning) {
    Add-Check -Group "Frontend" -Name "GET / returns HTML" -Status "SKIP" -Detail "Server not running"
    Add-Check -Group "Frontend" -Name "HTML contains expected marker" -Status "SKIP" -Detail "Server not running"
} else {
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/" -UseBasicParsing -TimeoutSec 5
        $ct = $resp.Headers["Content-Type"]
        if ($ct -and $ct -match "text/html") {
            Add-Check -Group "Frontend" -Name "GET / returns HTML" -Status "PASS"
        } else {
            Add-Check -Group "Frontend" -Name "GET / returns HTML" -Status "FAIL" -Detail "Content-Type: $ct"
        }
        $html = $resp.Content
        if ($html -match 'id="root"' -or $html -match "GitCortex") {
            Add-Check -Group "Frontend" -Name "HTML contains expected marker" -Status "PASS"
        } else {
            Add-Check -Group "Frontend" -Name "HTML contains expected marker" -Status "WARN" -Detail "No 'id=root' or 'GitCortex' found"
        }
    } catch {
        Add-Check -Group "Frontend" -Name "GET / returns HTML" -Status "FAIL" -Detail $_.Exception.Message
        Add-Check -Group "Frontend" -Name "HTML contains expected marker" -Status "WARN" -Detail "Could not fetch"
    }
}

Write-Host ""

# ============================================================================
# Group 8: Database & Data Directory
# ============================================================================
Write-Host "--- 8. Database & Data Directory ---" -ForegroundColor White

$DataDir = Join-Path $env:APPDATA "bloop\gitcortex"
if (Test-Path $DataDir) {
    Add-Check -Group "Database" -Name "Data directory exists" -Status "PASS" -Detail $DataDir

    $TmpFile = Join-Path $DataDir ".selfcheck_tmp"
    try {
        [System.IO.File]::WriteAllText($TmpFile, "check")
        Remove-Item $TmpFile -Force -ErrorAction SilentlyContinue
        Add-Check -Group "Database" -Name "Data directory writable" -Status "PASS"
    } catch {
        Add-Check -Group "Database" -Name "Data directory writable" -Status "FAIL" -Detail "Cannot write to data directory"
    }

    $DbFile = Join-Path $DataDir "db.sqlite"
    if (Test-Path $DbFile) {
        Add-Check -Group "Database" -Name "db.sqlite exists" -Status "PASS"
        $DbSize = (Get-Item $DbFile).Length
        if ($DbSize -gt 0) {
            Add-Check -Group "Database" -Name "db.sqlite non-empty" -Status "PASS" -Detail "$([math]::Round($DbSize / 1KB, 1)) KB"
        } else {
            Add-Check -Group "Database" -Name "db.sqlite non-empty" -Status "FAIL" -Detail "File is empty (0 bytes)"
        }
    } else {
        Add-Check -Group "Database" -Name "db.sqlite exists" -Status "FAIL" -Detail "Not found (server may not have run yet)"
        Add-Check -Group "Database" -Name "db.sqlite non-empty" -Status "FAIL" -Detail "File missing"
    }
} else {
    $status = if ($ServerRunning -or -not $SkipServerTest) { "FAIL" } else { "WARN" }
    Add-Check -Group "Database" -Name "Data directory exists" -Status $status -Detail "$DataDir not found (server may not have run yet)"
    Add-Check -Group "Database" -Name "Data directory writable" -Status "SKIP" -Detail "Directory missing"
    Add-Check -Group "Database" -Name "db.sqlite exists" -Status "SKIP" -Detail "Directory missing"
    Add-Check -Group "Database" -Name "db.sqlite non-empty" -Status "SKIP" -Detail "Directory missing"
}

Write-Host ""

# ============================================================================
# Group 9: Windows Integration
# ============================================================================
Write-Host "--- 9. Windows Integration ---" -ForegroundColor White

# Firewall rule
try {
    $fwOutput = & netsh advfirewall firewall show rule name="GitCortex Server" 2>&1
    if ($fwOutput -match "GitCortex Server") {
        Add-Check -Group "Windows" -Name "Firewall rule exists" -Status "PASS"
    } else {
        Add-Check -Group "Windows" -Name "Firewall rule exists" -Status "WARN" -Detail "No firewall rule found"
    }
} catch {
    Add-Check -Group "Windows" -Name "Firewall rule exists" -Status "WARN" -Detail "Could not query firewall"
}

# User PATH check
try {
    $UserPath = (Get-ItemProperty -Path "HKCU:\Environment" -Name "Path" -ErrorAction SilentlyContinue).Path
    if ($UserPath) {
        if ($UserPath -match [regex]::Escape($InstallDir)) {
            Add-Check -Group "Windows" -Name "User PATH contains install dir" -Status "PASS"
        } else {
            Add-Check -Group "Windows" -Name "User PATH contains install dir" -Status "WARN" -Detail "Not found in user PATH"
        }
    } else {
        Add-Check -Group "Windows" -Name "User PATH readable" -Status "WARN" -Detail "Could not read user PATH"
    }
} catch {
    Add-Check -Group "Windows" -Name "User PATH readable" -Status "WARN" -Detail $_.Exception.Message
}

# VC++ Runtime
$VcDll = Join-Path $env:SystemRoot "System32\vcruntime140.dll"
if (Test-Path $VcDll) {
    Add-Check -Group "Windows" -Name "VC++ Runtime (vcruntime140.dll)" -Status "PASS"
} else {
    Add-Check -Group "Windows" -Name "VC++ Runtime (vcruntime140.dll)" -Status "WARN" -Detail "Not found in System32"
}

Write-Host ""

# ============================================================================
# Cleanup: stop test server if we started it
# ============================================================================
if ($ServerStartedByUs -and $ServerProc -and -not $ServerProc.HasExited) {
    try {
        $ServerProc.Kill()
        $ServerProc.WaitForExit(5000) | Out-Null
        if ($Verbose) { Write-Host "  [CLEANUP] Test server stopped" -ForegroundColor DarkGray }
    } catch {}
}

# ============================================================================
# Summary
# ============================================================================
Write-Host "====================================" -ForegroundColor Cyan
Write-Host "  Summary: $script:PassCount PASS | $script:FailCount FAIL | $script:WarnCount WARN" -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan

if ($script:FailCount -gt 0) {
    Write-Host "  FAIL:" -ForegroundColor Red
    $failIdx = 1
    foreach ($r in $script:Results | Where-Object { $_.Status -eq "FAIL" }) {
        $detail = if ($r.Detail) { " -- $($r.Detail)" } else { "" }
        Write-Host "    $failIdx. [$($r.Group)] $($r.Name)$detail" -ForegroundColor Red
        $failIdx++
    }
}

if ($script:WarnCount -gt 0 -and $Verbose) {
    Write-Host "  WARN:" -ForegroundColor Yellow
    $warnIdx = 1
    foreach ($r in $script:Results | Where-Object { $_.Status -eq "WARN" }) {
        $detail = if ($r.Detail) { " -- $($r.Detail)" } else { "" }
        Write-Host "    $warnIdx. [$($r.Group)] $($r.Name)$detail" -ForegroundColor Yellow
        $warnIdx++
    }
}

Write-Host "====================================" -ForegroundColor Cyan
Write-Host ""

if ($script:FailCount -gt 0) {
    exit 1
} else {
    exit 0
}
