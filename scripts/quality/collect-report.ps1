#Requires -Version 5.1
param(
    [switch]$Json,
    [int]$Limit = 10
)

$ErrorActionPreference = "Stop"

$ApiBase = "http://localhost:23456/api"

Write-Host "=== SoloDawn Quality Report ==="
Write-Host ""

$Response = Invoke-RestMethod -Uri "${ApiBase}/quality/runs?limit=${Limit}" -Method Get

if ($Json) {
    $Response | ConvertTo-Json -Depth 10
    exit 0
}

# Format as summary table
$Header = "{0,-38} {1,-12} {2,-14} {3,-20}" -f "RUN_ID", "STATUS", "ISSUES_COUNT", "CREATED_AT"
$Separator = "{0,-38} {1,-12} {2,-14} {3,-20}" -f "------", "------", "------------", "----------"
Write-Host $Header
Write-Host $Separator

$Runs = if ($Response -is [array]) { $Response } elseif ($Response.runs) { $Response.runs } elseif ($Response.data) { $Response.data } else { @() }

foreach ($Run in $Runs) {
    $RunId = if ($Run.run_id) { $Run.run_id } elseif ($Run.id) { $Run.id } else { "N/A" }
    $Status = if ($Run.status) { $Run.status } else { "N/A" }
    $Issues = if ($null -ne $Run.issues_count) { $Run.issues_count } elseif ($null -ne $Run.issue_count) { $Run.issue_count } else { 0 }
    $Created = if ($Run.created_at) { $Run.created_at } else { "N/A" }

    $Line = "{0,-38} {1,-12} {2,-14} {3,-20}" -f $RunId, $Status, $Issues, $Created
    Write-Host $Line
}
