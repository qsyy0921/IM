param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-isr-observation-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

$kafkaBrokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094"
$deliveryTopic = "im.delivery.events"
$probeTopic = "nexusim.kafka.isr.probe." + (Get-Date -Format "yyyyMMddHHmmss")
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

function Invoke-DockerExecCapture {
    param(
        [string]$ExecContainer,
        [string[]]$Arguments
    )
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = @(& docker exec $ExecContainer @Arguments 2>&1 | ForEach-Object { $_.ToString() })
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    return [ordered]@{
        exit_code = $exitCode
        output = ($output -join "`n")
    }
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

function Get-KafkaTopicStates {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [string]$Topic
    )
    $lines = Invoke-KafkaExec -ExecContainer $ExecContainer -Arguments @(
        "kafka-topics",
        "--bootstrap-server", $BootstrapServer,
        "--describe",
        "--topic", $Topic
    )
    $states = @()
    foreach ($line in $lines) {
        if ($line -match "Partition:\s*(\d+)\s+Leader:\s*(-?\d+)\s+Replicas:\s*([0-9,\-]+)\s+Isr:\s*([0-9,\-]+)") {
            $replicas = @($matches[3].Split(",") | Where-Object { $_ -ne "" })
            $isr = @($matches[4].Split(",") | Where-Object { $_ -ne "" })
            $states += [ordered]@{
                partition = [int]$matches[1]
                leader = [int]$matches[2]
                replicas = $replicas
                isr = $isr
                replica_count = $replicas.Count
                isr_count = $isr.Count
            }
        }
    }
    if ($states.Count -eq 0) {
        throw "Could not parse topic state for $Topic."
    }
    return $states
}

function Wait-ForKafkaTopicISR {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [string]$Topic,
        [int]$ExpectedISRCount,
        [string]$StoppedBrokerId,
        [int]$TimeoutSeconds = 120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $stableChecks = 0
    $lastStates = @()
    $lastError = ""
    do {
        Start-Sleep -Seconds 2
        try {
            if (-not (Test-KafkaAdminReady -ExecContainer $ExecContainer -BootstrapServer $BootstrapServer)) {
                throw "Kafka admin API is not ready on $ExecContainer"
            }
            $lastStates = @(Get-KafkaTopicStates -ExecContainer $ExecContainer -BootstrapServer $BootstrapServer -Topic $Topic)
            $bad = @($lastStates | Where-Object {
                $_.isr_count -ne $ExpectedISRCount -or
                $_.leader -eq [int]$StoppedBrokerId -or
                ($_.isr -contains $StoppedBrokerId)
            })
            if ($bad.Count -eq 0) {
                $stableChecks++
            } else {
                $stableChecks = 0
            }
            $lastError = ""
        } catch {
            $stableChecks = 0
            $lastError = $_.Exception.Message
        }
    } while ($stableChecks -lt 3 -and (Get-Date) -lt $deadline)

    if ($stableChecks -lt 3) {
        throw "Kafka ISR did not reach expected state before timeout; topic=$Topic expected_isr=$ExpectedISRCount stopped_broker_id=$StoppedBrokerId stable_checks=$stableChecks last_error=$lastError states=$($lastStates | ConvertTo-Json -Compress)"
    }
    return $lastStates
}

function New-KafkaProbeTopic {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [string]$Topic
    )
    Invoke-KafkaExec -ExecContainer $ExecContainer -Arguments @(
        "kafka-topics",
        "--bootstrap-server", $BootstrapServer,
        "--create",
        "--if-not-exists",
        "--topic", $Topic,
        "--partitions", "1",
        "--replication-factor", "3",
        "--config", "min.insync.replicas=2"
    ) | Out-Null
}

function Test-KafkaProbeProduce {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [string]$Topic
    )
    $payload = "{`"run`":`"$RunName`",`"topic`":`"$Topic`",`"created_at`":`"$((Get-Date).ToString("o"))`"}"
    $shell = "printf '%s\n' '$payload' | kafka-console-producer --bootstrap-server $BootstrapServer --topic $Topic --producer-property acks=all --producer-property request.timeout.ms=5000 --producer-property delivery.timeout.ms=8000 --producer-property max.block.ms=5000"
    return Invoke-DockerExecCapture -ExecContainer $ExecContainer -Arguments @("bash", "-lc", $shell)
}

function New-KafkaProduceObservation {
    param([object]$ProbeResult)
    $output = [string]$ProbeResult.output
    $containsNotEnoughReplicas = $output -match "NOT_ENOUGH_REPLICAS|NotEnoughReplicas"
    return [ordered]@{
        exit_code = $ProbeResult.exit_code
        accepted = ($ProbeResult.exit_code -eq 0 -and -not $containsNotEnoughReplicas)
        contains_not_enough_replicas = $containsNotEnoughReplicas
        output = $output
    }
}

function Get-HealthyKafkaBrokerIds {
    $healthy = @()
    foreach ($id in @($brokerMap.Keys | Sort-Object)) {
        $container = $brokerMap[$id].DockerContainer
        $health = docker inspect -f "{{.State.Health.Status}}" $container 2>$null
        if ($LASTEXITCODE -eq 0 -and $health -eq "healthy") {
            $healthy += $id
        }
    }
    return $healthy
}

function Wait-ForKafkaContainersHealthy {
    param([int]$TimeoutSeconds = 180)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        Start-Sleep -Seconds 2
        $healthy = @(Get-HealthyKafkaBrokerIds)
    } while ($healthy.Count -lt 3 -and (Get-Date) -lt $deadline)
    if ($healthy.Count -lt 3) {
        throw "Kafka HA containers did not all become healthy; healthy=$($healthy -join ',')"
    }
}

docker compose -f deploy/local/docker-compose.kafka-ha.yml down -v | Out-Null
docker compose -f deploy/local/docker-compose.yml up -d postgres redis | Out-Null
& .\tools\local-up-kafka-ha.ps1

$summary = [ordered]@{
    run_name = $RunName
    git_commit = (git rev-parse HEAD)
    git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
    kafka_brokers = $kafkaBrokers
    delivery_topic = $deliveryTopic
    probe_topic = $probeTopic
    topic_replication_factor = 3
    topic_min_insync_replicas = 2
    pg_dsn = $PgDsn
    postgres_exec_container = $PostgresExecContainer
}

$stoppedContainers = @()

try {
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

    New-KafkaProbeTopic -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092" -Topic $probeTopic
    $summary.before_controller_broker_id = Get-KafkaControllerBrokerId -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092"
    $summary.delivery_topic_before = @(Get-KafkaTopicStates -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092" -Topic $deliveryTopic)
    $summary.probe_topic_before = @(Get-KafkaTopicStates -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092" -Topic $probeTopic)
    $summary.probe_produce_before = New-KafkaProduceObservation -ProbeResult (Test-KafkaProbeProduce -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092" -Topic $probeTopic)

    $firstStopBrokerId = @($brokerMap.Keys | Where-Object { $_ -ne $summary.before_controller_broker_id } | Sort-Object)[0]
    $summary.first_stopped_broker_id = $firstStopBrokerId
    $summary.first_stopped_container = $brokerMap[$firstStopBrokerId].DockerContainer
    docker stop $summary.first_stopped_container | Out-Null
    $stoppedContainers += $summary.first_stopped_container

    $adminBrokerId = @($brokerMap.Keys | Where-Object { $_ -ne $firstStopBrokerId } | Sort-Object)[0]
    $adminExecContainer = $brokerMap[$adminBrokerId].ExecContainer
    $adminBootstrapServer = $brokerMap[$adminBrokerId].BootstrapServer
    $summary.delivery_topic_after_one_broker_stop = @(Wait-ForKafkaTopicISR -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $deliveryTopic -ExpectedISRCount 2 -StoppedBrokerId $firstStopBrokerId)
    $summary.probe_topic_after_one_broker_stop = @(Wait-ForKafkaTopicISR -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $probeTopic -ExpectedISRCount 2 -StoppedBrokerId $firstStopBrokerId)
    $summary.probe_produce_after_one_broker_stop = New-KafkaProduceObservation -ProbeResult (Test-KafkaProbeProduce -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $probeTopic)

    $oneBrokerDownRun = "$RunName-one-broker-down"
    $oneBrokerDownArgs = @{
        PgDsn = $PgDsn
        PostgresExecContainer = $PostgresExecContainer
        KafkaBrokers = $kafkaBrokers
        KafkaExecContainer = $adminExecContainer
        KafkaAdminBootstrap = $adminBootstrapServer
        KafkaTopicReplicationFactor = 3
        ResultRoot = $ResultRoot
        RunName = $oneBrokerDownRun
        SkipBuild = $true
    }
    & .\tools\local-distributed-smoke.ps1 @oneBrokerDownArgs

    $secondStopBrokerId = @($brokerMap.Keys | Where-Object { $_ -ne $firstStopBrokerId -and $_ -ne $adminBrokerId } | Sort-Object)[0]
    $summary.second_stopped_broker_id = $secondStopBrokerId
    $summary.second_stopped_container = $brokerMap[$secondStopBrokerId].DockerContainer
    docker stop $summary.second_stopped_container | Out-Null
    $stoppedContainers += $summary.second_stopped_container
    Start-Sleep -Seconds 10

    $remainingBrokerId = @($brokerMap.Keys | Where-Object { $_ -ne $firstStopBrokerId -and $_ -ne $secondStopBrokerId } | Sort-Object)[0]
    $remainingExecContainer = $brokerMap[$remainingBrokerId].ExecContainer
    $remainingBootstrapServer = $brokerMap[$remainingBrokerId].BootstrapServer
    $summary.remaining_broker_id_after_two_stops = $remainingBrokerId
    $summary.admin_ready_after_two_broker_stops = Test-KafkaAdminReady -ExecContainer $remainingExecContainer -BootstrapServer $remainingBootstrapServer
    try {
        $summary.probe_topic_after_two_broker_stops = @(Get-KafkaTopicStates -ExecContainer $remainingExecContainer -BootstrapServer $remainingBootstrapServer -Topic $probeTopic)
    } catch {
        $summary.probe_topic_after_two_broker_stops_error = $_.Exception.Message
    }
    $summary.probe_produce_after_two_broker_stops = New-KafkaProduceObservation -ProbeResult (Test-KafkaProbeProduce -ExecContainer $remainingExecContainer -BootstrapServer $remainingBootstrapServer -Topic $probeTopic)

    foreach ($container in @($stoppedContainers | Select-Object -Unique)) {
        docker start $container | Out-Null
    }
    $stoppedContainers = @()
    Wait-ForKafkaContainersHealthy

    $restoreRun = "$RunName-after-restore"
    $restoreArgs = @{
        PgDsn = $PgDsn
        PostgresExecContainer = $PostgresExecContainer
        KafkaBrokers = $kafkaBrokers
        KafkaExecContainer = "nexusim-kafka-ha-0"
        KafkaAdminBootstrap = "kafka-ha-0:29092"
        KafkaTopicReplicationFactor = 3
        ResultRoot = $ResultRoot
        RunName = $restoreRun
        SkipBuild = $true
    }
    & .\tools\local-distributed-smoke.ps1 @restoreArgs

    $summary.before_summary = Join-Path (Join-Path $ResultRoot $beforeRun) "pushgateway-summary.json"
    $summary.one_broker_down_summary = Join-Path (Join-Path $ResultRoot $oneBrokerDownRun) "pushgateway-summary.json"
    $summary.after_restore_summary = Join-Path (Join-Path $ResultRoot $restoreRun) "pushgateway-summary.json"
    $summary.completed_at = (Get-Date).ToString("o")
} finally {
    foreach ($container in @($stoppedContainers | Select-Object -Unique)) {
        docker start $container *> $null
    }
}

$summaryPath = Join-Path $resultDir "kafka-isr-observation-summary.json"
$summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

Write-Host "kafka_isr_first_stopped_broker_id=$($summary.first_stopped_broker_id)"
Write-Host "kafka_isr_second_stopped_broker_id=$($summary.second_stopped_broker_id)"
Write-Host "kafka_isr_admin_ready_after_two_broker_stops=$($summary.admin_ready_after_two_broker_stops)"
Write-Host "kafka_isr_probe_after_two_broker_stops_accepted=$($summary.probe_produce_after_two_broker_stops.accepted)"
Write-Host "kafka_isr_probe_after_two_broker_stops_not_enough_replicas=$($summary.probe_produce_after_two_broker_stops.contains_not_enough_replicas)"
Write-Host "kafka_isr_observation_summary=$summaryPath"
