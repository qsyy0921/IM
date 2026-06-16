param(
    [string]$SummaryRoot = "",
    [string[]]$SummaryPath = @(),
    [string]$RequiredWindow = "7d",
    [string]$MaxObservationGap = "24h",
    [int]$MinObservations = 2,
    [int64]$NowUnixMS = 0,
    [string]$OutputPath = "",
    [switch]$AllowFailedGate,
    [switch]$AllowRegisteredLegacyDescriptors,
    [switch]$AllowObservedLegacyTraffic,
    [switch]$AllowOtherTraffic,
    [switch]$AllowMissingFacadeTraffic
)

$ErrorActionPreference = "Stop"

function Convert-DurationToMilliseconds {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        return [int64]0
    }

    $trimmed = $Value.Trim()
    if ($trimmed -match '^(?<amount>\d+)(?<unit>ms|s|m|h|d)$') {
        $amount = [int64]$Matches.amount
        switch ($Matches.unit.ToLowerInvariant()) {
            "ms" { return $amount }
            "s" { return $amount * 1000 }
            "m" { return $amount * 60 * 1000 }
            "h" { return $amount * 60 * 60 * 1000 }
            "d" { return $amount * 24 * 60 * 60 * 1000 }
        }
    }

    try {
        return [int64][TimeSpan]::Parse($trimmed).TotalMilliseconds
    } catch {
        throw "Duration must be like 24h, 7d, 30m or 00:30:00"
    }
}

function Get-Int64OrZero {
    param([object]$Value)
    if ($null -eq $Value) {
        return [int64]0
    }
    return [int64]$Value
}

function Add-Failure {
    param(
        [System.Collections.Generic.List[string]]$Failures,
        [string]$Message
    )
    $Failures.Add($Message)
    Write-Host "FAIL $Message" -ForegroundColor Red
}

$summaryFiles = [System.Collections.Generic.List[string]]::new()
foreach ($path in $SummaryPath) {
    if ([string]::IsNullOrWhiteSpace($path)) {
        continue
    }
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Summary file does not exist: $path"
    }
    $summaryFiles.Add((Resolve-Path -LiteralPath $path).Path)
}

if (-not [string]::IsNullOrWhiteSpace($SummaryRoot)) {
    if (-not (Test-Path -LiteralPath $SummaryRoot)) {
        throw "Summary root does not exist: $SummaryRoot"
    }
    Get-ChildItem -LiteralPath $SummaryRoot -Recurse -File -Filter "legacy-observation-summary.json" |
        ForEach-Object { $summaryFiles.Add($_.FullName) }
}

$distinctSummaryFiles = $summaryFiles | Sort-Object -Unique
if ($distinctSummaryFiles.Count -eq 0) {
    throw "No legacy-observation-summary.json files found. Pass -SummaryRoot or -SummaryPath."
}

$observations = @()
foreach ($file in $distinctSummaryFiles) {
    $raw = Get-Content -LiteralPath $file -Raw
    $summary = $raw | ConvertFrom-Json
    $generatedAt = Get-Int64OrZero $summary.generated_at_unix_ms
    if ($generatedAt -le 0) {
        $generatedAt = Get-Int64OrZero $summary.snapshot_generated_at_unix_ms
    }
    if ($generatedAt -le 0) {
        throw "Observation summary has no generated timestamp: $file"
    }

    $observations += [pscustomobject]@{
        Path = $file
        RunName = [string]$summary.run_name
        GeneratedAtUnixMS = $generatedAt
        GatePassed = [bool]$summary.gate_passed
        RegisteredLegacyDescriptors = [bool]$summary.registered_legacy_descriptors
        FacadeRequests = Get-Int64OrZero $summary.facade_requests
        LegacyDescriptorRequests = Get-Int64OrZero $summary.legacy_descriptor_requests
        OtherRequests = Get-Int64OrZero $summary.other_requests
        LegacyLastSeenUnixMS = Get-Int64OrZero $summary.legacy_descriptor_last_seen_unix_ms
    }
}

$observations = $observations | Sort-Object GeneratedAtUnixMS
$requiredWindowMS = Convert-DurationToMilliseconds $RequiredWindow
$maxGapMS = Convert-DurationToMilliseconds $MaxObservationGap
$nowMS = $NowUnixMS
if ($nowMS -le 0) {
    $nowMS = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
}

$failures = [System.Collections.Generic.List[string]]::new()
$observationCount = $observations.Count
if ($MinObservations -lt 1) {
    throw "MinObservations must be >= 1"
}
if ($observationCount -lt $MinObservations) {
    Add-Failure -Failures $failures -Message "not enough observations: required=$MinObservations actual=$observationCount"
}

$firstObservationMS = [int64]$observations[0].GeneratedAtUnixMS
$lastObservationMS = [int64]$observations[$observations.Count - 1].GeneratedAtUnixMS
$observedWindowMS = $lastObservationMS - $firstObservationMS
if ($observedWindowMS -lt 0) {
    Add-Failure -Failures $failures -Message "observation timestamps are inconsistent"
} elseif ($requiredWindowMS -gt 0 -and $observedWindowMS -lt $requiredWindowMS) {
    Add-Failure -Failures $failures -Message "observation window too short: required_window_ms=$requiredWindowMS actual_window_ms=$observedWindowMS"
}

$maxObservedGapMS = [int64]0
for ($i = 1; $i -lt $observations.Count; $i++) {
    $previous = [int64]$observations[$i - 1].GeneratedAtUnixMS
    $current = [int64]$observations[$i].GeneratedAtUnixMS
    $gap = $current - $previous
    if ($gap -gt $maxObservedGapMS) {
        $maxObservedGapMS = $gap
    }
    if ($gap -lt 0) {
        Add-Failure -Failures $failures -Message "observation timestamps are not monotonic near $($observations[$i].Path)"
    } elseif ($maxGapMS -gt 0 -and $gap -gt $maxGapMS) {
        Add-Failure -Failures $failures -Message "observation gap too large: max_gap_ms=$maxGapMS actual_gap_ms=$gap path=$($observations[$i].Path)"
    }
}

$totalFacadeRequests = [int64]0
$totalLegacyRequests = [int64]0
$totalOtherRequests = [int64]0
$latestLegacyLastSeenMS = [int64]0
foreach ($observation in $observations) {
    $totalFacadeRequests += [int64]$observation.FacadeRequests
    $totalLegacyRequests += [int64]$observation.LegacyDescriptorRequests
    $totalOtherRequests += [int64]$observation.OtherRequests
    if ([int64]$observation.LegacyLastSeenUnixMS -gt $latestLegacyLastSeenMS) {
        $latestLegacyLastSeenMS = [int64]$observation.LegacyLastSeenUnixMS
    }

    if (-not $AllowFailedGate -and -not [bool]$observation.GatePassed) {
        Add-Failure -Failures $failures -Message "observation gate failed: path=$($observation.Path)"
    }
    if (-not $AllowRegisteredLegacyDescriptors -and [bool]$observation.RegisteredLegacyDescriptors) {
        Add-Failure -Failures $failures -Message "registered legacy descriptors observed: path=$($observation.Path)"
    }
    if (-not $AllowObservedLegacyTraffic -and ([int64]$observation.LegacyDescriptorRequests -gt 0 -or [int64]$observation.LegacyLastSeenUnixMS -gt 0)) {
        Add-Failure -Failures $failures -Message "legacy descriptor traffic observed: requests=$($observation.LegacyDescriptorRequests) last_seen_unix_ms=$($observation.LegacyLastSeenUnixMS) path=$($observation.Path)"
    }
    if (-not $AllowOtherTraffic -and [int64]$observation.OtherRequests -gt 0) {
        Add-Failure -Failures $failures -Message "non-facade/non-legacy traffic observed: other_requests=$($observation.OtherRequests) path=$($observation.Path)"
    }
}

if (-not $AllowMissingFacadeTraffic -and $totalFacadeRequests -le 0) {
    Add-Failure -Failures $failures -Message "no facade traffic observed across window"
}

$result = [ordered]@{
    checked_at_unix_ms = $nowMS
    status = $(if ($failures.Count -eq 0) { "PASS" } else { "FAIL" })
    observation_count = $observationCount
    min_observations = $MinObservations
    first_observation_unix_ms = $firstObservationMS
    last_observation_unix_ms = $lastObservationMS
    observed_window_ms = $observedWindowMS
    required_window_ms = $requiredWindowMS
    max_observation_gap_ms = $maxGapMS
    max_observed_gap_ms = $maxObservedGapMS
    total_facade_requests = $totalFacadeRequests
    total_legacy_descriptor_requests = $totalLegacyRequests
    total_other_requests = $totalOtherRequests
    latest_legacy_descriptor_last_seen_unix_ms = $latestLegacyLastSeenMS
    failures = @($failures)
    observations = @($observations | ForEach-Object {
        [ordered]@{
            run_name = $_.RunName
            path = $_.Path
            generated_at_unix_ms = $_.GeneratedAtUnixMS
            gate_passed = $_.GatePassed
            registered_legacy_descriptors = $_.RegisteredLegacyDescriptors
            facade_requests = $_.FacadeRequests
            legacy_descriptor_requests = $_.LegacyDescriptorRequests
            other_requests = $_.OtherRequests
            legacy_descriptor_last_seen_unix_ms = $_.LegacyLastSeenUnixMS
        }
    })
}

if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $result | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputPath -Encoding UTF8
}

if ($failures.Count -gt 0) {
    Write-Host "api_gateway_legacy_observation_window_status=FAIL"
    Write-Host "api_gateway_legacy_observation_window_count=$observationCount"
    Write-Host "api_gateway_legacy_observation_window_ms=$observedWindowMS"
    if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
        Write-Host "api_gateway_legacy_observation_window_summary=$OutputPath"
    }
    exit 1
}

Write-Host "OK   api-gateway legacy descriptor observation window"
Write-Host "     observations=$observationCount observed_window_ms=$observedWindowMS max_observed_gap_ms=$maxObservedGapMS facade_requests=$totalFacadeRequests legacy_requests=$totalLegacyRequests other_requests=$totalOtherRequests"
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    Write-Host "api_gateway_legacy_observation_window_summary=$OutputPath"
}
