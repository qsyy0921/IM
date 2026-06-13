param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$ExerciseAuditRepair,
    [string]$PolicyTlsCaFile = "",
    [string]$PolicyTlsServerName = "",
    [string]$PolicyTlsClientCertFile = "",
    [string]$PolicyTlsClientKeyFile = "",
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

function Invoke-PostgresScalar {
    param([string]$Sql)
    $output = docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -t `
        -A `
        -v ON_ERROR_STOP=1 `
        -c $Sql
    if ($LASTEXITCODE -ne 0) {
        throw "postgres scalar query failed: $Sql"
    }
    return ($output | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1).Trim()
}

function Invoke-PostgresCommand {
    param([string]$Sql)
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -c $Sql | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "postgres command failed: $Sql"
    }
}

function Get-KafkaTopicEndOffset {
    param([string]$Topic)
    $output = docker exec nexusim-kafka kafka-get-offsets `
        --bootstrap-server localhost:9092 `
        --topic $Topic `
        --time latest
    if ($LASTEXITCODE -ne 0) {
        throw "read kafka topic offset failed: $Topic"
    }
    $total = 0L
    foreach ($line in $output) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }
        $parts = $line.Trim().Split(":")
        if ($parts.Length -lt 3) {
            continue
        }
        $total += [int64]$parts[2]
    }
    return $total
}

function Wait-PolicyAuditPublished {
    param(
        [string]$EventId,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $status = Invoke-PostgresScalar -Sql "SELECT status FROM policy_decision_audit_outbox WHERE event_id = '$EventId';"
        if ($status -eq "PUBLISHED") {
            return
        }
        Start-Sleep -Milliseconds 200
    }
    throw "timed out waiting for repaired policy audit event to be published: $EventId"
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
    $runnerArgs = @(
        "--brokers", $KafkaBrokers,
        "--topic", $topic,
        "--consumer-group", $consumerGroup,
        "--audit-topic", $auditTopic,
        "--policy-grpc-addr", $policyGrpcAddr,
        "--pg-dsn", $PgDsn,
        "--result-dir", $resultDir,
        "--cleanup=true"
    )
    if (-not [string]::IsNullOrWhiteSpace($PolicyTlsCaFile)) {
        $runnerArgs += @("--policy-tls-ca-file", $PolicyTlsCaFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($PolicyTlsServerName)) {
        $runnerArgs += @("--policy-tls-server-name", $PolicyTlsServerName)
    }
    if (-not [string]::IsNullOrWhiteSpace($PolicyTlsClientCertFile)) {
        $runnerArgs += @("--policy-tls-client-cert-file", $PolicyTlsClientCertFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($PolicyTlsClientKeyFile)) {
        $runnerArgs += @("--policy-tls-client-key-file", $PolicyTlsClientKeyFile)
    }
    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "policy contact projection smoke failed with exit code $LASTEXITCODE"
    }

    if ($ExerciseAuditRepair) {
        $summaryPath = Join-Path $resultDir "policy-contact-summary.json"
        $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
        $repairEventId = Invoke-PostgresScalar -Sql "SELECT event_id FROM policy_decision_audit_outbox WHERE tenant_id = '$($summary.tenant_id)' ORDER BY id LIMIT 1;"
        if ([string]::IsNullOrWhiteSpace($repairEventId)) {
            throw "could not find policy audit outbox event to repair"
        }
        Invoke-PostgresCommand -Sql "UPDATE policy_decision_audit_outbox SET status = 'DLQ', retry_count = 5, last_error = 'manual smoke dlq', dead_lettered_at = now(), published_at = NULL, next_retry_at = NULL, updated_at = now() WHERE event_id = '$repairEventId';"

        $repairProc = Start-NexusProcess -Name "policy-outbox-repair" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
            NEXUSIM_POLICY_SERVICE_MODE = "outbox-repair"
            NEXUSIM_PG_DSN = $PgDsn
            NEXUSIM_POLICY_OUTBOX_REPAIR_EVENT_IDS = $repairEventId
            NEXUSIM_POLICY_OUTBOX_REPAIR_OPERATOR = "policy-repair-smoke"
            NEXUSIM_POLICY_OUTBOX_REPAIR_REASON = "smoke redrive after synthetic dlq"
        }
        if (-not $repairProc.WaitForExit(30000)) {
            Stop-Process -Id $repairProc.Id -Force -ErrorAction SilentlyContinue
            throw "policy outbox repair process timed out"
        }
        $repairProc.Refresh()
        if ($null -ne $repairProc.ExitCode -and $repairProc.ExitCode -ne 0) {
            throw "policy outbox repair process failed with exit code $($repairProc.ExitCode)"
        }
        Wait-PolicyAuditPublished -EventId $repairEventId

        $repairAuditCount = [int64](Invoke-PostgresScalar -Sql "SELECT COUNT(*) FROM policy_decision_audit_outbox_repair_audit WHERE event_id = '$repairEventId';")
        $auditOffset = Get-KafkaTopicEndOffset -Topic $auditTopic
        if ($repairAuditCount -lt 1) {
            throw "expected policy audit outbox repair audit row for $repairEventId"
        }
        if ($auditOffset -lt 4) {
            throw "expected policy audit kafka topic end offset >= 4 after redrive, got $auditOffset"
        }
        $summary | Add-Member -NotePropertyName "policy_decision_audit_repair" -NotePropertyValue ([pscustomobject]@{
            event_id = $repairEventId
            repaired = 1
            skipped = 0
            repair_audit_count = $repairAuditCount
            kafka_end_offset = $auditOffset
        }) -Force
        $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
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
if ($summary.policy_decision_audit_repair) {
    Write-Host "audit_repair_event_id=$($summary.policy_decision_audit_repair.event_id)"
    Write-Host "audit_repair_kafka_end_offset=$($summary.policy_decision_audit_repair.kafka_end_offset)"
}
