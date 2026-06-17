$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-api-gateway-legacy-removal-plan.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-api-gateway-legacy-removal-plan-" + [System.Guid]::NewGuid().ToString("N"))

function Write-Summary {
    param(
        [string]$Path,
        [string]$Status = "PASS",
        [int64]$ObservationCount = 3,
        [int64]$MinObservations = 3,
        [int64]$ObservedWindowMS = 700000,
        [int64]$RequiredWindowMS = 700000,
        [int64]$MaxObservedGapMS = 400000,
        [int64]$MaxObservationGapMS = 500000,
        [int64]$FacadeRequests = 21,
        [int64]$LegacyRequests = 0,
        [int64]$OtherRequests = 0,
        [int64]$LatestLegacyLastSeenMS = 0,
        [string[]]$Failures = @()
    )

    $summary = [ordered]@{
        checked_at_unix_ms = 900000
        status = $Status
        observation_count = $ObservationCount
        min_observations = $MinObservations
        first_observation_unix_ms = 100000
        last_observation_unix_ms = 800000
        observed_window_ms = $ObservedWindowMS
        required_window_ms = $RequiredWindowMS
        max_observation_gap_ms = $MaxObservationGapMS
        max_observed_gap_ms = $MaxObservedGapMS
        total_facade_requests = $FacadeRequests
        total_legacy_descriptor_requests = $LegacyRequests
        total_other_requests = $OtherRequests
        latest_legacy_descriptor_last_seen_unix_ms = $LatestLegacyLastSeenMS
        failures = @($Failures)
        observations = @()
    }
    $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Writer {
    param(
        [string]$SummaryPath,
        [string]$PlanPath
    )

    $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -ObservationWindowSummaryPath $SummaryPath `
        -PlanOutputPath $PlanPath `
        -Operator "operator_1" `
        -ChangeID "change_legacy_descriptor_1" `
        -TargetEnvironment "local-target" `
        -NowUnixMS 950000 2>&1
    if ($LASTEXITCODE -ne 0) {
        $output | Out-Host
        throw "write-api-gateway-legacy-removal-plan.ps1 failed"
    }
    if (-not (Test-Path -LiteralPath $PlanPath -PathType Leaf)) {
        throw "Expected legacy removal plan to be written: $PlanPath"
    }
    return (Get-Content -LiteralPath $PlanPath -Raw | ConvertFrom-Json)
}

New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $readySummaryPath = Join-Path $tempRoot "ready-summary.json"
    $readyPlanPath = Join-Path $tempRoot "ready-plan.json"
    Write-Summary -Path $readySummaryPath
    $readyPlan = Invoke-Writer -SummaryPath $readySummaryPath -PlanPath $readyPlanPath

    if ($readyPlan.schema_version -ne "nexusim.api_gateway.legacy_descriptor_removal_plan.v1" -or
        $readyPlan.service -ne "api-gateway" -or
        $readyPlan.plan_type -ne "legacy_descriptor_removal" -or
        $readyPlan.executes -ne $false -or
        $readyPlan.status -ne "READY" -or
        $readyPlan.ready_for_removal -ne $true -or
        $readyPlan.required_approval -ne $true -or
        $readyPlan.evidence.total_facade_requests -ne 21 -or
        @($readyPlan.blockers).Count -ne 0) {
        throw "READY legacy removal plan fields are incorrect."
    }

    $blockedSummaryPath = Join-Path $tempRoot "blocked-summary.json"
    $blockedPlanPath = Join-Path $tempRoot "blocked-plan.json"
    Write-Summary `
        -Path $blockedSummaryPath `
        -Status "FAIL" `
        -ObservedWindowMS 1000 `
        -RequiredWindowMS 700000 `
        -FacadeRequests 0 `
        -LegacyRequests 2 `
        -OtherRequests 1 `
        -LatestLegacyLastSeenMS 800000 `
        -Failures @("legacy descriptor traffic observed")
    $blockedPlan = Invoke-Writer -SummaryPath $blockedSummaryPath -PlanPath $blockedPlanPath

    if ($blockedPlan.status -ne "BLOCKED" -or
        $blockedPlan.ready_for_removal -ne $false -or
        @($blockedPlan.blockers).Count -lt 4) {
        throw "BLOCKED legacy removal plan did not preserve blockers."
    }
    if (($blockedPlan | ConvertTo-Json -Depth 8) -match "token|authorization|bearer|email@example.com") {
        throw "Legacy removal plan must stay low-sensitive."
    }
}
finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   api-gateway legacy removal plan self-test"
