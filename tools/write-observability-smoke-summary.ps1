param(
    [Parameter(Mandatory = $true)]
    [string]$OutputDir,
    [Parameter(Mandatory = $true)]
    [string]$RunName,
    [Parameter(Mandatory = $true)]
    [int]$RuleGroupCount,
    [Parameter(Mandatory = $true)]
    [string[]]$ExpectedDashboardUids,
    [Parameter(Mandatory = $true)]
    [string[]]$FoundDashboardUids,
    [bool]$AlertmanagerChecked = $false,
    [string[]]$ActiveAlertmanagerUrls = @(),
    [string]$Scope = "local Prometheus/Grafana provisioning smoke; not a production SLO or Alertmanager validation"
)

$ErrorActionPreference = "Stop"

function Convert-ToUniqueSortedStrings {
    param([string[]]$Values)

    $set = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($value in @($Values)) {
        $text = ([string]$value).Trim()
        if ($text.Length -gt 0) {
            [void]$set.Add($text)
        }
    }

    return @($set | Sort-Object)
}

if ($RunName.Trim().Length -eq 0) {
    throw "RunName is required."
}

$outputPath = [System.IO.Path]::GetFullPath($OutputDir)
$expected = Convert-ToUniqueSortedStrings -Values $ExpectedDashboardUids
$found = Convert-ToUniqueSortedStrings -Values $FoundDashboardUids
$activeAlertmanagers = Convert-ToUniqueSortedStrings -Values $ActiveAlertmanagerUrls

if ($expected.Count -eq 0) {
    throw "ExpectedDashboardUids must not be empty."
}
if ($RuleGroupCount -lt $expected.Count) {
    throw "Prometheus rule group count $RuleGroupCount is lower than expected dashboard count $($expected.Count)."
}

$foundSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($uid in $found) {
    [void]$foundSet.Add($uid)
}

$missing = @($expected | Where-Object { -not $foundSet.Contains($_) })
if ($missing.Count -gt 0) {
    throw "Missing Grafana dashboard uid(s): $($missing -join ', ')"
}
if ($AlertmanagerChecked -and $activeAlertmanagers.Count -eq 0) {
    throw "AlertmanagerChecked requires at least one active Alertmanager URL."
}

New-Item -ItemType Directory -Force -Path $outputPath | Out-Null

$summary = [pscustomobject]@{
    run_name = $RunName
    created_at = (Get-Date).ToUniversalTime().ToString("o")
    result_dir = $outputPath
    scope = $Scope
    prometheus_ready = $true
    prometheus_rule_group_count = $RuleGroupCount
    grafana_ready = $true
    alertmanager_checked = [bool]$AlertmanagerChecked
    alertmanager_ready = [bool]($AlertmanagerChecked -and $activeAlertmanagers.Count -gt 0)
    active_alertmanager_urls = $activeAlertmanagers
    expected_dashboard_uids = $expected
    found_dashboard_uids = $found
    missing_dashboard_uids = $missing
    dashboard_count = [pscustomobject]@{
        expected = $expected.Count
        found = $found.Count
    }
}

$jsonPath = Join-Path $outputPath "observability-smoke-summary.json"
$markdownPath = Join-Path $outputPath "observability-smoke-report.md"

$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $jsonPath -Encoding UTF8

$markdown = @()
$markdown += "# Local Observability Smoke"
$markdown += ""
$markdown += "- Run: $RunName"
$markdown += "- Created at: $($summary.created_at)"
$markdown += "- Scope: $Scope"
$markdown += "- Prometheus: ready, $RuleGroupCount rule groups loaded"
$markdown += "- Grafana: ready, $($found.Count)/$($expected.Count) dashboards found"
if ($AlertmanagerChecked) {
    $markdown += "- Alertmanager: ready, active targets: $($activeAlertmanagers -join ', ')"
}
else {
    $markdown += "- Alertmanager: not checked in this run"
}
$markdown += ""
$markdown += "## Dashboard UIDs"
$markdown += ""
foreach ($uid in $expected) {
    $markdown += "- $uid"
}
$markdown += ""
$markdown += "This report is local provisioning evidence only. It is not a production SLO, Alertmanager, retention, or production dashboard validation."

$markdown | Set-Content -LiteralPath $markdownPath -Encoding UTF8

Write-Host "OK   observability smoke summary written: $jsonPath"
Write-Host "OK   observability smoke report written: $markdownPath"
