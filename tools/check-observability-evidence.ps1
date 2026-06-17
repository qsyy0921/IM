$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-observability-evidence.ps1"
$adder = Join-Path $PSScriptRoot "add-observability-evidence.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing observability evidence validator: $validator"
}
if (-not (Test-Path -LiteralPath $adder -PathType Leaf)) {
    throw "Missing observability evidence adder: $adder"
}

function Write-JsonFile {
    param(
        [string]$Path,
        $Value
    )

    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $Value | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Validator {
    param(
        [string]$ManifestPath,
        [switch]$RequireFiles,
        [string]$MarkdownPath = ""
    )

    try {
        $invocationArgs = @{
            ManifestPath = $ManifestPath
        }
        if ($RequireFiles) {
            $invocationArgs.RequireFiles = $true
        }
        if ($MarkdownPath.Trim().Length -gt 0) {
            $invocationArgs.MarkdownPath = $MarkdownPath
        }
        $output = & $validator @invocationArgs 2>&1
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

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\observability-evidence.json"
$schemaOnlyResult = Invoke-Validator -ManifestPath $repoManifest
if ($schemaOnlyResult.ExitCode -ne 0) {
    Write-Host "FAIL repo observability evidence manifest schema should pass." -ForegroundColor Red
    Write-Host $schemaOnlyResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-observability-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $reportPath = Join-Path $tempRoot "observability-report.md"
    @(
        "# Fixture observability report",
        "",
        "This verifies local debug metrics. It is not a production SLO or observability platform claim."
    ) | Set-Content -LiteralPath $reportPath -Encoding UTF8

    $summaryPath = Join-Path $tempRoot "policy-smoke-summary.json"
    Write-JsonFile -Path $summaryPath -Value ([ordered]@{
        success = $true
        allow = [ordered]@{ git_dirty = $false }
        deny = [ordered]@{ git_dirty = $false }
        allow_debug_metrics = [ordered]@{
            service = "policy-service"
            grpc = [ordered]@{ total_requests = 4; total_errors = 0 }
            decisions = [ordered]@{ total = 4; allowed = 4; denied = 0; errors = 0 }
        }
        deny_debug_metrics = [ordered]@{
            service = "policy-service"
            grpc = [ordered]@{ total_requests = 4; total_errors = 0 }
            decisions = [ordered]@{ total = 4; allowed = 0; denied = 4; errors = 0 }
        }
    })

    $manifestPath = Join-Path $tempRoot "observability-evidence.json"
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "policy fixture"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $reportPath
                require_clean_git = $true
                note = "fixture"
            }
        )
    })

    $markdownPath = Join-Path $tempRoot "observability-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -RequireFiles -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL observability evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("NexusIM Observability Evidence") -or -not $markdown.Contains("policy fixture") -or -not $markdown.Contains("not a production SLO")) {
        Write-Host "FAIL observability evidence markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badManifestPath = Join-Path $tempRoot "bad-observability-evidence.json"
    Write-JsonFile -Path $badManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "bad duplicate"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $reportPath
                note = "fixture"
            },
            [ordered]@{
                name = "bad duplicate"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $reportPath
                note = "fixture"
            }
        )
    })
    $badResult = Invoke-Validator -ManifestPath $badManifestPath
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL duplicate observability evidence entries should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("duplicate")) {
        Write-Host "FAIL bad observability evidence fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }

    $targetSummaryPath = Join-Path $tempRoot "observability-smoke-summary.json"
    Write-JsonFile -Path $targetSummaryPath -Value ([ordered]@{
        run_name = "target-fixture"
        prometheus_ready = $true
        grafana_ready = $true
        prometheus_rule_group_count = 9
        expected_dashboard_uids = @(
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
        found_dashboard_uids = @(
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
        missing_dashboard_uids = @()
        dashboard_count = [ordered]@{ expected = 9; found = 9 }
        alertmanager_checked = $false
        alertmanager_ready = $false
        active_alertmanager_urls = @()
        scope = "target environment Prometheus/Grafana smoke evidence; not a production SLO or Alertmanager validation"
    })

    try {
        $addResultOutput = & $adder `
            -ManifestPath $manifestPath `
            -Name "target fixture" `
            -Kind "prometheus-grafana-smoke" `
            -SummaryPath $targetSummaryPath `
            -ReportPath $reportPath `
            -ExpectedDashboardCount 9 `
            -Note "fixture target observability evidence" 2>&1
    }
    catch {
        Write-Host "FAIL add-observability-evidence.ps1 should append target fixture." -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }
    $afterAdd = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if (@($afterAdd.entries).Count -ne 2 -or @($afterAdd.entries | Where-Object { $_.name -eq "target fixture" }).Count -ne 1) {
        Write-Host "FAIL add-observability-evidence.ps1 did not append expected entry." -ForegroundColor Red
        exit 1
    }
    $afterAddResult = Invoke-Validator -ManifestPath $manifestPath -RequireFiles
    if ($afterAddResult.ExitCode -ne 0) {
        Write-Host "FAIL manifest after add should validate with files." -ForegroundColor Red
        Write-Host $afterAddResult.Output -ForegroundColor Red
        exit 1
    }

    $duplicateAddFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -Name "target fixture" `
            -Kind "prometheus-grafana-smoke" `
            -SummaryPath $targetSummaryPath `
            -ReportPath $reportPath `
            -ExpectedDashboardCount 9 `
            -Note "fixture target observability evidence" 2>$null | Out-Null
    }
    catch {
        $duplicateAddFailed = ($_.Exception.Message -match "already exists")
    }
    if (-not $duplicateAddFailed) {
        Write-Host "FAIL add-observability-evidence.ps1 should reject duplicate names." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   observability evidence self-test"
