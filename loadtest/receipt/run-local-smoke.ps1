param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$ConversationTarget = "127.0.0.1:11696",
    [string]$MessageTarget = "127.0.0.1:11695",
    [string]$DeliveryTarget = "127.0.0.1:11697",
    [string]$ReceiptTarget = "127.0.0.1:11699",
    [string]$ConversationTlsCaFile = "",
    [string]$ConversationTlsServerName = "",
    [string]$ConversationTlsClientCertFile = "",
    [string]$ConversationTlsClientKeyFile = "",
    [string]$MessageTlsCaFile = "",
    [string]$MessageTlsServerName = "",
    [string]$MessageTlsClientCertFile = "",
    [string]$MessageTlsClientKeyFile = "",
    [string]$DeliveryTlsCaFile = "",
    [string]$DeliveryTlsServerName = "",
    [string]$DeliveryTlsClientCertFile = "",
    [string]$DeliveryTlsClientKeyFile = "",
    [string]$ReceiptTlsCaFile = "",
    [string]$ReceiptTlsServerName = "",
    [string]$ReceiptTlsClientCertFile = "",
    [string]$ReceiptTlsClientKeyFile = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "receipt-service-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$timelineTopic = "conversation.timeline.receipt." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryTopic = "im.delivery.events"
$receiptTopic = "im.receipt.events"
$deliveryConsumerGroup = "nexusim-delivery-receipt-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$receiptConsumerGroup = "nexusim-receipt-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$receiptEventsConsumerGroup = "nexusim-receipt-events-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\delivery-service.exe ./services/delivery-service/cmd/delivery-service
    go build -o bin\receipt-service.exe ./services/receipt-service/cmd/receipt-service
    go build -o bin\receipt-smoke.exe ./loadtest/receipt
}

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = Resolve-Path $Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "nexusim-postgres:$containerPath"
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
}

function Ensure-KafkaTopic {
    param([string]$Topic)
    docker exec nexusim-kafka kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Topic `
        --partitions 3 `
        --replication-factor 1 | Out-Null
}

function Reset-ConsumerGroupToLatest {
    param(
        [string]$Group,
        [string]$Topic
    )
    docker exec nexusim-kafka kafka-consumer-groups `
        --bootstrap-server localhost:9092 `
        --group $Group `
        --topic $Topic `
        --reset-offsets `
        --to-latest `
        --execute | Out-Null
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
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $proc = Start-Process -FilePath $FilePath `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
}

$processes = @()
try {
    Apply-PostgresMigration -Path "migrations\postgres\message\000001_message_core.sql" -Name "nexusim_message_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000001_conversation_core.sql" -Name "nexusim_conversation_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000002_member_change_saga_v2.sql" -Name "nexusim_conversation_member_change_saga_v2.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000003_member_change_event_unique.sql" -Name "nexusim_conversation_member_change_event_unique.sql"
    Apply-PostgresMigration -Path "migrations\postgres\delivery\000001_delivery_core.sql" -Name "nexusim_delivery_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000001_receipt_core.sql" -Name "nexusim_receipt_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000002_conversation_summary.sql" -Name "nexusim_receipt_conversation_summary.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000003_receipt_source_event_type.sql" -Name "nexusim_receipt_source_event_type.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000004_conversation_summary_source_event_type.sql" -Name "nexusim_receipt_conversation_summary_source_event_type.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000005_conversation_archive.sql" -Name "nexusim_receipt_conversation_archive.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000006_conversation_pin.sql" -Name "nexusim_receipt_conversation_pin.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000007_conversation_mute.sql" -Name "nexusim_receipt_conversation_mute.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000008_conversation_unread_filter.sql" -Name "nexusim_receipt_conversation_unread_filter.sql"

    Ensure-KafkaTopic -Topic $timelineTopic
    Ensure-KafkaTopic -Topic $deliveryTopic
    Ensure-KafkaTopic -Topic $receiptTopic
    Reset-ConsumerGroupToLatest -Group $deliveryConsumerGroup -Topic $timelineTopic
    Reset-ConsumerGroupToLatest -Group $receiptConsumerGroup -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $receiptEventsConsumerGroup -Topic $receiptTopic

    $conversationService = Join-Path $repo "bin\conversation-service.exe"
    $messageService = Join-Path $repo "bin\message-service.exe"
    $deliveryService = Join-Path $repo "bin\delivery-service.exe"
    $receiptService = Join-Path $repo "bin\receipt-service.exe"
    $runner = Join-Path $repo "bin\receipt-smoke.exe"

    $processes += Start-NexusProcess -Name "conversation-grpc" -FilePath $conversationService -Port 11696 -Env @{
        NEXUSIM_CONVERSATION_SERVICE_MODE = "grpc"
        NEXUSIM_CONVERSATION_GRPC_ADDR = "127.0.0.1:11696"
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "message-relay" -FilePath $messageService -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_KAFKA_TOPIC = $timelineTopic
        NEXUSIM_OUTBOX_WORKERS = "1"
        NEXUSIM_OUTBOX_BATCH_SIZE = "100"
        NEXUSIM_OUTBOX_POLL_INTERVAL = "200ms"
    }

    $processes += Start-NexusProcess -Name "delivery-timeline-consumer" -FilePath $deliveryService -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "timeline-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_TIMELINE_TOPIC = $timelineTopic
        NEXUSIM_DELIVERY_CONSUMER_GROUP = $deliveryConsumerGroup
    }

    $processes += Start-NexusProcess -Name "delivery-grpc" -FilePath $deliveryService -Port 11697 -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "grpc"
        NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11697"
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "delivery-outbox-relay" -FilePath $deliveryService -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_DELIVERY_OUTBOX_POLL_INTERVAL = "200ms"
    }

    $processes += Start-NexusProcess -Name "receipt-delivery-consumer" -FilePath $receiptService -Env @{
        NEXUSIM_RECEIPT_SERVICE_MODE = "delivery-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_RECEIPT_CONSUMER_GROUP = $receiptConsumerGroup
    }

    $processes += Start-NexusProcess -Name "receipt-outbox-relay" -FilePath $receiptService -Env @{
        NEXUSIM_RECEIPT_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_RECEIPT_EVENTS_TOPIC = $receiptTopic
        NEXUSIM_RECEIPT_OUTBOX_POLL_INTERVAL = "200ms"
    }

    $processes += Start-NexusProcess -Name "receipt-grpc" -FilePath $receiptService -Port 11699 -Env @{
        NEXUSIM_RECEIPT_SERVICE_MODE = "grpc"
        NEXUSIM_RECEIPT_GRPC_ADDR = "127.0.0.1:11699"
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "message-grpc" -FilePath $messageService -Port 11695 -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        NEXUSIM_GRPC_ADDR = "127.0.0.1:11695"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_SERVICE_ADDR = "127.0.0.1:11696"
        NEXUSIM_CONVERSATION_RPC_TIMEOUT = "500ms"
        NEXUSIM_MOCK_PERMISSION_VERSION = "2"
    }

    $runnerArgs = @(
        "--conversation-target", $ConversationTarget,
        "--message-target", $MessageTarget,
        "--delivery-target", $DeliveryTarget,
        "--receipt-target", $ReceiptTarget,
        "--pg-dsn", $PgDsn,
        "--result-dir", $resultDir,
        "--tenant-id", "tenant-receipt-smoke",
        "--conversation-id", "conv-receipt-smoke",
        "--owner-user-id", "owner-1",
        "--receiver-user-id", "receipt-user-1",
        "--receiver-device-id", "receipt-device-1",
        "--delivery-consumer-group", $deliveryConsumerGroup,
        "--receipt-consumer-group", $receiptConsumerGroup,
        "--kafka-brokers", $KafkaBrokers,
        "--receipt-events-topic", $receiptTopic,
        "--receipt-events-consumer-group", $receiptEventsConsumerGroup,
        "--wait-timeout", "30s",
        "--request-timeout", "5s"
    )
    if (-not [string]::IsNullOrWhiteSpace($ConversationTlsCaFile)) {
        $runnerArgs += @("--conversation-tls-ca-file", $ConversationTlsCaFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($ConversationTlsServerName)) {
        $runnerArgs += @("--conversation-tls-server-name", $ConversationTlsServerName)
    }
    if (-not [string]::IsNullOrWhiteSpace($ConversationTlsClientCertFile)) {
        $runnerArgs += @("--conversation-tls-client-cert-file", $ConversationTlsClientCertFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($ConversationTlsClientKeyFile)) {
        $runnerArgs += @("--conversation-tls-client-key-file", $ConversationTlsClientKeyFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($MessageTlsCaFile)) {
        $runnerArgs += @("--message-tls-ca-file", $MessageTlsCaFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($MessageTlsServerName)) {
        $runnerArgs += @("--message-tls-server-name", $MessageTlsServerName)
    }
    if (-not [string]::IsNullOrWhiteSpace($MessageTlsClientCertFile)) {
        $runnerArgs += @("--message-tls-client-cert-file", $MessageTlsClientCertFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($MessageTlsClientKeyFile)) {
        $runnerArgs += @("--message-tls-client-key-file", $MessageTlsClientKeyFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($DeliveryTlsCaFile)) {
        $runnerArgs += @("--delivery-tls-ca-file", $DeliveryTlsCaFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($DeliveryTlsServerName)) {
        $runnerArgs += @("--delivery-tls-server-name", $DeliveryTlsServerName)
    }
    if (-not [string]::IsNullOrWhiteSpace($DeliveryTlsClientCertFile)) {
        $runnerArgs += @("--delivery-tls-client-cert-file", $DeliveryTlsClientCertFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($DeliveryTlsClientKeyFile)) {
        $runnerArgs += @("--delivery-tls-client-key-file", $DeliveryTlsClientKeyFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($ReceiptTlsCaFile)) {
        $runnerArgs += @("--receipt-tls-ca-file", $ReceiptTlsCaFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($ReceiptTlsServerName)) {
        $runnerArgs += @("--receipt-tls-server-name", $ReceiptTlsServerName)
    }
    if (-not [string]::IsNullOrWhiteSpace($ReceiptTlsClientCertFile)) {
        $runnerArgs += @("--receipt-tls-client-cert-file", $ReceiptTlsClientCertFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($ReceiptTlsClientKeyFile)) {
        $runnerArgs += @("--receipt-tls-client-key-file", $ReceiptTlsClientKeyFile)
    }

    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "receipt smoke runner failed with exit code $LASTEXITCODE"
    }
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "timeline_topic=$timelineTopic"
Write-Host "delivery_topic=$deliveryTopic"
Write-Host "receipt_topic=$receiptTopic"
Write-Host "delivery_consumer_group=$deliveryConsumerGroup"
Write-Host "receipt_consumer_group=$receiptConsumerGroup"
Write-Host "receipt_events_consumer_group=$receiptEventsConsumerGroup"
