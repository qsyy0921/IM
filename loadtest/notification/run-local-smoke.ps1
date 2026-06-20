param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$NotificationGrpcAddr = "",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$NotificationEventsTopic = "",
    [string]$ProviderMode = "noop",
    [string]$ProviderID = "",
    [string]$WebhookUrl = "",
    [string]$WebhookBearerToken = "",
    [switch]$WithDeliveryWorker,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    if ($WithDeliveryWorker) {
        $RunName = "notification-service-delivery-worker-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
    } else {
        $RunName = "notification-service-outbox-relay-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
    }
}
if (-not $NotificationEventsTopic) {
    $NotificationEventsTopic = "im.notification.events.$RunName"
}
if (-not $ProviderID) {
    $ProviderID = "local-$ProviderMode"
}
if ($WithDeliveryWorker -and $ProviderMode -eq "webhook" -and -not $WebhookUrl) {
    throw "WebhookUrl is required when ProviderMode=webhook"
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\notification-service.exe") ./services/notification-service/cmd/notification-service
    go build -o (Join-Path $repoRoot "bin\notification-smoke.exe") ./loadtest/notification
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

function Apply-NotificationMigrations {
    Get-ChildItem -Path (Join-Path $repoRoot "migrations\postgres\notification") -Filter "*.sql" |
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

if (-not $NotificationGrpcAddr) {
    $notificationGrpcPort = Get-FreeTcpPort
    $NotificationGrpcAddr = "127.0.0.1:$notificationGrpcPort"
} else {
    $notificationGrpcPort = [int](($NotificationGrpcAddr -split ":")[-1])
}

$processes = @()
try {
    Apply-NotificationMigrations
    Ensure-KafkaTopic -Topic $NotificationEventsTopic

    $processes += Start-NexusProcess -Name "notification-grpc" -FilePath (Join-Path $repoRoot "bin\notification-service.exe") -Port $notificationGrpcPort -Env @{
        NEXUSIM_NOTIFICATION_SERVICE_MODE = "grpc"
        NEXUSIM_NOTIFICATION_GRPC_ADDR = $NotificationGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_NOTIFICATION_DEBUG_ADDR = ""
        NEXUSIM_NOTIFICATION_DESTINATION_HASH_KEY = "local-notification-destination-hash-key"
    }

    if ($WithDeliveryWorker) {
        $deliveryEnv = @{
            NEXUSIM_NOTIFICATION_SERVICE_MODE = "delivery-worker"
            NEXUSIM_PG_DSN = $PgDsn
            NEXUSIM_NOTIFICATION_PROVIDER_MODE = $ProviderMode
            NEXUSIM_NOTIFICATION_PROVIDER_ID = $ProviderID
            NEXUSIM_NOTIFICATION_DELIVERY_BATCH_SIZE = "100"
            NEXUSIM_NOTIFICATION_DELIVERY_POLL_INTERVAL = "200ms"
            NEXUSIM_NOTIFICATION_DEBUG_ADDR = ""
        }
        if ($ProviderMode -eq "webhook") {
            $deliveryEnv["NEXUSIM_NOTIFICATION_WEBHOOK_URL"] = $WebhookUrl
            $deliveryEnv["NEXUSIM_NOTIFICATION_WEBHOOK_BEARER_TOKEN"] = $WebhookBearerToken
        }
        $processes += Start-NexusProcess -Name "notification-delivery-worker" -FilePath (Join-Path $repoRoot "bin\notification-service.exe") -Env $deliveryEnv
    }

    $processes += Start-NexusProcess -Name "notification-outbox-relay" -FilePath (Join-Path $repoRoot "bin\notification-service.exe") -Env @{
        NEXUSIM_NOTIFICATION_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_NOTIFICATION_EVENTS_TOPIC = $NotificationEventsTopic
        NEXUSIM_NOTIFICATION_OUTBOX_BATCH_SIZE = "100"
        NEXUSIM_NOTIFICATION_OUTBOX_POLL_INTERVAL = "200ms"
        NEXUSIM_NOTIFICATION_DEBUG_ADDR = ""
    }

    $runner = Join-Path $repoRoot "bin\notification-smoke.exe"
    $runnerArgs = @(
        "--pg-dsn", $PgDsn,
        "--notification-target", $NotificationGrpcAddr,
        "--result-root", $ResultRoot,
        "--run-name", $RunName,
        "--kafka-brokers", $KafkaBrokers,
        "--notification-events-topic", $NotificationEventsTopic,
        "--wait-timeout", "20s"
    )
    if ($WithDeliveryWorker) {
        $runnerArgs += "--expect-delivered"
    }
    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "notification smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "notification_grpc_addr=$NotificationGrpcAddr"
    Write-Host "notification_events_topic=$NotificationEventsTopic"
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
