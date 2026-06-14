param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-failover-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

$kafkaBrokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094"
$kafkaTopic = "im.delivery.events"
$brokerMap = @{
    "1" = @{
        Host = "127.0.0.1:19092"
        ExecContainer = "nexusim-kafka-ha-0"
        DockerContainer = "nexusim-kafka-ha-0"
    }
    "2" = @{
        Host = "127.0.0.1:19093"
        ExecContainer = "nexusim-kafka-ha-1"
        DockerContainer = "nexusim-kafka-ha-1"
    }
    "3" = @{
        Host = "127.0.0.1:19094"
        ExecContainer = "nexusim-kafka-ha-2"
        DockerContainer = "nexusim-kafka-ha-2"
    }
}

function Invoke-KafkaExec {
    param(
        [string]$ExecContainer,
        [string[]]$Arguments
    )
    $output = @(& docker exec $ExecContainer @Arguments)
    if ($LASTEXITCODE -ne 0) {
        throw "Kafka command failed in ${ExecContainer}: $($Arguments -join ' ')"
    }
    return $output
}

function Get-DeliveryTopicLeaderBrokerId {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer
    )
    $lines = Invoke-KafkaExec -ExecContainer $ExecContainer -Arguments @(
        "kafka-topics",
        "--bootstrap-server", $BootstrapServer,
        "--describe",
        "--topic", $kafkaTopic
    )
    foreach ($line in $lines) {
        if ($line -match "Partition:\s*0\s+Leader:\s*(\d+)") {
            return $matches[1]
        }
    }
    throw "Could not determine leader for topic $kafkaTopic partition 0."
}

function Test-KafkaAdminReady {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer
    )
    try {
        & docker exec $ExecContainer kafka-broker-api-versions --bootstrap-server $BootstrapServer *> $null
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

function Wait-ForKafkaFailover {
    param(
        [string]$StoppedBrokerId,
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [int]$TimeoutSeconds = 120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $currentLeader = ""
    $stableChecks = 0
    $lastError = ""
    do {
        Start-Sleep -Seconds 2
        try {
            if (-not (Test-KafkaAdminReady -ExecContainer $ExecContainer -BootstrapServer $BootstrapServer)) {
                throw "Kafka admin API is not ready on $ExecContainer"
            }
            $currentLeader = Get-DeliveryTopicLeaderBrokerId -ExecContainer $ExecContainer -BootstrapServer $BootstrapServer
            if ($currentLeader -ne $StoppedBrokerId) {
                $stableChecks++
            } else {
                $stableChecks = 0
            }
            $lastError = ""
        } catch {
            $currentLeader = ""
            $stableChecks = 0
            $lastError = $_.Exception.Message
        }
    } while (($currentLeader -eq "" -or $currentLeader -eq $StoppedBrokerId -or $stableChecks -lt 3) -and (Get-Date) -lt $deadline)

    if ($currentLeader -eq "" -or $currentLeader -eq $StoppedBrokerId -or $stableChecks -lt 3) {
        throw "Kafka failover did not complete before timeout; stopped_broker_id=$StoppedBrokerId current_leader=$currentLeader stable_checks=$stableChecks last_error=$lastError"
    }

    Start-Sleep -Seconds 5
    return $currentLeader
}

docker compose -f deploy/local/docker-compose.kafka-ha.yml down -v | Out-Null
docker compose -f deploy/local/docker-compose.yml up -d postgres redis | Out-Null
& .\tools\local-up-kafka-ha.ps1

$summary = [ordered]@{
    run_name = $RunName
    git_commit = (git rev-parse HEAD)
    git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
    kafka_brokers = $kafkaBrokers
    topic_replication_factor = 3
    pg_dsn = $PgDsn
    postgres_exec_container = $PostgresExecContainer
}

$beforeRun = "$RunName-before"
$beforeArgs = @{
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    KafkaBrokers = $kafkaBrokers
    KafkaExecContainer = "nexusim-kafka-ha-0"
    KafkaAdminBootstrap = "kafka-ha-0:29092"
    KafkaTopicReplicationFactor = 3
    ResultRoot = $ResultRoot
    RunName = $beforeRun
}
if ($SkipBuild) {
    $beforeArgs.SkipBuild = $true
}
& .\tools\local-distributed-smoke.ps1 @beforeArgs

$summary.before_leader_broker_id = Get-DeliveryTopicLeaderBrokerId -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092"
$summary.stopped_container = $brokerMap[$summary.before_leader_broker_id].DockerContainer

docker stop $summary.stopped_container | Out-Null

$remainingBrokerIds = @($brokerMap.Keys | Where-Object { $_ -ne $summary.before_leader_broker_id } | Sort-Object)
$adminBrokerId = $remainingBrokerIds[0]
$adminExecContainer = $brokerMap[$adminBrokerId].ExecContainer
$adminBootstrapServer = "kafka-ha-" + ([int]$adminBrokerId - 1) + ":29092"
$summary.after_leader_broker_id = Wait-ForKafkaFailover -StoppedBrokerId $summary.before_leader_broker_id -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer

$afterRun = "$RunName-after"
$afterArgs = @{
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    KafkaBrokers = $kafkaBrokers
    KafkaExecContainer = $adminExecContainer
    KafkaAdminBootstrap = $adminBootstrapServer
    KafkaTopicReplicationFactor = 3
    ResultRoot = $ResultRoot
    RunName = $afterRun
    SkipBuild = $true
}
& .\tools\local-distributed-smoke.ps1 @afterArgs

docker start $summary.stopped_container | Out-Null

$summary.before_summary = Join-Path (Join-Path $ResultRoot $beforeRun) "pushgateway-summary.json"
$summary.after_summary = Join-Path (Join-Path $ResultRoot $afterRun) "pushgateway-summary.json"
$summary.completed_at = (Get-Date).ToString("o")

$summaryPath = Join-Path $resultDir "kafka-failover-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

Write-Host "kafka_failover_before_leader_broker_id=$($summary.before_leader_broker_id)"
Write-Host "kafka_failover_after_leader_broker_id=$($summary.after_leader_broker_id)"
Write-Host "kafka_failover_stopped_container=$($summary.stopped_container)"
Write-Host "kafka_failover_summary=$summaryPath"
