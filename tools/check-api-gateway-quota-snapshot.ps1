param(
    [string]$MetricsUrl = "http://127.0.0.1:11904/debug/metrics",
    [string]$SnapshotPath = "",
    [string]$RequiredSource = "",
    [string]$MaxAllowedAge = "",
    [switch]$RequireRateLimitEnabled,
    [switch]$RequireVersionedSnapshot,
    [switch]$RequireChecksum,
    [switch]$RequireChecksumPolicy,
    [switch]$RequireURLHTTPS,
    [switch]$RequireURLBearerToken,
    [switch]$RequireURLTLS,
    [switch]$RequireURLClientCert,
    [switch]$AllowStale,
    [switch]$AllowReloadErrors,
    [switch]$RequireNoRedisErrors,
    [switch]$RequireNoIdentityErrors,
    [int]$MinTenantPlans = 0,
    [int]$MaxTrackedKeys = 0,
    [string]$MaxReloadAge = "",
    [int64]$NowUnixMS = 0
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
    param(
        [string]$Value,
        [string]$Name
    )

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
        throw "$Name must be a duration like 24h, 7d, 30m or 00:30:00"
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
$checksumRequired = Get-BoolOrFalse $rateLimit.tenant_plan_require_checksum
$urlBearerTokenConfigured = Get-BoolOrFalse $rateLimit.tenant_plan_url_bearer_token_configured
$urlRequireHTTPS = Get-BoolOrFalse $rateLimit.tenant_plan_url_require_https
$urlTLSConfigured = Get-BoolOrFalse $rateLimit.tenant_plan_url_tls_configured
$urlClientCertConfigured = Get-BoolOrFalse $rateLimit.tenant_plan_url_client_cert_configured
$maxAgeMS = Get-Int64OrZero $rateLimit.tenant_plan_max_age_ms
$ageMS = Get-Int64OrZero $rateLimit.tenant_plan_age_ms
$stale = Get-BoolOrFalse $rateLimit.tenant_plan_stale
$reloadErrors = Get-Int64OrZero $rateLimit.tenant_plan_reload_error_count
$reloadAtMS = Get-Int64OrZero $rateLimit.tenant_plan_reloaded_at_unix_ms
$tenantPlans = Get-Int64OrZero $rateLimit.tenant_plan_count
$trackedKeys = Get-Int64OrZero $rateLimit.tracked_keys
$redisErrors = Get-Int64OrZero $rateLimit.redis_error_count
$identityErrors = Get-Int64OrZero $rateLimit.identity_error_count
$requiredMaxAgeMS = Convert-DurationToMilliseconds -Value $MaxAllowedAge -Name "MaxAllowedAge"
$requiredMaxReloadAgeMS = Convert-DurationToMilliseconds -Value $MaxReloadAge -Name "MaxReloadAge"
$nowMS = $NowUnixMS
if ($nowMS -le 0) {
    $nowMS = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
}
if ($generatedAtMS -gt 0) {
    $ageMS = $nowMS - $generatedAtMS
}

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

if ($RequireChecksumPolicy -and -not $checksumRequired) {
    Write-Host "FAIL api-gateway tenant quota snapshot checksum policy is not enabled." -ForegroundColor Red
    $failed = $true
}

if ($RequireURLHTTPS -and -not $urlRequireHTTPS) {
    Write-Host "FAIL api-gateway tenant quota URL source HTTPS guard is not enabled." -ForegroundColor Red
    $failed = $true
}

if ($RequireURLBearerToken -and -not $urlBearerTokenConfigured) {
    Write-Host "FAIL api-gateway tenant quota URL source bearer token is not configured." -ForegroundColor Red
    $failed = $true
}

if ($RequireURLTLS -and -not $urlTLSConfigured) {
    Write-Host "FAIL api-gateway tenant quota URL source TLS configuration is not enabled." -ForegroundColor Red
    $failed = $true
}

if ($RequireURLClientCert -and -not $urlClientCertConfigured) {
    Write-Host "FAIL api-gateway tenant quota URL source client certificate is not configured." -ForegroundColor Red
    $failed = $true
}

if ($requiredMaxAgeMS -gt 0) {
    if ($generatedAtMS -le 0) {
        Write-Host "FAIL api-gateway tenant quota snapshot has no generated_at_unix_ms; cannot prove max age." -ForegroundColor Red
        $failed = $true
    }
}

if ($generatedAtMS -gt 0 -and $ageMS -lt 0) {
    Write-Host "FAIL api-gateway tenant quota snapshot generated_at_unix_ms is from the future: generated_at_unix_ms=$generatedAtMS now_unix_ms=$nowMS." -ForegroundColor Red
    $failed = $true
} elseif ($requiredMaxAgeMS -gt 0 -and $ageMS -gt $requiredMaxAgeMS) {
    Write-Host "FAIL api-gateway tenant quota snapshot is older than allowed: max_allowed_age_ms=$requiredMaxAgeMS actual_age_ms=$ageMS." -ForegroundColor Red
    $failed = $true
}

if ($stale -and -not $AllowStale) {
    Write-Host "FAIL api-gateway tenant quota snapshot is stale: max_age_ms=$maxAgeMS age_ms=$ageMS." -ForegroundColor Red
    $failed = $true
}

if ($reloadErrors -gt 0 -and -not $AllowReloadErrors) {
    Write-Host "FAIL api-gateway tenant quota reload errors observed: reload_error_count=$reloadErrors." -ForegroundColor Red
    $failed = $true
}

if ($RequireNoRedisErrors -and $redisErrors -gt 0) {
    Write-Host "FAIL api-gateway rate-limit Redis errors observed: redis_error_count=$redisErrors." -ForegroundColor Red
    $failed = $true
}

if ($RequireNoIdentityErrors -and $identityErrors -gt 0) {
    Write-Host "FAIL api-gateway rate-limit tenant identity errors observed: identity_error_count=$identityErrors." -ForegroundColor Red
    $failed = $true
}

if ($MinTenantPlans -gt 0 -and $tenantPlans -lt $MinTenantPlans) {
    Write-Host "FAIL api-gateway tenant quota plan count is below minimum: min_tenant_plans=$MinTenantPlans actual_tenant_plans=$tenantPlans." -ForegroundColor Red
    $failed = $true
}

if ($MaxTrackedKeys -gt 0 -and $trackedKeys -gt $MaxTrackedKeys) {
    Write-Host "FAIL api-gateway rate-limit tracked key count is above maximum: max_tracked_keys=$MaxTrackedKeys actual_tracked_keys=$trackedKeys." -ForegroundColor Red
    $failed = $true
}

if ($requiredMaxReloadAgeMS -gt 0) {
    if ($reloadAtMS -le 0) {
        Write-Host "FAIL api-gateway tenant quota snapshot has no successful reload timestamp; cannot prove max reload age." -ForegroundColor Red
        $failed = $true
    } else {
        $reloadAgeMS = $nowMS - $reloadAtMS
        if ($reloadAgeMS -lt 0) {
            Write-Host "FAIL api-gateway tenant quota reload timestamp is from the future: reloaded_at_unix_ms=$reloadAtMS now_unix_ms=$nowMS." -ForegroundColor Red
            $failed = $true
        } elseif ($reloadAgeMS -gt $requiredMaxReloadAgeMS) {
            Write-Host "FAIL api-gateway tenant quota reload is older than allowed: max_reload_age_ms=$requiredMaxReloadAgeMS actual_reload_age_ms=$reloadAgeMS reloaded_at_unix_ms=$reloadAtMS." -ForegroundColor Red
            $failed = $true
        }
    }
}

if ($failed) {
    exit 1
}

Write-Host "OK   api-gateway tenant quota snapshot gate"
Write-Host "     enabled=$enabled source=$source version=$version generated_at_unix_ms=$generatedAtMS checksum_present=$checksumPresent checksum_required=$checksumRequired url_require_https=$urlRequireHTTPS url_bearer_token_configured=$urlBearerTokenConfigured url_tls_configured=$urlTLSConfigured url_client_cert_configured=$urlClientCertConfigured max_age_ms=$maxAgeMS age_ms=$ageMS stale=$stale reload_errors=$reloadErrors redis_errors=$redisErrors identity_errors=$identityErrors tenant_plans=$tenantPlans tracked_keys=$trackedKeys reloaded_at_unix_ms=$reloadAtMS now_unix_ms=$nowMS"
