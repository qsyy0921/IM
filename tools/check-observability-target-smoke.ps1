$ErrorActionPreference = "Stop"

$runner = Join-Path $PSScriptRoot "run-observability-target-smoke.ps1"

if (-not (Test-Path -LiteralPath $runner -PathType Leaf)) {
    throw "Missing observability target smoke runner: $runner"
}

$expectedDashboardUids = @(
    "nexusim-api-gateway",
    "nexusim-contacts-service",
    "nexusim-conversation-service",
    "nexusim-delivery-service",
    "nexusim-identity-service",
    "nexusim-message-service",
    "nexusim-policy-service",
    "nexusim-push-gateway",
    "nexusim-receipt-service"
)

function New-FixtureDir {
    param(
        [string]$Root,
        [string]$Name,
        [string[]]$DashboardUids,
        [int]$RuleGroupCount,
        [bool]$IncludeAlertmanager = $false
    )

    $dir = Join-Path $Root $Name
    New-Item -ItemType Directory -Force -Path $dir | Out-Null

    $groups = @()
    for ($i = 0; $i -lt $RuleGroupCount; $i++) {
        $groups += [pscustomobject]@{ name = "nexusim-rule-group-$i" }
    }
    [pscustomobject]@{
        status = "success"
        data = [pscustomobject]@{ groups = $groups }
    } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $dir "prometheus-rules.json") -Encoding UTF8

    $dashboards = @($DashboardUids | ForEach-Object {
        [pscustomobject]@{
            uid = $_
            title = $_
            type = "dash-db"
        }
    })
    $dashboards | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $dir "grafana-search.json") -Encoding UTF8

    if ($IncludeAlertmanager) {
        [pscustomobject]@{
            status = "success"
            data = [pscustomobject]@{
                activeAlertmanagers = @(
                    [pscustomobject]@{ url = "http://alertmanager.target.local:9093" }
                )
            }
        } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $dir "prometheus-alertmanagers.json") -Encoding UTF8
    }

    return $dir
}

function Invoke-Runner {
    param(
        [string]$ResultRoot,
        [string]$RunName,
        [string]$FixtureDir,
        [switch]$IncludeAlertmanager
    )

    try {
        $output = & $runner `
            -FixtureDir $FixtureDir `
            -ResultRoot $ResultRoot `
            -RunName $RunName `
            -ExpectedDashboardUids $expectedDashboardUids `
            -IncludeAlertmanager:$IncludeAlertmanager 2>&1
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-observability-target-smoke-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $resultRoot = Join-Path $tempRoot "results"
    $goodFixture = New-FixtureDir `
        -Root $tempRoot `
        -Name "good" `
        -DashboardUids $expectedDashboardUids `
        -RuleGroupCount $expectedDashboardUids.Count
    $good = Invoke-Runner `
        -ResultRoot $resultRoot `
        -RunName "target-observability-good" `
        -FixtureDir $goodFixture
    if ($good.ExitCode -ne 0) {
        Write-Host "FAIL target observability fixture should pass." -ForegroundColor Red
        Write-Host $good.Output -ForegroundColor Red
        exit 1
    }
    $summaryPath = Join-Path $resultRoot "target-observability-good\observability-smoke-summary.json"
    $validationPath = Join-Path $resultRoot "target-observability-good\observability-smoke-validation.json"
    $reportPath = Join-Path $resultRoot "target-observability-good\observability-smoke-report.md"
    foreach ($path in @($summaryPath, $validationPath, $reportPath)) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            Write-Host "FAIL target observability runner did not write expected file: $path" -ForegroundColor Red
            exit 1
        }
    }
    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    if ($summary.dashboard_count.expected -ne 9 -or $summary.dashboard_count.found -ne 9 -or $summary.scope -notmatch "not a production SLO") {
        Write-Host "FAIL target observability summary has wrong dashboard count or scope." -ForegroundColor Red
        exit 1
    }

    $alertFixture = New-FixtureDir `
        -Root $tempRoot `
        -Name "alertmanager" `
        -DashboardUids $expectedDashboardUids `
        -RuleGroupCount $expectedDashboardUids.Count `
        -IncludeAlertmanager $true
    $alert = Invoke-Runner `
        -ResultRoot $resultRoot `
        -RunName "target-observability-alertmanager" `
        -FixtureDir $alertFixture `
        -IncludeAlertmanager
    if ($alert.ExitCode -ne 0) {
        Write-Host "FAIL target observability Alertmanager fixture should pass." -ForegroundColor Red
        Write-Host $alert.Output -ForegroundColor Red
        exit 1
    }
    $alertSummary = Get-Content -LiteralPath (Join-Path $resultRoot "target-observability-alertmanager\observability-smoke-summary.json") -Raw | ConvertFrom-Json
    if (-not $alertSummary.alertmanager_checked -or -not $alertSummary.alertmanager_ready -or $alertSummary.active_alertmanager_urls.Count -ne 1) {
        Write-Host "FAIL target observability Alertmanager summary has wrong status." -ForegroundColor Red
        exit 1
    }

    $missingFixture = New-FixtureDir `
        -Root $tempRoot `
        -Name "missing-dashboard" `
        -DashboardUids @($expectedDashboardUids | Where-Object { $_ -ne "nexusim-policy-service" }) `
        -RuleGroupCount $expectedDashboardUids.Count
    $missing = Invoke-Runner `
        -ResultRoot $resultRoot `
        -RunName "target-observability-missing" `
        -FixtureDir $missingFixture
    if ($missing.ExitCode -eq 0) {
        Write-Host "FAIL target observability runner should reject missing dashboard." -ForegroundColor Red
        exit 1
    }
    if (-not $missing.Output.Contains("Missing Grafana dashboard")) {
        Write-Host "FAIL missing dashboard fixture returned unexpected error." -ForegroundColor Red
        Write-Host $missing.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   observability target smoke self-test"
