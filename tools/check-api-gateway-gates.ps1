$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$quotaGate = Join-Path $PSScriptRoot "check-api-gateway-quota-snapshot.ps1"
$legacyGate = Join-Path $PSScriptRoot "check-api-gateway-legacy-descriptor-migration.ps1"
$legacyObservation = Join-Path $PSScriptRoot "record-api-gateway-legacy-observation.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-api-gateway-gates-" + [System.Guid]::NewGuid().ToString("N"))

function Write-JsonFile {
    param(
        [string]$Path,
        [string]$Content
    )
    $Content | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-GateExpectPass {
    param([string[]]$Arguments)
    $output = & $powerShellExe @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        $output | Out-Host
        throw "Expected gate to pass, exit_code=$LASTEXITCODE args=$($Arguments -join ' ')"
    }
}

function Invoke-GateExpectFail {
    param([string[]]$Arguments)
    $output = & $powerShellExe @Arguments 2>&1
    if ($LASTEXITCODE -eq 0) {
        $output | Out-Host
        throw "Expected gate to fail, args=$($Arguments -join ' ')"
    }
}

New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
try {
    $quotaGood = Join-Path $tempDir "quota-good.json"
    $quotaMissingClientCert = Join-Path $tempDir "quota-missing-client-cert.json"
    $quotaRedisErrors = Join-Path $tempDir "quota-redis-errors.json"
    $quotaIdentityErrors = Join-Path $tempDir "quota-identity-errors.json"
    $quotaTooFewPlans = Join-Path $tempDir "quota-too-few-plans.json"
    $quotaTooManyKeys = Join-Path $tempDir "quota-too-many-keys.json"
    $quotaOldReload = Join-Path $tempDir "quota-old-reload.json"
    $quotaFutureSnapshot = Join-Path $tempDir "quota-future-snapshot.json"
    $quotaGoodJson = @'
{
  "rate_limit": {
    "enabled": true,
    "tenant_plan_count": 2,
    "tenant_plan_source": "url",
    "tenant_plan_version": "quota-v1.gate",
    "tenant_plan_generated_at_unix_ms": 1000000,
    "tenant_plan_checksum_present": true,
    "tenant_plan_require_checksum": true,
    "tenant_plan_url_bearer_token_configured": true,
    "tenant_plan_url_require_https": true,
    "tenant_plan_url_tls_configured": true,
    "tenant_plan_url_client_cert_configured": true,
    "tenant_plan_max_age_ms": 3600000,
    "tenant_plan_age_ms": 0,
    "tenant_plan_stale": false,
    "tenant_plan_reload_error_count": 0,
    "tenant_plan_reloaded_at_unix_ms": 1000000,
    "redis_error_count": 0,
    "identity_error_count": 0,
    "tracked_keys": 42
  }
}
'@
    Write-JsonFile -Path $quotaGood -Content $quotaGoodJson
    Write-JsonFile -Path $quotaMissingClientCert -Content ($quotaGoodJson -replace '"tenant_plan_url_client_cert_configured": true', '"tenant_plan_url_client_cert_configured": false')
    Write-JsonFile -Path $quotaRedisErrors -Content ($quotaGoodJson -replace '"redis_error_count": 0', '"redis_error_count": 1')
    Write-JsonFile -Path $quotaIdentityErrors -Content ($quotaGoodJson -replace '"identity_error_count": 0', '"identity_error_count": 1')
    Write-JsonFile -Path $quotaTooFewPlans -Content ($quotaGoodJson -replace '"tenant_plan_count": 2', '"tenant_plan_count": 0')
    Write-JsonFile -Path $quotaTooManyKeys -Content ($quotaGoodJson -replace '"tracked_keys": 42', '"tracked_keys": 101')
    Write-JsonFile -Path $quotaOldReload -Content ($quotaGoodJson -replace '"tenant_plan_reloaded_at_unix_ms": 1000000', '"tenant_plan_reloaded_at_unix_ms": 300000')
    Write-JsonFile -Path $quotaFutureSnapshot -Content ($quotaGoodJson -replace '"tenant_plan_generated_at_unix_ms": 1000000', '"tenant_plan_generated_at_unix_ms": 1006000')

    $quotaStrongArgs = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $quotaGate,
        "-RequireRateLimitEnabled",
        "-RequiredSource", "url",
        "-RequireVersionedSnapshot",
        "-RequireChecksum",
        "-RequireChecksumPolicy",
        "-RequireURLHTTPS",
        "-RequireURLBearerToken",
        "-RequireURLTLS",
        "-RequireURLClientCert",
        "-MaxAllowedAge", "1h",
        "-RequireNoRedisErrors",
        "-RequireNoIdentityErrors",
        "-MinTenantPlans", "1",
        "-MaxTrackedKeys", "100",
        "-MaxReloadAge", "10m",
        "-NowUnixMS", "1005000"
    )
    Invoke-GateExpectPass -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaGood))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaMissingClientCert))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaRedisErrors))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaIdentityErrors))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaTooFewPlans))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaTooManyKeys))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaOldReload))
    Invoke-GateExpectFail -Arguments ($quotaStrongArgs + @("-SnapshotPath", $quotaFutureSnapshot))

    $legacyGood = Join-Path $tempDir "legacy-good.json"
    $legacyNoFacade = Join-Path $tempDir "legacy-no-facade.json"
    $legacyFutureOptIn = Join-Path $tempDir "legacy-future-opt-in.json"
    $legacyGoodJson = @'
{
  "generated_at_ms": 100000,
  "runtime": {
    "register_legacy_descriptors": false,
    "legacy_descriptors_allowed_until_unix_ms": 0
  },
  "grpc": {
    "facade_requests": 12,
    "legacy_descriptor_requests": 0,
    "legacy_descriptor_last_seen_unix_ms": 0,
    "other_requests": 0
  }
}
'@
    Write-JsonFile -Path $legacyGood -Content $legacyGoodJson
    Write-JsonFile -Path $legacyNoFacade -Content ($legacyGoodJson -replace '"facade_requests": 12', '"facade_requests": 0')
    Write-JsonFile -Path $legacyFutureOptIn -Content ($legacyGoodJson -replace '"legacy_descriptors_allowed_until_unix_ms": 0', '"legacy_descriptors_allowed_until_unix_ms": 200000')

    $legacyStrongArgs = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $legacyGate,
        "-NowUnixMS", "100500",
        "-MaxSnapshotAge", "1h",
        "-RequireFacadeTraffic",
        "-DisallowOtherTraffic",
        "-RequiredQuietDuration", "7d",
        "-RequireLegacyOptInExpiredOrUnset"
    )
    Invoke-GateExpectPass -Arguments ($legacyStrongArgs + @("-SnapshotPath", $legacyGood))
    Invoke-GateExpectFail -Arguments ($legacyStrongArgs + @("-SnapshotPath", $legacyNoFacade))
    Invoke-GateExpectFail -Arguments ($legacyStrongArgs + @("-SnapshotPath", $legacyFutureOptIn))

    $observationRoot = Join-Path $tempDir "legacy-observation"
    Invoke-GateExpectPass -Arguments @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $legacyObservation,
        "-SnapshotPath", $legacyGood,
        "-OutputRoot", $observationRoot,
        "-RunName", "legacy-observation-pass",
        "-RequiredQuietDuration", "7d",
        "-MaxSnapshotAge", "1h",
        "-NowUnixMS", "100500"
    )
    $observationSummary = Join-Path $observationRoot "legacy-observation-pass\legacy-observation-summary.json"
    $observationReport = Join-Path $observationRoot "legacy-observation-pass\legacy-observation-report.md"
    if (-not (Test-Path -LiteralPath $observationSummary)) {
        throw "Expected legacy observation summary to be written: $observationSummary"
    }
    if (-not (Test-Path -LiteralPath $observationReport)) {
        throw "Expected legacy observation report to be written: $observationReport"
    }
    Invoke-GateExpectFail -Arguments @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $legacyObservation,
        "-SnapshotPath", $legacyNoFacade,
        "-OutputRoot", $observationRoot,
        "-RunName", "legacy-observation-fail",
        "-RequiredQuietDuration", "7d",
        "-MaxSnapshotAge", "1h",
        "-NowUnixMS", "100500"
    )
    $failedObservationSummary = Join-Path $observationRoot "legacy-observation-fail\legacy-observation-summary.json"
    if (-not (Test-Path -LiteralPath $failedObservationSummary)) {
        throw "Expected failed legacy observation summary to be written: $failedObservationSummary"
    }
}
finally {
    Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   api-gateway gate self-tests"
