param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [ValidateSet("accept", "decline", "cancel", "delete", "block", "unblock", "remark", "readd")]
    [string]$Scenario = "accept",
    [string]$ContactsGrpcTlsCertFile = "",
    [string]$ContactsGrpcTlsKeyFile = "",
    [string]$ContactsGrpcTlsClientCaFile = "",
    [string]$ContactsGrpcTlsRequireClientCert = "",
    [string]$ContactsGrpcTlsClientAllowedDnsNames = "",
    [string]$ContactsGrpcTlsClientAllowedUris = "",
    [string]$ContactsTlsCaFile = "",
    [string]$ContactsTlsServerName = "",
    [string]$ContactsTlsClientCertFile = "",
    [string]$ContactsTlsClientKeyFile = "",
    [switch]$VerifiedAuthMetadata,
    [switch]$GatewayFacade,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$nexusIMRepoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $nexusIMRepoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $nexusIMRepoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "contacts-$Scenario-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ($GatewayFacade -and $VerifiedAuthMetadata) {
    throw "-GatewayFacade and -VerifiedAuthMetadata cannot be combined"
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$topicSuffix = Get-Date -Format "yyyyMMdd-HHmmss"
$contactTopic = "im.contact.events.contacts-$Scenario-smoke.$topicSuffix"
$tenantId = "tenant-contacts-$Scenario-smoke-$topicSuffix"
$senderUserId = "contacts-sender"
$receiverUserId = "contacts-receiver"
$contactsGrpcPort = 0
$apiGatewayGrpcPort = 0
$gatewayAuthSecret = "nexusim-contacts-facade-smoke-secret"

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\contacts-service.exe ./services/contacts-service/cmd/contacts-service
    go build -o bin\api-gateway.exe ./services/api-gateway/cmd/api-gateway
    go build -o bin\contacts-loadtest.exe ./loadtest/contacts
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
    if (-not $summary.success) {
        throw "contacts smoke failed: $($summary.error)"
    }
    if ($summary.send_contact_request.status -ne "CONTACT_REQUEST_STATUS_PENDING") {
        throw "unexpected send status: $($summary.send_contact_request.status)"
    }
    if ($Scenario -eq "accept" -and $summary.respond_contact_request.status -ne "CONTACT_REQUEST_STATUS_ACCEPTED") {
        throw "unexpected accept status: $($summary.respond_contact_request.status)"
    }
    if ($Scenario -eq "decline" -and $summary.respond_contact_request.status -ne "CONTACT_REQUEST_STATUS_DECLINED") {
        throw "unexpected decline status: $($summary.respond_contact_request.status)"
    }
    if ($Scenario -eq "readd") {
        if ($summary.second_send_contact_request.status -ne "CONTACT_REQUEST_STATUS_PENDING") {
            throw "unexpected second send status: $($summary.second_send_contact_request.status)"
        }
        if ($summary.second_respond_contact_request.status -ne "CONTACT_REQUEST_STATUS_ACCEPTED") {
            throw "unexpected second accept status: $($summary.second_respond_contact_request.status)"
        }
    }
    if ($summary.contacts_outbox.pending -ne 0 -or $summary.contacts_outbox.dlq -ne 0 -or $summary.contacts_outbox.published -lt 2) {
        throw "contacts outbox did not drain: pending=$($summary.contacts_outbox.pending) published=$($summary.contacts_outbox.published) dlq=$($summary.contacts_outbox.dlq)"
    }
    if ($summary.contact_kafka_events.Count -lt 2) {
        throw "expected at least 2 contact Kafka events, got $($summary.contact_kafka_events.Count)"
    }
    if ($GatewayFacade -and -not $summary.gateway_facade) {
        throw "expected gateway_facade=true in summary"
    }
    if ($GatewayFacade -and $summary.gateway_auth_mode -ne "hmac") {
        throw "expected gateway_auth_mode=hmac in summary, got $($summary.gateway_auth_mode)"
    }
}

$processes = @()
try {
    Get-ChildItem -Path "migrations\postgres\contacts" -Filter "*.sql" |
        Sort-Object Name |
        ForEach-Object {
            Apply-PostgresMigration -Path $_.FullName -Name $_.Name
        }
    Ensure-KafkaTopic -Topic $contactTopic

    $contactsService = Join-Path $repo "bin\contacts-service.exe"
    $apiGateway = Join-Path $repo "bin\api-gateway.exe"
    $runner = Join-Path $repo "bin\contacts-loadtest.exe"
    $contactsGrpcPort = Get-FreeTcpPort
    $contactsGrpcAddr = "127.0.0.1:$contactsGrpcPort"
    $apiGatewayGrpcPort = Get-FreeTcpPort
    $apiGatewayGrpcAddr = "127.0.0.1:$apiGatewayGrpcPort"

    $processes += Start-NexusProcess -Name "contacts-grpc" -FilePath $contactsService -Port $contactsGrpcPort -Env @{
        NEXUSIM_CONTACTS_SERVICE_MODE = "grpc"
        NEXUSIM_CONTACTS_AUTH_MODE = $(if ($VerifiedAuthMetadata -or $GatewayFacade) { "metadata" } else { "body" })
        NEXUSIM_CONTACTS_GRPC_ADDR = $contactsGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_CONTACTS_GRPC_TLS_CERT_FILE = $ContactsGrpcTlsCertFile
        NEXUSIM_CONTACTS_GRPC_TLS_KEY_FILE = $ContactsGrpcTlsKeyFile
        NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_CA_FILE = $ContactsGrpcTlsClientCaFile
        NEXUSIM_CONTACTS_GRPC_TLS_REQUIRE_CLIENT_CERT = $ContactsGrpcTlsRequireClientCert
        NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES = $ContactsGrpcTlsClientAllowedDnsNames
        NEXUSIM_CONTACTS_GRPC_TLS_CLIENT_ALLOWED_URIS = $ContactsGrpcTlsClientAllowedUris
    }

    if ($GatewayFacade) {
        $processes += Start-NexusProcess -Name "api-gateway-grpc" -FilePath $apiGateway -Port $apiGatewayGrpcPort -Env @{
            NEXUSIM_API_GATEWAY_MODE = "grpc"
            NEXUSIM_API_GATEWAY_GRPC_ADDR = $apiGatewayGrpcAddr
            NEXUSIM_API_GATEWAY_AUTH_MODE = "hmac"
            NEXUSIM_API_GATEWAY_AUTH_HMAC_SECRET = $gatewayAuthSecret
            NEXUSIM_API_GATEWAY_AUTH_AUDIENCE = "api-gateway"
            NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS = "false"
            NEXUSIM_API_GATEWAY_CONTACTS_ADDR = $contactsGrpcAddr
            NEXUSIM_API_GATEWAY_CONTACTS_TLS_CA_FILE = $ContactsTlsCaFile
            NEXUSIM_API_GATEWAY_CONTACTS_TLS_SERVER_NAME = $ContactsTlsServerName
            NEXUSIM_API_GATEWAY_CONTACTS_TLS_CLIENT_CERT_FILE = $ContactsTlsClientCertFile
            NEXUSIM_API_GATEWAY_CONTACTS_TLS_CLIENT_KEY_FILE = $ContactsTlsClientKeyFile
        }
    }

    $processes += Start-NexusProcess -Name "contacts-outbox-relay" -FilePath $contactsService -Env @{
        NEXUSIM_CONTACTS_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_CONTACT_EVENTS_TOPIC = $contactTopic
        NEXUSIM_CONTACTS_OUTBOX_BATCH_SIZE = "100"
        NEXUSIM_CONTACTS_OUTBOX_POLL_INTERVAL = "200ms"
    }

    $runnerArgs = @(
        "--target", $(if ($GatewayFacade) { $apiGatewayGrpcAddr } else { $contactsGrpcAddr }),
        "--pg-dsn", $PgDsn,
        "--kafka-brokers", $KafkaBrokers,
        "--contact-topic", $contactTopic,
        "--tenant-id", $tenantId,
        "--sender-user-id", $senderUserId,
        "--receiver-user-id", $receiverUserId,
        "--scenario", $Scenario,
        "--cleanup",
        "--wait-timeout", "15s",
        "--result-dir", $resultDir
    )
    if ($VerifiedAuthMetadata) {
        $runnerArgs += @("--verified-auth-metadata")
    }
    if ($GatewayFacade) {
        $runnerArgs += @(
            "--gateway-facade",
            "--gateway-auth-mode", "hmac",
            "--gateway-auth-hmac-secret", $gatewayAuthSecret,
            "--gateway-auth-audience", "api-gateway"
        )
    }
    if ((-not $GatewayFacade) -and -not [string]::IsNullOrWhiteSpace($ContactsTlsCaFile)) {
        $runnerArgs += @("--contacts-tls-ca-file", $ContactsTlsCaFile)
    }
    if ((-not $GatewayFacade) -and -not [string]::IsNullOrWhiteSpace($ContactsTlsServerName)) {
        $runnerArgs += @("--contacts-tls-server-name", $ContactsTlsServerName)
    }
    if ((-not $GatewayFacade) -and -not [string]::IsNullOrWhiteSpace($ContactsTlsClientCertFile)) {
        $runnerArgs += @("--contacts-tls-client-cert-file", $ContactsTlsClientCertFile)
    }
    if ((-not $GatewayFacade) -and -not [string]::IsNullOrWhiteSpace($ContactsTlsClientKeyFile)) {
        $runnerArgs += @("--contacts-tls-client-key-file", $ContactsTlsClientKeyFile)
    }

    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "contacts smoke runner failed with exit code $LASTEXITCODE"
    }

    Assert-Summary -Path (Join-Path $resultDir "contacts-summary.json")
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "contact_topic=$contactTopic"
Write-Host "contacts_grpc_addr=127.0.0.1:$contactsGrpcPort"
if ($GatewayFacade) {
    Write-Host "api_gateway_grpc_addr=127.0.0.1:$apiGatewayGrpcPort"
}
