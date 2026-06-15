param(
    [int]$TimeoutSeconds = 120,
    [switch]$SkipImageBuild,
    [switch]$KeepRunning
)

$ErrorActionPreference = "Stop"

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
