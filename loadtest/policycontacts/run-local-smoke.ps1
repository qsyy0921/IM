param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "policy-contact-projection-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$topicSuffix = $RunName -replace '[^A-Za-z0-9._-]', '-'
$topic = "im.contact.events.$topicSuffix"
$auditTopic = "im.policy.events.$topicSuffix"
$consumerGroup = "nexusim-policy-contact-$topicSuffix"

New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\policy-service.exe ./services/policy-service/cmd/policy-service
    go build -o bin\policy-contact-loadtest.exe ./loadtest/policycontacts
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

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try {
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env
    )
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    return Start-Process -FilePath $FilePath `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
}

function Stop-Processes {
    param([array]$Processes)
    foreach ($proc in $Processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Get-ChildItem -Path "migrations\postgres\policy" -Filter "*.sql" |
    Sort-Object Name |
    ForEach-Object {
        Apply-PostgresMigration -Path $_.FullName -Name $_.Name
    }
Ensure-KafkaTopic -Topic $topic
Ensure-KafkaTopic -Topic $auditTopic
$policyGrpcPort = Get-FreeTcpPort
$policyGrpcAddr = "127.0.0.1:$policyGrpcPort"

$processes = @()
try {
    $processes += Start-NexusProcess -Name "policy-contact-consumer" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "contact-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_CONTACT_EVENTS_TOPIC = $topic
        NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP = $consumerGroup
    }
    $processes += Start-NexusProcess -Name "policy-grpc" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "grpc"
        NEXUSIM_POLICY_GRPC_ADDR = $policyGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_POLICY_RULES_ENABLED = "true"
        NEXUSIM_POLICY_MESSAGE_ALLOWED = "true"
        NEXUSIM_POLICY_PERMISSION_VERSION = "1"
        NEXUSIM_POLICY_CLASSIFICATION = "POLICY_STATIC_ALLOW"
    }
    $processes += Start-NexusProcess -Name "policy-outbox-relay" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "outbox-relay"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC = $auditTopic
        NEXUSIM_POLICY_OUTBOX_POLL_INTERVAL = "100ms"
    }

    Start-Sleep -Milliseconds 1000

    $runner = Join-Path $repo "bin\policy-contact-loadtest.exe"
    & $runner `
        --brokers $KafkaBrokers `
        --topic $topic `
        --consumer-group $consumerGroup `
        --audit-topic $auditTopic `
        --policy-grpc-addr $policyGrpcAddr `
        --pg-dsn $PgDsn `
        --result-dir $resultDir `
        --cleanup=true
    if ($LASTEXITCODE -ne 0) {
        throw "policy contact projection smoke failed with exit code $LASTEXITCODE"
    }
} finally {
    Stop-Processes $processes
}

$summary = Get-Content -LiteralPath (Join-Path $resultDir "policy-contact-summary.json") -Raw | ConvertFrom-Json
if (-not $summary.success) {
    throw "policy contact projection smoke failed: $($summary.error)"
}

Write-Host "result_dir=$resultDir"
Write-Host "topic=$topic"
Write-Host "audit_topic=$auditTopic"
Write-Host "after_blocked=$($summary.after_blocked.status)/$($summary.after_blocked.edge_version)"
Write-Host "after_unblocked=$($summary.after_unblocked.status)/$($summary.after_unblocked.edge_version)"
Write-Host "audit_outbox_published=$($summary.policy_decision_audit_outbox_status.published)"
Write-Host "audit_kafka_event_count=$($summary.policy_audit_kafka_event_count)"
