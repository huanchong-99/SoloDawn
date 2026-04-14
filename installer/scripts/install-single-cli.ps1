# Install or uninstall a single AI CLI tool (Windows).
# Usage: install-single-cli.ps1 <action> <cli_name>
#   action: install | uninstall
#   cli_name: claude-code | codex | gemini-cli | amp | cursor-agent | qwen-code | copilot | opencode | droid
param(
    [Parameter(Mandatory=$true)][string]$Action,
    [Parameter(Mandatory=$true)][string]$CliName
)

$ErrorActionPreference = "Stop"

# --- Logging helpers ---
function Log-Info  { param([string]$msg) Write-Host "[INFO]  $msg" }
function Log-Error { param([string]$msg) Write-Host "[ERROR] $msg" -ForegroundColor Red }
function Log-Warn  { param([string]$msg) Write-Host "[WARN]  $msg" -ForegroundColor Yellow }

# --- Resolve tools ---
# Check bundled tools first (legacy installs), then fall back to system PATH.
$InstallDir = $env:SOLODAWN_INSTALL_DIR
if (-not $InstallDir) {
    $InstallDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
}

$ExtraPaths = @()

# Legacy bundled paths (older installs that still have these)
$NodeDir = Join-Path $InstallDir "node_portable"
if (Test-Path $NodeDir) { $ExtraPaths += $NodeDir }
$NpmGlobalBin = Join-Path $NodeDir "node_modules\.bin"
if (Test-Path $NpmGlobalBin) { $ExtraPaths += $NpmGlobalBin }
$GitDir = Join-Path $InstallDir "mingit\cmd"
if (Test-Path $GitDir) { $ExtraPaths += $GitDir }
$GhDir = Join-Path $InstallDir "gh"
if (Test-Path $GhDir) { $ExtraPaths += $GhDir }

# System npm global bin (where npm install -g puts executables)
$AppDataNpm = Join-Path $env:APPDATA "npm"
if (Test-Path $AppDataNpm) { $ExtraPaths += $AppDataNpm }

if ($ExtraPaths.Count -gt 0) {
    $env:PATH = ($ExtraPaths -join ";") + ";" + $env:PATH
}

# Suppress npm update notices
$env:NO_UPDATE_NOTIFIER = "1"
$env:NPM_CONFIG_UPDATE_NOTIFIER = "false"

# --- Validation ---
$ValidClis = @("claude-code","codex","gemini-cli","amp","cursor-agent","qwen-code","copilot","opencode","droid")
if ($ValidClis -notcontains $CliName) {
    Log-Error "Invalid CLI name: $CliName"
    Log-Error "Valid names: $($ValidClis -join ', ')"
    exit 1
}

if ($Action -ne "install" -and $Action -ne "uninstall") {
    Log-Error "Invalid action: $Action (must be 'install' or 'uninstall')"
    exit 1
}

# --- Package mapping ---
function Resolve-Package {
    param([string]$cli)
    switch ($cli) {
        "claude-code"  { if ($env:CLAUDE_CODE_NPM_PKG)  { $env:CLAUDE_CODE_NPM_PKG }  else { "@anthropic-ai/claude-code" } }
        "codex"        { if ($env:CODEX_NPM_PKG)        { $env:CODEX_NPM_PKG }        else { "@openai/codex" } }
        "gemini-cli"   { if ($env:GEMINI_NPM_PKG)       { $env:GEMINI_NPM_PKG }       else { "@google/gemini-cli" } }
        "amp"          { if ($env:AMP_NPM_PKG)           { $env:AMP_NPM_PKG }           else { "@sourcegraph/amp" } }
        "qwen-code"    { if ($env:QWEN_NPM_PKG)         { $env:QWEN_NPM_PKG }         else { "@qwen-code/qwen-code" } }
        "opencode"     { if ($env:OPENCODE_NPM_PKG)     { $env:OPENCODE_NPM_PKG }     else { "opencode-ai" } }
        "droid"        { if ($env:KILOCODE_NPM_PKG)     { $env:KILOCODE_NPM_PKG }     else { "@kilocode/cli" } }
        "cursor-agent" { if ($env:CURSOR_AGENT_NPM_PKG) { $env:CURSOR_AGENT_NPM_PKG } else { "cursor-agent" } }
        "copilot"      { "__gh_extension__" }
        default        { Log-Error "Unknown CLI: $cli"; exit 1 }
    }
}

# --- Detection command mapping ---
function Get-DetectCommand {
    param([string]$cli)
    switch ($cli) {
        "claude-code"  { "claude --version" }
        "codex"        { "codex --version" }
        "gemini-cli"   { "gemini --version" }
        "amp"          { "amp --version" }
        "qwen-code"    { "qwen --version" }
        "opencode"     { "opencode --version" }
        "droid"        { "droid --version" }
        "cursor-agent" { "cursor-agent --version" }
        "copilot"      { "gh copilot --version" }
        default        { "" }
    }
}

# --- Copilot handlers ---
function Install-Copilot {
    $ghCmd = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $ghCmd) {
        Log-Error "gh CLI not found; cannot install gh-copilot extension"
        exit 1
    }

    $extensions = & gh extension list 2>$null
    if ($extensions -match "github/gh-copilot") {
        Log-Info "GitHub Copilot extension already installed"
        return
    }

    Log-Info "Installing GitHub Copilot CLI extension..."
    & gh extension install github/gh-copilot 2>&1 | ForEach-Object { Write-Host $_ }
}

function Uninstall-Copilot {
    $ghCmd = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $ghCmd) {
        Log-Error "gh CLI not found; cannot uninstall gh-copilot extension"
        exit 1
    }

    Log-Info "Removing GitHub Copilot CLI extension..."
    & gh extension remove github/gh-copilot 2>&1 | ForEach-Object { Write-Host $_ }
}

# --- npm install with retry ---
function Npm-InstallGlobal {
    param([string]$Package, [int]$MaxRetries = 3)

    for ($i = 1; $i -le $MaxRetries; $i++) {
        Log-Info "Attempt $i/$MaxRetries - npm install -g $Package"
        try {
            & npm install -g $Package 2>&1 | ForEach-Object { Write-Host $_ }
            if ($LASTEXITCODE -eq 0) {
                return
            }
        } catch {
            Log-Warn "Attempt $i failed: $_"
        }
        if ($i -lt $MaxRetries) {
            Log-Warn "Retrying in 3 seconds..."
            Start-Sleep -Seconds 3
        }
    }
    Log-Error "npm install -g $Package failed after $MaxRetries attempts"
    exit 1
}

# --- Main logic ---
$Package = Resolve-Package -cli $CliName

if ($Action -eq "install") {
    Log-Info "Installing CLI: $CliName"

    if ($Package -eq "__gh_extension__") {
        Install-Copilot
    } else {
        # Verify node and npm are available
        $nodeCmd = Get-Command node -ErrorAction SilentlyContinue
        if (-not $nodeCmd) {
            Log-Error "node not found in PATH"
            exit 1
        }
        $npmCmd = Get-Command npm -ErrorAction SilentlyContinue
        if (-not $npmCmd) {
            Log-Error "npm not found in PATH"
            exit 1
        }

        Npm-InstallGlobal -Package $Package -MaxRetries 3
    }

    # Post-install verification
    $DetectCmd = Get-DetectCommand -cli $CliName
    if ($DetectCmd) {
        Log-Info "Verifying installation..."
        try {
            $parts = $DetectCmd -split " ", 2
            $output = & $parts[0] $parts[1] 2>&1
            $version = ($output | Select-Object -First 1)
            Log-Info "Verified ${CliName}: $version"
        } catch {
            Log-Warn "Verification failed for $CliName (command may need PATH refresh)"
        }
    }

    Log-Info "Install complete: $CliName"

} elseif ($Action -eq "uninstall") {
    Log-Info "Uninstalling CLI: $CliName"

    if ($Package -eq "__gh_extension__") {
        Uninstall-Copilot
    } else {
        $npmCmd = Get-Command npm -ErrorAction SilentlyContinue
        if (-not $npmCmd) {
            Log-Error "npm not found in PATH"
            exit 1
        }

        # Strip version specifier for uninstall.
        # Handles scoped names with hyphens (e.g. @anthropic-ai/claude-code@1.0)
        # by anchoring a capture group on the full @scope/name before the version @.
        $UninstallPkg = $Package
        if ($UninstallPkg -match '^(@[^/]+/[^@]+)@(.+)$') {
            # Scoped: @scope/name@version -> @scope/name
            $UninstallPkg = $Matches[1]
        } elseif ($UninstallPkg -match '^([^@]+)@(.+)$') {
            # Unscoped: name@version -> name (first @ is the version separator)
            $UninstallPkg = $Matches[1]
        }
        # else: no version specifier, leave $UninstallPkg as-is.

        Log-Info "Running: npm uninstall -g $UninstallPkg"
        & npm uninstall -g $UninstallPkg 2>&1 | ForEach-Object { Write-Host $_ }
    }

    Log-Info "Uninstall complete: $CliName"
}

exit 0
