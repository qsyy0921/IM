param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$TenantId = "",
    [string]$ConversationId = "",
    [string]$SenderUserId = "demo-sender",
    [string]$ReceiverUserId = "demo-receiver",
    [string]$ReceiverDeviceId = "demo-device-1",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repo = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repo

. .\tools\go-env.ps1

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "e2e-demo-api-gateway-facade-secure-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
if ([string]::IsNullOrWhiteSpace($TenantId)) {
    $TenantId = "tenant-$safeRunName"
}
if ([string]::IsNullOrWhiteSpace($ConversationId)) {
    $ConversationId = "conv-$safeRunName"
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$certDir = Join-Path $resultDir "certs"
$timelineTopic = "conversation.timeline.demo.secure." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryTopic = "im.delivery.events"
$receiptTopic = "im.receipt.events"
$identityTopic = "im.identity.events"
$policyTopic = "im.policy.events.demo.secure." + (Get-Date -Format "yyyyMMdd-HHmmss")
$deliveryConsumerGroup = "nexusim-delivery-demo-secure-" + (Get-Date -Format "yyyyMMddHHmmss")
$receiptConsumerGroup = "nexusim-receipt-demo-secure-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushConsumerGroup = "nexusim-push-demo-secure-" + (Get-Date -Format "yyyyMMddHHmmss")
$pushIdentityConsumerGroup = "nexusim-push-identity-demo-secure-" + (Get-Date -Format "yyyyMMddHHmmss")

$conversationTarget = "127.0.0.1:11896"
$messageTarget = "127.0.0.1:11895"
$policyTarget = "127.0.0.1:11900"
$deliveryTarget = "127.0.0.1:11897"
$pushTarget = "127.0.0.1:11898"
$receiptTarget = "127.0.0.1:11899"
$apiGatewayTarget = "127.0.0.1:11903"
$pushURL = "wss://$pushTarget"
$gatewayAuthSecret = "nexusim-secure-demo-gateway-secret"

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null
New-Item -ItemType Directory -Force $certDir | Out-Null

if (-not $SkipBuild) {
    go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\policy-service.exe ./services/policy-service/cmd/policy-service
    go build -o bin\delivery-service.exe ./services/delivery-service/cmd/delivery-service
    go build -o bin\receipt-service.exe ./services/receipt-service/cmd/receipt-service
    go build -o bin\push-gateway.exe ./services/push-gateway/cmd/push-gateway
    go build -o bin\api-gateway.exe ./services/api-gateway/cmd/api-gateway
    go build -o bin\nexusim-e2e-demo.exe ./loadtest/demo
}

function Invoke-OpenSSL {
    param([string[]]$ArgumentList)
    & openssl @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "openssl failed: openssl $($ArgumentList -join ' ')"
    }
}

function New-SmokeCA {
    param([string]$Directory)
    $caKey = Join-Path $Directory "ca.key"
    $caCert = Join-Path $Directory "ca.crt"
    $caConf = Join-Path $Directory "ca.openssl.cnf"
    @(
        "[req]",
        "distinguished_name = req_distinguished_name",
        "x509_extensions = v3_ca",
        "prompt = no",
        "",
        "[req_distinguished_name]",
        "CN = NexusIM Local Smoke CA",
        "",
        "[v3_ca]",
        "basicConstraints = critical,CA:TRUE",
        "keyUsage = critical,keyCertSign,cRLSign",
        "subjectKeyIdentifier = hash"
    ) | Set-Content -LiteralPath $caConf -Encoding ASCII
    Invoke-OpenSSL -ArgumentList @("genrsa", "-out", $caKey, "2048")
    Invoke-OpenSSL -ArgumentList @(
        "req", "-x509", "-new", "-nodes",
        "-key", $caKey,
        "-sha256",
        "-days", "2",
        "-config", $caConf,
        "-subj", "/CN=NexusIM Local Smoke CA",
        "-out", $caCert
    )
    return @{ Key = $caKey; Cert = $caCert }
}

function New-SmokeCert {
    param(
        [string]$Directory,
        [string]$Name,
        [string]$CommonName,
        [string[]]$DnsNames,
        [string[]]$IpAddresses,
        [string[]]$Uris,
        [ValidateSet("server", "client")]
        [string]$Kind,
        [string]$CAKey,
        [string]$CACert
    )
    $key = Join-Path $Directory "$Name.key"
    $csr = Join-Path $Directory "$Name.csr"
    $cert = Join-Path $Directory "$Name.crt"
    $conf = Join-Path $Directory "$Name.openssl.cnf"
    $eku = if ($Kind -eq "server") { "serverAuth" } else { "clientAuth" }

    $lines = @(
        "[req]",
        "distinguished_name = req_distinguished_name",
        "req_extensions = v3_req",
        "prompt = no",
        "",
        "[req_distinguished_name]",
        "CN = $CommonName",
        "",
        "[v3_req]",
        "basicConstraints = CA:FALSE",
        "keyUsage = digitalSignature, keyEncipherment",
        "extendedKeyUsage = $eku",
        "subjectAltName = @alt_names",
        "",
        "[alt_names]"
    )
    $index = 1
    foreach ($dnsName in $DnsNames) {
        if (-not [string]::IsNullOrWhiteSpace($dnsName)) {
            $lines += "DNS.$index = $dnsName"
            $index++
        }
    }
    $index = 1
    foreach ($ipAddress in $IpAddresses) {
        if (-not [string]::IsNullOrWhiteSpace($ipAddress)) {
            $lines += "IP.$index = $ipAddress"
            $index++
        }
    }
    $index = 1
    foreach ($uri in $Uris) {
        if (-not [string]::IsNullOrWhiteSpace($uri)) {
            $lines += "URI.$index = $uri"
            $index++
        }
    }
    Set-Content -LiteralPath $conf -Value ($lines -join "`n") -Encoding ASCII

    Invoke-OpenSSL -ArgumentList @("genrsa", "-out", $key, "2048")
    Invoke-OpenSSL -ArgumentList @("req", "-new", "-key", $key, "-out", $csr, "-config", $conf)
    Invoke-OpenSSL -ArgumentList @(
        "x509", "-req",
        "-in", $csr,
        "-CA", $CACert,
        "-CAkey", $CAKey,
        "-CAcreateserial",
        "-out", $cert,
        "-days", "2",
        "-sha256",
        "-extensions", "v3_req",
        "-extfile", $conf
    )
    return @{ Key = $key; Cert = $cert }
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

function Invoke-PostgresScalar {
    param([string]$Sql)
    $output = docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -At `
        -v ON_ERROR_STOP=1 `
        -c $Sql
    if ($LASTEXITCODE -ne 0) {
        throw "postgres scalar query failed"
    }
    if ($null -eq $output) {
        return ""
    }
    return (($output | Select-Object -Last 1) -as [string]).Trim()
}

function Wait-TenantOutboxSettled {
    param(
        [string]$TableName,
        [string]$TenantID,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $safeTenantID = $TenantID.Replace("'", "''")
    $lastStatus = ""
    while ((Get-Date) -lt $deadline) {
        $lastStatus = Invoke-PostgresScalar -Sql "SELECT COALESCE(string_agg(status || ':' || count, ',' ORDER BY status), '') FROM (SELECT status, count(*) AS count FROM $TableName WHERE tenant_id='$safeTenantID' GROUP BY status) s;"
        $notPublished = Invoke-PostgresScalar -Sql "SELECT COUNT(*) FROM $TableName WHERE tenant_id='$safeTenantID' AND status <> 'PUBLISHED';"
        if ($notPublished -eq "0") {
            return
        }
        Start-Sleep -Milliseconds 250
    }
    throw "$TableName did not settle for tenant $TenantID before timeout; status=$lastStatus"
}

function Assert-TenantOutboxPublishedCount {
    param(
        [string]$TableName,
        [string]$TenantID,
        [int]$MinCount = 1
    )
    $safeTenantID = $TenantID.Replace("'", "''")
    $published = Invoke-PostgresScalar -Sql "SELECT COUNT(*) FROM $TableName WHERE tenant_id='$safeTenantID' AND status = 'PUBLISHED';"
    if ([int64]$published -lt $MinCount) {
        throw "$TableName published count for tenant $TenantID is $published, expected at least $MinCount"
    }
}

function Clear-LocalMessageOutboxSmokeResiduals {
    $cleanupSQL = @'
DELETE FROM message_outbox
WHERE status <> 'PUBLISHED'
  AND (
    tenant_id LIKE 'tenant-it-%'
    OR tenant_id LIKE 'tenant-outbox-concurrent-%'
    OR tenant_id LIKE 'tenant-policy-context-%'
    OR tenant_id LIKE 'tenant-push-%'
    OR tenant_id LIKE 'tenant-push-gateway-%'
    OR tenant_id LIKE 'tenant-receipt-%'
    OR tenant_id LIKE 'tenant-e2e-demo-%'
    OR tenant_id LIKE 'tenant-e2e-demo-secure-%'
  );
'@
    $cleanupFile = Join-Path $resultDir "cleanup-message-outbox-residuals.sql"
    $cleanupLog = Join-Path $logDir "preflight-cleanup.out.log"
    Set-Content -LiteralPath $cleanupFile -Value $cleanupSQL -Encoding ASCII
    docker cp $cleanupFile "nexusim-postgres:/tmp/cleanup-message-outbox-residuals.sql" | Out-Null
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f /tmp/cleanup-message-outbox-residuals.sql |
        Tee-Object -FilePath $cleanupLog | Out-Null
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

function Assert-TcpPortAvailable {
    param(
        [string]$HostName,
        [int]$Port
    )
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $connect = $client.BeginConnect($HostName, $Port, $null, $null)
        if ($connect.AsyncWaitHandle.WaitOne(300)) {
            $client.EndConnect($connect)
            throw "Port ${HostName}:${Port} is already in use; stop the existing process before running secure demo smoke."
        }
    } catch [System.Net.Sockets.SocketException] {
        return
    } finally {
        $client.Close()
    }
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port = 0
    )
    $existingNexusEnvKeys = @([Environment]::GetEnvironmentVariables("Process").Keys | Where-Object { $_ -like "NEXUSIM_*" })
    foreach ($key in $existingNexusEnvKeys) {
        [Environment]::SetEnvironmentVariable([string]$key, $null, "Process")
    }
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
    if ($proc.HasExited) {
        throw "$Name exited during startup; see $err"
    }
    return $proc
}

$ca = New-SmokeCA -Directory $certDir
$conversationServer = New-SmokeCert -Directory $certDir -Name "conversation-service" -CommonName "conversation-service.nexusim.local" -DnsNames @("conversation-service.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert
$messageServer = New-SmokeCert -Directory $certDir -Name "message-service" -CommonName "message-service.nexusim.local" -DnsNames @("message-service.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert
$policyServer = New-SmokeCert -Directory $certDir -Name "policy-service" -CommonName "policy-service.nexusim.local" -DnsNames @("policy-service.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert
$deliveryServer = New-SmokeCert -Directory $certDir -Name "delivery-service" -CommonName "delivery-service.nexusim.local" -DnsNames @("delivery-service.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert
$receiptServer = New-SmokeCert -Directory $certDir -Name "receipt-service" -CommonName "receipt-service.nexusim.local" -DnsNames @("receipt-service.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert
$pushServer = New-SmokeCert -Directory $certDir -Name "push-gateway" -CommonName "push-gateway.nexusim.local" -DnsNames @("push-gateway.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert
$apiGatewayServer = New-SmokeCert -Directory $certDir -Name "api-gateway" -CommonName "api-gateway.nexusim.local" -DnsNames @("api-gateway.nexusim.local", "localhost") -IpAddresses @("127.0.0.1") -Uris @() -Kind server -CAKey $ca.Key -CACert $ca.Cert

$apiGatewayClient = New-SmokeCert -Directory $certDir -Name "api-gateway-client" -CommonName "api-gateway.nexusim.local" -DnsNames @("api-gateway.nexusim.local") -IpAddresses @() -Uris @("spiffe://nexusim/api-gateway") -Kind client -CAKey $ca.Key -CACert $ca.Cert
$messageClient = New-SmokeCert -Directory $certDir -Name "message-service-client" -CommonName "message-service.nexusim.local" -DnsNames @("message-service.nexusim.local") -IpAddresses @() -Uris @("spiffe://nexusim/message-service") -Kind client -CAKey $ca.Key -CACert $ca.Cert
$pushClient = New-SmokeCert -Directory $certDir -Name "push-gateway-client" -CommonName "push-gateway.nexusim.local" -DnsNames @("push-gateway.nexusim.local") -IpAddresses @() -Uris @("spiffe://nexusim/push-gateway") -Kind client -CAKey $ca.Key -CACert $ca.Cert
$desktopClient = New-SmokeCert -Directory $certDir -Name "desktop-client" -CommonName "desktop-client.nexusim.local" -DnsNames @("desktop-client.nexusim.local") -IpAddresses @() -Uris @("spiffe://nexusim/desktop-client") -Kind client -CAKey $ca.Key -CACert $ca.Cert

$processes = @()
try {
    foreach ($port in @(11895, 11896, 11897, 11898, 11899, 11900, 11901, 11902, 11903, 11904)) {
        Assert-TcpPortAvailable -HostName "127.0.0.1" -Port $port
    }

    Apply-PostgresMigration -Path "migrations\postgres\message\000001_message_core.sql" -Name "nexusim_message_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000001_conversation_core.sql" -Name "nexusim_conversation_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000002_member_change_saga_v2.sql" -Name "nexusim_conversation_member_change_saga_v2.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000003_member_change_event_unique.sql" -Name "nexusim_conversation_member_change_event_unique.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000004_owner_transfer_contract.sql" -Name "nexusim_conversation_owner_transfer_contract.sql"
    Apply-PostgresMigration -Path "migrations\postgres\delivery\000001_delivery_core.sql" -Name "nexusim_delivery_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000001_receipt_core.sql" -Name "nexusim_receipt_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000002_conversation_summary.sql" -Name "nexusim_receipt_conversation_summary.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000003_receipt_source_event_type.sql" -Name "nexusim_receipt_source_event_type.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000004_conversation_summary_source_event_type.sql" -Name "nexusim_receipt_conversation_summary_source_event_type.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000005_conversation_archive.sql" -Name "nexusim_receipt_conversation_archive.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000006_conversation_pin.sql" -Name "nexusim_receipt_conversation_pin.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000007_conversation_mute.sql" -Name "nexusim_receipt_conversation_mute.sql"
    Apply-PostgresMigration -Path "migrations\postgres\receipt\000008_conversation_unread_filter.sql" -Name "nexusim_receipt_conversation_unread_filter.sql"
    foreach ($policyMigration in Get-ChildItem -Path "migrations\postgres\policy" -Filter "*.sql" | Sort-Object Name) {
        Apply-PostgresMigration -Path $policyMigration.FullName -Name ("nexusim_policy_" + $policyMigration.Name)
    }

    Ensure-KafkaTopic -Topic $timelineTopic
    Ensure-KafkaTopic -Topic $deliveryTopic
    Ensure-KafkaTopic -Topic $receiptTopic
    Ensure-KafkaTopic -Topic $identityTopic
    Ensure-KafkaTopic -Topic $policyTopic
    Reset-ConsumerGroupToLatest -Group $deliveryConsumerGroup -Topic $timelineTopic
    Reset-ConsumerGroupToLatest -Group $receiptConsumerGroup -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $pushConsumerGroup -Topic $deliveryTopic
    Reset-ConsumerGroupToLatest -Group $pushIdentityConsumerGroup -Topic $identityTopic
    Clear-LocalMessageOutboxSmokeResiduals

    $conversationService = Join-Path $repo "bin\conversation-service.exe"
    $messageService = Join-Path $repo "bin\message-service.exe"
    $policyService = Join-Path $repo "bin\policy-service.exe"
    $deliveryService = Join-Path $repo "bin\delivery-service.exe"
    $receiptService = Join-Path $repo "bin\receipt-service.exe"
    $pushGateway = Join-Path $repo "bin\push-gateway.exe"
    $apiGateway = Join-Path $repo "bin\api-gateway.exe"
    $runner = Join-Path $repo "bin\nexusim-e2e-demo.exe"

    $processes += Start-NexusProcess -Name "conversation-grpc" -FilePath $conversationService -Port 11896 -Env @{
        NEXUSIM_CONVERSATION_SERVICE_MODE = "grpc"
        NEXUSIM_CONVERSATION_GRPC_ADDR = $conversationTarget
        NEXUSIM_CONVERSATION_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE = $conversationServer.Cert
        NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE = $conversationServer.Key
        NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = "api-gateway.nexusim.local,message-service.nexusim.local"
        NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/api-gateway,spiffe://nexusim/message-service"
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

    $processes += Start-NexusProcess -Name "policy-grpc" -FilePath $policyService -Port 11900 -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "grpc"
        NEXUSIM_POLICY_GRPC_ADDR = $policyTarget
        NEXUSIM_POLICY_MESSAGE_ALLOWED = "true"
        NEXUSIM_POLICY_PERMISSION_VERSION = "2"
        NEXUSIM_POLICY_CLASSIFICATION = "POLICY_DEMO_ALLOWED"
        NEXUSIM_POLICY_RULES_ENABLED = "true"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_POLICY_DEBUG_ADDR = "127.0.0.1:11901"
        NEXUSIM_POLICY_GRPC_TLS_CERT_FILE = $policyServer.Cert
        NEXUSIM_POLICY_GRPC_TLS_KEY_FILE = $policyServer.Key
        NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_POLICY_GRPC_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = "message-service.nexusim.local"
        NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/message-service"
    }

    $processes += Start-NexusProcess -Name "policy-outbox-relay" -FilePath $policyService -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC = $policyTopic
        NEXUSIM_POLICY_OUTBOX_POLL_INTERVAL = "200ms"
    }

    $processes += Start-NexusProcess -Name "delivery-timeline-consumer" -FilePath $deliveryService -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "timeline-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_TIMELINE_TOPIC = $timelineTopic
        NEXUSIM_DELIVERY_CONSUMER_GROUP = $deliveryConsumerGroup
    }

    $processes += Start-NexusProcess -Name "delivery-grpc" -FilePath $deliveryService -Port 11897 -Env @{
        NEXUSIM_DELIVERY_SERVICE_MODE = "grpc"
        NEXUSIM_DELIVERY_GRPC_ADDR = $deliveryTarget
        NEXUSIM_DELIVERY_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_DELIVERY_GRPC_TLS_CERT_FILE = $deliveryServer.Cert
        NEXUSIM_DELIVERY_GRPC_TLS_KEY_FILE = $deliveryServer.Key
        NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = "api-gateway.nexusim.local,push-gateway.nexusim.local"
        NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/api-gateway,spiffe://nexusim/push-gateway"
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

    $processes += Start-NexusProcess -Name "receipt-grpc" -FilePath $receiptService -Port 11899 -Env @{
        NEXUSIM_RECEIPT_SERVICE_MODE = "grpc"
        NEXUSIM_RECEIPT_GRPC_ADDR = $receiptTarget
        NEXUSIM_RECEIPT_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_RECEIPT_GRPC_TLS_CERT_FILE = $receiptServer.Cert
        NEXUSIM_RECEIPT_GRPC_TLS_KEY_FILE = $receiptServer.Key
        NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = "api-gateway.nexusim.local"
        NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/api-gateway"
    }

    $processes += Start-NexusProcess -Name "push-gateway" -FilePath $pushGateway -Port 11898 -Env @{
        NEXUSIM_PUSH_GATEWAY_MODE = "all"
        NEXUSIM_PUSH_WS_ADDR = $pushTarget
        NEXUSIM_DELIVERY_GRPC_ADDR = $deliveryTarget
        NEXUSIM_DELIVERY_GRPC_TIMEOUT = "2s"
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_DELIVERY_EVENTS_TOPIC = $deliveryTopic
        NEXUSIM_IDENTITY_EVENTS_TOPIC = $identityTopic
        NEXUSIM_PUSH_CONSUMER_GROUP = $pushConsumerGroup
        NEXUSIM_PUSH_IDENTITY_CONSUMER_GROUP = $pushIdentityConsumerGroup
        NEXUSIM_PUSH_WS_TLS_CERT_FILE = $pushServer.Cert
        NEXUSIM_PUSH_WS_TLS_KEY_FILE = $pushServer.Key
        NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES = "desktop-client.nexusim.local"
        NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/desktop-client"
        NEXUSIM_PUSH_DEBUG_ADDR = "127.0.0.1:11902"
        NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE = $ca.Cert
        NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME = "delivery-service.nexusim.local"
        NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE = $pushClient.Cert
        NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE = $pushClient.Key
    }

    $processes += Start-NexusProcess -Name "message-grpc" -FilePath $messageService -Port 11895 -Env @{
        NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        NEXUSIM_GRPC_ADDR = $messageTarget
        NEXUSIM_MESSAGE_AUTH_MODE = "metadata"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_SERVICE_ADDR = $conversationTarget
        NEXUSIM_CONVERSATION_RPC_TIMEOUT = "500ms"
        NEXUSIM_CONVERSATION_SERVICE_TLS_CA_FILE = $ca.Cert
        NEXUSIM_CONVERSATION_SERVICE_TLS_SERVER_NAME = "conversation-service.nexusim.local"
        NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_CERT_FILE = $messageClient.Cert
        NEXUSIM_CONVERSATION_SERVICE_TLS_CLIENT_KEY_FILE = $messageClient.Key
        NEXUSIM_POLICY_SERVICE_ADDR = $policyTarget
        NEXUSIM_POLICY_RPC_TIMEOUT = "2s"
        NEXUSIM_POLICY_SERVICE_TLS_CA_FILE = $ca.Cert
        NEXUSIM_POLICY_SERVICE_TLS_SERVER_NAME = "policy-service.nexusim.local"
        NEXUSIM_POLICY_SERVICE_TLS_CLIENT_CERT_FILE = $messageClient.Cert
        NEXUSIM_POLICY_SERVICE_TLS_CLIENT_KEY_FILE = $messageClient.Key
        NEXUSIM_MESSAGE_GRPC_TLS_CERT_FILE = $messageServer.Cert
        NEXUSIM_MESSAGE_GRPC_TLS_KEY_FILE = $messageServer.Key
        NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_MESSAGE_GRPC_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = "api-gateway.nexusim.local"
        NEXUSIM_MESSAGE_GRPC_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/api-gateway"
    }

    $processes += Start-NexusProcess -Name "api-gateway-grpc" -FilePath $apiGateway -Port 11903 -Env @{
        NEXUSIM_API_GATEWAY_MODE = "grpc"
        NEXUSIM_API_GATEWAY_GRPC_ADDR = $apiGatewayTarget
        NEXUSIM_API_GATEWAY_AUTH_MODE = "hmac"
        NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET = $gatewayAuthSecret
        NEXUSIM_API_GATEWAY_AUTH_AUDIENCE = "api-gateway"
        NEXUSIM_API_GATEWAY_CONVERSATION_ADDR = $conversationTarget
        NEXUSIM_API_GATEWAY_CONVERSATION_TLS_CA_FILE = $ca.Cert
        NEXUSIM_API_GATEWAY_CONVERSATION_TLS_SERVER_NAME = "conversation-service.nexusim.local"
        NEXUSIM_API_GATEWAY_CONVERSATION_TLS_CLIENT_CERT_FILE = $apiGatewayClient.Cert
        NEXUSIM_API_GATEWAY_CONVERSATION_TLS_CLIENT_KEY_FILE = $apiGatewayClient.Key
        NEXUSIM_API_GATEWAY_MESSAGE_ADDR = $messageTarget
        NEXUSIM_API_GATEWAY_MESSAGE_TLS_CA_FILE = $ca.Cert
        NEXUSIM_API_GATEWAY_MESSAGE_TLS_SERVER_NAME = "message-service.nexusim.local"
        NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_CERT_FILE = $apiGatewayClient.Cert
        NEXUSIM_API_GATEWAY_MESSAGE_TLS_CLIENT_KEY_FILE = $apiGatewayClient.Key
        NEXUSIM_API_GATEWAY_DELIVERY_ADDR = $deliveryTarget
        NEXUSIM_API_GATEWAY_DELIVERY_TLS_CA_FILE = $ca.Cert
        NEXUSIM_API_GATEWAY_DELIVERY_TLS_SERVER_NAME = "delivery-service.nexusim.local"
        NEXUSIM_API_GATEWAY_DELIVERY_TLS_CLIENT_CERT_FILE = $apiGatewayClient.Cert
        NEXUSIM_API_GATEWAY_DELIVERY_TLS_CLIENT_KEY_FILE = $apiGatewayClient.Key
        NEXUSIM_API_GATEWAY_RECEIPT_ADDR = $receiptTarget
        NEXUSIM_API_GATEWAY_RECEIPT_TLS_CA_FILE = $ca.Cert
        NEXUSIM_API_GATEWAY_RECEIPT_TLS_SERVER_NAME = "receipt-service.nexusim.local"
        NEXUSIM_API_GATEWAY_RECEIPT_TLS_CLIENT_CERT_FILE = $apiGatewayClient.Cert
        NEXUSIM_API_GATEWAY_RECEIPT_TLS_CLIENT_KEY_FILE = $apiGatewayClient.Key
        NEXUSIM_API_GATEWAY_GRPC_TLS_CERT_FILE = $apiGatewayServer.Cert
        NEXUSIM_API_GATEWAY_GRPC_TLS_KEY_FILE = $apiGatewayServer.Key
        NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_CA_FILE = $ca.Cert
        NEXUSIM_API_GATEWAY_GRPC_TLS_REQUIRE_CLIENT_CERT = "true"
        NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = "desktop-client.nexusim.local"
        NEXUSIM_API_GATEWAY_GRPC_TLS_CLIENT_ALLOWED_URIS = "spiffe://nexusim/desktop-client"
        NEXUSIM_API_GATEWAY_DEBUG_ADDR = "127.0.0.1:11904"
    }

    $runnerArgs = @(
        "--pg-dsn", $PgDsn,
        "--conversation-target", $apiGatewayTarget,
        "--message-target", $apiGatewayTarget,
        "--delivery-target", $apiGatewayTarget,
        "--receipt-target", $apiGatewayTarget,
        "--gateway-facade",
        "--push-url", $pushURL,
        "--result-dir", $resultDir,
        "--tenant-id", $TenantId,
        "--conversation-id", $ConversationId,
        "--sender-user-id", $SenderUserId,
        "--receiver-user-id", $ReceiverUserId,
        "--receiver-device-id", $ReceiverDeviceId,
        "--gateway-auth-mode", "hmac",
        "--gateway-auth-hmac-secret", $gatewayAuthSecret,
        "--gateway-auth-audience", "api-gateway",
        "--conversation-tls-ca-file", $ca.Cert,
        "--conversation-tls-server-name", "api-gateway.nexusim.local",
        "--conversation-tls-client-cert-file", $desktopClient.Cert,
        "--conversation-tls-client-key-file", $desktopClient.Key,
        "--message-tls-ca-file", $ca.Cert,
        "--message-tls-server-name", "api-gateway.nexusim.local",
        "--message-tls-client-cert-file", $desktopClient.Cert,
        "--message-tls-client-key-file", $desktopClient.Key,
        "--delivery-tls-ca-file", $ca.Cert,
        "--delivery-tls-server-name", "api-gateway.nexusim.local",
        "--delivery-tls-client-cert-file", $desktopClient.Cert,
        "--delivery-tls-client-key-file", $desktopClient.Key,
        "--receipt-tls-ca-file", $ca.Cert,
        "--receipt-tls-server-name", "api-gateway.nexusim.local",
        "--receipt-tls-client-cert-file", $desktopClient.Cert,
        "--receipt-tls-client-key-file", $desktopClient.Key,
        "--push-tls-ca-file", $ca.Cert,
        "--push-tls-server-name", "push-gateway.nexusim.local",
        "--push-tls-client-cert-file", $desktopClient.Cert,
        "--push-tls-client-key-file", $desktopClient.Key,
        "--policy-kafka-brokers", $KafkaBrokers,
        "--policy-topic", $policyTopic,
        "--policy-readback-min", "1",
        "--wait-timeout", "30s",
        "--request-timeout", "5s"
    )
    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "secure e2e demo runner failed with exit code $LASTEXITCODE"
    }
    Wait-TenantOutboxSettled -TableName "message_outbox" -TenantID $TenantId
    Wait-TenantOutboxSettled -TableName "policy_decision_audit_outbox" -TenantID $TenantId
    Assert-TenantOutboxPublishedCount -TableName "policy_decision_audit_outbox" -TenantID $TenantId -MinCount 1
    Wait-TenantOutboxSettled -TableName "delivery_outbox" -TenantID $TenantId
    Wait-TenantOutboxSettled -TableName "receipt_outbox" -TenantID $TenantId
    $apiGatewayMetricsPath = Join-Path $resultDir "api-gateway-debug-metrics.json"
    Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:11904/debug/metrics" -OutFile $apiGatewayMetricsPath | Out-Null
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
Write-Host "identity_topic=$identityTopic"
Write-Host "policy_topic=$policyTopic"
Write-Host "delivery_consumer_group=$deliveryConsumerGroup"
Write-Host "receipt_consumer_group=$receiptConsumerGroup"
Write-Host "push_consumer_group=$pushConsumerGroup"
Write-Host "push_identity_consumer_group=$pushIdentityConsumerGroup"
Write-Host "api_gateway_target=$apiGatewayTarget"
Write-Host "api_gateway_debug_metrics=$apiGatewayMetricsPath"
Write-Host "push_url=$pushURL"
