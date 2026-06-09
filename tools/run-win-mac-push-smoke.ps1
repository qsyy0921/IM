param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$MacHost = "172.31.50.2",
    [string]$MacUser = "qsyy0921",
    [string]$MacPath = "/Users/qsyy0921/Desktop/IM/_local/distributed-smoke",
    [ValidateSet("docker", "native")]
    [string]$MacRunMode = "docker",
    [string]$MacImageTag = "nexusim/push-gateway:local",
    [ValidateSet("docker", "native")]
    [string]$WindowsDeliveryRunMode = "docker",
    [string]$WindowsWiredHost = "172.31.50.1",
    [string]$RedisAddrForWindows = "127.0.0.1:6379",
    [string]$RedisAddrForMac = "172.31.50.1:6379",
    [string]$ReceiverDeviceIds = "push-device-1",
    [ValidateSet("full", "cross-instance-resume")]
    [string]$Scenario = "full",
    [switch]$SkipBuild,
    [switch]$SkipMacSync
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "push-gateway-win-mac-redis-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$timelineTopic = "conversation.timeline.winmac." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryTopic = "im.delivery.events"
$deliveryConsumerGroup = "nexusim-delivery-winmac-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushConsumerGroup = "nexusim-push-winmac-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushRouteKeyPrefix = "nexusim:push:$safeRunName"
$pushWSGatewayID = "push-mac-ws-$safeRunName"
$pushReconnectGatewayID = "push-win-reconnect-$safeRunName"
$pushConsumerGatewayID = "push-win-consumer-$safeRunName"
$remoteLogDir = "$MacPath/logs/$safeRunName"

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipMacSync) {
    & .\tools\sync-mac-distributed-smoke.ps1 -MacHost $MacHost -MacUser $MacUser -MacPath $MacPath -SkipBuild:($SkipBuild -or $MacRunMode -eq "docker")
    if ($MacRunMode -eq "docker") {
        & .\tools\sync-mac-push-docker-image.ps1 -MacHost $MacHost -MacUser $MacUser -MacPath $MacPath -ImageTag $MacImageTag -SkipBuild:$SkipBuild
    }
}

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

function Test-MacTcp {
    param(
        [string]$HostName,
        [int]$Port
    )
    ssh -o BatchMode=yes "${MacUser}@${MacHost}" "nc -z -w 2 '$HostName' '$Port'"
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [string]$WaitHost = "127.0.0.1",
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
        Wait-Tcp -HostName $WaitHost -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
}

function Start-NexusDockerContainer {
    param(
        [string]$Name,
        [string]$Image,
        [hashtable]$Env,
        [string[]]$Ports,
        [string]$WaitHost = "127.0.0.1",
        [int]$WaitPort = 0
    )
    docker image inspect $Image | Out-Null
    $existingContainer = docker ps -aq --filter "name=^$Name$"
    if ($existingContainer) {
        docker rm -f $Name | Out-Null
    }
    $args = @("run", "-d", "--name", $Name)
    foreach ($port in $Ports) {
        $args += @("-p", $port)
    }
    foreach ($key in $Env.Keys) {
        $args += @("-e", "$key=$($Env[$key])")
    }
    $args += $Image
    docker @args | Out-Null
    if ($WaitPort -gt 0) {
        Wait-Tcp -HostName $WaitHost -Port $WaitPort
    } else {
        Start-Sleep -Milliseconds 800
    }
    return "docker:$Name"
}

function Start-MacPushGatewayWS {
    $containerName = "nexusim-push-ws-$safeRunName"
    if ($MacRunMode -eq "docker") {
        $remoteScript = @"
set -e
docker rm -f '$containerName' >/dev/null 2>&1 || true
docker run -d --name '$containerName' \
  -p 11598:11598 \
  -e NEXUSIM_PUSH_GATEWAY_MODE='ws' \
  -e NEXUSIM_PUSH_WS_ADDR='0.0.0.0:11598' \
  -e NEXUSIM_DELIVERY_GRPC_ADDR='${WindowsWiredHost}:11597' \
  -e NEXUSIM_DELIVERY_GRPC_TIMEOUT='2s' \
  -e NEXUSIM_PUSH_SESSION_QUEUE_SIZE='32' \
  -e NEXUSIM_PUSH_WRITE_TIMEOUT='2s' \
  -e NEXUSIM_PUSH_TEST_WRITE_DELAY='0s' \
  -e NEXUSIM_PUSH_ROUTE_BACKEND='redis' \
  -e NEXUSIM_PUSH_GATEWAY_ID='$pushWSGatewayID' \
  -e NEXUSIM_PUSH_REDIS_ADDR='$RedisAddrForMac' \
  -e NEXUSIM_PUSH_REDIS_KEY_PREFIX='$pushRouteKeyPrefix' \
  -e NEXUSIM_PUSH_ROUTE_TTL='90s' \
  '$MacImageTag'
"@
        $containerID = (ssh -o BatchMode=yes "${MacUser}@${MacHost}" $remoteScript | Select-Object -Last 1).Trim()
        if (-not $containerID) {
            throw "Mac push-gateway container did not return an id"
        }
        Wait-Tcp -HostName $MacHost -Port 11598 -TimeoutSeconds 30
        return "docker:$containerName"
    }

    $remoteScript = @"
set -e
cd '$MacPath'
mkdir -p '$remoteLogDir'
export NEXUSIM_PUSH_GATEWAY_MODE='ws'
export NEXUSIM_PUSH_WS_ADDR='0.0.0.0:11598'
export NEXUSIM_DELIVERY_GRPC_ADDR='${WindowsWiredHost}:11597'
export NEXUSIM_DELIVERY_GRPC_TIMEOUT='2s'
export NEXUSIM_PUSH_SESSION_QUEUE_SIZE='32'
export NEXUSIM_PUSH_WRITE_TIMEOUT='2s'
export NEXUSIM_PUSH_TEST_WRITE_DELAY='0s'
export NEXUSIM_PUSH_ROUTE_BACKEND='redis'
export NEXUSIM_PUSH_GATEWAY_ID='$pushWSGatewayID'
export NEXUSIM_PUSH_REDIS_ADDR='$RedisAddrForMac'
export NEXUSIM_PUSH_REDIS_KEY_PREFIX='$pushRouteKeyPrefix'
export NEXUSIM_PUSH_ROUTE_TTL='90s'
nohup ./bin/darwin-arm64/push-gateway > '$remoteLogDir/push-gateway-ws.out.log' 2> '$remoteLogDir/push-gateway-ws.err.log' < /dev/null &
echo `$!
"@
    $output = ssh -o BatchMode=yes "${MacUser}@${MacHost}" $remoteScript
    $remotePid = ($output | Select-Object -Last 1).Trim()
    if (-not $remotePid) {
        throw "Mac push-gateway did not return a pid"
    }
    Wait-Tcp -HostName $MacHost -Port 11598 -TimeoutSeconds 30
    return $remotePid
}

$processes = @()
$containers = @()
$macHandle = ""
try {
    Ensure-KafkaTopic -Topic $timelineTopic
    Ensure-KafkaTopic -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $pushConsumerGroup -Topic $deliveryTopic

    Test-MacTcp -HostName $WindowsWiredHost -Port 6379

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

    if ($WindowsDeliveryRunMode -eq "docker") {
        $containerPgDsn = $PgDsn -replace "localhost:5432", "host.docker.internal:5432" -replace "127.0.0.1:5432", "host.docker.internal:5432"
        $containers += Start-NexusDockerContainer -Name "nexusim-delivery-grpc-$safeRunName" -Image "nexusim/delivery-service:local" -Ports @("11597:11597") -WaitHost $WindowsWiredHost -WaitPort 11597 -Env @{
            NEXUSIM_DELIVERY_SERVICE_MODE = "grpc"
            NEXUSIM_DELIVERY_GRPC_ADDR = "0.0.0.0:11597"
            NEXUSIM_PG_DSN = $containerPgDsn
        }
    } else {
        $processes += Start-NexusProcess -Name "delivery-grpc" -FilePath $deliveryService -WaitHost $WindowsWiredHost -Port 11597 -Env @{
            NEXUSIM_DELIVERY_SERVICE_MODE = "grpc"
            NEXUSIM_DELIVERY_GRPC_ADDR = "0.0.0.0:11597"
            NEXUSIM_PG_DSN = $PgDsn
        }
    }
    Test-MacTcp -HostName $WindowsWiredHost -Port 11597
    if ($WindowsDeliveryRunMode -eq "docker") {
        Start-Sleep -Seconds 2
    }

    $processes += Start-NexusProcess -Name "delivery-outbox-relay" -FilePath $deliveryService -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_DELIVERY_OUTBOX_POLL_INTERVAL = "200ms"
    }

    $macHandle = Start-MacPushGatewayWS

    if ($Scenario -eq "cross-instance-resume") {
        $processes += Start-NexusProcess -Name "push-gateway-ws-reconnect" -FilePath $pushGateway -Port 11599 -Env @{
            NEXUSIM_PUSH_GATEWAY_MODE = "ws"
            NEXUSIM_PUSH_WS_ADDR = "127.0.0.1:11599"
            NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11597"
            NEXUSIM_DELIVERY_GRPC_TIMEOUT = "2s"
            NEXUSIM_PUSH_SESSION_QUEUE_SIZE = "32"
            NEXUSIM_PUSH_WRITE_TIMEOUT = "2s"
            NEXUSIM_PUSH_TEST_WRITE_DELAY = "0s"
            NEXUSIM_PUSH_ROUTE_BACKEND = "redis"
            NEXUSIM_PUSH_GATEWAY_ID = $pushReconnectGatewayID
            NEXUSIM_PUSH_REDIS_ADDR = $RedisAddrForWindows
            NEXUSIM_PUSH_REDIS_KEY_PREFIX = $pushRouteKeyPrefix
            NEXUSIM_PUSH_ROUTE_TTL = "90s"
        }
    }

    $processes += Start-NexusProcess -Name "push-gateway-consumer" -FilePath $pushGateway -Env @{
        NEXUSIM_PUSH_GATEWAY_MODE = "delivery-consumer"
        NEXUSIM_PUSH_DEBUG_ADDR = "127.0.0.1:11600"
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_PUSH_CONSUMER_GROUP = $pushConsumerGroup
        NEXUSIM_PUSH_ROUTE_BACKEND = "redis"
        NEXUSIM_PUSH_GATEWAY_ID = $pushConsumerGatewayID
        NEXUSIM_PUSH_REDIS_ADDR = $RedisAddrForWindows
        NEXUSIM_PUSH_REDIS_KEY_PREFIX = $pushRouteKeyPrefix
        NEXUSIM_PUSH_ROUTE_TTL = "90s"
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
        --push-url "ws://${MacHost}:11598" `
        --pg-dsn $PgDsn `
        --result-dir $resultDir `
        --tenant-id "tenant-winmac-push-smoke" `
        --conversation-id "conv-winmac-push-smoke" `
        --owner-user-id "owner-1" `
        --receiver-user-id "push-user-1" `
        --receiver-device-id "push-device-1" `
        --receiver-device-ids $ReceiverDeviceIds `
        --scenario $Scenario `
        --reconnect-push-url $(if ($Scenario -eq "cross-instance-resume") { "ws://127.0.0.1:11599" } else { "ws://${MacHost}:11598" }) `
        --push-metrics-url "http://${MacHost}:11598/debug/metrics" `
        --reconnect-push-metrics-url $(if ($Scenario -eq "cross-instance-resume") { "http://127.0.0.1:11599/debug/metrics" } else { "" }) `
        --consumer-push-metrics-url "http://127.0.0.1:11600/debug/metrics" `
        --route-backend redis `
        --redis-key-prefix $pushRouteKeyPrefix `
        --push-ws-gateway-id $pushWSGatewayID `
        --push-reconnect-gateway-id $(if ($Scenario -eq "cross-instance-resume") { $pushReconnectGatewayID } else { "" }) `
        --push-consumer-gateway-id $pushConsumerGatewayID `
        --wait-timeout 25s `
        --request-timeout 3s
    if ($LASTEXITCODE -ne 0) {
        throw "pushgateway win-mac smoke runner failed with exit code $LASTEXITCODE"
    }
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
    foreach ($container in $containers) {
        if ($container.StartsWith("docker:")) {
            $containerName = $container.Substring("docker:".Length)
            docker rm -f $containerName | Out-Null 2>$null
        }
    }
    if ($macHandle) {
        if ($macHandle.StartsWith("docker:")) {
            $containerName = $macHandle.Substring("docker:".Length)
            ssh -o BatchMode=yes "${MacUser}@${MacHost}" "docker rm -f '$containerName' >/dev/null 2>&1 || true" | Out-Null
        } else {
            ssh -o BatchMode=yes "${MacUser}@${MacHost}" "kill '$macHandle' >/dev/null 2>&1 || true" | Out-Null
        }
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "timeline_topic=$timelineTopic"
Write-Host "delivery_consumer_group=$deliveryConsumerGroup"
Write-Host "push_consumer_group=$pushConsumerGroup"
Write-Host "route_backend=redis"
Write-Host "scenario=$Scenario"
Write-Host "mac_host=$MacHost"
Write-Host "mac_path=$MacPath"
Write-Host "mac_run_mode=$MacRunMode"
Write-Host "windows_delivery_run_mode=$WindowsDeliveryRunMode"
if ($MacRunMode -eq "docker") {
    Write-Host "mac_image_tag=$MacImageTag"
}
Write-Host "redis_addr_for_mac=$RedisAddrForMac"
Write-Host "redis_key_prefix=$pushRouteKeyPrefix"
Write-Host "push_ws_gateway_id=$pushWSGatewayID"
if ($Scenario -eq "cross-instance-resume") {
    Write-Host "push_reconnect_gateway_id=$pushReconnectGatewayID"
}
Write-Host "push_consumer_gateway_id=$pushConsumerGatewayID"
