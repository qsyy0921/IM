$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$policyPath = Join-Path $repoRoot "deploy\local\otel-sampling-policy.json"

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\", "/")
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if ($fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($prefix.Length)
    }
    return $fullPath
}

function Add-Violation {
    param(
        [System.Collections.Generic.List[string]]$Violations,
        [string]$Path,
        [string]$Message
    )

    $Violations.Add("$(Convert-ToRepoRelativePath -Path $Path): $Message")
}

if (-not (Test-Path -LiteralPath $policyPath)) {
    throw "Missing OTel sampling policy file: $policyPath"
}

$policy = Get-Content -LiteralPath $policyPath -Raw | ConvertFrom-Json
$violations = [System.Collections.Generic.List[string]]::new()

foreach ($service in $policy.services) {
    $serviceName = [string]$service.service
    $envPrefix = [string]$service.env_prefix
    $serviceRoot = Join-Path $repoRoot "services\$serviceName"
    $mainPath = Join-Path $serviceRoot "cmd\$serviceName\main.go"
    $mainTestPath = Join-Path $serviceRoot "cmd\$serviceName\main_test.go"
    $tracePath = Join-Path $serviceRoot "internal\infrastructure\monitoring\otel_trace.go"
    $traceTestPath = Join-Path $serviceRoot "internal\infrastructure\monitoring\otel_trace_test.go"

    if (-not (Test-Path -LiteralPath $mainPath)) {
        $violations.Add("services\${serviceName}: service listed in OTel sampling policy but cmd main.go is missing")
        continue
    }
    if (-not (Test-Path -LiteralPath $mainTestPath)) {
        Add-Violation $violations $mainPath "service listed in OTel sampling policy but cmd main_test.go is missing"
        continue
    }
    if (-not (Test-Path -LiteralPath $tracePath)) {
        Add-Violation $violations $mainPath "service listed in OTel sampling policy but monitoring/otel_trace.go is missing"
    }

    $main = Get-Content -LiteralPath $mainPath -Raw
    $mainTest = Get-Content -LiteralPath $mainTestPath -Raw
    $trace = ""
    if (Test-Path -LiteralPath $tracePath) {
        $trace = Get-Content -LiteralPath $tracePath -Raw
    }

    foreach ($suffix in @("_TRACES_ENABLED", "_SERVICE_NAME", "_TRACES_EXPORTER", "_TRACES_OTLP_ENDPOINT", "_TRACES_OTLP_INSECURE", "_TRACES_SAMPLING_RATIO")) {
        $envName = "$envPrefix$suffix"
        if (-not $main.Contains($envName)) {
            Add-Violation $violations $mainPath "missing OTel env wiring for $envName"
        }
    }

    foreach ($required in @("NewTraceRuntime", "traceRuntime.Shutdown", "WithTraceStats(traceRuntime.Snapshot")) {
        if (-not $main.Contains($required)) {
            Add-Violation $violations $mainPath "missing OTel runtime wiring: $required"
        }
    }

    foreach ($requiredTestValue in @("$($envPrefix)_TRACES_ENABLED", "$($envPrefix)_TRACES_SAMPLING_RATIO", "$($envPrefix)_TRACES_OTLP_INSECURE")) {
        if (-not $mainTest.Contains($requiredTestValue)) {
            Add-Violation $violations $mainTestPath "missing OTel env validation coverage for $requiredTestValue"
        }
    }

    if ($main.Contains("TraceRecorder:")) {
        if (-not $main.Contains("TraceRecorder:     traceRuntime")) {
            Add-Violation $violations $mainPath "WebSocket trace recorder is not wired to traceRuntime"
        }
        if (-not $trace.Contains("StartWebSocketConnection")) {
            Add-Violation $violations $tracePath "WebSocket trace runtime is missing StartWebSocketConnection"
        }
        $websocketTestPath = Join-Path $serviceRoot "internal\api\websocket\server_test.go"
        if (-not (Test-Path -LiteralPath $websocketTestPath)) {
            Add-Violation $violations $mainPath "WebSocket service is missing trace recorder server test"
        } else {
            $websocketTest = Get-Content -LiteralPath $websocketTestPath -Raw
            if (-not $websocketTest.Contains("TestWebSocketTraceRecorderReceivesLowSensitiveContext")) {
                Add-Violation $violations $websocketTestPath "WebSocket trace recorder is missing low-sensitive context regression test"
            }
        }
    } else {
        if (-not $main.Contains("traceRuntime.UnaryServerInterceptor()")) {
            Add-Violation $violations $mainPath "gRPC service is missing traceRuntime.UnaryServerInterceptor wiring"
        }
        if (-not (Test-Path -LiteralPath $traceTestPath)) {
            Add-Violation $violations $tracePath "gRPC trace runtime is missing otel_trace_test.go"
        } else {
            $traceTest = Get-Content -LiteralPath $traceTestPath -Raw
            foreach ($requiredTestValue in @("traceparent", "rpc.system", "grpc")) {
                if (-not $traceTest.Contains($requiredTestValue)) {
                    Add-Violation $violations $traceTestPath "gRPC trace test is missing expected low-sensitive coverage token '$requiredTestValue'"
                }
            }
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   OTel service wiring guardrails"
