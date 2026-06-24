param(
    [string[]]$OptionalAdapter = @("memory-service", "retrieval-gateway", "rag-service", "agent-action-executor", "rag-agent-demo"),
    [switch]$IncludePythonWorker,
    [switch]$PreflightOnly,
    [switch]$AllowMissing,
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$GatePolicyPath = "docs/runbook/ai-eval/gate-policy.local.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$RAGTarget = "127.0.0.1:10610",
    [string]$SummaryTarget = "127.0.0.1:10620",
    [string]$RetrievalTarget = "127.0.0.1:10590",
    [string]$SearchTarget = "127.0.0.1:10570",
    [string]$MemoryTarget = "127.0.0.1:10580",
    [string]$AgentTarget = "127.0.0.1:10630",
    [string]$ActionExecutorTarget = "127.0.0.1:10660",
    [string]$MCPGatewayTarget = "127.0.0.1:10650",
    [string]$SkillRegistryTarget = "127.0.0.1:10640",
    [string]$PolicyTarget = "127.0.0.1:10800",
    [string]$WorkflowTarget = "127.0.0.1:10750",
    [string]$Python = "python",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$RequestTimeout = "30s",
    [switch]$NoApplyMigration
)

$ErrorActionPreference = "Stop"

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function ConvertTo-NameList {
    param([string[]]$Names)

    $result = New-Object System.Collections.Generic.List[string]
    foreach ($name in @($Names)) {
        foreach ($part in ([string]$name).Split(",")) {
            $trimmed = $part.Trim()
            if ($trimmed.Length -gt 0 -and -not $result.Contains($trimmed)) {
                $result.Add($trimmed)
            }
        }
    }
    return $result.ToArray()
}

function Split-Endpoint {
    param(
        [string]$Endpoint,
        [int]$DefaultPort
    )

    $trimmed = $Endpoint.Trim()
    $hostValue = $trimmed
    $portValue = $DefaultPort
    if ($trimmed.Contains(":")) {
        $lastColon = $trimmed.LastIndexOf(":")
        $hostValue = $trimmed.Substring(0, $lastColon)
        $portText = $trimmed.Substring($lastColon + 1)
        $parsed = 0
        if ([int]::TryParse($portText, [ref]$parsed)) {
            $portValue = $parsed
        }
    }
    if ($hostValue -eq "" -or $hostValue -eq "localhost") {
        $hostValue = "127.0.0.1"
    }
    return [pscustomobject]@{
        host = $hostValue
        port = $portValue
    }
}

function Get-PostgresEndpoint {
    param([string]$DSN)

    $hostValue = "127.0.0.1"
    $portValue = 5432
    if ($DSN -match "@([^/:?]+)(?::([0-9]+))?") {
        $hostValue = $Matches[1]
        if ($Matches[2]) {
            $portValue = [int]$Matches[2]
        }
    }
    if ($hostValue -eq "localhost") {
        $hostValue = "127.0.0.1"
    }
    return [pscustomobject]@{
        host = $hostValue
        port = $portValue
    }
}

function Test-TcpEndpoint {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutMS = 1000
    )

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $async = $client.BeginConnect($HostName, $Port, $null, $null)
        if (-not $async.AsyncWaitHandle.WaitOne($TimeoutMS)) {
            return $false
        }
        $client.EndConnect($async)
        return $true
    }
    catch {
        return $false
    }
    finally {
        $client.Close()
    }
}

function Add-EndpointCheck {
    param(
        [System.Collections.Generic.List[object]]$Checks,
        [System.Collections.Generic.HashSet[string]]$Seen,
        [string]$Name,
        [string]$Endpoint,
        [int]$DefaultPort
    )

    if (-not $Seen.Add($Name)) {
        return
    }
    $parsed = Split-Endpoint -Endpoint $Endpoint -DefaultPort $DefaultPort
    $Checks.Add([pscustomobject]@{
        name = $Name
        host = $parsed.host
        port = $parsed.port
        ok = Test-TcpEndpoint -HostName $parsed.host -Port $parsed.port
    })
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$adapters = ConvertTo-NameList -Names $OptionalAdapter
if ($IncludePythonWorker -and -not ($adapters -contains "python-ai-worker")) {
    $adapters += "python-ai-worker"
}
Assert-Condition ($adapters.Count -gt 0) "at least one optional adapter must be selected"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "ai-eval-service-stack-gate-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}

$checks = New-Object System.Collections.Generic.List[object]
$seenChecks = New-Object System.Collections.Generic.HashSet[string]
if ($adapters -contains "rag-service") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "rag-service" -Endpoint $RAGTarget -DefaultPort 10610
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "retrieval-gateway" -Endpoint $RetrievalTarget -DefaultPort 10590
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "search-service" -Endpoint $SearchTarget -DefaultPort 10570
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "memory-service" -Endpoint $MemoryTarget -DefaultPort 10580
}
if ($adapters -contains "memory-service") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "memory-service" -Endpoint $MemoryTarget -DefaultPort 10580
    $firstBroker = ($KafkaBrokers.Split(",") | Select-Object -First 1).Trim()
    if ($firstBroker.Length -gt 0) {
        Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "kafka" -Endpoint $firstBroker -DefaultPort 9092
    }
}
if ($adapters -contains "retrieval-gateway") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "retrieval-gateway" -Endpoint $RetrievalTarget -DefaultPort 10590
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "search-service" -Endpoint $SearchTarget -DefaultPort 10570
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "memory-service" -Endpoint $MemoryTarget -DefaultPort 10580
}
if ($adapters -contains "retrieval-gateway-negative") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "retrieval-gateway" -Endpoint $RetrievalTarget -DefaultPort 10590
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "search-service" -Endpoint $SearchTarget -DefaultPort 10570
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "memory-service" -Endpoint $MemoryTarget -DefaultPort 10580
}
if ($adapters -contains "summary-service") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "summary-service" -Endpoint $SummaryTarget -DefaultPort 10620
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "retrieval-gateway" -Endpoint $RetrievalTarget -DefaultPort 10590
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "search-service" -Endpoint $SearchTarget -DefaultPort 10570
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "memory-service" -Endpoint $MemoryTarget -DefaultPort 10580
}
if ($adapters -contains "agent-action-executor") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "agent-service" -Endpoint $AgentTarget -DefaultPort 10630
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "action-executor" -Endpoint $ActionExecutorTarget -DefaultPort 10660
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "retrieval-gateway" -Endpoint $RetrievalTarget -DefaultPort 10590
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "mcp-gateway" -Endpoint $MCPGatewayTarget -DefaultPort 10650
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "skill-registry" -Endpoint $SkillRegistryTarget -DefaultPort 10640
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "policy-service" -Endpoint $PolicyTarget -DefaultPort 10800
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "workflow-service" -Endpoint $WorkflowTarget -DefaultPort 10750
}
if ($adapters -contains "rag-agent-demo") {
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "rag-service" -Endpoint $RAGTarget -DefaultPort 10610
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "agent-service" -Endpoint $AgentTarget -DefaultPort 10630
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "action-executor" -Endpoint $ActionExecutorTarget -DefaultPort 10660
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "retrieval-gateway" -Endpoint $RetrievalTarget -DefaultPort 10590
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "search-service" -Endpoint $SearchTarget -DefaultPort 10570
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "memory-service" -Endpoint $MemoryTarget -DefaultPort 10580
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "mcp-gateway" -Endpoint $MCPGatewayTarget -DefaultPort 10650
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "skill-registry" -Endpoint $SkillRegistryTarget -DefaultPort 10640
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "policy-service" -Endpoint $PolicyTarget -DefaultPort 10800
    Add-EndpointCheck -Checks $checks -Seen $seenChecks -Name "workflow-service" -Endpoint $WorkflowTarget -DefaultPort 10750
}
if ($checks.Count -gt 0) {
    $pgEndpoint = Get-PostgresEndpoint -DSN $PGDSN
    if ($seenChecks.Add("postgres")) {
        $checks.Add([pscustomobject]@{
            name = "postgres"
            host = $pgEndpoint.host
            port = $pgEndpoint.port
            ok = Test-TcpEndpoint -HostName $pgEndpoint.host -Port $pgEndpoint.port
        })
    }
}

$missing = @($checks | Where-Object { -not [bool]$_.ok })
$preflightStatus = "ready"
if ($missing.Count -gt 0) {
    $preflightStatus = "missing"
}
$preflight = [pscustomobject]@{
    schema_version = 1
    status = $preflightStatus
    scope = "ai-eval optional service-stack gate preflight; low-sensitive endpoint readiness only"
    run_name = $RunName
    selected_optional_adapters = $adapters
    checks = $checks
}
$preflightPath = Join-Path $resultDir "ai-eval-service-stack-preflight-summary.json"
$preflight | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $preflightPath -Encoding UTF8

if ($missing.Count -gt 0) {
    $missingText = ($missing | ForEach-Object { "$($_.name)=$($_.host):$($_.port)" }) -join ", "
    if (-not $AllowMissing) {
        throw "AI eval service-stack preflight failed; missing: $missingText; summary: $preflightPath"
    }
    Write-Warning "AI eval service-stack preflight missing endpoints: $missingText"
}

if ($PreflightOnly) {
    Write-Host "OK   ai-eval service-stack preflight summary written: $preflightPath"
    return
}

if ($missing.Count -gt 0) {
    throw "cannot run live gate with missing endpoints; rerun without -PreflightOnly after starting services"
}

$gateOutputPath = Join-Path $resultDir "ai-eval-regression-gate-summary.json"
$gateScript = Join-Path $PSScriptRoot "run-ai-eval-regression-gate-smoke.ps1"
$optionalAdapterValue = $adapters -join ","
if ($NoApplyMigration) {
    & $gateScript `
        -CasePath $CasePath `
        -GatePolicyPath $GatePolicyPath `
        -PGDSN $PGDSN `
        -KafkaBrokers $KafkaBrokers `
        -MemoryTarget $MemoryTarget `
        -RetrievalTarget $RetrievalTarget `
        -RAGTarget $RAGTarget `
        -SummaryTarget $SummaryTarget `
        -AgentTarget $AgentTarget `
        -ActionExecutorTarget $ActionExecutorTarget `
        -WorkflowTarget $WorkflowTarget `
        -Python $Python `
        -ResultRoot $ResultRoot `
        -RunName $RunName `
        -OutputPath $gateOutputPath `
        -TenantID "nexusim-local" `
        -UserID "ai-eval-service-stack-smoke" `
        -DeviceID "ai-eval-service-stack-smoke-device" `
        -RequestTimeout $RequestTimeout `
        -OptionalAdapter $optionalAdapterValue `
        -NoApplyMigration
} else {
    & $gateScript `
        -CasePath $CasePath `
        -GatePolicyPath $GatePolicyPath `
        -PGDSN $PGDSN `
        -KafkaBrokers $KafkaBrokers `
        -MemoryTarget $MemoryTarget `
        -RetrievalTarget $RetrievalTarget `
        -RAGTarget $RAGTarget `
        -SummaryTarget $SummaryTarget `
        -AgentTarget $AgentTarget `
        -ActionExecutorTarget $ActionExecutorTarget `
        -WorkflowTarget $WorkflowTarget `
        -Python $Python `
        -ResultRoot $ResultRoot `
        -RunName $RunName `
        -OutputPath $gateOutputPath `
        -TenantID "nexusim-local" `
        -UserID "ai-eval-service-stack-smoke" `
        -DeviceID "ai-eval-service-stack-smoke-device" `
        -RequestTimeout $RequestTimeout `
        -OptionalAdapter $optionalAdapterValue
}
if ($LASTEXITCODE -ne 0) {
    throw "AI eval service-stack gate smoke failed with exit code $LASTEXITCODE"
}

Write-Host "OK   ai-eval service-stack gate smoke completed: $gateOutputPath"
