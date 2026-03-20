# Generate GITCORTEX_ENCRYPTION_KEY, auto-detect bash path, write to .env file.
# Usage: generate-key.ps1 -EnvFile <path> [-InstallDir <path>]
param(
    [Parameter(Mandatory=$true)][string]$EnvFile,
    [string]$InstallDir
)

$ErrorActionPreference = "Stop"

function Generate-AsciiKey {
    param([int]$Length)
    $chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    $result = ""
    for ($i = 0; $i -lt $Length; $i++) {
        $buf = New-Object byte[] 1
        $rng.GetBytes($buf)
        $idx = $buf[0] % $chars.Length
        $result += $chars[$idx]
    }
    return $result
}

$EncryptionKey = Generate-AsciiKey -Length 32
if (-not $InstallDir) {
    $InstallDir = Split-Path -Parent $EnvFile
}

# Auto-detect bash.exe for Claude Code
$BashCandidates = @(
    "$InstallDir\git\usr\bin\bash.exe",
    "$env:ProgramFiles\Git\usr\bin\bash.exe",
    "${env:ProgramFiles(x86)}\Git\usr\bin\bash.exe",
    "$env:LOCALAPPDATA\Programs\Git\usr\bin\bash.exe"
)
$BashPath = $BashCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1

# Build bash path line
if ($BashPath) {
    $BashLine = "CLAUDE_CODE_GIT_BASH_PATH=$BashPath"
    Write-Host "[INFO] Detected bash at: $BashPath"
} else {
    $BashLine = "# CLAUDE_CODE_GIT_BASH_PATH=  (not found -- install Git for Windows and set manually)"
    Write-Host "[WARN] bash.exe not found. Set CLAUDE_CODE_GIT_BASH_PATH manually after installing Git." -ForegroundColor Yellow
}

# Build .env content
$envContent = @"
# GitCortex Environment Configuration
# Generated during installation - DO NOT share these values.

# Encryption key for API key storage (32 bytes, required)
GITCORTEX_ENCRYPTION_KEY=$EncryptionKey

# API authentication token (optional, enables Bearer auth on all endpoints)
# Uncomment to require Bearer token for API access:
# GITCORTEX_API_TOKEN=your-token-here

# Local mode: skip API token requirement (localhost-only, safe)
GITCORTEX_LOCAL_MODE=1

# Server configuration
BACKEND_PORT=23456
HOST=127.0.0.1

# Claude Code requires git-bash on Windows
$BashLine

# Suppress npm update notices in CLI output
NO_UPDATE_NOTIFIER=1
NPM_CONFIG_UPDATE_NOTIFIER=false

# Logging level (debug/info/warn/error)
RUST_LOG=info
"@

# Write or append
if (Test-Path $EnvFile) {
    $existing = Get-Content $EnvFile -Raw
    if ($existing -match "GITCORTEX_ENCRYPTION_KEY=") {
        Write-Host "[INFO] GITCORTEX_ENCRYPTION_KEY already exists in $EnvFile, skipping."
    } else {
        $existing = [System.IO.File]::ReadAllText($EnvFile)
        [System.IO.File]::WriteAllText($EnvFile, $existing + "`n" + $envContent, (New-Object System.Text.UTF8Encoding $false))
        Write-Host "[INFO] Keys appended to $EnvFile (UTF-8 no BOM)"
    }
} else {
    # Write UTF-8 WITHOUT BOM -- critical for Rust env parsing
    [System.IO.File]::WriteAllText($EnvFile, $envContent, (New-Object System.Text.UTF8Encoding $false))
    Write-Host "[INFO] Created $EnvFile with generated keys (UTF-8 no BOM)"
}
