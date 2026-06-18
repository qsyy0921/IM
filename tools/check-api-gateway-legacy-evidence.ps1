$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-api-gateway-legacy-evidence.ps1"
$adder = Join-Path $PSScriptRoot "add-api-gateway-legacy-evidence.ps1"
$planWriter = Join-Path $PSScriptRoot "write-api-gateway-legacy-removal-plan.ps1"
$planValidator = Join-Path $PSScriptRoot "validate-api-gateway-legacy-removal-plan.ps1"

foreach ($tool in @($validator, $adder, $planWriter, $planValidator)) {
    if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) {
        throw "Missing api-gateway legacy evidence dependency: $tool"
    }
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
        [switch]$RequireFiles,
        [string]$MarkdownPath = ""
    )

    try {
        $args = @{ ManifestPath = $ManifestPath }
        if ($ExpectedResultRoot.Trim().Length -gt 0) {
            $args.ExpectedResultRoot = $ExpectedResultRoot
        }
        if ($RequireFiles) {
            $args.RequireFiles = $true
        }
        if ($MarkdownPath.Trim().Length -gt 0) {
            $args.MarkdownPath = $MarkdownPath
        }
        $output = & $validator @args 2>&1
        return [pscustomobject]@{ ExitCode = 0; Output = (($output | Out-String).Trim()) }
    }
    catch {
        return [pscustomobject]@{ ExitCode = 1; Output = [string]$_.Exception.Message }
    }
}

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\api-gateway-legacy-evidence.json"
$repoResult = Invoke-Validator -ManifestPath $repoManifest
if ($repoResult.ExitCode -ne 0) {
    Write-Host "FAIL repo api-gateway legacy evidence manifest schema should pass." -ForegroundColor Red
    Write-Host $repoResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-api-gateway-legacy-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $observationWindowSummary = Join-Path $tempRoot "legacy-observation-window-summary.json"
    Write-JsonFile -Path $observationWindowSummary -Value ([ordered]@{
        checked_at_unix_ms = 950000
        status = "PASS"
        observation_count = 3
        min_observations = 3
        first_observation_unix_ms = 100000
        last_observation_unix_ms = 800000
        observed_window_ms = 700000
        required_window_ms = 700000
        max_observation_gap_ms = 500000
        max_observed_gap_ms = 400000
        total_facade_requests = 21
        total_legacy_descriptor_requests = 0
        total_other_requests = 0
        latest_legacy_descriptor_last_seen_unix_ms = 0
        failures = @()
        observations = @()
    })

    $legacyReport = Join-Path $tempRoot "api-gateway-legacy-report.md"
    @(
        "# api-gateway Legacy Descriptor Evidence",
        "",
        "This is local api-gateway legacy descriptor migration evidence.",
        "",
        "It is not a production migration, SLO, or removal approval claim."
    ) | Set-Content -LiteralPath $legacyReport -Encoding UTF8

    $planPath = Join-Path $tempRoot "legacy-removal-plan.json"
    & $planWriter `
        -ObservationWindowSummaryPath $observationWindowSummary `
        -PlanOutputPath $planPath `
        -Operator "operator_1" `
        -ChangeID "change_legacy_descriptor_1" `
        -TargetEnvironment "local_target" `
        -NowUnixMS 960000 | Out-Null

    $validationSummaryPath = Join-Path $tempRoot "legacy-removal-plan-validation.json"
    & $planValidator -PlanPath $planPath -OutputPath $validationSummaryPath | Out-Null

    $manifestPath = Join-Path $tempRoot "api-gateway-legacy-evidence.json"
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "self-test observation"
                kind = "legacy-observation-window"
                summary_path = $observationWindowSummary
                report_path = $legacyReport
                expected_status = "PASS"
                note = "fixture observation evidence"
            },
            [ordered]@{
                name = "self-test removal plan"
                kind = "legacy-removal-plan"
                summary_path = $observationWindowSummary
                plan_path = $planPath
                validation_summary_path = $validationSummaryPath
                report_path = $legacyReport
                expected_status = "READY"
                require_ready_removal = $true
                note = "fixture removal plan evidence"
            }
        )
    })

    $markdownPath = Join-Path $tempRoot "api-gateway-legacy-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -ExpectedResultRoot $tempRoot -RequireFiles -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL api-gateway legacy evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("NexusIM api-gateway Legacy Evidence") -or -not $markdown.Contains("self-test observation") -or -not $markdown.Contains("not a production migration")) {
        Write-Host "FAIL api-gateway legacy evidence markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $duplicateManifest = Join-Path $tempRoot "duplicate-api-gateway-legacy-evidence.json"
    Write-JsonFile -Path $duplicateManifest -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{ name = "dup"; kind = "legacy-observation-window"; summary_path = $observationWindowSummary; note = "fixture" },
            [ordered]@{ name = "dup"; kind = "legacy-observation-window"; summary_path = $observationWindowSummary; note = "fixture" }
        )
    })
    $duplicateResult = Invoke-Validator -ManifestPath $duplicateManifest -ExpectedResultRoot $tempRoot
    if ($duplicateResult.ExitCode -eq 0 -or -not $duplicateResult.Output.Contains("duplicate")) {
        Write-Host "FAIL duplicate api-gateway legacy evidence entries should fail." -ForegroundColor Red
        Write-Host $duplicateResult.Output -ForegroundColor Red
        exit 1
    }

    $repoLocalManifest = Join-Path $tempRoot "repo-local-api-gateway-legacy-evidence.json"
    Write-JsonFile -Path $repoLocalManifest -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "repo local"
                kind = "legacy-observation-window"
                summary_path = "docs/runbook/loadtest/api-gateway/legacy-observation-window-summary.json"
                note = "fixture"
            }
        )
    })
    $repoLocalResult = Invoke-Validator -ManifestPath $repoLocalManifest -ExpectedResultRoot $tempRoot
    if ($repoLocalResult.ExitCode -eq 0 -or -not $repoLocalResult.Output.Contains("must point under")) {
        Write-Host "FAIL repo-local api-gateway legacy evidence path should fail." -ForegroundColor Red
        Write-Host $repoLocalResult.Output -ForegroundColor Red
        exit 1
    }

    & $adder `
        -ManifestPath $manifestPath `
        -ExpectedResultRoot $tempRoot `
        -Name "self-test appended observation" `
        -Kind "legacy-observation-window" `
        -SummaryPath $observationWindowSummary `
        -ReportPath $legacyReport `
        -ExpectedStatus "PASS" `
        -Note "fixture appended observation evidence" | Out-Null
    $afterAdd = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if (@($afterAdd.entries | Where-Object { $_.name -eq "self-test appended observation" }).Count -ne 1) {
        Write-Host "FAIL add-api-gateway-legacy-evidence.ps1 did not append expected entry." -ForegroundColor Red
        exit 1
    }

    $beforeFailedAdd = Get-Content -LiteralPath $manifestPath -Raw
    $failedAddRolledBack = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -ExpectedResultRoot $tempRoot `
            -Name "bad repo path" `
            -Kind "legacy-observation-window" `
            -SummaryPath "docs/runbook/loadtest/api-gateway/legacy-observation-window-summary.json" `
            -ExpectedStatus "PASS" `
            -Note "bad path fixture" 2>$null | Out-Null
    }
    catch {
        $failedAddRolledBack = ($_.Exception.Message -match "must point under")
    }
    if (-not $failedAddRolledBack) {
        Write-Host "FAIL add-api-gateway-legacy-evidence.ps1 should reject repo-local summary_path." -ForegroundColor Red
        exit 1
    }
    $afterFailedAdd = Get-Content -LiteralPath $manifestPath -Raw
    if ($afterFailedAdd -ne $beforeFailedAdd) {
        Write-Host "FAIL failed add-api-gateway-legacy-evidence.ps1 call should restore original manifest." -ForegroundColor Red
        exit 1
    }

    $sensitiveAddFailed = $false
    try {
        & $adder `
            -ManifestPath $manifestPath `
            -ExpectedResultRoot $tempRoot `
            -Name "operator@example.com" `
            -Kind "legacy-observation-window" `
            -SummaryPath $observationWindowSummary `
            -ExpectedStatus "PASS" `
            -Note "fixture appended observation evidence" 2>$null | Out-Null
    }
    catch {
        $sensitiveAddFailed = ($_.Exception.Message -match "low-sensitive evidence metadata")
    }
    if (-not $sensitiveAddFailed) {
        Write-Host "FAIL add-api-gateway-legacy-evidence.ps1 should reject sensitive metadata." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   api-gateway legacy evidence self-test"
