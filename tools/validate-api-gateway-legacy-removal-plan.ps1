param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $PlanPath -PathType Leaf)) {
    throw "Missing api-gateway legacy removal plan: $PlanPath"
}

function Get-Sha256Hex {
    param([byte[]]$Bytes)

    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash($Bytes)
    } finally {
        $sha.Dispose()
    }
    return -join ($hash | ForEach-Object { $_.ToString("x2") })
}

function Assert-String {
    param(
        [object]$Value,
        [string]$Name
    )
    if ([string]::IsNullOrWhiteSpace([string]$Value)) {
        throw "Legacy removal plan missing required field: $Name"
    }
}

function Assert-Boolean {
    param(
        [object]$Value,
        [string]$Name
    )
    if ($Value -isnot [bool]) {
        throw "Legacy removal plan field must be boolean: $Name"
    }
}

function Assert-NonNegativeInt64 {
    param(
        [object]$Value,
        [string]$Name
    )
    if ($null -eq $Value) {
        throw "Legacy removal plan missing numeric field: $Name"
    }
    $number = [int64]$Value
    if ($number -lt 0) {
        throw "Legacy removal plan field must be non-negative: $Name"
    }
    return $number
}

$raw = Get-Content -LiteralPath $PlanPath -Raw
try {
    $plan = $raw | ConvertFrom-Json
}
catch {
    throw "Invalid api-gateway legacy removal plan JSON: $($_.Exception.Message)"
}

if ($plan.schema_version -ne "nexusim.api_gateway.legacy_descriptor_removal_plan.v1") {
    throw "Unsupported api-gateway legacy removal plan schema_version: $($plan.schema_version)"
}
if ($plan.service -ne "api-gateway") {
    throw "Legacy removal plan service must be api-gateway."
}
if ($plan.plan_type -ne "legacy_descriptor_removal") {
    throw "Legacy removal plan has unsupported plan_type: $($plan.plan_type)"
}
if ($plan.executes -ne $false) {
    throw "Legacy removal plan must be non-executing."
}
if ($plan.required_approval -ne $true) {
    throw "Legacy removal plan must require approval."
}
if (@("READY", "BLOCKED") -notcontains [string]$plan.status) {
    throw "Legacy removal plan status must be READY or BLOCKED."
}
Assert-Boolean -Value $plan.ready_for_removal -Name "ready_for_removal"

Assert-String $plan.operator "operator"
Assert-String $plan.source_observation_window_summary_path "source_observation_window_summary_path"
if ($plan.generated_at_unix_ms -le 0) {
    throw "Legacy removal plan generated_at_unix_ms must be positive."
}
if ($null -eq $plan.evidence) {
    throw "Legacy removal plan must include evidence."
}

$observationCount = Assert-NonNegativeInt64 $plan.evidence.observation_count "evidence.observation_count"
$minObservations = Assert-NonNegativeInt64 $plan.evidence.min_observations "evidence.min_observations"
$observedWindowMS = Assert-NonNegativeInt64 $plan.evidence.observed_window_ms "evidence.observed_window_ms"
$requiredWindowMS = Assert-NonNegativeInt64 $plan.evidence.required_window_ms "evidence.required_window_ms"
$maxObservationGapMS = Assert-NonNegativeInt64 $plan.evidence.max_observation_gap_ms "evidence.max_observation_gap_ms"
$maxObservedGapMS = Assert-NonNegativeInt64 $plan.evidence.max_observed_gap_ms "evidence.max_observed_gap_ms"
$facadeRequests = Assert-NonNegativeInt64 $plan.evidence.total_facade_requests "evidence.total_facade_requests"
$legacyRequests = Assert-NonNegativeInt64 $plan.evidence.total_legacy_descriptor_requests "evidence.total_legacy_descriptor_requests"
$otherRequests = Assert-NonNegativeInt64 $plan.evidence.total_other_requests "evidence.total_other_requests"
$latestLegacyLastSeenMS = Assert-NonNegativeInt64 $plan.evidence.latest_legacy_descriptor_last_seen_unix_ms "evidence.latest_legacy_descriptor_last_seen_unix_ms"

$blockers = @($plan.blockers)
if ($plan.status -eq "READY") {
    if ($plan.ready_for_removal -ne $true) {
        throw "READY legacy removal plan must set ready_for_removal=true."
    }
    if ($blockers.Count -ne 0) {
        throw "READY legacy removal plan must not contain blockers."
    }
    if ($observationCount -lt 1 -or ($minObservations -gt 0 -and $observationCount -lt $minObservations)) {
        throw "READY legacy removal plan has insufficient observations."
    }
    if ($requiredWindowMS -gt 0 -and $observedWindowMS -lt $requiredWindowMS) {
        throw "READY legacy removal plan has insufficient quiet window."
    }
    if ($maxObservationGapMS -gt 0 -and $maxObservedGapMS -gt $maxObservationGapMS) {
        throw "READY legacy removal plan has excessive observation gap."
    }
    if ($facadeRequests -le 0) {
        throw "READY legacy removal plan must include facade traffic evidence."
    }
    if ($legacyRequests -gt 0 -or $latestLegacyLastSeenMS -gt 0) {
        throw "READY legacy removal plan must not include legacy descriptor traffic."
    }
    if ($otherRequests -gt 0) {
        throw "READY legacy removal plan must not include other gRPC traffic."
    }
} else {
    if ($plan.ready_for_removal -ne $false) {
        throw "BLOCKED legacy removal plan must set ready_for_removal=false."
    }
    if ($blockers.Count -eq 0) {
        throw "BLOCKED legacy removal plan must include at least one blocker."
    }
}

$plannedSteps = @($plan.planned_steps)
$rollbackSteps = @($plan.rollback_steps)
if ($plannedSteps.Count -lt 3) {
    throw "Legacy removal plan must include planned_steps."
}
if ($rollbackSteps.Count -lt 2) {
    throw "Legacy removal plan must include rollback_steps."
}

$sensitivePattern = "(?i)(authorization|bearer\s+\S+|token\s*[:=]|password\s*[:=]|secret\s*[:=]|email@example\.com)"
if ($raw -match $sensitivePattern) {
    throw "Legacy removal plan contains a sensitive-looking field or value."
}

$summary = [ordered]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    valid = $true
    service = "api-gateway"
    plan_type = "legacy_descriptor_removal"
    status = [string]$plan.status
    ready_for_removal = [bool]$plan.ready_for_removal
    plan_sha256 = Get-Sha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($raw))
    executes = $false
    observation_count = $observationCount
    observed_window_ms = $observedWindowMS
    total_facade_requests = $facadeRequests
    total_legacy_descriptor_requests = $legacyRequests
    total_other_requests = $otherRequests
    blocker_count = $blockers.Count
    note = "Validation only. This script does not remove descriptors, change configuration, contact services, or copy request data."
}

$json = $summary | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
