#Requires -Version 5.1
param(
    [string]$Tier = "repo",
    [string]$Mode = "shadow",
    [switch]$IncludeBaseline,
    [switch]$IncludeSecurity,
    [switch]$All
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "../..")

if ($All) {
    $IncludeBaseline = $true
    $IncludeSecurity = $true
}

Write-Host "=== SoloDawn Quality Gate ==="
Write-Host "Project root: $ProjectRoot"
Write-Host "Tier: $Tier"
Write-Host "Mode: $Mode"
Write-Host ""

Set-Location $ProjectRoot

# Run the quality engine via cargo
cargo run --package quality -- `
  --project-root "$ProjectRoot" `
  --tier $Tier `
  --mode $Mode

if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "Quality gate failed with exit code $LASTEXITCODE."
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "Quality gate completed successfully."

# Run baseline verification if requested
if ($IncludeBaseline) {
    Write-Host ""
    Write-Host "=== Running Baseline Verification ==="
    & bash "$ProjectRoot/scripts/verify-baseline.sh"
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

# Run security audit if requested
if ($IncludeSecurity) {
    Write-Host ""
    Write-Host "=== Running Security Audit ==="
    & bash "$ProjectRoot/scripts/audit-security.sh"
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
