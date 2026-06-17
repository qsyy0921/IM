$ErrorActionPreference = "Stop"

$writer = Join-Path $PSScriptRoot "write-observability-smoke-summary.ps1"
$validator = Join-Path $PSScriptRoot "validate-observability-smoke-summary.ps1"

if (-not (Test-Path -LiteralPath $writer -PathType Leaf)) {
    throw "Missing observability smoke summary writer: $writer"
}
if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing observability smoke summary validator: $validator"
}

function Invoke-Writer {
    param(
        [string]$OutputDir,
        [int]$RuleGroupCount,
        [string[]]$ExpectedDashboardUids,
        [string[]]$FoundDashboardUids,
        [bool]$AlertmanagerChecked = $false,
        [string[]]$ActiveAlertmanagerUrls = @()
    )

    try {
        $output = & $writer `
            -OutputDir $OutputDir `
            -RunName "observability-summary-selftest" `
            -RuleGroupCount $RuleGroupCount `
            -ExpectedDashboardUids $ExpectedDashboardUids `
            -FoundDashboardUids $FoundDashboardUids `
            -AlertmanagerChecked $AlertmanagerChecked `
            -ActiveAlertmanagerUrls $ActiveAlertmanagerUrls 2>&1
        return [pscustomobject]@{
            ExitCode = 0
            Output = (($output | Out-String).Trim())
        }
    }
    catch {
        return [pscustomobject]@{
            ExitCode = 1
            Output = [string]$_.Exception.Message
        }
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-observability-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodDir = Join-Path $tempRoot "good"
    $goodResult = Invoke-Writer `
        -OutputDir $goodDir `
        -RuleGroupCount 2 `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -FoundDashboardUids @("nexusim-message-service", "nexusim-api-gateway")
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL observability summary fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summaryPath = Join-Path $goodDir "observability-smoke-summary.json"
    $reportPath = Join-Path $goodDir "observability-smoke-report.md"
    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    if (-not $summary.prometheus_ready -or -not $summary.grafana_ready) {
        Write-Host "FAIL observability summary did not mark Prometheus/Grafana ready." -ForegroundColor Red
        exit 1
    }
    if ($summary.prometheus_rule_group_count -ne 2 -or $summary.dashboard_count.expected -ne 2 -or $summary.dashboard_count.found -ne 2) {
        Write-Host "FAIL observability summary produced wrong counts." -ForegroundColor Red
        exit 1
    }
    if ($summary.alertmanager_checked -or $summary.alertmanager_ready) {
        Write-Host "FAIL observability summary should mark Alertmanager unchecked by default." -ForegroundColor Red
        exit 1
    }
    if ($summary.scope -notmatch "not a production SLO") {
        Write-Host "FAIL observability summary scope must state non-production SLO boundary." -ForegroundColor Red
        exit 1
    }

    $report = Get-Content -LiteralPath $reportPath -Raw
    if (-not $report.Contains("2/2 dashboards found") -or -not $report.Contains("Alertmanager: not checked") -or -not $report.Contains("not a production SLO")) {
        Write-Host "FAIL observability summary markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }
    & $validator `
        -SummaryPath $summaryPath `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") | Out-Null

    $alertDir = Join-Path $tempRoot "alertmanager"
    $alertResult = Invoke-Writer `
        -OutputDir $alertDir `
        -RuleGroupCount 2 `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -FoundDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -AlertmanagerChecked $true `
        -ActiveAlertmanagerUrls @("http://host.docker.internal:19093")
    if ($alertResult.ExitCode -ne 0) {
        Write-Host "FAIL alertmanager fixture should pass." -ForegroundColor Red
        if ($alertResult.Output) {
            Write-Host $alertResult.Output -ForegroundColor Red
        }
        exit 1
    }
    $alertSummary = Get-Content -LiteralPath (Join-Path $alertDir "observability-smoke-summary.json") -Raw | ConvertFrom-Json
    if (-not $alertSummary.alertmanager_checked -or -not $alertSummary.alertmanager_ready -or $alertSummary.active_alertmanager_urls.Count -ne 1) {
        Write-Host "FAIL alertmanager fixture produced wrong alertmanager status." -ForegroundColor Red
        exit 1
    }
    & $validator `
        -SummaryPath (Join-Path $alertDir "observability-smoke-summary.json") `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -RequireAlertmanager | Out-Null

    $badAlertDir = Join-Path $tempRoot "bad-alertmanager"
    $badAlertResult = Invoke-Writer `
        -OutputDir $badAlertDir `
        -RuleGroupCount 2 `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -FoundDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -AlertmanagerChecked $true
    if ($badAlertResult.ExitCode -eq 0) {
        Write-Host "FAIL alertmanager checked without active target should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badAlertResult.Output.Contains("AlertmanagerChecked requires")) {
        Write-Host "FAIL alertmanager checked fixture did not report missing active target." -ForegroundColor Red
        if ($badAlertResult.Output) {
            Write-Host $badAlertResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $missingDir = Join-Path $tempRoot "missing"
    $missingResult = Invoke-Writer `
        -OutputDir $missingDir `
        -RuleGroupCount 2 `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -FoundDashboardUids @("nexusim-api-gateway")
    if ($missingResult.ExitCode -eq 0) {
        Write-Host "FAIL missing dashboard fixture should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $missingResult.Output.Contains("Missing Grafana dashboard")) {
        Write-Host "FAIL missing dashboard fixture did not report missing dashboard." -ForegroundColor Red
        if ($missingResult.Output) {
            Write-Host $missingResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $ruleDir = Join-Path $tempRoot "rules"
    $ruleResult = Invoke-Writer `
        -OutputDir $ruleDir `
        -RuleGroupCount 1 `
        -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") `
        -FoundDashboardUids @("nexusim-api-gateway", "nexusim-message-service")
    if ($ruleResult.ExitCode -eq 0) {
        Write-Host "FAIL low rule group count fixture should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $ruleResult.Output.Contains("rule group count")) {
        Write-Host "FAIL low rule group count fixture did not report rule group count." -ForegroundColor Red
        if ($ruleResult.Output) {
            Write-Host $ruleResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $tamperedScopeDir = Join-Path $tempRoot "tampered-scope"
    New-Item -ItemType Directory -Force -Path $tamperedScopeDir | Out-Null
    $tamperedScopePath = Join-Path $tamperedScopeDir "observability-smoke-summary.json"
    Copy-Item -LiteralPath $summaryPath -Destination $tamperedScopePath
    $tamperedScope = Get-Content -LiteralPath $tamperedScopePath -Raw | ConvertFrom-Json
    $tamperedScope.scope = "production SLO"
    $tamperedScope | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $tamperedScopePath -Encoding UTF8
    try {
        & $validator `
            -SummaryPath $tamperedScopePath `
            -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") | Out-Null
        Write-Host "FAIL validator should reject production SLO scope." -ForegroundColor Red
        exit 1
    }
    catch {
        if (-not ([string]$_.Exception.Message).Contains("not a production SLO")) {
            Write-Host "FAIL validator rejected production SLO scope with unexpected message." -ForegroundColor Red
            Write-Host ([string]$_.Exception.Message) -ForegroundColor Red
            exit 1
        }
    }

    $tamperedDashboardDir = Join-Path $tempRoot "tampered-dashboard"
    New-Item -ItemType Directory -Force -Path $tamperedDashboardDir | Out-Null
    $tamperedDashboardPath = Join-Path $tamperedDashboardDir "observability-smoke-summary.json"
    Copy-Item -LiteralPath $summaryPath -Destination $tamperedDashboardPath
    $tamperedDashboard = Get-Content -LiteralPath $tamperedDashboardPath -Raw | ConvertFrom-Json
    $tamperedDashboard.found_dashboard_uids = @("nexusim-api-gateway")
    $tamperedDashboard.dashboard_count.found = 1
    $tamperedDashboard | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $tamperedDashboardPath -Encoding UTF8
    try {
        & $validator `
            -SummaryPath $tamperedDashboardPath `
            -ExpectedDashboardUids @("nexusim-api-gateway", "nexusim-message-service") | Out-Null
        Write-Host "FAIL validator should reject missing found dashboard." -ForegroundColor Red
        exit 1
    }
    catch {
        if (-not ([string]$_.Exception.Message).Contains("found_dashboard_uids")) {
            Write-Host "FAIL validator rejected missing found dashboard with unexpected message." -ForegroundColor Red
            Write-Host ([string]$_.Exception.Message) -ForegroundColor Red
            exit 1
        }
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   observability smoke summary self-test"
