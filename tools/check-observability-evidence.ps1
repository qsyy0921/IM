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
        [string]$ExpectedResultRoot = "",
        [string]$ReportRoot = "",
        [switch]$RequireFiles,
        [string]$MarkdownPath = ""
    )

    try {
        $invocationArgs = @{
            ManifestPath = $ManifestPath
        }
        if ($ExpectedResultRoot.Trim().Length -gt 0) {
            $invocationArgs.ExpectedResultRoot = $ExpectedResultRoot
        }
        if ($ReportRoot.Trim().Length -gt 0) {
            $invocationArgs.ReportRoot = $ReportRoot
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

$repoRoot = Split-Path -Parent $PSScriptRoot
$repoManifestObject = Get-Content -LiteralPath $repoManifest -Raw | ConvertFrom-Json
$hasImagePrepareEvidence = @($repoManifestObject.entries | Where-Object { $_.kind -eq "observability-image-prepare-plan" }).Count -gt 0
if ($hasImagePrepareEvidence) {
    $progressDoc = Join-Path $repoRoot "docs\runbook\development-progress.md"
    $localDoc = Join-Path $repoRoot "docs\runbook\observability-local.md"
    foreach ($doc in @($progressDoc, $localDoc)) {
        if (-not (Test-Path -LiteralPath $doc -PathType Leaf)) {
            Write-Host "FAIL missing observability progress document: $doc" -ForegroundColor Red
            exit 1
        }
        $content = Get-Content -LiteralPath $doc -Raw
        if (-not $content.Contains("observability-image-prepare-plan")) {
            Write-Host "FAIL observability evidence wording should mention observability-image-prepare-plan in $doc." -ForegroundColor Red
            exit 1
        }
    }
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
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -ExpectedResultRoot $tempRoot -ReportRoot $tempRoot -RequireFiles -MarkdownPath $markdownPath
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

    $repoLocalSummaryManifestPath = Join-Path $tempRoot "repo-local-observability-evidence.json"
    Write-JsonFile -Path $repoLocalSummaryManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "repo-local summary fixture"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = "docs/runbook/loadtest/policy-service/policy-smoke-summary.json"
                report_path = $reportPath
                note = "fixture"
            }
        )
    })
    $repoLocalSummaryResult = Invoke-Validator -ManifestPath $repoLocalSummaryManifestPath -ExpectedResultRoot $tempRoot -ReportRoot $tempRoot
    if ($repoLocalSummaryResult.ExitCode -eq 0) {
        Write-Host "FAIL observability evidence with repo-local summary_path should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $repoLocalSummaryResult.Output.Contains("must point under")) {
        Write-Host "FAIL repo-local observability summary path fixture returned unexpected error." -ForegroundColor Red
        Write-Host $repoLocalSummaryResult.Output -ForegroundColor Red
        exit 1
    }

    $externalReportManifestPath = Join-Path $tempRoot "external-report-observability-evidence.json"
    $externalReportRoot = Join-Path $tempRoot "external-report-root"
    $externalReportPath = Join-Path $externalReportRoot "observability-report.md"
    New-Item -ItemType Directory -Force -Path $externalReportRoot | Out-Null
    @(
        "# Fixture observability report",
        "",
        "This verifies local debug metrics. It is not a production SLO or observability platform claim."
    ) | Set-Content -LiteralPath $externalReportPath -Encoding UTF8
    Write-JsonFile -Path $externalReportManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "external report fixture"
                kind = "service-debug-smoke"
                service = "policy-service"
                summary_path = $summaryPath
                report_path = $externalReportPath
                note = "fixture"
            }
        )
    })
    $externalReportResult = Invoke-Validator -ManifestPath $externalReportManifestPath -ExpectedResultRoot $tempRoot -ReportRoot (Join-Path $tempRoot "report-root")
    if ($externalReportResult.ExitCode -eq 0) {
        Write-Host "FAIL observability evidence with service report outside ReportRoot should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $externalReportResult.Output.Contains("must stay under")) {
        Write-Host "FAIL external observability report path fixture returned unexpected error." -ForegroundColor Red
        Write-Host $externalReportResult.Output -ForegroundColor Red
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
            -ExpectedResultRoot $tempRoot `
            -ReportRoot $tempRoot `
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
    $afterAddResult = Invoke-Validator -ManifestPath $manifestPath -ExpectedResultRoot $tempRoot -ReportRoot $tempRoot -RequireFiles
    if ($afterAddResult.ExitCode -ne 0) {
        Write-Host "FAIL manifest after add should validate with files." -ForegroundColor Red
        Write-Host $afterAddResult.Output -ForegroundColor Red
        exit 1
    }

    $imagePlanDir = Join-Path $tempRoot "image-plan"
    New-Item -ItemType Directory -Force -Path $imagePlanDir | Out-Null
    $imagePlanPath = Join-Path $imagePlanDir "observability-image-prepare-plan.json"
    $imagePlanReportPath = Join-Path $imagePlanDir "observability-image-prepare-plan.md"
    Write-JsonFile -Path $imagePlanPath -Value ([ordered]@{
        generated_at_utc = (Get-Date).ToUniversalTime().ToString("o")
        docker_available = $true
        include_alertmanager = $true
        allow_image_pull = $false
        platform = "linux/arm64"
        missing_count = 3
        images = @(
            [ordered]@{ name = "prometheus"; image = "fixture/prometheus:missing"; status = "missing"; pull_command = "docker pull --platform linux/arm64 fixture/prometheus:missing" },
            [ordered]@{ name = "grafana"; image = "fixture/grafana:missing"; status = "missing"; pull_command = "docker pull --platform linux/arm64 fixture/grafana:missing" },
            [ordered]@{ name = "alertmanager"; image = "fixture/alertmanager:missing"; status = "missing"; pull_command = "docker pull --platform linux/arm64 fixture/alertmanager:missing" }
        )
        boundary = "local observability image preparation only; does not start containers or prove production observability"
    })
    @(
        "# NexusIM Observability Image Prepare Plan",
        "",
        "This is a local observability image preparation plan. It is not a production SLO claim.",
        "",
        "| Image role | Image | Status | Pull command |",
        "| --- | --- | --- | --- |",
        "| prometheus | `fixture/prometheus:missing` | missing | `docker pull --platform linux/arm64 fixture/prometheus:missing` |",
        "| grafana | `fixture/grafana:missing` | missing | `docker pull --platform linux/arm64 fixture/grafana:missing` |",
        "| alertmanager | `fixture/alertmanager:missing` | missing | `docker pull --platform linux/arm64 fixture/alertmanager:missing` |"
    ) | Set-Content -LiteralPath $imagePlanReportPath -Encoding UTF8

    try {
        & $adder `
            -ManifestPath $manifestPath `
            -ExpectedResultRoot $tempRoot `
            -ReportRoot $tempRoot `
            -Name "image prepare fixture" `
            -Kind "observability-image-prepare-plan" `
            -SummaryPath $imagePlanPath `
            -ReportPath $imagePlanReportPath `
            -Note "fixture local observability image preparation evidence" 2>&1 | Out-Null
    }
    catch {
        Write-Host "FAIL add-observability-evidence.ps1 should append image prepare fixture." -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }

    $afterImagePlanResult = Invoke-Validator -ManifestPath $manifestPath -ExpectedResultRoot $tempRoot -ReportRoot $tempRoot -RequireFiles
    if ($afterImagePlanResult.ExitCode -ne 0) {
        Write-Host "FAIL manifest after image prepare add should validate with files." -ForegroundColor Red
        Write-Host $afterImagePlanResult.Output -ForegroundColor Red
        exit 1
    }

    $duplicateAddFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -ExpectedResultRoot $tempRoot `
            -ReportRoot $tempRoot `
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

    $sensitiveAddFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -ExpectedResultRoot $tempRoot `
            -ReportRoot $tempRoot `
            -Name "sensitive target fixture" `
            -Kind "prometheus-grafana-smoke" `
            -SummaryPath $targetSummaryPath `
            -ReportPath $reportPath `
            -ExpectedDashboardCount 9 `
            -Note "Bearer abcdefghijklmnop" 2>$null | Out-Null
    }
    catch {
        $sensitiveAddFailed = ($_.Exception.Message -match "low-sensitive evidence metadata")
    }
    if (-not $sensitiveAddFailed) {
        Write-Host "FAIL add-observability-evidence.ps1 should reject sensitive metadata." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   observability evidence self-test"
