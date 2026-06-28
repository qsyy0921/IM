param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild,
    [switch]$KeepCollector
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "policy-otel-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

function Test-CollectorRunning {
    $name = docker ps --filter "name=^/nexusim-otel-collector$" --format "{{.Names}}" 2>$null
    return ($name -eq "nexusim-otel-collector")
}

function Set-PolicyOTelEnv {
    param([string]$Name, [string]$Value)
    [Environment]::SetEnvironmentVariable($Name, $Value, "Process")
}

$collectorWasRunning = Test-CollectorRunning
if (-not $collectorWasRunning) {
    .\tools\local-up-otel.ps1
}

$previous = @{}
foreach ($name in @(
    "NEXUSIM_POLICY_OTEL_TRACES_ENABLED",
    "NEXUSIM_POLICY_OTEL_TRACES_EXPORTER",
    "NEXUSIM_POLICY_OTEL_TRACES_OTLP_ENDPOINT",
    "NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE",
    "NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO"
)) {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}

try {
    Set-PolicyOTelEnv -Name "NEXUSIM_POLICY_OTEL_TRACES_ENABLED" -Value "true"
    Set-PolicyOTelEnv -Name "NEXUSIM_POLICY_OTEL_TRACES_EXPORTER" -Value "otlp-grpc"
    Set-PolicyOTelEnv -Name "NEXUSIM_POLICY_OTEL_TRACES_OTLP_ENDPOINT" -Value "127.0.0.1:14317"
    Set-PolicyOTelEnv -Name "NEXUSIM_POLICY_OTEL_TRACES_OTLP_INSECURE" -Value "true"
    Set-PolicyOTelEnv -Name "NEXUSIM_POLICY_OTEL_TRACES_SAMPLING_RATIO" -Value "1"

    $policyArgs = @(
        "-ResultRoot", $ResultRoot,
        "-RunName", $RunName
    )
    if ($SkipBuild) {
        $policyArgs += "-SkipBuild"
    }
    $smokeStartedAt = (Get-Date).ToUniversalTime().ToString("o")
    .\loadtest\policy\run-local-smoke.ps1 @policyArgs

    Start-Sleep -Seconds 3
    $collectorLogsPath = Join-Path $resultDir "otel-collector-tail.log"
    docker logs --since $smokeStartedAt --tail 800 nexusim-otel-collector 2>&1 | Set-Content -LiteralPath $collectorLogsPath -Encoding UTF8
    $logs = Get-Content -LiteralPath $collectorLogsPath -Raw

    if ($logs -notmatch "/nexusim\.policy\.v1\.PolicyService/CheckMessageAction") {
        throw "policy-service OpenTelemetry span was not found in collector debug logs. See $collectorLogsPath"
    }
    if ($logs -notmatch "policy-service") {
        throw "policy-service resource name was not found in collector debug logs. See $collectorLogsPath"
    }

    $summaryPath = Join-Path $resultDir "policy-otel-smoke-summary.json"
    [pscustomobject]@{
        success = $true
        result_dir = $resultDir
        collector_was_running = $collectorWasRunning
        collector_logs = $collectorLogsPath
        expected_span = "/nexusim.policy.v1.PolicyService/CheckMessageAction"
        exporter = "otlp-grpc"
        endpoint = "127.0.0.1:14317"
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

    Write-Host "result_dir=$resultDir"
    Write-Host "otel_span_found=true"
} finally {
    foreach ($entry in $previous.GetEnumerator()) {
        if ($null -eq $entry.Value) {
            [Environment]::SetEnvironmentVariable($entry.Key, $null, "Process")
        } else {
            [Environment]::SetEnvironmentVariable($entry.Key, [string]$entry.Value, "Process")
        }
    }
    if (-not $collectorWasRunning -and -not $KeepCollector) {
        .\tools\local-down-otel.ps1
    }
}
