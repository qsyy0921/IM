param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$BindHost = "127.0.0.1",
    [string]$ClientHost = "",
    [string]$RunName = "",
    [switch]$SkipBuild,
    [switch]$RunDesktopWebViewLoginSmoke,
    [switch]$DesktopWebViewSkipWebBuild,
    [switch]$RunAndroidWebViewLoginSmoke,
    [switch]$AndroidWebViewSkipWebBuild
)

$ErrorActionPreference = "Stop"

$nexusIMRepoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $nexusIMRepoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $nexusIMRepoRoot -Name "ResultRoot"

$repo = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repo

. .\tools\go-env.ps1

if ([string]::IsNullOrWhiteSpace($ClientHost)) {
    $ClientHost = $BindHost
}

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "client-web-bff-push-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
$tenantId = "tenant-$safeRunName"
$conversationId = "conv-$safeRunName"
$senderUserId = "client-web-sender-$safeRunName"
$receiverUserId = "client-web-receiver-$safeRunName"
$senderDeviceId = "client-web-sender-device"
$receiverDeviceId = "client-web-receiver-device"
$senderPassword = "ClientWebSenderPassw0rd!"
$receiverPassword = "ClientWebReceiverPassw0rd!"
$gatewayAuthSecret = "client-web-gateway-secret-$safeRunName"

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

function Get-FreeTcpPort {
    param([string]$HostName)
    $bindAddress = [System.Net.IPAddress]::Parse($HostName)
    $listener = [System.Net.Sockets.TcpListener]::new($bindAddress, 0)
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
        [int]$Port = 0,
        [string]$WaitHost = "127.0.0.1",
        [string[]]$ArgumentList = @()
    )
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $FilePath
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    foreach ($arg in $ArgumentList) {
        [void]$startInfo.ArgumentList.Add($arg)
    }
    foreach ($key in @($startInfo.Environment.Keys)) {
        if ($key.StartsWith("NEXUSIM_")) {
            [void]$startInfo.Environment.Remove($key)
        }
    }
    foreach ($key in $Env.Keys) {
        $startInfo.Environment[$key] = [string]$Env[$key]
    }
    $proc = [System.Diagnostics.Process]::new()
    $proc.StartInfo = $startInfo
    if (-not $proc.Start()) {
        throw "failed to start $Name"
    }
    $proc.StandardOutput.BaseStream.CopyToAsync([System.IO.File]::Open($out, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write, [System.IO.FileShare]::ReadWrite)) | Out-Null
    $proc.StandardError.BaseStream.CopyToAsync([System.IO.File]::Open($err, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write, [System.IO.FileShare]::ReadWrite)) | Out-Null
    if ($Port -gt 0) {
        Wait-Tcp -HostName $WaitHost -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
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

function Ensure-KafkaTopic {
    param([string]$Topic)
    docker exec nexusim-kafka kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Topic `
        --partitions 3 `
        --replication-factor 1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "failed to ensure Kafka topic $Topic"
    }
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
    if ($LASTEXITCODE -ne 0) {
        throw "failed to reset Kafka consumer group $Group for topic $Topic"
    }
}

if (-not $SkipBuild) {
    go build -o bin\identity-service.exe ./services/identity-service/cmd/identity-service
    go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\delivery-service.exe ./services/delivery-service/cmd/delivery-service
    go build -o bin\receipt-service.exe ./services/receipt-service/cmd/receipt-service
    go build -o bin\push-gateway.exe ./services/push-gateway/cmd/push-gateway
    go build -o bin\api-gateway.exe ./services/api-gateway/cmd/api-gateway
    go build -o bin\nexusim-client-web-smoke.exe ./loadtest/clientweb
}

$identityPort = Get-FreeTcpPort -HostName $BindHost
$conversationPort = Get-FreeTcpPort -HostName $BindHost
$messagePort = Get-FreeTcpPort -HostName $BindHost
$deliveryPort = Get-FreeTcpPort -HostName $BindHost
$receiptPort = Get-FreeTcpPort -HostName $BindHost
$pushPort = Get-FreeTcpPort -HostName $BindHost
$apiGatewayPort = Get-FreeTcpPort -HostName $BindHost
$bffPort = Get-FreeTcpPort -HostName $BindHost

$identityListen = "${BindHost}:$identityPort"
$conversationListen = "${BindHost}:$conversationPort"
$messageListen = "${BindHost}:$messagePort"
$deliveryListen = "${BindHost}:$deliveryPort"
$receiptListen = "${BindHost}:$receiptPort"
$pushListen = "${BindHost}:$pushPort"
$apiGatewayListen = "${BindHost}:$apiGatewayPort"
$bffListen = "${BindHost}:$bffPort"

$identityTarget = "${ClientHost}:$identityPort"
$conversationTarget = "${ClientHost}:$conversationPort"
$messageTarget = "${ClientHost}:$messagePort"
$deliveryTarget = "${ClientHost}:$deliveryPort"
$receiptTarget = "${ClientHost}:$receiptPort"
$pushTarget = "${ClientHost}:$pushPort"
$apiGatewayTarget = "${ClientHost}:$apiGatewayPort"
$bffTarget = "${ClientHost}:$bffPort"
$bffBaseURL = "http://$bffTarget"
$pushURL = "ws://$pushTarget/ws"

$timelineTopic = "conversation.timeline.client-web." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryTopic = "im.delivery.events"
$identityTopic = "im.identity.events"
$deliveryConsumerGroup = "nexusim-delivery-client-web-" + (Get-Date -Format "yyyyMMddHHmmss")
$receiptConsumerGroup = "nexusim-receipt-client-web-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushConsumerGroup = "nexusim-push-client-web-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushIdentityConsumerGroup = "nexusim-push-identity-client-web-" + (Get-Date -Format "yyyyMMddHHmmss")

$processes = @()
try {
    Apply-PostgresMigration -Path "migrations\postgres\message\000001_message_core.sql" -Name "nexusim_message_core.sql"
    foreach ($migration in Get-ChildItem -Path "migrations\postgres\conversation" -Filter "*.sql" | Sort-Object Name) {
        Apply-PostgresMigration -Path $migration.FullName -Name ("nexusim_conversation_" + $migration.Name)
    }
    foreach ($migration in Get-ChildItem -Path "migrations\postgres\delivery" -Filter "*.sql" | Sort-Object Name) {
        Apply-PostgresMigration -Path $migration.FullName -Name ("nexusim_delivery_" + $migration.Name)
    }
    foreach ($migration in Get-ChildItem -Path "migrations\postgres\receipt" -Filter "*.sql" | Sort-Object Name) {
        Apply-PostgresMigration -Path $migration.FullName -Name ("nexusim_receipt_" + $migration.Name)
    }
    foreach ($migration in Get-ChildItem -Path "migrations\postgres\policy" -Filter "*.sql" | Sort-Object Name) {
        Apply-PostgresMigration -Path $migration.FullName -Name ("nexusim_policy_" + $migration.Name)
    }
    foreach ($migration in Get-ChildItem -Path "migrations\postgres\identity" -Filter "*.sql" | Sort-Object Name) {
        Apply-PostgresMigration -Path $migration.FullName -Name ("nexusim_identity_" + $migration.Name)
    }

    Ensure-KafkaTopic -Topic $timelineTopic
    Ensure-KafkaTopic -Topic $deliveryTopic
    Ensure-KafkaTopic -Topic $identityTopic
    Reset-ConsumerGroupToLatest -Group $deliveryConsumerGroup -Topic $timelineTopic
    Reset-ConsumerGroupToLatest -Group $receiptConsumerGroup -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $pushConsumerGroup -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $pushIdentityConsumerGroup -Topic $identityTopic

    $identityService = Join-Path $repo "bin\identity-service.exe"
    $conversationService = Join-Path $repo "bin\conversation-service.exe"
    $messageService = Join-Path $repo "bin\message-service.exe"
    $deliveryService = Join-Path $repo "bin\delivery-service.exe"
    $receiptService = Join-Path $repo "bin\receipt-service.exe"
    $pushGateway = Join-Path $repo "bin\push-gateway.exe"
    $apiGateway = Join-Path $repo "bin\api-gateway.exe"
    $runner = Join-Path $repo "bin\nexusim-client-web-smoke.exe"

    $processes += Start-NexusProcess -Name "identity-grpc" -FilePath $identityService -Port $identityPort -WaitHost $ClientHost -Env @{
        NEXUSIM_IDENTITY_SERVICE_MODE = "grpc"
        NEXUSIM_IDENTITY_GRPC_ADDR = $identityListen
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET = $gatewayAuthSecret
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE = "noop"
        NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN = "false"
    }
    $processes += Start-NexusProcess -Name "conversation-grpc" -FilePath $conversationService -Port $conversationPort -WaitHost $ClientHost -Env @{
        NEXUSIM_CONVERSATION_SERVICE_MODE = "grpc"
        NEXUSIM_CONVERSATION_GRPC_ADDR = $conversationListen
        NEXUSIM_CONVERSATION_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
    }
    $processes += Start-NexusProcess -Name "delivery-timeline-consumer" -FilePath $deliveryService -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "timeline-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_TIMELINE_TOPIC = $timelineTopic
        NEXUSIM_DELIVERY_CONSUMER_GROUP = $deliveryConsumerGroup
    }
    $processes += Start-NexusProcess -Name "delivery-grpc" -FilePath $deliveryService -Port $deliveryPort -WaitHost $ClientHost -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "grpc"
        NEXUSIM_DELIVERY_GRPC_ADDR = $deliveryListen
        NEXUSIM_DELIVERY_AUTH_MODE = "metadata"
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
    $processes += Start-NexusProcess -Name "receipt-grpc" -FilePath $receiptService -Port $receiptPort -WaitHost $ClientHost -Env @{
        NEXUSIM_RECEIPT_SERVICE_MODE = "grpc"
        NEXUSIM_RECEIPT_GRPC_ADDR = $receiptListen
        NEXUSIM_RECEIPT_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
    }
    $processes += Start-NexusProcess -Name "message-relay" -FilePath $messageService -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_KAFKA_TOPIC = $timelineTopic
        NEXUSIM_OUTBOX_POLL_INTERVAL = "200ms"
    }
    $processes += Start-NexusProcess -Name "message-grpc" -FilePath $messageService -Port $messagePort -WaitHost $ClientHost -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        NEXUSIM_GRPC_ADDR = $messageListen
        NEXUSIM_MESSAGE_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_SERVICE_ADDR = $conversationTarget
    }
    $processes += Start-NexusProcess -Name "push-gateway" -FilePath $pushGateway -Port $pushPort -WaitHost $ClientHost -Env @{
        NEXUSIM_PUSH_GATEWAY_MODE = "all"
        NEXUSIM_PUSH_WS_ADDR = $pushListen
        NEXUSIM_PUSH_AUTH_MODE = "hmac"
        NEXUSIM_PUSH_AUTH_HMAC_SECRET = $gatewayAuthSecret
        NEXUSIM_DELIVERY_GRPC_ADDR = $deliveryTarget
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_IDENTITY_EVENTS_TOPIC = $identityTopic
        NEXUSIM_PUSH_CONSUMER_GROUP = $pushConsumerGroup
        NEXUSIM_PUSH_IDENTITY_CONSUMER_GROUP = $pushIdentityConsumerGroup
    }
    $processes += Start-NexusProcess -Name "api-gateway" -FilePath $apiGateway -Port $apiGatewayPort -WaitHost $ClientHost -Env @{
        NEXUSIM_API_GATEWAY_MODE = "grpc"
        NEXUSIM_API_GATEWAY_GRPC_ADDR = $apiGatewayListen
        NEXUSIM_API_GATEWAY_AUTH_MODE = "hmac"
        NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET = $gatewayAuthSecret
        NEXUSIM_API_GATEWAY_AUTH_AUDIENCE = "api-gateway"
        NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS = "false"
        NEXUSIM_API_GATEWAY_IDENTITY_ADDR = $identityTarget
        NEXUSIM_API_GATEWAY_CONVERSATION_ADDR = $conversationTarget
        NEXUSIM_API_GATEWAY_MESSAGE_ADDR = $messageTarget
        NEXUSIM_API_GATEWAY_DELIVERY_ADDR = $deliveryTarget
        NEXUSIM_API_GATEWAY_RECEIPT_ADDR = $receiptTarget
        NEXUSIM_API_GATEWAY_BFF_ADDR = $bffListen
        NEXUSIM_API_GATEWAY_BFF_ALLOWED_ORIGINS = "http://127.0.0.1:5173,http://localhost:5173,http://${ClientHost}:5173,http://tauri.localhost,https://tauri.localhost,tauri://localhost"
    }
    Wait-Tcp -HostName $ClientHost -Port $bffPort

    & $runner `
        --pg-dsn $PgDsn `
        --identity-target $identityTarget `
        --gateway-target $apiGatewayTarget `
        --bff-base-url $bffBaseURL `
        --push-url $pushURL `
        --result-dir $resultDir `
        --tenant-id $tenantId `
        --conversation-id $conversationId `
        --sender-user-id $senderUserId `
        --sender-password $senderPassword `
        --sender-device-id $senderDeviceId `
        --receiver-user-id $receiverUserId `
        --receiver-password $receiverPassword `
        --receiver-device-id $receiverDeviceId `
        --gateway-auth-hmac-secret $gatewayAuthSecret `
        --cleanup
    if ($LASTEXITCODE -ne 0) {
        throw "client web smoke runner failed with exit code $LASTEXITCODE"
    }

    if ($RunDesktopWebViewLoginSmoke) {
        $desktopFixturePath = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-desktop-webview-login-" + [System.Guid]::NewGuid().ToString("N") + ".json")
        $desktopSummaryPath = Join-Path $resultDir "desktop-webview-login-summary.json"
        try {
            $desktopFixture = [ordered]@{
                apiBaseURL = $bffBaseURL
                pushWebSocketURL = $pushURL
                tenantID = $tenantId
                userID = $receiverUserId
                authProof = $receiverPassword
                deviceID = "desktop-webview-login-device"
                conversationID = $conversationId
                senderUserID = $senderUserId
                senderAuthProof = $senderPassword
                senderDeviceID = "desktop-webview-login-sender"
                messageText = "NexusIM desktop WebView login smoke $safeRunName"
            }
            [System.IO.File]::WriteAllText(
                $desktopFixturePath,
                ($desktopFixture | ConvertTo-Json -Depth 4),
                [System.Text.UTF8Encoding]::new($false)
            )

            $desktopSmokeArgs = @(
                "--prefix", "clients",
                "run", "smoke:desktop-webview-login",
                "--",
                "--fixture", $desktopFixturePath,
                "--run-id", "desktop-webview-login-$safeRunName",
                "--output", $desktopSummaryPath
            )
            if ($DesktopWebViewSkipWebBuild) {
                $desktopSmokeArgs += "--skip-web-build"
            }
            & npm @desktopSmokeArgs
            if ($LASTEXITCODE -ne 0) {
                throw "desktop WebView login smoke failed with exit code $LASTEXITCODE"
            }
            if (-not (Test-Path $desktopSummaryPath)) {
                throw "desktop WebView login smoke did not write summary"
            }
        } finally {
            Remove-Item -LiteralPath $desktopFixturePath -Force -ErrorAction SilentlyContinue
        }
    }

    if ($RunAndroidWebViewLoginSmoke) {
        $androidFixturePath = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-android-webview-login-" + [System.Guid]::NewGuid().ToString("N") + ".json")
        $androidSummaryPath = Join-Path $resultDir "android-webview-login-summary.json"
        try {
            $androidFixture = [ordered]@{
                apiBaseURL = $bffBaseURL
                pushWebSocketURL = $pushURL
                tenantID = $tenantId
                userID = $receiverUserId
                authProof = $receiverPassword
                deviceID = "android-webview-login-device"
                conversationID = $conversationId
                senderUserID = $senderUserId
                senderAuthProof = $senderPassword
                senderDeviceID = "android-webview-login-sender"
                messageText = "NexusIM Android WebView login smoke $safeRunName"
            }
            [System.IO.File]::WriteAllText(
                $androidFixturePath,
                ($androidFixture | ConvertTo-Json -Depth 4),
                [System.Text.UTF8Encoding]::new($false)
            )

            $androidSmokeArgs = @(
                "--prefix", "clients",
                "run", "smoke:android-webview-login",
                "--",
                "--fixture", $androidFixturePath,
                "--run-id", "android-webview-login-$safeRunName",
                "--output", $androidSummaryPath
            )
            if ($AndroidWebViewSkipWebBuild) {
                $androidSmokeArgs += "--skip-web-build"
            }
            & npm @androidSmokeArgs
            if ($LASTEXITCODE -ne 0) {
                throw "Android WebView login smoke failed with exit code $LASTEXITCODE"
            }
            if (-not (Test-Path $androidSummaryPath)) {
                throw "Android WebView login smoke did not write summary"
            }
        } finally {
            Remove-Item -LiteralPath $androidFixturePath -Force -ErrorAction SilentlyContinue
        }
    }
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "bind_host=$BindHost"
Write-Host "client_host=$ClientHost"
Write-Host "bff_base_url=$bffBaseURL"
Write-Host "push_url=$pushURL"
Write-Host "timeline_topic=$timelineTopic"
Write-Host "delivery_topic=$deliveryTopic"
Write-Host "delivery_consumer_group=$deliveryConsumerGroup"
Write-Host "receipt_consumer_group=$receiptConsumerGroup"
Write-Host "push_consumer_group=$pushConsumerGroup"
