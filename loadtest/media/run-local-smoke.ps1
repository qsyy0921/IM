param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$MediaGrpcAddr = "",
    [string]$FakeObjectBaseURL = "http://media.local/fake",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "media-service-grpc-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\media-service.exe") ./services/media-service/cmd/media-service
    go build -o (Join-Path $repoRoot "bin\media-smoke.exe") ./loadtest/media
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
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 20
    )
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
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port
    )
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

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $psi
    $null = $process.Start()
    Register-ObjectEvent `
        -InputObject $process `
        -EventName OutputDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stdout | Out-Null
    Register-ObjectEvent `
        -InputObject $process `
        -EventName ErrorDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stderr | Out-Null
    $process.BeginOutputReadLine()
    $process.BeginErrorReadLine()

    Wait-Tcp -HostName "127.0.0.1" -Port $Port
    return $process
}

if (-not $MediaGrpcAddr) {
    $mediaGrpcPort = Get-FreeTcpPort
    $MediaGrpcAddr = "127.0.0.1:$mediaGrpcPort"
} else {
    $mediaGrpcPort = [int](($MediaGrpcAddr -split ":")[-1])
}

$processes = @()
try {
    $processes += Start-NexusProcess -Name "media-grpc" -FilePath (Join-Path $repoRoot "bin\media-service.exe") -Port $mediaGrpcPort -Env @{
        NEXUSIM_MEDIA_SERVICE_MODE = "grpc"
        NEXUSIM_MEDIA_GRPC_ADDR = $MediaGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEDIA_DEBUG_ADDR = ""
        NEXUSIM_MEDIA_FAKE_OBJECT_BASE_URL = $FakeObjectBaseURL
    }

    $runner = Join-Path $repoRoot "bin\media-smoke.exe"
    & $runner `
        --pg-dsn $PgDsn `
        --media-target $MediaGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName
    if ($LASTEXITCODE -ne 0) {
        throw "media smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "media_grpc_addr=$MediaGrpcAddr"
    Write-Host "result_dir=$resultDir"
} finally {
    foreach ($process in $processes) {
        if ($process -and -not $process.HasExited) {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
        }
        if ($process) {
            $process.Dispose()
        }
    }
    Get-EventSubscriber |
        Where-Object { $_.SourceObject -is [System.Diagnostics.Process] } |
        Unregister-Event -ErrorAction SilentlyContinue
}
