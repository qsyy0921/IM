param(
    [string]$MetricsUrl = "http://127.0.0.1:11904/debug/metrics",
    [string]$SnapshotPath = "",
    [string]$RequiredQuietDuration = "",
    [string]$MaxSnapshotAge = "",
    [int64]$NowUnixMS = 0,
    [switch]$AllowRegisteredLegacyDescriptors,
    [switch]$AllowObservedLegacyTraffic,
    [switch]$RequireFacadeTraffic,
    [switch]$DisallowOtherTraffic
)

$ErrorActionPreference = "Stop"

function Read-MetricsSnapshot {
    if (-not [string]::IsNullOrWhiteSpace($SnapshotPath)) {
        if (-not (Test-Path -LiteralPath $SnapshotPath)) {
            throw "Metrics snapshot file does not exist: $SnapshotPath"
        }
        return Get-Content -LiteralPath $SnapshotPath -Raw | ConvertFrom-Json
    }
    return Invoke-RestMethod -Method Get -Uri $MetricsUrl -TimeoutSec 5
}

function Get-Int64OrZero {
    param([object]$Value)
    if ($null -eq $Value) {
        return [int64]0
    }
    return [int64]$Value
}

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
        throw "RequiredQuietDuration must be a duration like 24h, 7d, 30m or 00:30:00"
    }
}

$snapshot = Read-MetricsSnapshot
$snapshotGeneratedAtMS = [int64]0
if ($null -ne $snapshot.generated_at_ms) {
    $snapshotGeneratedAtMS = Get-Int64OrZero $snapshot.generated_at_ms
}

$registered = $false
if ($null -ne $snapshot.runtime -and $null -ne $snapshot.runtime.register_legacy_descriptors) {
    $registered = [bool]$snapshot.runtime.register_legacy_descriptors
}

$legacyAllowedUntilMS = [int64]0
if ($null -ne $snapshot.runtime -and $null -ne $snapshot.runtime.legacy_descriptors_allowed_until_unix_ms) {
    $legacyAllowedUntilMS = Get-Int64OrZero $snapshot.runtime.legacy_descriptors_allowed_until_unix_ms
}

$legacyRequests = [int64]0
$legacyLastSeenMS = [int64]0
$facadeRequests = [int64]0
$otherRequests = [int64]0
if ($null -ne $snapshot.grpc) {
    $facadeRequests = Get-Int64OrZero $snapshot.grpc.facade_requests
    $legacyRequests = Get-Int64OrZero $snapshot.grpc.legacy_descriptor_requests
    $legacyLastSeenMS = Get-Int64OrZero $snapshot.grpc.legacy_descriptor_last_seen_unix_ms
    $otherRequests = Get-Int64OrZero $snapshot.grpc.other_requests
}

$quietMS = Convert-DurationToMilliseconds $RequiredQuietDuration
$maxSnapshotAgeMS = Convert-DurationToMilliseconds $MaxSnapshotAge
$nowMS = $NowUnixMS
if ($nowMS -le 0) {
    $nowMS = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
}

$failed = $false
if ($registered -and -not $AllowRegisteredLegacyDescriptors) {
    Write-Host "FAIL api-gateway still has legacy descriptors registered. Set NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false before removal." -ForegroundColor Red
    $failed = $true
}

if ($registered -and $legacyAllowedUntilMS -gt 0 -and $legacyAllowedUntilMS -le $nowMS) {
    Write-Host "FAIL api-gateway legacy descriptor opt-in deadline has expired: allowed_until_unix_ms=$legacyAllowedUntilMS now_unix_ms=$nowMS." -ForegroundColor Red
    $failed = $true
}

if ($maxSnapshotAgeMS -gt 0) {
    if ($snapshotGeneratedAtMS -le 0) {
        Write-Host "FAIL api-gateway metrics snapshot has no generated_at_ms; cannot prove max snapshot age." -ForegroundColor Red
        $failed = $true
    } else {
        $snapshotAgeMS = $nowMS - $snapshotGeneratedAtMS
        if ($snapshotAgeMS -lt 0) {
            Write-Host "FAIL api-gateway metrics snapshot is from the future: generated_at_ms=$snapshotGeneratedAtMS now_unix_ms=$nowMS." -ForegroundColor Red
            $failed = $true
        } elseif ($snapshotAgeMS -gt $maxSnapshotAgeMS) {
            Write-Host "FAIL api-gateway metrics snapshot is too old: max_snapshot_age_ms=$maxSnapshotAgeMS actual_snapshot_age_ms=$snapshotAgeMS generated_at_ms=$snapshotGeneratedAtMS." -ForegroundColor Red
            $failed = $true
        }
    }
}

if ($RequireFacadeTraffic -and $facadeRequests -le 0) {
    Write-Host "FAIL api-gateway facade traffic has not been observed; cannot prove clients migrated to GatewayService." -ForegroundColor Red
    $failed = $true
}

if (($legacyRequests -gt 0 -or $legacyLastSeenMS -gt 0) -and -not $AllowObservedLegacyTraffic) {
    if ($quietMS -gt 0) {
        if ($legacyLastSeenMS -le 0) {
            Write-Host "FAIL api-gateway observed legacy descriptor traffic but last_seen_unix_ms is missing; cannot prove required quiet window." -ForegroundColor Red
            $failed = $true
        } else {
            $quietAgeMS = $nowMS - $legacyLastSeenMS
            if ($quietAgeMS -lt $quietMS) {
                Write-Host "FAIL api-gateway legacy descriptor quiet window is too short: required_quiet_ms=$quietMS actual_quiet_ms=$quietAgeMS requests=$legacyRequests last_seen_unix_ms=$legacyLastSeenMS." -ForegroundColor Red
                $failed = $true
            }
        }
    } else {
        Write-Host "FAIL api-gateway observed legacy descriptor traffic: requests=$legacyRequests last_seen_unix_ms=$legacyLastSeenMS. Keep removal blocked until the target environment stays clean." -ForegroundColor Red
        $failed = $true
    }
}

if ($DisallowOtherTraffic -and $otherRequests -gt 0) {
    Write-Host "FAIL api-gateway observed non-facade/non-legacy gRPC traffic: other_requests=$otherRequests." -ForegroundColor Red
    $failed = $true
}

if ($failed) {
    exit 1
}

Write-Host "OK   api-gateway legacy descriptor migration gate"
Write-Host "     registered=$registered facade_requests=$facadeRequests legacy_requests=$legacyRequests other_requests=$otherRequests legacy_last_seen_unix_ms=$legacyLastSeenMS legacy_allowed_until_unix_ms=$legacyAllowedUntilMS required_quiet_ms=$quietMS max_snapshot_age_ms=$maxSnapshotAgeMS snapshot_generated_at_ms=$snapshotGeneratedAtMS now_unix_ms=$nowMS"
