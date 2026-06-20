param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$VectorGrpcAddr = "",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$VectorEventsTopic = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "vector-index-outbox-relay-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if (-not $VectorEventsTopic) {
    $VectorEventsTopic = "im.vector.events.$RunName"
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\vector-index-service.exe") ./services/vector-index-service/cmd/vector-index-service
    go build -o (Join-Path $repoRoot "bin\vector-index-smoke.exe") ./loadtest/vectorindex
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

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = Resolve-Path $Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "nexusim-postgres:$containerPath" | Out-Null
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
}

function Apply-VectorMigrations {
    Get-ChildItem -Path (Join-Path $repoRoot "migrations\postgres\vector-index") -Filter "*.sql" |
        Sort-Object Name |
        ForEach-Object {
            Apply-PostgresMigration -Path $_.FullName -Name $_.Name
        }
}

function Ensure-KafkaTopic {
    param([string]$Topic)
    docker exec nexusim-kafka kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Topic `
        --partitions 1 `
        --replication-factor 1 | Out-Null
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
        [int]$Port = 0
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

    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $process
}

if (-not $VectorGrpcAddr) {
    $vectorGrpcPort = Get-FreeTcpPort
    $VectorGrpcAddr = "127.0.0.1:$vectorGrpcPort"
} else {
    $vectorGrpcPort = [int](($VectorGrpcAddr -split ":")[-1])
}

$processes = @()
try {
    Apply-VectorMigrations
    Ensure-KafkaTopic -Topic $VectorEventsTopic

    $processes += Start-NexusProcess -Name "vector-index-grpc" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Port $vectorGrpcPort -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "grpc"
        NEXUSIM_VECTOR_INDEX_GRPC_ADDR = $VectorGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
    }

    $processes += Start-NexusProcess -Name "vector-index-outbox-relay" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_VECTOR_EVENTS_TOPIC = $VectorEventsTopic
        NEXUSIM_VECTOR_OUTBOX_BATCH_SIZE = "100"
        NEXUSIM_VECTOR_OUTBOX_POLL_INTERVAL = "200ms"
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
    }

    $processes += Start-NexusProcess -Name "vector-index-rebuild-worker" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "rebuild-worker"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_VECTOR_REBUILD_BATCH_SIZE = "10"
        NEXUSIM_VECTOR_REBUILD_POLL_INTERVAL = "200ms"
        NEXUSIM_VECTOR_REBUILD_ERROR_BACKOFF = "200ms"
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
    }

    $runner = Join-Path $repoRoot "bin\vector-index-smoke.exe"
    $runnerArgs = @(
        "--pg-dsn", $PgDsn,
        "--vector-target", $VectorGrpcAddr,
        "--result-root", $ResultRoot,
        "--run-name", $RunName,
        "--kafka-brokers", $KafkaBrokers,
        "--vector-events-topic", $VectorEventsTopic,
        "--expect-rebuild-completed",
        "--wait-timeout", "20s"
    )
    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "vector-index smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "vector_grpc_addr=$VectorGrpcAddr"
    Write-Host "vector_events_topic=$VectorEventsTopic"
    Write-Host "kafka_brokers=$KafkaBrokers"
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
