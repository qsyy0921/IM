param(
    [string]$MetricsUrl = "http://127.0.0.1:11904/debug/metrics",
    [string]$SnapshotPath = "",
    [string]$OutputRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$RequiredQuietDuration = "7d",
    [string]$MaxSnapshotAge = "30m",
    [int64]$NowUnixMS = 0,
    [switch]$AllowRegisteredLegacyDescriptors,
    [switch]$AllowObservedLegacyTraffic,
    [switch]$AllowMissingFacadeTraffic,
    [switch]$AllowOtherTraffic,
    [switch]$AllowFutureLegacyOptIn
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

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

function Read-MetricsSnapshotRaw {
    if (-not [string]::IsNullOrWhiteSpace($SnapshotPath)) {
        if (-not (Test-Path -LiteralPath $SnapshotPath)) {
            throw "Metrics snapshot file does not exist: $SnapshotPath"
        }
        return Get-Content -LiteralPath $SnapshotPath -Raw
    }
    $response = Invoke-WebRequest -UseBasicParsing -Method Get -Uri $MetricsUrl -TimeoutSec 5
    return [string]$response.Content
}

function Assert-LowSensitiveMetricsUrl {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        throw "MetricsUrl is required in live mode."
    }
    try {
        $uri = [System.Uri]$Value
    } catch {
        throw "MetricsUrl must be a valid HTTP(S) URL."
    }
    if (-not $uri.IsAbsoluteUri -or @("http", "https") -notcontains $uri.Scheme.ToLowerInvariant()) {
        throw "MetricsUrl must be an absolute HTTP(S) URL."
    }
    if (-not [string]::IsNullOrWhiteSpace($uri.UserInfo) -or
        -not [string]::IsNullOrWhiteSpace($uri.Query) -or
        -not [string]::IsNullOrWhiteSpace($uri.Fragment)) {
        throw "MetricsUrl must not contain userinfo, query, or fragment because it is written into the observation report."
    }
    if ($Value -match "(?i)(bearer\s+\S+|token\s*=|password\s*=|secret\s*=|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.)") {
        throw "MetricsUrl contains a sensitive-looking value."
    }
}

function Assert-ObservationOutputRootOutsideRepository {
    param(
        [string]$Value,
        [string]$RepositoryRoot
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        throw "OutputRoot is required."
    }

    $fullOutputRoot = [System.IO.Path]::GetFullPath($Value)
    $fullRepositoryRoot = [System.IO.Path]::GetFullPath($RepositoryRoot).TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
    $repositoryPrefix = $fullRepositoryRoot + [System.IO.Path]::DirectorySeparatorChar

    if ($fullOutputRoot.Equals($fullRepositoryRoot, [System.StringComparison]::OrdinalIgnoreCase) -or
        $fullOutputRoot.StartsWith($repositoryPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "OutputRoot must not be inside the repository. Store raw api-gateway observation output under H:\NexusIM\loadtest-results or another external scratch directory."
    }
}

function Add-Argument {
    param(
        [System.Collections.Generic.List[string]]$Arguments,
        [string]$Name,
        [string]$Value
    )
    if (-not [string]::IsNullOrWhiteSpace($Value)) {
        $Arguments.Add($Name)
        $Arguments.Add($Value)
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$legacyGate = Join-Path $PSScriptRoot "check-api-gateway-legacy-descriptor-migration.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

Assert-ObservationOutputRootOutsideRepository -Value $OutputRoot -RepositoryRoot $repoRoot

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "api-gateway-legacy-observation-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
Assert-LowSensitiveRepairActor -Value $RunName -FieldName "RunName"
if ([string]::IsNullOrWhiteSpace($SnapshotPath)) {
    Assert-LowSensitiveMetricsUrl -Value $MetricsUrl
}

$runDir = Join-Path $OutputRoot $RunName
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

$snapshotRaw = Read-MetricsSnapshotRaw
$snapshotObject = $snapshotRaw | ConvertFrom-Json
$snapshotOutput = Join-Path $runDir "api-gateway-metrics.json"
$snapshotRaw | Set-Content -LiteralPath $snapshotOutput -Encoding UTF8

$gateArguments = [System.Collections.Generic.List[string]]::new()
$gateArguments.Add("-NoProfile")
$gateArguments.Add("-ExecutionPolicy")
$gateArguments.Add("Bypass")
$gateArguments.Add("-File")
$gateArguments.Add($legacyGate)
$gateArguments.Add("-SnapshotPath")
$gateArguments.Add($snapshotOutput)
Add-Argument -Arguments $gateArguments -Name "-RequiredQuietDuration" -Value $RequiredQuietDuration
Add-Argument -Arguments $gateArguments -Name "-MaxSnapshotAge" -Value $MaxSnapshotAge
if ($NowUnixMS -gt 0) {
    $gateArguments.Add("-NowUnixMS")
    $gateArguments.Add([string]$NowUnixMS)
}
if ($AllowRegisteredLegacyDescriptors) {
    $gateArguments.Add("-AllowRegisteredLegacyDescriptors")
}
if ($AllowObservedLegacyTraffic) {
    $gateArguments.Add("-AllowObservedLegacyTraffic")
}
if (-not $AllowMissingFacadeTraffic) {
    $gateArguments.Add("-RequireFacadeTraffic")
}
if (-not $AllowOtherTraffic) {
    $gateArguments.Add("-DisallowOtherTraffic")
}
if (-not $AllowFutureLegacyOptIn) {
    $gateArguments.Add("-RequireLegacyOptInExpiredOrUnset")
}

$gateOutput = & $powerShellExe @gateArguments 2>&1
$gateExitCode = $LASTEXITCODE
$gateOutputText = ($gateOutput | Out-String).TrimEnd()
$gateOutputPath = Join-Path $runDir "legacy-gate-output.txt"
$gateOutputText | Set-Content -LiteralPath $gateOutputPath -Encoding UTF8

$nowMS = $NowUnixMS
if ($nowMS -le 0) {
    $nowMS = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
}

$snapshotGeneratedAtMS = Get-Int64OrZero $snapshotObject.generated_at_ms
$registered = $false
$legacyAllowedUntilMS = [int64]0
if ($null -ne $snapshotObject.runtime) {
    if ($null -ne $snapshotObject.runtime.register_legacy_descriptors) {
        $registered = [bool]$snapshotObject.runtime.register_legacy_descriptors
    }
    $legacyAllowedUntilMS = Get-Int64OrZero $snapshotObject.runtime.legacy_descriptors_allowed_until_unix_ms
}

$facadeRequests = [int64]0
$legacyRequests = [int64]0
$legacyLastSeenMS = [int64]0
$otherRequests = [int64]0
if ($null -ne $snapshotObject.grpc) {
    $facadeRequests = Get-Int64OrZero $snapshotObject.grpc.facade_requests
    $legacyRequests = Get-Int64OrZero $snapshotObject.grpc.legacy_descriptor_requests
    $legacyLastSeenMS = Get-Int64OrZero $snapshotObject.grpc.legacy_descriptor_last_seen_unix_ms
    $otherRequests = Get-Int64OrZero $snapshotObject.grpc.other_requests
}

$requiredQuietMS = Convert-DurationToMilliseconds $RequiredQuietDuration
$maxSnapshotAgeMS = Convert-DurationToMilliseconds $MaxSnapshotAge
$legacyQuietAgeMS = [int64]0
if ($legacyLastSeenMS -gt 0) {
    $legacyQuietAgeMS = $nowMS - $legacyLastSeenMS
}
$snapshotAgeMS = [int64]0
if ($snapshotGeneratedAtMS -gt 0) {
    $snapshotAgeMS = $nowMS - $snapshotGeneratedAtMS
}

$summary = [ordered]@{
    run_name = $RunName
    generated_at_unix_ms = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    source = $(if ([string]::IsNullOrWhiteSpace($SnapshotPath)) { "live" } else { "snapshot" })
    metrics_url = $(if ([string]::IsNullOrWhiteSpace($SnapshotPath)) { $MetricsUrl } else { "" })
    input_snapshot_path = $SnapshotPath
    output_dir = $runDir
    snapshot_path = $snapshotOutput
    gate_output_path = $gateOutputPath
    gate_passed = ($gateExitCode -eq 0)
    gate_exit_code = $gateExitCode
    registered_legacy_descriptors = $registered
    facade_requests = $facadeRequests
    legacy_descriptor_requests = $legacyRequests
    other_requests = $otherRequests
    legacy_descriptor_last_seen_unix_ms = $legacyLastSeenMS
    legacy_descriptors_allowed_until_unix_ms = $legacyAllowedUntilMS
    snapshot_generated_at_unix_ms = $snapshotGeneratedAtMS
    snapshot_age_ms = $snapshotAgeMS
    max_snapshot_age_ms = $maxSnapshotAgeMS
    required_quiet_ms = $requiredQuietMS
    legacy_quiet_age_ms = $legacyQuietAgeMS
    require_facade_traffic = (-not $AllowMissingFacadeTraffic)
    disallow_other_traffic = (-not $AllowOtherTraffic)
    require_legacy_opt_in_expired_or_unset = (-not $AllowFutureLegacyOptIn)
}

$summaryPath = Join-Path $runDir "legacy-observation-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

$statusText = if ($gateExitCode -eq 0) { "PASS" } else { "FAIL" }
$reportPath = Join-Path $runDir "legacy-observation-report.md"
$report = @"
# api-gateway Legacy Descriptor Observation

This report is generated by `tools/record-api-gateway-legacy-observation.ps1`.

## Result

- gate_status: `$statusText`
- run_name: `$RunName`
- source: `$($summary.source)`
- snapshot_path: `$snapshotOutput`
- gate_output_path: `$gateOutputPath`

## Snapshot Summary

- registered_legacy_descriptors: `$registered`
- facade_requests: `$facadeRequests`
- legacy_descriptor_requests: `$legacyRequests`
- other_requests: `$otherRequests`
- legacy_descriptor_last_seen_unix_ms: `$legacyLastSeenMS`
- legacy_descriptors_allowed_until_unix_ms: `$legacyAllowedUntilMS`
- snapshot_generated_at_unix_ms: `$snapshotGeneratedAtMS`
- snapshot_age_ms: `$snapshotAgeMS`
- max_snapshot_age_ms: `$maxSnapshotAgeMS`
- required_quiet_ms: `$requiredQuietMS`
- legacy_quiet_age_ms: `$legacyQuietAgeMS`

## Gate Output

```text
$gateOutputText
```

## Boundary

This observation proves only the supplied api-gateway metrics snapshot and selected quiet-window gate options. It is intended for local development, migration review and interview evidence. It does not prove production SLO, complete traffic migration across all environments, or permanent legacy descriptor removal.
"@
$report | Set-Content -LiteralPath $reportPath -Encoding UTF8

Write-Host "api_gateway_legacy_observation_status=$statusText"
Write-Host "api_gateway_legacy_observation_dir=$runDir"
Write-Host "api_gateway_legacy_observation_summary=$summaryPath"
Write-Host "api_gateway_legacy_observation_report=$reportPath"

if ($gateExitCode -ne 0) {
    exit $gateExitCode
}
