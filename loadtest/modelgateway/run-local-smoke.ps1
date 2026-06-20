param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$ModelGatewayGrpcAddr = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "model-gateway-grpc-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\model-gateway.exe") ./services/model-gateway/cmd/model-gateway
    go build -o (Join-Path $repoRoot "bin\model-gateway-smoke.exe") ./loadtest/modelgateway
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Wait-Tcp {
    param([string]$HostName, [int]$Port, [int]$TimeoutSeconds = 20)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 200
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

function Start-NexusProcess {
    param([string]$Name, [string]$FilePath, [hashtable]$Env, [int]$Port)
    $stdout = Join-Path $logDir "$Name.out.log"
    $stderr = Join-Path $logDir "$Name.err.log"
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $FilePath
    $psi.WorkingDirectory = $repoRoot
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    foreach ($key in $Env.Keys) {
        $psi.Environment[$key] = [string]$Env[$key]
    }
    $process = [System.Diagnostics.Process]::Start($psi)
    $outSub = Register-ObjectEvent -InputObject $process -EventName OutputDataReceived -Action {
        if ($EventArgs.Data) { Add-Content -Path $Event.MessageData -Value $EventArgs.Data }
    } -MessageData $stdout
    $errSub = Register-ObjectEvent -InputObject $process -EventName ErrorDataReceived -Action {
        if ($EventArgs.Data) { Add-Content -Path $Event.MessageData -Value $EventArgs.Data }
    } -MessageData $stderr
    $process.BeginOutputReadLine()
    $process.BeginErrorReadLine()
    Wait-Tcp -HostName "127.0.0.1" -Port $Port
    return [pscustomobject]@{ Process = $process; Subscriptions = @($outSub, $errSub) }
}

if (-not $ModelGatewayGrpcAddr) {
    $ModelGatewayGrpcAddr = "127.0.0.1:" + (Get-FreeTcpPort)
}
$port = [int]($ModelGatewayGrpcAddr.Split(":")[-1])

$processes = @()
try {
    $processes += Start-NexusProcess -Name "model-gateway" -FilePath (Join-Path $repoRoot "bin\model-gateway.exe") -Port $port -Env @{
        NEXUSIM_MODEL_GATEWAY_MODE = "grpc"
        NEXUSIM_MODEL_GATEWAY_GRPC_ADDR = $ModelGatewayGrpcAddr
        NEXUSIM_MODEL_GATEWAY_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    & (Join-Path $repoRoot "bin\model-gateway-smoke.exe") `
        --pg-dsn $PgDsn `
        --target $ModelGatewayGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName
    if ($LASTEXITCODE -ne 0) {
        throw "model-gateway smoke failed with exit code $LASTEXITCODE"
    }
    Write-Host "run_name=$RunName"
    Write-Host "model_gateway_grpc_addr=$ModelGatewayGrpcAddr"
    Write-Host "result_dir=$resultDir"
} finally {
    foreach ($entry in $processes) {
        if ($entry.Process -and -not $entry.Process.HasExited) {
            $entry.Process.Kill()
            $entry.Process.WaitForExit()
        }
        foreach ($sub in $entry.Subscriptions) {
            Unregister-Event -SubscriptionId $sub.Id -ErrorAction SilentlyContinue
            Remove-Job -Id $sub.Id -Force -ErrorAction SilentlyContinue
        }
    }
}
