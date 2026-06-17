param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [ValidateSet("owner-transfer")]
    [string]$Scenario = "owner-transfer",
    [switch]$VerifiedAuthMetadata,
    [string]$ConversationGrpcTlsCertFile = "",
    [string]$ConversationGrpcTlsKeyFile = "",
    [string]$ConversationGrpcTlsClientCaFile = "",
    [string]$ConversationGrpcTlsRequireClientCert = "",
    [string]$ConversationGrpcTlsClientAllowedDnsNames = "",
    [string]$ConversationGrpcTlsClientAllowedUris = "",
    [string]$ConversationTlsCaFile = "",
    [string]$ConversationTlsServerName = "",
    [string]$ConversationTlsClientCertFile = "",
    [string]$ConversationTlsClientKeyFile = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$nexusIMRepoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $nexusIMRepoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $nexusIMRepoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "conversation-owner-transfer-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$timelineTopic = "conversation.timeline.ownertransfer." + (Get-Date -Format "yyyyMMdd-HHmmss")
$conversationGrpcPort = 0
$tenantId = "tenant-owner-transfer-smoke"
$conversationId = "conv-owner-transfer-smoke"
$previousOwnerId = "owner-1"
$newOwnerId = "owner-transfer-user"
$idempotencyPrefix = "owner-transfer-" + (Get-Date -Format "yyyyMMddHHmmss")
$authMode = if ($VerifiedAuthMetadata) { "metadata" } else { "body" }

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\memberchange-loadtest.exe ./loadtest/memberchange
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

function Invoke-PostgresSql {
    param(
        [string]$Sql,
        [string]$Name
    )
    $path = Join-Path $resultDir $Name
    Set-Content -LiteralPath $path -Value $Sql -Encoding UTF8
    docker cp $path "nexusim-postgres:/tmp/$Name"
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f "/tmp/$Name" | Out-Null
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

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
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

function Assert-Summary {
    param([string]$Path)
    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if ($summary.success_count -ne 1 -or $summary.error_count -ne 0) {
        throw "owner transfer smoke failed: success=$($summary.success_count) error=$($summary.error_count)"
    }
    if ($summary.change_type -ne "OWNER_TRANSFER") {
        throw "unexpected change_type=$($summary.change_type)"
    }
    if ($summary.outbox_pending_count -ne 0 -or $summary.outbox_published_count -ne 1) {
        throw "outbox did not drain: pending=$($summary.outbox_pending_count) published=$($summary.outbox_published_count)"
    }
    if ($summary.saga_done_count -ne 1 -or $summary.sample_get_status -ne "MEMBER_CHANGE_STATUS_DONE") {
        throw "saga did not reach DONE: done=$($summary.saga_done_count) status=$($summary.sample_get_status)"
    }
    if ($summary.owner_transfer_owner_count -ne 1) {
        throw "expected exactly one active owner, got $($summary.owner_transfer_owner_count)"
    }
    if ($summary.owner_transfer_previous_owner_role -ne "MEMBER_ROLE_ADMIN") {
        throw "previous owner role=$($summary.owner_transfer_previous_owner_role), want MEMBER_ROLE_ADMIN"
    }
    if ($summary.owner_transfer_new_owner_role -ne "MEMBER_ROLE_OWNER") {
        throw "new owner role=$($summary.owner_transfer_new_owner_role), want MEMBER_ROLE_OWNER"
    }
}

$processes = @()
try {
    Apply-PostgresMigration -Path "migrations\postgres\message\000001_message_core.sql" -Name "nexusim_message_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000001_conversation_core.sql" -Name "nexusim_conversation_core.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000002_member_change_saga_v2.sql" -Name "nexusim_conversation_member_change_saga_v2.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000003_member_change_event_unique.sql" -Name "nexusim_conversation_member_change_event_unique.sql"
    Apply-PostgresMigration -Path "migrations\postgres\conversation\000004_owner_transfer_contract.sql" -Name "nexusim_conversation_owner_transfer_contract.sql"
    Ensure-KafkaTopic -Topic $timelineTopic

    $seedSql = @"
BEGIN;
DELETE FROM message_outbox WHERE tenant_id = '$tenantId';
DELETE FROM conversation_timeline_events WHERE tenant_id = '$tenantId';
DELETE FROM member_change_saga WHERE tenant_id = '$tenantId';
DELETE FROM conversation_seq WHERE tenant_id = '$tenantId';
DELETE FROM conversation_members WHERE tenant_id = '$tenantId';
DELETE FROM conversations WHERE tenant_id = '$tenantId';

INSERT INTO conversations (
    tenant_id,
    conversation_id,
    conversation_type,
    status,
    conversation_mode,
    fanout_mode,
    fanout_policy_version,
    member_version,
    permission_version,
    current_seq_shard
) VALUES (
    '$tenantId',
    '$conversationId',
    'GROUP',
    'ACTIVE',
    'LOCAL_ROW_LOCK',
    'WRITE_FANOUT',
    1,
    10,
    20,
    'local'
);

INSERT INTO conversation_members (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    join_seq,
    member_version,
    permission_version
) VALUES
    ('$tenantId', '$conversationId', '$previousOwnerId', 'OWNER', 'ACTIVE', 0, 10, 20),
    ('$tenantId', '$conversationId', '$newOwnerId', 'ADMIN', 'ACTIVE', 0, 10, 20);
COMMIT;
"@
    Invoke-PostgresSql -Sql $seedSql -Name "owner-transfer-seed.sql"

    $conversationService = Join-Path $repo "bin\conversation-service.exe"
    $messageService = Join-Path $repo "bin\message-service.exe"
    $runner = Join-Path $repo "bin\memberchange-loadtest.exe"
    $conversationGrpcPort = Get-FreeTcpPort
    $conversationGrpcAddr = "127.0.0.1:$conversationGrpcPort"

    $processes += Start-NexusProcess -Name "conversation-grpc" -FilePath $conversationService -Port $conversationGrpcPort -Env @{
        NEXUSIM_CONVERSATION_SERVICE_MODE = "grpc"
        NEXUSIM_CONVERSATION_GRPC_ADDR = $conversationGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONVERSATION_AUTH_MODE = $authMode
        NEXUSIM_CONVERSATION_GRPC_TLS_CERT_FILE = $ConversationGrpcTlsCertFile
        NEXUSIM_CONVERSATION_GRPC_TLS_KEY_FILE = $ConversationGrpcTlsKeyFile
        NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_CA_FILE = $ConversationGrpcTlsClientCaFile
        NEXUSIM_CONVERSATION_GRPC_TLS_REQUIRE_CLIENT_CERT = $ConversationGrpcTlsRequireClientCert
        NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = $ConversationGrpcTlsClientAllowedDnsNames
        NEXUSIM_CONVERSATION_GRPC_TLS_CLIENT_ALLOWED_URIS = $ConversationGrpcTlsClientAllowedUris
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

    $processes += Start-NexusProcess -Name "conversation-member-worker" -FilePath $conversationService -Env @{
        NEXUSIM_CONVERSATION_SERVICE_MODE = "member-change-worker"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEMBER_CHANGE_PROGRESS_POLL_INTERVAL = "200ms"
        NEXUSIM_MEMBER_CHANGE_PROGRESS_BATCH_SIZE = "100"
    }

    & $runner `
        --target $conversationGrpcAddr `
        --vus 1 `
        --duration 3s `
        --request-count 1 `
        --request-timeout 5s `
        --tenant-id $tenantId `
        --conversation-id $conversationId `
        --operator-user-id $previousOwnerId `
        --list-user-id $previousOwnerId `
        --target-user-id $newOwnerId `
        --change-type owner-transfer `
        --idempotency-prefix $idempotencyPrefix `
        --expected-member-version 10 `
        --verified-auth-metadata=$VerifiedAuthMetadata `
        --conversation-tls-ca-file $ConversationTlsCaFile `
        --conversation-tls-server-name $ConversationTlsServerName `
        --conversation-tls-client-cert-file $ConversationTlsClientCertFile `
        --conversation-tls-client-key-file $ConversationTlsClientKeyFile `
        --stats-wait 5s `
        --pg-dsn $PgDsn `
        --result-dir $resultDir
    if ($LASTEXITCODE -ne 0) {
        throw "memberchange smoke runner failed with exit code $LASTEXITCODE"
    }

    Assert-Summary -Path (Join-Path $resultDir "memberchange-summary.json")
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "timeline_topic=$timelineTopic"
Write-Host "conversation_grpc_addr=127.0.0.1:$conversationGrpcPort"
