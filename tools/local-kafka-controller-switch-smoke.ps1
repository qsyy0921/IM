param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-controller-switch-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

$kafkaBrokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094"
$brokerMap = @{
    "1" = @{
        ExecContainer = "nexusim-kafka-ha-0"
        DockerContainer = "nexusim-kafka-ha-0"
        BootstrapServer = "kafka-ha-0:29092"
    }
    "2" = @{
        ExecContainer = "nexusim-kafka-ha-1"
        DockerContainer = "nexusim-kafka-ha-1"
        BootstrapServer = "kafka-ha-1:29092"
    }
    "3" = @{
        ExecContainer = "nexusim-kafka-ha-2"
        DockerContainer = "nexusim-kafka-ha-2"
        BootstrapServer = "kafka-ha-2:29092"
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

function Get-KafkaControllerBrokerId {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer
    )
    $lines = Invoke-KafkaExec -ExecContainer $ExecContainer -Arguments @(
        "kafka-metadata-quorum",
        "--bootstrap-server", $BootstrapServer,
        "describe",
        "--status"
    )
    foreach ($line in $lines) {
        if ($line -match "LeaderId:\s*(\d+)") {
            return $matches[1]
        }
    }
    throw "Could not determine Kafka KRaft controller leader."
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

function Wait-ForKafkaControllerSwitch {
    param(
        [string]$StoppedBrokerId,
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [int]$TimeoutSeconds = 120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $currentController = ""
    $stableChecks = 0
    $lastError = ""
    do {
        Start-Sleep -Seconds 2
        try {
            if (-not (Test-KafkaAdminReady -ExecContainer $ExecContainer -BootstrapServer $BootstrapServer)) {
                throw "Kafka admin API is not ready on $ExecContainer"
            }
            $currentController = Get-KafkaControllerBrokerId -ExecContainer $ExecContainer -BootstrapServer $BootstrapServer
            if ($currentController -ne $StoppedBrokerId) {
                $stableChecks++
            } else {
                $stableChecks = 0
            }
            $lastError = ""
        } catch {
            $currentController = ""
            $stableChecks = 0
            $lastError = $_.Exception.Message
        }
    } while (($currentController -eq "" -or $currentController -eq $StoppedBrokerId -or $stableChecks -lt 3) -and (Get-Date) -lt $deadline)

    if ($currentController -eq "" -or $currentController -eq $StoppedBrokerId -or $stableChecks -lt 3) {
        throw "Kafka controller switch did not complete before timeout; stopped_broker_id=$StoppedBrokerId current_controller=$currentController stable_checks=$stableChecks last_error=$lastError"
    }

    Start-Sleep -Seconds 5
    return $currentController
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

$summary.before_controller_broker_id = Get-KafkaControllerBrokerId -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092"
$summary.stopped_container = $brokerMap[$summary.before_controller_broker_id].DockerContainer

docker stop $summary.stopped_container | Out-Null

$remainingBrokerIds = @($brokerMap.Keys | Where-Object { $_ -ne $summary.before_controller_broker_id } | Sort-Object)
$adminBrokerId = $remainingBrokerIds[0]
$adminExecContainer = $brokerMap[$adminBrokerId].ExecContainer
$adminBootstrapServer = $brokerMap[$adminBrokerId].BootstrapServer
$summary.after_controller_broker_id = Wait-ForKafkaControllerSwitch -StoppedBrokerId $summary.before_controller_broker_id -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer

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

$summaryPath = Join-Path $resultDir "kafka-controller-switch-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

Write-Host "kafka_controller_before_broker_id=$($summary.before_controller_broker_id)"
Write-Host "kafka_controller_after_broker_id=$($summary.after_controller_broker_id)"
Write-Host "kafka_controller_stopped_container=$($summary.stopped_container)"
Write-Host "kafka_controller_switch_summary=$summaryPath"
