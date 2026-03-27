#Requires -Version 5.1
param(
    [string]$Branch = ""
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "../..")

Set-Location $ProjectRoot

# Default to current branch
if ([string]::IsNullOrEmpty($Branch)) {
    $Branch = git rev-parse --abbrev-ref HEAD
}

Write-Host "=== SoloDawn Branch Quality Gate ==="
Write-Host "Project root: $ProjectRoot"
Write-Host "Branch:       $Branch"
Write-Host ""

# Compute changed files vs main
$ChangedFiles = (git diff --name-only "main...$Branch") -join ","

if ([string]::IsNullOrEmpty($ChangedFiles)) {
    Write-Host "No changed files detected between main and $Branch."
    Write-Host "Branch quality gate passed (nothing to check)."
    exit 0
}

Write-Host "Changed files: $ChangedFiles"
Write-Host ""

cargo run --package quality -- `
  --tier branch `
  --config quality/quality-gate.yaml `
  --working-dir "$ProjectRoot" `
  --changed-files "$ChangedFiles"

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "Branch quality gate passed."
} else {
    Write-Host ""
    Write-Host "Branch quality gate failed."
    exit $LASTEXITCODE
}
