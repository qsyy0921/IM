param(
    [string]$MetricsUrl = "http://127.0.0.1:11904/debug/metrics",
    [string]$SnapshotPath = "",
    [string]$RequiredQuietDuration = "",
    [int64]$NowUnixMS = 0,
    [switch]$AllowRegisteredLegacyDescriptors,
    [switch]$AllowObservedLegacyTraffic
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
if ($null -ne $snapshot.grpc) {
    $legacyRequests = Get-Int64OrZero $snapshot.grpc.legacy_descriptor_requests
    $legacyLastSeenMS = Get-Int64OrZero $snapshot.grpc.legacy_descriptor_last_seen_unix_ms
}

$quietMS = Convert-DurationToMilliseconds $RequiredQuietDuration
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

if ($failed) {
    exit 1
}

Write-Host "OK   api-gateway legacy descriptor migration gate"
Write-Host "     registered=$registered legacy_requests=$legacyRequests legacy_last_seen_unix_ms=$legacyLastSeenMS legacy_allowed_until_unix_ms=$legacyAllowedUntilMS required_quiet_ms=$quietMS now_unix_ms=$nowMS"
