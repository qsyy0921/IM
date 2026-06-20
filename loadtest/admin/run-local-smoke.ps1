param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$AdminGrpcAddr = "",
    [string]$ControlPlaneGrpcAddr = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "admin-config-publish-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\admin-service.exe") ./services/admin-service/cmd/admin-service
    go build -o (Join-Path $repoRoot "bin\control-plane-service.exe") ./services/control-plane-service/cmd/control-plane-service
    go build -o (Join-Path $repoRoot "bin\admin-smoke.exe") ./loadtest/admin
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
    param([string]$Name, [string]$FilePath, [hashtable]$Env, [int]$Port = 0)
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
    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 500
    }
    return [pscustomobject]@{ Process = $process; Subscriptions = @($outSub, $errSub) }
}

if (-not $AdminGrpcAddr) {
    $AdminGrpcAddr = "127.0.0.1:" + (Get-FreeTcpPort)
}
if (-not $ControlPlaneGrpcAddr) {
    $ControlPlaneGrpcAddr = "127.0.0.1:" + (Get-FreeTcpPort)
}
$adminPort = [int]($AdminGrpcAddr.Split(":")[-1])
$controlPort = [int]($ControlPlaneGrpcAddr.Split(":")[-1])

$processes = @()
try {
    $processes += Start-NexusProcess -Name "control-plane-service" -FilePath (Join-Path $repoRoot "bin\control-plane-service.exe") -Port $controlPort -Env @{
        NEXUSIM_CONTROL_PLANE_SERVICE_MODE = "grpc"
        NEXUSIM_CONTROL_PLANE_GRPC_ADDR = $ControlPlaneGrpcAddr
        NEXUSIM_CONTROL_PLANE_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "admin-service-grpc" -FilePath (Join-Path $repoRoot "bin\admin-service.exe") -Port $adminPort -Env @{
        NEXUSIM_ADMIN_SERVICE_MODE = "grpc"
        NEXUSIM_ADMIN_GRPC_ADDR = $AdminGrpcAddr
        NEXUSIM_ADMIN_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "admin-service-operation-worker" -FilePath (Join-Path $repoRoot "bin\admin-service.exe") -Env @{
        NEXUSIM_ADMIN_SERVICE_MODE = "operation-worker"
        NEXUSIM_ADMIN_DEBUG_ADDR = ""
        NEXUSIM_ADMIN_OPERATION_POLL_INTERVAL = "200ms"
        NEXUSIM_ADMIN_OPERATION_BATCH_SIZE = "5"
        NEXUSIM_ADMIN_CONTROL_PLANE_RPC_TIMEOUT = "3s"
        NEXUSIM_CONTROL_PLANE_GRPC_ADDR = $ControlPlaneGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
    }

    & (Join-Path $repoRoot "bin\admin-smoke.exe") `
        --mode config-publish-smoke `
        --pg-dsn $PgDsn `
        --target $AdminGrpcAddr `
        --control-plane-target $ControlPlaneGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName
    if ($LASTEXITCODE -ne 0) {
        throw "admin config publish smoke failed with exit code $LASTEXITCODE"
    }
    Write-Host "run_name=$RunName"
    Write-Host "admin_grpc_addr=$AdminGrpcAddr"
    Write-Host "control_plane_grpc_addr=$ControlPlaneGrpcAddr"
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
