param(
    [string]$MetricsUrl = "http://127.0.0.1:11904/debug/metrics",
    [string]$SnapshotPath = "",
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

$snapshot = Read-MetricsSnapshot
$registered = $false
if ($null -ne $snapshot.runtime -and $null -ne $snapshot.runtime.register_legacy_descriptors) {
    $registered = [bool]$snapshot.runtime.register_legacy_descriptors
}

$legacyRequests = [int64]0
$legacyLastSeenMS = [int64]0
if ($null -ne $snapshot.grpc) {
    $legacyRequests = Get-Int64OrZero $snapshot.grpc.legacy_descriptor_requests
    $legacyLastSeenMS = Get-Int64OrZero $snapshot.grpc.legacy_descriptor_last_seen_unix_ms
}

$failed = $false
if ($registered -and -not $AllowRegisteredLegacyDescriptors) {
    Write-Host "FAIL api-gateway still has legacy descriptors registered. Set NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false before removal." -ForegroundColor Red
    $failed = $true
}

if (($legacyRequests -gt 0 -or $legacyLastSeenMS -gt 0) -and -not $AllowObservedLegacyTraffic) {
    Write-Host "FAIL api-gateway observed legacy descriptor traffic: requests=$legacyRequests last_seen_unix_ms=$legacyLastSeenMS. Keep removal blocked until the target environment stays clean." -ForegroundColor Red
    $failed = $true
}

if ($failed) {
    exit 1
}

Write-Host "OK   api-gateway legacy descriptor migration gate"
Write-Host "     registered=$registered legacy_requests=$legacyRequests legacy_last_seen_unix_ms=$legacyLastSeenMS"
