param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$TenantId = "",
    [string]$ConversationId = "",
    [string]$ReceiverDeviceIds = "push-device-1",
    [ValidateSet("full", "message-change-notify", "resume-replay", "cross-instance-resume", "slow-client", "redis-fault", "redis-sentinel-failover", "redis-sentinel-master-stop", "identity-revoke")]
    [string]$Scenario = "full",
    [ValidateSet("edit", "revoke", "delete")]
    [string]$MessageChangeAction = "edit",
    [ValidateSet("memory", "redis")]
    [string]$RouteBackend = "memory",
    [ValidateSet("mock", "hmac", "jwt")]
    [string]$PushAuthMode = "mock",
    [switch]$UseIdentityServiceToken,
    [ValidateSet("device", "session")]
    [string]$IdentityRevokeScope = "device",
    [string]$PushAuthHmacSecret = "local-push-smoke-secret",
    [string]$PushAuthHmacPreviousSecrets = "",
    [string]$PushAuthTokenSigningSecret = "",
    [ValidateSet("legacy", "jwt", "jwt-rs256", "rs256")]
    [string]$IdentityGatewayTokenFormat = "legacy",
    [ValidateSet("issue_gateway_token", "login", "register_login")]
    [string]$IdentityTokenMethod = "issue_gateway_token",
    [string]$IdentityLoginPassword = "push-smoke-password",
    [string]$PushAuthTokenTtl = "10m",
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
if (-not $TenantId) {
    $TenantId = "tenant-$safeRunName"
}
if (-not $ConversationId) {
    $ConversationId = "conv-$safeRunName"
}
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$timelineTopic = "conversation.timeline.pushgateway." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryTopic = "im.delivery.events"
$identityTopic = "im.identity.events"
$deliveryConsumerGroup = "nexusim-delivery-push-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushConsumerGroup = "nexusim-push-gateway-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushIdentityConsumerGroup = "nexusim-push-gateway-identity-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
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
$runnerRequestTimeout = "3s"
if ($Scenario -eq "redis-sentinel-failover") {
    $runnerRequestTimeout = "60s"
}
if ($Scenario -eq "redis-sentinel-master-stop") {
    $runnerRequestTimeout = "90s"
}

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

if ($Scenario -eq "redis-sentinel-failover") {
    if ($RouteBackend -ne "redis") {
        throw "redis-sentinel-failover requires -RouteBackend redis"
    }
    if ($RedisMode -ne "sentinel") {
        throw "redis-sentinel-failover requires -RedisMode sentinel"
    }
    if (-not $RedisFaultCommand) {
        $failoverScript = Join-Path $resultDir "redis-sentinel-failover.ps1"
        @'
$ErrorActionPreference = "Stop"
$before = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
if ($before.Count -lt 2) {
    throw "Sentinel did not return a current master before failover."
}
$beforeHost = $before[0].Trim()
$beforePort = $before[1].Trim()
$beforeAddr = "${beforeHost}:${beforePort}"
Write-Output "sentinel_master_before=$beforeAddr"
docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL failover mymaster | Out-Null
$deadline = (Get-Date).AddSeconds(45)
$afterAddr = ""
$afterHost = ""
$afterPort = ""
do {
    Start-Sleep -Seconds 1
    $after = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
    if ($after.Count -ge 2) {
        $afterHost = $after[0].Trim()
        $afterPort = $after[1].Trim()
        $afterAddr = "${afterHost}:${afterPort}"
    }
} while (($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) -and (Get-Date) -lt $deadline)
if ($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) {
    throw "Sentinel failover did not change master before timeout; before=$beforeAddr after=$afterAddr"
}
$ping = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort ping)
if ($ping.Count -lt 1 -or $ping[0].Trim() -ne "PONG") {
    throw "New Sentinel master did not respond to PING: $($ping -join ',')"
}
$role = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort role)
if ($role.Count -lt 1 -or $role[0].Trim() -ne "master") {
    throw "New Sentinel master role is not master: $($role -join ',')"
}
Write-Output "sentinel_master_after=$afterAddr"
'@ | Set-Content -LiteralPath $failoverScript -Encoding UTF8
        $RedisFaultCommand = "& '$failoverScript'"
    }
}
if ($Scenario -eq "redis-sentinel-master-stop") {
    if ($RouteBackend -ne "redis") {
        throw "redis-sentinel-master-stop requires -RouteBackend redis"
    }
    if ($RedisMode -ne "sentinel") {
        throw "redis-sentinel-master-stop requires -RedisMode sentinel"
    }
    if (-not $RedisFaultCommand) {
        $failoverScript = Join-Path $resultDir "redis-sentinel-stop-master.ps1"
        @'
$ErrorActionPreference = "Stop"
$portToContainer = @{
    "6380" = "nexusim-redis-ha-master"
    "6381" = "nexusim-redis-ha-replica-1"
    "6382" = "nexusim-redis-ha-replica-2"
}
$before = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
if ($before.Count -lt 2) {
    throw "Sentinel did not return a current master before stop."
}
$beforeHost = $before[0].Trim()
$beforePort = $before[1].Trim()
$beforeAddr = "${beforeHost}:${beforePort}"
if (-not $portToContainer.ContainsKey($beforePort)) {
    throw "No local Redis container mapping for Sentinel master port $beforePort"
}
$container = $portToContainer[$beforePort]
Set-Content -LiteralPath (Join-Path $PSScriptRoot "redis-sentinel-stopped-container.txt") -Value $container -Encoding ASCII
Write-Output "sentinel_master_before=$beforeAddr"
Write-Output "stopped_container=$container"
docker stop $container | Out-Null
$deadline = (Get-Date).AddSeconds(75)
$afterAddr = ""
$afterHost = ""
$afterPort = ""
do {
    Start-Sleep -Seconds 1
    $after = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster)
    if ($after.Count -ge 2) {
        $afterHost = $after[0].Trim()
        $afterPort = $after[1].Trim()
        $afterAddr = "${afterHost}:${afterPort}"
    }
} while (($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) -and (Get-Date) -lt $deadline)
if ($afterAddr -eq "" -or $afterAddr -eq $beforeAddr) {
    throw "Sentinel did not promote a different master after stopping $container; before=$beforeAddr after=$afterAddr"
}
$ping = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort ping)
if ($ping.Count -lt 1 -or $ping[0].Trim() -ne "PONG") {
    throw "Promoted Sentinel master did not respond to PING: $($ping -join ',')"
}
$role = @(docker exec nexusim-redis-sentinel-1 redis-cli -h $afterHost -p $afterPort role)
if ($role.Count -lt 1 -or $role[0].Trim() -ne "master") {
    throw "Promoted Sentinel master role is not master: $($role -join ',')"
}
Write-Output "sentinel_master_after=$afterAddr"
'@ | Set-Content -LiteralPath $failoverScript -Encoding UTF8
        $RedisFaultCommand = "& '$failoverScript'"
    }
    if (-not $RedisRestoreCommand) {
        $restoreScript = Join-Path $resultDir "redis-sentinel-restore-stopped.ps1"
        @'
$ErrorActionPreference = "Stop"
$stoppedFile = Join-Path $PSScriptRoot "redis-sentinel-stopped-container.txt"
if (-not (Test-Path -LiteralPath $stoppedFile)) {
    Write-Output "sentinel_restore=skipped_no_stopped_container_file"
    return
}
$container = (Get-Content -LiteralPath $stoppedFile -Raw).Trim()
if ($container -eq "") {
    Write-Output "sentinel_restore=skipped_empty_container"
    return
}
docker start $container | Out-Null
$deadline = (Get-Date).AddSeconds(60)
do {
    Start-Sleep -Seconds 1
    $state = docker inspect -f "{{.State.Health.Status}}" $container 2>$null
} while ($state -ne "healthy" -and (Get-Date) -lt $deadline)
Write-Output "sentinel_restored_container=$container"
Write-Output "sentinel_restored_health=$state"
'@ | Set-Content -LiteralPath $restoreScript -Encoding UTF8
        $RedisRestoreCommand = "& '$restoreScript'"
    }
}

. .\tools\go-env.ps1

if ($UseIdentityServiceToken -and $PushAuthMode -notin @("hmac", "jwt")) {
    throw "-UseIdentityServiceToken requires -PushAuthMode hmac or jwt"
}
if ($PushAuthMode -eq "jwt") {
    if (-not $UseIdentityServiceToken) {
        throw "-PushAuthMode jwt requires -UseIdentityServiceToken"
    }
    if ($IdentityGatewayTokenFormat -notin @("jwt-rs256", "rs256")) {
        throw "-PushAuthMode jwt requires -IdentityGatewayTokenFormat jwt-rs256"
    }
}
if ($Scenario -eq "identity-revoke" -and -not $UseIdentityServiceToken) {
    throw "identity-revoke scenario requires -UseIdentityServiceToken"
}
if ($Scenario -eq "identity-revoke" -and $PushAuthMode -ne "hmac") {
    throw "identity-revoke scenario currently requires -PushAuthMode hmac"
}

if (-not $SkipBuild) {
    go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\delivery-service.exe ./services/delivery-service/cmd/delivery-service
    go build -o bin\identity-service.exe ./services/identity-service/cmd/identity-service
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

function Add-PushAuthEnv {
    param([hashtable]$Env)
    $Env["NEXUSIM_PUSH_AUTH_MODE"] = $PushAuthMode
    $Env["NEXUSIM_PUSH_AUTH_HMAC_SECRET"] = $PushAuthHmacSecret
    $Env["NEXUSIM_PUSH_AUTH_HMAC_PREVIOUS_SECRETS"] = $PushAuthHmacPreviousSecrets
    if ($rs256SmokeKeyMaterial) {
        $Env["NEXUSIM_PUSH_AUTH_JWKS_FILE"] = $rs256SmokeKeyMaterial.JwksFile
        $Env["NEXUSIM_PUSH_AUTH_TRUSTED_ISSUERS"] = "nexusim-identity"
    }
    return $Env
}

function New-RS256SmokeKeyMaterial {
    param(
        [string]$Directory,
        [string]$KeyID
    )
    $generator = Join-Path $Directory "generate-rs256-smoke-key.go"
    @'
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"math/big"
)

func base64URL(bytes []byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func main() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	jwks, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"kid": os.Args[1],
			"alg": "RS256",
			"n":   base64URL(key.PublicKey.N.Bytes()),
			"e":   base64URL(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}},
	})
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
		"private_key_pem": string(privatePEM),
		"jwks_json":       string(jwks),
	}); err != nil {
		panic(err)
	}
}
'@ | Set-Content -LiteralPath $generator -Encoding UTF8
    $raw = & go run $generator $KeyID
    if ($LASTEXITCODE -ne 0) {
        throw "failed to generate RS256 smoke key"
    }
    $material = $raw | ConvertFrom-Json
    $privateKeyFile = Join-Path $Directory "identity-gateway-rs256-private.pem"
    $jwksFile = Join-Path $Directory "push-auth-rs256-jwks.json"
    Set-Content -LiteralPath $privateKeyFile -Value $material.private_key_pem -Encoding ASCII
    Set-Content -LiteralPath $jwksFile -Value $material.jwks_json -Encoding ASCII
    return @{
        KeyID = $KeyID
        PrivateKeyFile = $privateKeyFile
        JwksFile = $jwksFile
    }
}

$processes = @()
try {
    $rs256SmokeKeyMaterial = $null
    if ($UseIdentityServiceToken -and $IdentityGatewayTokenFormat -in @("jwt-rs256", "rs256")) {
        $rs256SmokeKeyMaterial = New-RS256SmokeKeyMaterial -Directory $resultDir -KeyID "push-smoke-gateway-rs256"
    }

    Ensure-KafkaTopic -Topic $timelineTopic
    Ensure-KafkaTopic -Topic $deliveryTopic
    if ($Scenario -eq "identity-revoke") {
        Ensure-KafkaTopic -Topic $identityTopic
    }
    Reset-ConsumerGroupToLatest -Group $pushConsumerGroup -Topic $deliveryTopic
    if ($Scenario -eq "identity-revoke") {
        Reset-ConsumerGroupToLatest -Group $pushIdentityConsumerGroup -Topic $identityTopic
    }
    if ($UseIdentityServiceToken) {
        $identityMigrations = Get-ChildItem -LiteralPath (Join-Path $repo "migrations\postgres\identity") -Filter "*.sql" | Sort-Object Name
        foreach ($identityMigration in $identityMigrations) {
            $target = "/tmp/" + $identityMigration.Name
            docker cp $identityMigration.FullName "nexusim-postgres:$target" | Out-Null
            docker exec nexusim-postgres psql -U nexusim -d nexusim -f $target | Out-Null
        }
    }

    $conversationService = Join-Path $repo "bin\conversation-service.exe"
    $messageService = Join-Path $repo "bin\message-service.exe"
    $deliveryService = Join-Path $repo "bin\delivery-service.exe"
    $identityService = Join-Path $repo "bin\identity-service.exe"
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

    if ($UseIdentityServiceToken) {
        $identityEnv = @{
            NEXUSIM_IDENTITY_SERVICE_MODE = "grpc"
            NEXUSIM_IDENTITY_GRPC_ADDR = "127.0.0.1:11610"
            NEXUSIM_PG_DSN = $PgDsn
            NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET = $PushAuthHmacSecret
            NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT = $IdentityGatewayTokenFormat
            NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEY_ID = $(if ($rs256SmokeKeyMaterial) { $rs256SmokeKeyMaterial.KeyID } else { "push-smoke-gateway-hs256" })
            NEXUSIM_IDENTITY_GATEWAY_TOKEN_ISSUER = "nexusim-identity"
        }
        if ($rs256SmokeKeyMaterial) {
            $identityEnv["NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE"] = $rs256SmokeKeyMaterial.PrivateKeyFile
        }
        $processes += Start-NexusProcess -Name "identity-grpc" -FilePath $identityService -Port 11610 -Env $identityEnv
        if ($Scenario -eq "identity-revoke") {
            $processes += Start-NexusProcess -Name "identity-outbox-relay" -FilePath $identityService -Env @{
                NEXUSIM_IDENTITY_SERVICE_MODE = "outbox-relay"
                NEXUSIM_PG_DSN = $PgDsn
                NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
                NEXUSIM_IDENTITY_EVENTS_TOPIC = $identityTopic
                NEXUSIM_IDENTITY_OUTBOX_POLL_INTERVAL = "200ms"
            }
        }
    }

    if ($RouteBackend -eq "redis") {
        $processes += Start-NexusProcess -Name "push-gateway-ws" -FilePath $pushGateway -Port 11598 -Env (Add-PushRedisEnv (Add-PushAuthEnv @{
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
        }))
        if ($Scenario -eq "cross-instance-resume" -or $Scenario -eq "redis-sentinel-failover" -or $Scenario -eq "redis-sentinel-master-stop") {
            $processes += Start-NexusProcess -Name "push-gateway-ws-reconnect" -FilePath $pushGateway -Port 11599 -Env (Add-PushRedisEnv (Add-PushAuthEnv @{
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
            }))
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
        if ($Scenario -eq "identity-revoke") {
            $processes += Start-NexusProcess -Name "push-gateway-identity-consumer" -FilePath $pushGateway -Env (Add-PushRedisEnv @{
                NEXUSIM_PUSH_GATEWAY_MODE = "identity-consumer"
                NEXUSIM_PUSH_DEBUG_ADDR = "127.0.0.1:11601"
                NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
                NEXUSIM_IDENTITY_EVENTS_TOPIC = $identityTopic
                NEXUSIM_PUSH_IDENTITY_CONSUMER_GROUP = $pushIdentityConsumerGroup
                NEXUSIM_PUSH_ROUTE_BACKEND = "redis"
                NEXUSIM_PUSH_GATEWAY_ID = "push-identity-$safeRunName"
                NEXUSIM_PUSH_ROUTE_TTL = "90s"
            })
        }
    } else {
        $processes += Start-NexusProcess -Name "push-gateway" -FilePath $pushGateway -Port 11598 -Env (Add-PushAuthEnv @{
            NEXUSIM_PUSH_GATEWAY_MODE = "all"
            NEXUSIM_PUSH_WS_ADDR = "127.0.0.1:11598"
            NEXUSIM_DELIVERY_GRPC_ADDR = "127.0.0.1:11597"
            NEXUSIM_DELIVERY_GRPC_TIMEOUT = "2s"
            NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
            NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
            NEXUSIM_IDENTITY_EVENTS_TOPIC = $identityTopic
            NEXUSIM_PUSH_CONSUMER_GROUP = $pushConsumerGroup
            NEXUSIM_PUSH_IDENTITY_CONSUMER_GROUP = $pushIdentityConsumerGroup
            NEXUSIM_PUSH_SESSION_QUEUE_SIZE = $pushSessionQueueSize
            NEXUSIM_PUSH_WRITE_TIMEOUT = $pushWriteTimeout
            NEXUSIM_PUSH_TEST_WRITE_DELAY = $pushTestWriteDelay
        })
    }

    $processes += Start-NexusProcess -Name "message-grpc" -FilePath $messageService -Port 11595 -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        NEXUSIM_GRPC_ADDR = "127.0.0.1:11595"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_SERVICE_ADDR = "127.0.0.1:11596"
        NEXUSIM_CONVERSATION_RPC_TIMEOUT = "500ms"
        NEXUSIM_MOCK_PERMISSION_VERSION = "2"
    }

    $reconnectPushURL = if ($Scenario -eq "cross-instance-resume" -or $Scenario -eq "redis-sentinel-failover" -or $Scenario -eq "redis-sentinel-master-stop") { "ws://127.0.0.1:11599" } else { "ws://127.0.0.1:11598" }
    $reconnectMetricsURL = if ($Scenario -eq "cross-instance-resume" -or $Scenario -eq "redis-sentinel-failover" -or $Scenario -eq "redis-sentinel-master-stop") { "http://127.0.0.1:11599/debug/metrics" } else { "" }
    $consumerMetricsURL = if ($RouteBackend -eq "redis") { "http://127.0.0.1:11600/debug/metrics" } else { "" }
    $runnerArgs = @(
        "--conversation-target", "127.0.0.1:11596",
        "--message-target", "127.0.0.1:11595",
        "--delivery-target", "127.0.0.1:11597",
        "--push-url", "ws://127.0.0.1:11598",
        "--reconnect-push-url", $reconnectPushURL,
        "--pg-dsn", $PgDsn,
        "--result-dir", $resultDir,
        "--tenant-id", $TenantId,
        "--conversation-id", $ConversationId,
        "--owner-user-id", "owner-1",
        "--receiver-user-id", "push-user-1",
        "--receiver-device-id", "push-device-1",
        "--receiver-device-ids", $ReceiverDeviceIds,
        "--scenario", $Scenario,
        "--message-change-action", $MessageChangeAction,
        "--identity-revoke-scope", $IdentityRevokeScope,
        "--slow-message-count", [string]$SlowMessageCount,
        "--push-metrics-url", "http://127.0.0.1:11598/debug/metrics",
        "--route-backend", $RouteBackend,
        "--push-auth-mode", $PushAuthMode,
        "--push-auth-hmac-secret", $PushAuthHmacSecret,
        "--push-auth-hmac-previous-secrets", $PushAuthHmacPreviousSecrets,
        "--push-auth-token-signing-secret", $PushAuthTokenSigningSecret,
        "--push-auth-token-ttl", $PushAuthTokenTtl,
        "--identity-gateway-token-format", $IdentityGatewayTokenFormat,
        "--identity-token-method", $IdentityTokenMethod,
        "--identity-login-password", $IdentityLoginPassword,
        "--redis-key-prefix", $pushRouteKeyPrefix,
        "--push-ws-gateway-id", $pushWSGatewayID,
        "--push-consumer-gateway-id", $pushConsumerGatewayID,
        "--wait-timeout", "20s",
        "--request-timeout", $runnerRequestTimeout
    )
    if ($UseIdentityServiceToken) {
        $runnerArgs += @("--identity-target", "127.0.0.1:11610")
    }
    if ($RedisFaultCommand) {
        $runnerArgs += @("--redis-fault-command", $RedisFaultCommand)
    }
    if ($RedisRestoreCommand) {
        $runnerArgs += @("--redis-restore-command", $RedisRestoreCommand)
    }
    if ($reconnectMetricsURL) {
        $runnerArgs += @("--reconnect-push-metrics-url", $reconnectMetricsURL)
    }
    if ($consumerMetricsURL) {
        $runnerArgs += @("--consumer-push-metrics-url", $consumerMetricsURL)
    }
    if ($Scenario -eq "cross-instance-resume" -or $Scenario -eq "redis-sentinel-failover" -or $Scenario -eq "redis-sentinel-master-stop") {
        $runnerArgs += @("--push-reconnect-gateway-id", $pushReconnectGatewayID)
    }
    & $runner @runnerArgs
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
    if ($Scenario -eq "redis-sentinel-master-stop" -and $RedisRestoreCommand) {
        Invoke-Expression $RedisRestoreCommand
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "timeline_topic=$timelineTopic"
Write-Host "delivery_consumer_group=$deliveryConsumerGroup"
Write-Host "push_consumer_group=$pushConsumerGroup"
if ($Scenario -eq "identity-revoke") {
    Write-Host "identity_topic=$identityTopic"
    Write-Host "push_identity_consumer_group=$pushIdentityConsumerGroup"
}
Write-Host "route_backend=$RouteBackend"
Write-Host "push_auth_mode=$PushAuthMode"
if ($PushAuthMode -eq "hmac") {
    Write-Host "push_auth_hmac_previous_secrets_configured=$([bool]$PushAuthHmacPreviousSecrets)"
    Write-Host "push_auth_token_signing_secret_explicit=$([bool]$PushAuthTokenSigningSecret)"
    if ($UseIdentityServiceToken) {
        Write-Host "identity_gateway_token_format=$IdentityGatewayTokenFormat"
        Write-Host "identity_token_method=$IdentityTokenMethod"
    }
}
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
