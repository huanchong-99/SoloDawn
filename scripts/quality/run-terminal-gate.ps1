#Requires -Version 5.1
param(
    [string]$WorkingDir = "",
    [string]$ChangedFiles = ""
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "../..")

if ([string]::IsNullOrEmpty($WorkingDir)) {
    Write-Host "Error: -WorkingDir is required"
    exit 1
}

if ([string]::IsNullOrEmpty($ChangedFiles)) {
    Write-Host "Error: -ChangedFiles is required"
    exit 1
}

Write-Host "=== SoloDawn Terminal Quality Gate ==="
Write-Host "Project root: $ProjectRoot"
Write-Host "Working dir:  $WorkingDir"
Write-Host "Changed files: $ChangedFiles"
Write-Host ""

Set-Location $ProjectRoot

cargo run --package quality -- `
  --tier terminal `
  --config quality/quality-gate.yaml `
  --working-dir "$WorkingDir" `
  --changed-files "$ChangedFiles"

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "Terminal quality gate passed."
} else {
    Write-Host ""
    Write-Host "Terminal quality gate failed."
    exit $LASTEXITCODE
}
