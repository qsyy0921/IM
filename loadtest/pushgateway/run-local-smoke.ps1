param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$ReceiverDeviceIds = "push-device-1",
    [ValidateSet("full", "resume-replay", "cross-instance-resume", "slow-client", "redis-fault")]
    [string]$Scenario = "full",
    [ValidateSet("memory", "redis")]
    [string]$RouteBackend = "memory",
    [ValidateSet("single", "sentinel")]
    [string]$RedisMode = "single",
    [string]$RedisAddr = "127.0.0.1:6379",
    [string]$RedisSentinelAddrs = "127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381",
    [string]$RedisSentinelMasterName = "mymaster",
    [string]$RedisKeyPrefix = "",
    [string]$RedisFaultCommand = "",
    [string]$RedisRestoreCommand = "",
    [int]$SlowMessageCount = 128,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "push-gateway-$Scenario-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$timelineTopic = "conversation.timeline.pushgateway." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryTopic = "im.delivery.events"
$deliveryConsumerGroup = "nexusim-delivery-push-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushConsumerGroup = "nexusim-push-gateway-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushRouteKeyPrefix = $RedisKeyPrefix
if (-not $pushRouteKeyPrefix) {
    $pushRouteKeyPrefix = "nexusim:push:$safeRunName"
}
$pushGatewayID = "push-single-$safeRunName"
$pushWSGatewayID = "push-ws-$safeRunName"
$pushReconnectGatewayID = "push-ws-reconnect-$safeRunName"
$pushConsumerGatewayID = "push-consumer-$safeRunName"
$pushSessionQueueSize = "32"
$pushWriteTimeout = "2s"
$pushTestWriteDelay = "0s"
if ($Scenario -eq "slow-client") {
    $pushSessionQueueSize = "1"
    $pushWriteTimeout = "1ms"
    $pushTestWriteDelay = "50ms"
}
if ($Scenario -eq "redis-fault" -and -not $RedisFaultCommand) {
    $RedisFaultCommand = "docker stop nexusim-redis | Out-Null"
}
if ($Scenario -eq "redis-fault" -and -not $RedisRestoreCommand) {
    $RedisRestoreCommand = "docker start nexusim-redis | Out-Null"
}

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\delivery-service.exe ./services/delivery-service/cmd/delivery-service
    go build -o bin\push-gateway.exe ./services/push-gateway/cmd/push-gateway
    go build -o bin\pushgateway-smoke.exe ./loadtest/pushgateway
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

function Add-PushRedisEnv {
    param([hashtable]$Env)
    $Env["NEXUSIM_PUSH_REDIS_MODE"] = $RedisMode
    $Env["NEXUSIM_PUSH_REDIS_KEY_PREFIX"] = $pushRouteKeyPrefix
    if ($RedisMode -eq "sentinel") {
        $Env["NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS"] = $RedisSentinelAddrs
        $Env["NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME"] = $RedisSentinelMasterName
    } else {
        $Env["NEXUSIM_PUSH_REDIS_ADDR"] = $RedisAddr
    }
    return $Env
}

$processes = @()
try {
    Ensure-KafkaTopic -Topic $timelineTopic
    Ensure-KafkaTopic -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $pushConsumerGroup -Topic $deliveryTopic

    $conversationService = Join-Path $repo "bin\conversation-service.exe"
    $messageService = Join-Path $repo "bin\message-service.exe"
    $deliveryService = Join-Path $repo "bin\delivery-service.exe"
    $pushGateway = Join-Path $repo "bin\push-gateway.exe"
    $runner = Join-Path $repo "bin\pushgateway-smoke.exe"

    $processes += Start-NexusProcess -Name "conversation-grpc" -FilePath $conversationService -Port 11596 -Env @{
        NEXUSIM_CONVERSATION_SERVICE_MODE = "grpc"
        NEXUSIM_CONVERSATION_GRPC_ADDR = "127.0.0.1:11596"
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

    $processes += Start-NexusProcess -Name "delivery-grpc" -FilePath $deliveryService -Port 11597 -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "grpc"
        NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11597"
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "delivery-outbox-relay" -FilePath $deliveryService -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_DELIVERY_OUTBOX_POLL_INTERVAL = "200ms"
    }

    if ($RouteBackend -eq "redis") {
        $processes += Start-NexusProcess -Name "push-gateway-ws" -FilePath $pushGateway -Port 11598 -Env (Add-PushRedisEnv @{
            NEXUSIM_PUSH_GATEWAY_MODE = "ws"
            NEXUSIM_PUSH_WS_ADDR = "127.0.0.1:11598"
            NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11597"
            NEXUSIM_DELIVERY_GRPC_TIMEOUT = "2s"
            NEXUSIM_PUSH_SESSION_QUEUE_SIZE = $pushSessionQueueSize
            NEXUSIM_PUSH_WRITE_TIMEOUT = $pushWriteTimeout
            NEXUSIM_PUSH_TEST_WRITE_DELAY = $pushTestWriteDelay
            NEXUSIM_PUSH_ROUTE_BACKEND = "redis"
            NEXUSIM_PUSH_GATEWAY_ID = $pushWSGatewayID
            NEXUSIM_PUSH_ROUTE_TTL = "90s"
        })
        if ($Scenario -eq "cross-instance-resume") {
            $processes += Start-NexusProcess -Name "push-gateway-ws-reconnect" -FilePath $pushGateway -Port 11599 -Env (Add-PushRedisEnv @{
                NEXUSIM_PUSH_GATEWAY_MODE = "ws"
                NEXUSIM_PUSH_WS_ADDR = "127.0.0.1:11599"
                NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11597"
                NEXUSIM_DELIVERY_GRPC_TIMEOUT = "2s"
                NEXUSIM_PUSH_SESSION_QUEUE_SIZE = $pushSessionQueueSize
                NEXUSIM_PUSH_WRITE_TIMEOUT = $pushWriteTimeout
                NEXUSIM_PUSH_TEST_WRITE_DELAY = $pushTestWriteDelay
                NEXUSIM_PUSH_ROUTE_BACKEND = "redis"
                NEXUSIM_PUSH_GATEWAY_ID = $pushReconnectGatewayID
                NEXUSIM_PUSH_ROUTE_TTL = "90s"
            })
        }
        $processes += Start-NexusProcess -Name "push-gateway-consumer" -FilePath $pushGateway -Env (Add-PushRedisEnv @{
            NEXUSIM_PUSH_GATEWAY_MODE = "delivery-consumer"
            NEXUSIM_PUSH_DEBUG_ADDR = "127.0.0.1:11600"
            NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
            NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
            NEXUSIM_PUSH_CONSUMER_GROUP = $pushConsumerGroup
            NEXUSIM_PUSH_ROUTE_BACKEND = "redis"
            NEXUSIM_PUSH_GATEWAY_ID = $pushConsumerGatewayID
            NEXUSIM_PUSH_ROUTE_TTL = "90s"
        })
    } else {
        $processes += Start-NexusProcess -Name "push-gateway" -FilePath $pushGateway -Port 11598 -Env @{
            NEXUSIM_PUSH_GATEWAY_MODE = "all"
            NEXUSIM_PUSH_WS_ADDR = "127.0.0.1:11598"
            NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11597"
            NEXUSIM_DELIVERY_GRPC_TIMEOUT = "2s"
            NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
            NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
            NEXUSIM_PUSH_CONSUMER_GROUP = $pushConsumerGroup
            NEXUSIM_PUSH_SESSION_QUEUE_SIZE = $pushSessionQueueSize
            NEXUSIM_PUSH_WRITE_TIMEOUT = $pushWriteTimeout
            NEXUSIM_PUSH_TEST_WRITE_DELAY = $pushTestWriteDelay
        }
    }

    $processes += Start-NexusProcess -Name "message-grpc" -FilePath $messageService -Port 11595 -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        NEXUSIM_GRPC_ADDR = "127.0.0.1:11595"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_SERVICE_ADDR = "127.0.0.1:11596"
        NEXUSIM_CONVERSATION_RPC_TIMEOUT = "500ms"
        NEXUSIM_MOCK_PERMISSION_VERSION = "2"
    }

    & $runner `
        --conversation-target 127.0.0.1:11596 `
        --message-target 127.0.0.1:11595 `
        --delivery-target 127.0.0.1:11597 `
        --push-url ws://127.0.0.1:11598 `
        --reconnect-push-url $(if ($Scenario -eq "cross-instance-resume") { "ws://127.0.0.1:11599" } else { "ws://127.0.0.1:11598" }) `
        --pg-dsn $PgDsn `
        --result-dir $resultDir `
        --tenant-id "tenant-push-smoke" `
        --conversation-id "conv-push-smoke" `
        --owner-user-id "owner-1" `
        --receiver-user-id "push-user-1" `
        --receiver-device-id "push-device-1" `
        --receiver-device-ids $ReceiverDeviceIds `
        --scenario $Scenario `
        --slow-message-count $SlowMessageCount `
        --redis-fault-command $RedisFaultCommand `
        --redis-restore-command $RedisRestoreCommand `
        --push-metrics-url "http://127.0.0.1:11598/debug/metrics" `
        --reconnect-push-metrics-url $(if ($Scenario -eq "cross-instance-resume") { "http://127.0.0.1:11599/debug/metrics" } else { "" }) `
        --consumer-push-metrics-url $(if ($RouteBackend -eq "redis") { "http://127.0.0.1:11600/debug/metrics" } else { "" }) `
        --route-backend $RouteBackend `
        --redis-key-prefix $pushRouteKeyPrefix `
        --push-ws-gateway-id $pushWSGatewayID `
        --push-reconnect-gateway-id $(if ($Scenario -eq "cross-instance-resume") { $pushReconnectGatewayID } else { "" }) `
        --push-consumer-gateway-id $pushConsumerGatewayID `
        --wait-timeout 20s `
        --request-timeout 3s
    if ($LASTEXITCODE -ne 0) {
        throw "pushgateway smoke runner failed with exit code $LASTEXITCODE"
    }
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
    if ($Scenario -eq "redis-fault" -and $RedisRestoreCommand) {
        Invoke-Expression $RedisRestoreCommand
        Wait-Tcp -HostName "127.0.0.1" -Port 6379 -TimeoutSeconds 20
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "timeline_topic=$timelineTopic"
Write-Host "delivery_consumer_group=$deliveryConsumerGroup"
Write-Host "push_consumer_group=$pushConsumerGroup"
Write-Host "route_backend=$RouteBackend"
if ($RouteBackend -eq "redis") {
    Write-Host "redis_mode=$RedisMode"
    if ($RedisMode -eq "sentinel") {
        Write-Host "redis_sentinel_addrs=$RedisSentinelAddrs"
        Write-Host "redis_sentinel_master_name=$RedisSentinelMasterName"
    } else {
        Write-Host "redis_addr=$RedisAddr"
    }
    Write-Host "redis_key_prefix=$pushRouteKeyPrefix"
    Write-Host "push_ws_gateway_id=$pushWSGatewayID"
    Write-Host "push_consumer_gateway_id=$pushConsumerGatewayID"
}
