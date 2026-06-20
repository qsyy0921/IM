param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$MediaGrpcAddr = "",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$MediaEventsTopic = "",
    [string]$FakeObjectBaseURL = "http://media.local/fake",
    [switch]$WithOutboxRelay,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $prefix = if ($WithOutboxRelay) { "media-service-outbox-relay-smoke-" } else { "media-service-grpc-smoke-" }
    $RunName = $prefix + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ($WithOutboxRelay -and -not $MediaEventsTopic) {
    $MediaEventsTopic = "im.media.events.$RunName"
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

function Apply-MediaMigrations {
    Get-ChildItem -Path (Join-Path $repoRoot "migrations\postgres\media") -Filter "*.sql" |
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

    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
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
    if ($WithOutboxRelay) {
        Apply-MediaMigrations
        Ensure-KafkaTopic -Topic $MediaEventsTopic
    }

    $processes += Start-NexusProcess -Name "media-grpc" -FilePath (Join-Path $repoRoot "bin\media-service.exe") -Port $mediaGrpcPort -Env @{
        NEXUSIM_MEDIA_SERVICE_MODE = "grpc"
        NEXUSIM_MEDIA_GRPC_ADDR = $MediaGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEDIA_DEBUG_ADDR = ""
        NEXUSIM_MEDIA_FAKE_OBJECT_BASE_URL = $FakeObjectBaseURL
    }

    $processes += Start-NexusProcess -Name "media-processing-worker" -FilePath (Join-Path $repoRoot "bin\media-service.exe") -Env @{
        NEXUSIM_MEDIA_SERVICE_MODE = "processing-worker"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEDIA_PROCESSING_BATCH_SIZE = "50"
        NEXUSIM_MEDIA_PROCESSING_POLL_INTERVAL = "100ms"
        NEXUSIM_MEDIA_PROCESSING_RETRY_BASE_DELAY = "200ms"
        NEXUSIM_MEDIA_DEBUG_ADDR = ""
    }

    if ($WithOutboxRelay) {
        $processes += Start-NexusProcess -Name "media-outbox-relay" -FilePath (Join-Path $repoRoot "bin\media-service.exe") -Env @{
            NEXUSIM_MEDIA_SERVICE_MODE = "outbox-relay"
            NEXUSIM_PG_DSN = $PgDsn
            NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
            NEXUSIM_MEDIA_EVENTS_TOPIC = $MediaEventsTopic
            NEXUSIM_MEDIA_OUTBOX_BATCH_SIZE = "100"
            NEXUSIM_MEDIA_OUTBOX_POLL_INTERVAL = "200ms"
            NEXUSIM_MEDIA_DEBUG_ADDR = ""
        }
    }

    $runner = Join-Path $repoRoot "bin\media-smoke.exe"
    $runnerArgs = @(
        "--pg-dsn", $PgDsn,
        "--media-target", $MediaGrpcAddr,
        "--result-root", $ResultRoot,
        "--run-name", $RunName
    )
    if ($WithOutboxRelay) {
        $runnerArgs += @(
            "--kafka-brokers", $KafkaBrokers,
            "--media-events-topic", $MediaEventsTopic,
            "--wait-timeout", "20s"
        )
    }
    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "media smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "media_grpc_addr=$MediaGrpcAddr"
    if ($WithOutboxRelay) {
        Write-Host "media_events_topic=$MediaEventsTopic"
        Write-Host "kafka_brokers=$KafkaBrokers"
    }
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
