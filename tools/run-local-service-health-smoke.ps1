param(
    [int]$TimeoutSeconds = 120,
    [switch]$SkipImageBuild,
    [switch]$KeepRunning,
    [switch]$RecordResourceSnapshot,
    [string]$RunName = "",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results"
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

$repoRoot = Split-Path -Parent $PSScriptRoot
$baseCompose = Join-Path $repoRoot "deploy\local\docker-compose.yml"
$serviceCompose = Join-Path $repoRoot "deploy\local\docker-compose.services.yml"
$buildScript = Join-Path $PSScriptRoot "build-service-docker-images.ps1"

$serviceProcesses = @(
    "conversation-service-grpc",
    "policy-service-grpc",
    "message-service-grpc",
    "delivery-service-grpc",
    "receipt-service-grpc",
    "contacts-service-grpc",
    "identity-service-grpc",
    "push-gateway-ws",
    "api-gateway-grpc"
)

$baseProcesses = @("postgres", "redis", "kafka")

$checks = @(
    @{ Name = "api-gateway"; Url = "http://127.0.0.1:11904" },
    @{ Name = "identity-service"; Url = "http://127.0.0.1:11905" },
    @{ Name = "message-service"; Url = "http://127.0.0.1:11910" },
    @{ Name = "conversation-service"; Url = "http://127.0.0.1:11911" },
    @{ Name = "delivery-service"; Url = "http://127.0.0.1:11912" },
    @{ Name = "push-gateway"; Url = "http://127.0.0.1:11913" },
    @{ Name = "receipt-service"; Url = "http://127.0.0.1:11914" },
    @{ Name = "contacts-service"; Url = "http://127.0.0.1:11915" },
    @{ Name = "policy-service"; Url = "http://127.0.0.1:11916" }
)

function Invoke-Compose {
    param([string[]]$ComposeArgs)

    & docker compose -f $serviceCompose @ComposeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose $($ComposeArgs -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function Invoke-BaseCompose {
    param([string[]]$ComposeArgs)

    & docker compose -f $baseCompose @ComposeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose $($ComposeArgs -join ' ') failed with exit code $LASTEXITCODE"
    }
}

function Test-ContainerRunning {
    param([string]$ContainerName)

    $running = docker inspect -f "{{.State.Running}}" $ContainerName 2>$null
    return ($LASTEXITCODE -eq 0 -and $running -eq "true")
}

function Invoke-Endpoint {
    param(
        [string]$Name,
        [string]$Url,
        [string]$Path
    )

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "$Url$Path" -TimeoutSec 2
        if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
            return $true
        }
    }
    catch {
        return $false
    }
    return $false
}

function Wait-ServiceEndpoints {
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $pending = @{}
    foreach ($check in $checks) {
        $pending[[string]$check.Name] = [string]$check.Url
    }

    while ($pending.Count -gt 0 -and (Get-Date) -lt $deadline) {
        foreach ($name in @($pending.Keys)) {
            $url = $pending[$name]
            $healthOK = Invoke-Endpoint -Name $name -Url $url -Path "/healthz"
            $readyOK = Invoke-Endpoint -Name $name -Url $url -Path "/readyz"
            if ($healthOK -and $readyOK) {
                Write-Host "OK   $name healthz/readyz"
                $pending.Remove($name)
            }
        }
        if ($pending.Count -gt 0) {
            Start-Sleep -Seconds 2
        }
    }

    if ($pending.Count -gt 0) {
        foreach ($service in $serviceProcesses) {
            Write-Host "== logs: $service =="
            docker compose -f $serviceCompose logs --tail 80 $service 2>$null
        }
        $missing = ($pending.Keys | Sort-Object) -join ", "
        throw "local service health smoke timed out waiting for: $missing"
    }
}

function New-DefaultRunName {
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    return "local-service-health-smoke-$timestamp"
}

function Write-ResourceSnapshot {
    if (-not $RecordResourceSnapshot) {
        return
    }

    $actualRunName = if ($RunName.Trim().Length -gt 0) { $RunName.Trim() } else { New-DefaultRunName }
    $runDirectory = Join-Path $ResultRoot $actualRunName
    New-Item -ItemType Directory -Force -Path $runDirectory | Out-Null

    $serviceContainers = @($serviceProcesses | ForEach-Object { "nexusim-$_" })
    $baseContainers = @($baseProcesses | ForEach-Object { "nexusim-$_" })
    $containers = @($serviceContainers + $baseContainers)

    $statsPath = Join-Path $runDirectory "docker-stats.jsonl"
    $statsLines = & docker stats --no-stream --format "{{json .}}" @containers
    if ($LASTEXITCODE -ne 0) {
        throw "docker stats failed with exit code $LASTEXITCODE"
    }
    $statsLines | Set-Content -LiteralPath $statsPath -Encoding UTF8

    $endpointSummary = foreach ($check in $checks) {
        $url = [string]$check.Url
        [pscustomobject]@{
            service = [string]$check.Name
            healthz = Invoke-Endpoint -Name ([string]$check.Name) -Url $url -Path "/healthz"
            readyz = Invoke-Endpoint -Name ([string]$check.Name) -Url $url -Path "/readyz"
            url = $url
        }
    }
    $endpointPath = Join-Path $runDirectory "endpoint-summary.json"
    $endpointSummary | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $endpointPath -Encoding UTF8

    $summary = [pscustomobject]@{
        run_name = $actualRunName
        created_at = (Get-Date).ToUniversalTime().ToString("o")
        result_root = $ResultRoot
        service_count = $checks.Count
        service_containers = $serviceContainers
        base_containers = $baseContainers
        docker_stats_path = $statsPath
        endpoint_summary_path = $endpointPath
        scope = "single no-stream Docker stats snapshot after healthz/readyz pass; not a capacity benchmark"
    }
    $summaryPath = Join-Path $runDirectory "run-summary.json"
    $summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

    Write-Host "resource_snapshot_dir=$runDirectory"
}

Push-Location $repoRoot
try {
    if (-not (Test-Path -LiteralPath $baseCompose)) {
        throw "Missing local base compose file: $baseCompose"
    }
    if (-not (Test-Path -LiteralPath $serviceCompose)) {
        throw "Missing local service compose file: $serviceCompose"
    }
    if (-not $SkipImageBuild) {
        & $buildScript
        if ($LASTEXITCODE -ne 0) {
            throw "build-service-docker-images.ps1 failed with exit code $LASTEXITCODE"
        }
    }

    $baseWasRunning = @{}
    foreach ($service in $baseProcesses) {
        $baseWasRunning[$service] = Test-ContainerRunning -ContainerName "nexusim-$service"
    }

    $upArgs = @("up", "-d") + $serviceProcesses
    Invoke-BaseCompose -ComposeArgs @("up", "-d", "postgres", "redis", "kafka")
    Invoke-Compose -ComposeArgs $upArgs
    Wait-ServiceEndpoints
    Write-ResourceSnapshot

    Write-Host "OK   local service health smoke passed for $($checks.Count) services."
}
finally {
    if (-not $KeepRunning) {
        try {
            $stopServiceArgs = @("stop") + $serviceProcesses
            $removeServiceArgs = @("rm", "-f") + $serviceProcesses
            Invoke-Compose -ComposeArgs $stopServiceArgs
            Invoke-Compose -ComposeArgs $removeServiceArgs
            foreach ($service in $baseProcesses) {
                if ($baseWasRunning.ContainsKey($service) -and -not [bool]$baseWasRunning[$service]) {
                    Invoke-BaseCompose -ComposeArgs @("stop", $service)
                    Invoke-BaseCompose -ComposeArgs @("rm", "-f", $service)
                }
            }
        }
        catch {
            Write-Warning "local service health smoke cleanup failed: $($_.Exception.Message)"
        }
    }
    Pop-Location
}
