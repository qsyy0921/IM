param(
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
    [string[]]$ExpectedDashboardUids = @(),
    [switch]$RequireAlertmanager,
    [string]$OutputPath = ""
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

function Convert-JsonArrayToStrings {
    param($Values)

    if ($null -eq $Values) {
        return @()
    }

    return @(foreach ($value in @($Values)) {
        $text = ([string]$value).Trim()
        if ($text.Length -gt 0) {
            $text
        }
    })
}

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

$resolvedSummaryPath = [System.IO.Path]::GetFullPath($SummaryPath)
Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "SummaryPath does not exist: $resolvedSummaryPath"

$summary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json

$expectedFromSummary = Convert-JsonArrayToStrings -Values $summary.expected_dashboard_uids
$expectedFromCaller = Convert-ToUniqueSortedStrings -Values $ExpectedDashboardUids
if ($expectedFromCaller.Count -gt 0) {
    $expected = $expectedFromCaller
}
else {
    $expected = Convert-ToUniqueSortedStrings -Values $expectedFromSummary
}

$found = Convert-ToUniqueSortedStrings -Values (Convert-JsonArrayToStrings -Values $summary.found_dashboard_uids)
$missing = Convert-ToUniqueSortedStrings -Values (Convert-JsonArrayToStrings -Values $summary.missing_dashboard_uids)
$activeAlertmanagers = Convert-ToUniqueSortedStrings -Values (Convert-JsonArrayToStrings -Values $summary.active_alertmanager_urls)

Assert-Condition ($expected.Count -gt 0) "observability summary must contain expected_dashboard_uids or caller ExpectedDashboardUids."
Assert-Condition ([bool]$summary.prometheus_ready) "observability summary must mark prometheus_ready=true."
Assert-Condition ([bool]$summary.grafana_ready) "observability summary must mark grafana_ready=true."
Assert-Condition ([int]$summary.prometheus_rule_group_count -ge $expected.Count) "Prometheus rule group count $($summary.prometheus_rule_group_count) is lower than expected dashboard count $($expected.Count)."
Assert-Condition ($missing.Count -eq 0) "observability summary must not have missing_dashboard_uids: $($missing -join ', ')"
Assert-Condition (([string]$summary.scope) -match "not a production SLO") "observability summary scope must state it is not a production SLO."

$expectedSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($uid in $expected) {
    [void]$expectedSet.Add($uid)
}

$summaryExpectedSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($uid in $expectedFromSummary) {
    [void]$summaryExpectedSet.Add($uid)
}

$foundSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($uid in $found) {
    [void]$foundSet.Add($uid)
}

foreach ($uid in $expected) {
    Assert-Condition ($summaryExpectedSet.Contains($uid)) "observability summary expected_dashboard_uids is missing expected uid: $uid"
    Assert-Condition ($foundSet.Contains($uid)) "observability summary found_dashboard_uids is missing expected uid: $uid"
}

Assert-Condition ([int]$summary.dashboard_count.expected -eq $expected.Count) "observability summary dashboard_count.expected must equal expected dashboard count $($expected.Count)."
Assert-Condition ([int]$summary.dashboard_count.found -ge $expected.Count) "observability summary dashboard_count.found must be at least expected dashboard count $($expected.Count)."

if ($RequireAlertmanager) {
    Assert-Condition ([bool]$summary.alertmanager_checked) "observability summary must mark alertmanager_checked=true when RequireAlertmanager is set."
    Assert-Condition ([bool]$summary.alertmanager_ready) "observability summary must mark alertmanager_ready=true when RequireAlertmanager is set."
    Assert-Condition ($activeAlertmanagers.Count -gt 0) "observability summary must contain active_alertmanager_urls when RequireAlertmanager is set."
}

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    summary_path = $resolvedSummaryPath
    run_name = [string]$summary.run_name
    dashboard_count = $expected.Count
    prometheus_rule_group_count = [int]$summary.prometheus_rule_group_count
    alertmanager_checked = [bool]$summary.alertmanager_checked
    valid = $true
    scope = "local observability summary validation; not a production SLO"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = [System.IO.Path]::GetFullPath($OutputPath)
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   observability smoke summary validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 4
}
