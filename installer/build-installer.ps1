# ============================================================================
# SoloDawn Installer Build Script (Lightweight)
# Compiles Rust binaries and invokes Inno Setup compiler.
# No dependency downloads — assumes system Node.js, Git, npm.
# Usage: powershell -ExecutionPolicy Bypass -File build-installer.ps1
# ============================================================================
param(
    [switch]$SkipRustBuild,
    [string]$InnoSetupPath = "D:\InnoSetup6\ISCC.exe"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$BuildDir = Join-Path $ScriptDir "build"
$OutputDir = Join-Path $ScriptDir "output"

function Write-Step { param([string]$msg) Write-Host "`n=== $msg ===" -ForegroundColor Cyan }

# --- Create directories ---
New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

# ============================================================================
# Step 1: Build Rust binaries
# ============================================================================
if (-not $SkipRustBuild) {
    Write-Step "Building Rust binaries"
    Push-Location $ProjectRoot

    Write-Host "  Building frontend..."
    & pnpm install
    Push-Location (Join-Path $ProjectRoot "frontend")
    & pnpm build
    Pop-Location

    Write-Host "  Building solodawn-server..."
    & cargo build --release -p server
    if ($LASTEXITCODE -ne 0) { throw "cargo build -p server failed (exit=$LASTEXITCODE)" }
    $ServerSrc = "target\release\server.exe"
    if (-not (Test-Path $ServerSrc)) { throw "Missing artifact: $ServerSrc" }
    Copy-Item $ServerSrc (Join-Path $BuildDir "solodawn-server.exe") -Force -ErrorAction Stop

    Write-Host "  Building solodawn-tray..."
    & cargo build --release -p solodawn-tray
    if ($LASTEXITCODE -ne 0) { throw "cargo build -p solodawn-tray failed (exit=$LASTEXITCODE)" }
    $TraySrc = "target\release\solodawn-tray.exe"
    if (-not (Test-Path $TraySrc)) { throw "Missing artifact: $TraySrc" }
    Copy-Item $TraySrc (Join-Path $BuildDir "solodawn-tray.exe") -Force -ErrorAction Stop

    Pop-Location
}

# ============================================================================
# Step 2: Run Inno Setup compiler
# ============================================================================
Write-Step "Building installer"

if (-not (Test-Path $InnoSetupPath)) {
    Write-Host "[ERROR] Inno Setup compiler not found at: $InnoSetupPath" -ForegroundColor Red
    Write-Host "Install Inno Setup 6 from https://jrsoftware.org/isdl.php"
    exit 1
}

$IssFile = Join-Path $ScriptDir "solodawn.iss"
Write-Host "  Running ISCC: $IssFile"
& $InnoSetupPath $IssFile

if ($LASTEXITCODE -eq 0) {
    $OutputExe = Get-ChildItem $OutputDir -Filter "*.exe" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    Write-Host "`n[SUCCESS] Installer built: $($OutputExe.FullName)" -ForegroundColor Green
    Write-Host "Size: $([math]::Round($OutputExe.Length / 1MB, 1)) MB"
} else {
    Write-Host "`n[ERROR] Inno Setup compilation failed with exit code $LASTEXITCODE" -ForegroundColor Red
    exit 1
}
