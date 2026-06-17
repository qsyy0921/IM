param(
    [Parameter(Mandatory = $true)]
    [string]$ObservationWindowSummaryPath,
    [string]$PlanOutputPath = "",
    [string]$Operator = "manual",
    [string]$ChangeID = "",
    [string]$TargetEnvironment = "",
    [int64]$NowUnixMS = 0
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

function Get-Int64OrZero {
    param([object]$Value)
    if ($null -eq $Value) {
        return [int64]0
    }
    return [int64]$Value
}

function Add-Blocker {
    param(
        [System.Collections.Generic.List[string]]$Blockers,
        [string]$Message
    )
    $Blockers.Add($Message)
}

function Assert-OptionalLowSensitiveLegacyPlanLabel {
    param(
        [string]$Value,
        [string]$FieldName
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return
    }
    Assert-LowSensitiveRepairActor -Value $Value -FieldName $FieldName
}

if (-not (Test-Path -LiteralPath $ObservationWindowSummaryPath -PathType Leaf)) {
    throw "Missing api-gateway legacy observation-window summary: $ObservationWindowSummaryPath"
}

Assert-LowSensitiveRepairActor -Value $Operator -FieldName "Operator"
Assert-OptionalLowSensitiveLegacyPlanLabel -Value $ChangeID -FieldName "ChangeID"
Assert-OptionalLowSensitiveLegacyPlanLabel -Value $TargetEnvironment -FieldName "TargetEnvironment"

$nowMS = $NowUnixMS
if ($nowMS -le 0) {
    $nowMS = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
}

$summaryPath = (Resolve-Path -LiteralPath $ObservationWindowSummaryPath).Path
$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
$blockers = [System.Collections.Generic.List[string]]::new()

$status = [string]$summary.status
$observationCount = Get-Int64OrZero $summary.observation_count
$minObservations = Get-Int64OrZero $summary.min_observations
$observedWindowMS = Get-Int64OrZero $summary.observed_window_ms
$requiredWindowMS = Get-Int64OrZero $summary.required_window_ms
$maxObservedGapMS = Get-Int64OrZero $summary.max_observed_gap_ms
$maxObservationGapMS = Get-Int64OrZero $summary.max_observation_gap_ms
$facadeRequests = Get-Int64OrZero $summary.total_facade_requests
$legacyRequests = Get-Int64OrZero $summary.total_legacy_descriptor_requests
$otherRequests = Get-Int64OrZero $summary.total_other_requests
$latestLegacyLastSeenMS = Get-Int64OrZero $summary.latest_legacy_descriptor_last_seen_unix_ms

if ($status -ne "PASS") {
    Add-Blocker -Blockers $blockers -Message "observation window status is not PASS"
}
if ($observationCount -lt 1) {
    Add-Blocker -Blockers $blockers -Message "observation window has no observations"
}
if ($minObservations -gt 0 -and $observationCount -lt $minObservations) {
    Add-Blocker -Blockers $blockers -Message "observation count is below required minimum"
}
if ($requiredWindowMS -gt 0 -and $observedWindowMS -lt $requiredWindowMS) {
    Add-Blocker -Blockers $blockers -Message "observed quiet window is shorter than required"
}
if ($maxObservationGapMS -gt 0 -and $maxObservedGapMS -gt $maxObservationGapMS) {
    Add-Blocker -Blockers $blockers -Message "observation gap exceeds allowed maximum"
}
if ($facadeRequests -le 0) {
    Add-Blocker -Blockers $blockers -Message "no facade traffic was observed"
}
if ($legacyRequests -gt 0 -or $latestLegacyLastSeenMS -gt 0) {
    Add-Blocker -Blockers $blockers -Message "legacy descriptor traffic was observed"
}
if ($otherRequests -gt 0) {
    Add-Blocker -Blockers $blockers -Message "non-facade/non-legacy gRPC traffic was observed"
}

if ($null -ne $summary.failures) {
    foreach ($failure in @($summary.failures)) {
        $text = [string]$failure
        if (-not [string]::IsNullOrWhiteSpace($text)) {
            Add-Blocker -Blockers $blockers -Message ("observation gate failure: " + $text)
        }
    }
}

$ready = ($blockers.Count -eq 0)
$plan = [ordered]@{
    schema_version = "nexusim.api_gateway.legacy_descriptor_removal_plan.v1"
    generated_at_unix_ms = $nowMS
    service = "api-gateway"
    plan_type = "legacy_descriptor_removal"
    executes = $false
    status = $(if ($ready) { "READY" } else { "BLOCKED" })
    ready_for_removal = $ready
    operator = $Operator
    change_id = $ChangeID
    target_environment = $TargetEnvironment
    source_observation_window_summary_path = $summaryPath
    evidence = [ordered]@{
        observation_count = $observationCount
        min_observations = $minObservations
        first_observation_unix_ms = Get-Int64OrZero $summary.first_observation_unix_ms
        last_observation_unix_ms = Get-Int64OrZero $summary.last_observation_unix_ms
        observed_window_ms = $observedWindowMS
        required_window_ms = $requiredWindowMS
        max_observation_gap_ms = $maxObservationGapMS
        max_observed_gap_ms = $maxObservedGapMS
        total_facade_requests = $facadeRequests
        total_legacy_descriptor_requests = $legacyRequests
        total_other_requests = $otherRequests
        latest_legacy_descriptor_last_seen_unix_ms = $latestLegacyLastSeenMS
    }
    blockers = @($blockers)
    required_approval = $true
    planned_steps = @(
        "Confirm this plan belongs to the target environment and change window.",
        "Keep NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false in target config.",
        "Remove or keep disabled legacy descriptor registration only after operator approval.",
        "Deploy canary and observe api-gateway facade, legacy and other traffic counters.",
        "Archive a post-change legacy observation-window summary before deleting compatibility notes."
    )
    rollback_steps = @(
        "Re-enable the previous deploy artifact or config if required by the approved rollback plan.",
        "Record a fresh legacy observation snapshot after rollback.",
        "Keep legacy descriptors disabled again only after a new quiet-window plan is READY."
    )
    note = "Plan only. This script does not mutate configuration, delete descriptors, contact services, or read request payloads."
}

$json = $plan | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($PlanOutputPath)) {
    $parent = Split-Path -Parent $PlanOutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $PlanOutputPath -Encoding UTF8
}

$json
