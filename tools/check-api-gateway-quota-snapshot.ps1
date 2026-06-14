param(
    [string]$MetricsUrl = "http://127.0.0.1:11904/debug/metrics",
    [string]$SnapshotPath = "",
    [string]$RequiredSource = "",
    [string]$MaxAllowedAge = "",
    [switch]$RequireRateLimitEnabled,
    [switch]$RequireVersionedSnapshot,
    [switch]$RequireChecksum,
    [switch]$AllowStale,
    [switch]$AllowReloadErrors
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

function Get-StringOrEmpty {
    param([object]$Value)
    if ($null -eq $Value) {
        return ""
    }
    return [string]$Value
}

function Get-Int64OrZero {
    param([object]$Value)
    if ($null -eq $Value) {
        return [int64]0
    }
    return [int64]$Value
}

function Get-BoolOrFalse {
    param([object]$Value)
    if ($null -eq $Value) {
        return $false
    }
    return [bool]$Value
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
        throw "MaxAllowedAge must be a duration like 24h, 7d, 30m or 00:30:00"
    }
}

$snapshot = Read-MetricsSnapshot
if ($null -eq $snapshot.rate_limit) {
    Write-Host "FAIL api-gateway metrics snapshot does not contain rate_limit." -ForegroundColor Red
    exit 1
}

$rateLimit = $snapshot.rate_limit
$enabled = Get-BoolOrFalse $rateLimit.enabled
$source = (Get-StringOrEmpty $rateLimit.tenant_plan_source).Trim()
$version = (Get-StringOrEmpty $rateLimit.tenant_plan_version).Trim()
$generatedAtMS = Get-Int64OrZero $rateLimit.tenant_plan_generated_at_unix_ms
$checksumPresent = Get-BoolOrFalse $rateLimit.tenant_plan_checksum_present
$maxAgeMS = Get-Int64OrZero $rateLimit.tenant_plan_max_age_ms
$ageMS = Get-Int64OrZero $rateLimit.tenant_plan_age_ms
$stale = Get-BoolOrFalse $rateLimit.tenant_plan_stale
$reloadErrors = Get-Int64OrZero $rateLimit.tenant_plan_reload_error_count
$requiredMaxAgeMS = Convert-DurationToMilliseconds $MaxAllowedAge

$failed = $false
if ($RequireRateLimitEnabled -and -not $enabled) {
    Write-Host "FAIL api-gateway rate limiting is disabled." -ForegroundColor Red
    $failed = $true
}

$requiredSourceValue = $RequiredSource.Trim()
if ($requiredSourceValue -ne "" -and $source.ToLowerInvariant() -ne $requiredSourceValue.ToLowerInvariant()) {
    Write-Host "FAIL api-gateway tenant quota source mismatch: required_source=$requiredSourceValue actual_source=$source." -ForegroundColor Red
    $failed = $true
}

if ($RequireVersionedSnapshot -and ($version -eq "" -or $generatedAtMS -le 0)) {
    Write-Host "FAIL api-gateway tenant quota snapshot is not versioned: version=$version generated_at_unix_ms=$generatedAtMS." -ForegroundColor Red
    $failed = $true
}

if ($RequireChecksum -and -not $checksumPresent) {
    Write-Host "FAIL api-gateway tenant quota snapshot checksum is missing." -ForegroundColor Red
    $failed = $true
}

if ($requiredMaxAgeMS -gt 0) {
    if ($generatedAtMS -le 0) {
        Write-Host "FAIL api-gateway tenant quota snapshot has no generated_at_unix_ms; cannot prove max age." -ForegroundColor Red
        $failed = $true
    } elseif ($ageMS -gt $requiredMaxAgeMS) {
        Write-Host "FAIL api-gateway tenant quota snapshot is older than allowed: max_allowed_age_ms=$requiredMaxAgeMS actual_age_ms=$ageMS." -ForegroundColor Red
        $failed = $true
    }
}

if ($stale -and -not $AllowStale) {
    Write-Host "FAIL api-gateway tenant quota snapshot is stale: max_age_ms=$maxAgeMS age_ms=$ageMS." -ForegroundColor Red
    $failed = $true
}

if ($reloadErrors -gt 0 -and -not $AllowReloadErrors) {
    Write-Host "FAIL api-gateway tenant quota reload errors observed: reload_error_count=$reloadErrors." -ForegroundColor Red
    $failed = $true
}

if ($failed) {
    exit 1
}

Write-Host "OK   api-gateway tenant quota snapshot gate"
Write-Host "     enabled=$enabled source=$source version=$version generated_at_unix_ms=$generatedAtMS checksum_present=$checksumPresent max_age_ms=$maxAgeMS age_ms=$ageMS stale=$stale reload_errors=$reloadErrors"
