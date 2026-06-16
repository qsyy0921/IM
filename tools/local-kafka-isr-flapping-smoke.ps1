param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [int]$FlapCycles = 3,
    [int]$StableChecks = 3
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-isr-flapping-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ($FlapCycles -lt 1) {
    throw "FlapCycles must be >= 1."
}
if ($StableChecks -lt 1) {
    throw "StableChecks must be >= 1."
}

$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force -Path $resultDir | Out-Null

$probeTopic = "nexusim.kafka.isr.flap." + (Get-Date -Format "yyyyMMddHHmmss")
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

    $output = @(& docker exec $ExecContainer @Arguments 2>&1 | ForEach-Object { $_.ToString() })
    if ($LASTEXITCODE -ne 0) {
        throw "Kafka command failed in ${ExecContainer}: $($Arguments -join ' ')`n$($output -join "`n")"
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

function Wait-ForTopicISR {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [string]$Topic,
        [int]$ExpectedISRCount,
        [string]$BrokerId,
        [string]$BrokerExpectation,
        [int]$TimeoutSeconds = 180
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $stable = 0
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
                $brokerInISR = @($_.isr) -contains $BrokerId
                $brokerCheckFailed = (
                    ($BrokerExpectation -eq "absent" -and $brokerInISR) -or
                    ($BrokerExpectation -eq "present" -and -not $brokerInISR)
                )
                $_.isr_count -ne $ExpectedISRCount -or $_.leader -lt 0 -or $brokerCheckFailed
            })
            if ($bad.Count -eq 0) {
                $stable++
            } else {
                $stable = 0
            }
            $lastError = ""
        } catch {
            $stable = 0
            $lastError = $_.Exception.Message
        }
    } while ($stable -lt $StableChecks -and (Get-Date) -lt $deadline)

    if ($stable -lt $StableChecks) {
        throw "Kafka topic $Topic did not reach ISR=$ExpectedISRCount broker_expectation=$BrokerExpectation broker=$BrokerId stable_checks=$stable last_error=$lastError states=$($lastStates | ConvertTo-Json -Compress)"
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
        "--partitions", "3",
        "--replication-factor", "3",
        "--config", "min.insync.replicas=2"
    ) | Out-Null
}

function Test-KafkaProbeProduce {
    param(
        [string]$ExecContainer,
        [string]$BootstrapServer,
        [string]$Topic,
        [int]$Cycle,
        [string]$Phase
    )

    $payload = "{`"run`":`"$RunName`",`"cycle`":$Cycle,`"phase`":`"$Phase`",`"created_at`":`"$((Get-Date).ToString("o"))`"}"
    $shell = "printf '%s\n' '$payload' | kafka-console-producer --bootstrap-server $BootstrapServer --topic $Topic --producer-property acks=all --producer-property request.timeout.ms=5000 --producer-property delivery.timeout.ms=8000 --producer-property max.block.ms=5000"
    $probe = Invoke-DockerExecCapture -ExecContainer $ExecContainer -Arguments @("bash", "-lc", $shell)
    $output = [string]$probe.output
    return [ordered]@{
        exit_code = $probe.exit_code
        accepted = ($probe.exit_code -eq 0 -and -not ($output -match "NOT_ENOUGH_REPLICAS|NotEnoughReplicas"))
        contains_not_enough_replicas = ($output -match "NOT_ENOUGH_REPLICAS|NotEnoughReplicas")
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

Push-Location (Split-Path -Parent $PSScriptRoot)
$stoppedContainers = @()
try {
    & .\tools\local-up-kafka-ha.ps1

    New-KafkaProbeTopic -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092" -Topic $probeTopic
    $controllerBrokerID = Get-KafkaControllerBrokerId -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092"
    $flapBrokerID = @($brokerMap.Keys | Where-Object { $_ -ne $controllerBrokerID } | Sort-Object)[0]
    $adminBrokerID = @($brokerMap.Keys | Where-Object { $_ -ne $flapBrokerID } | Sort-Object)[0]
    $adminExecContainer = $brokerMap[$adminBrokerID].ExecContainer
    $adminBootstrapServer = $brokerMap[$adminBrokerID].BootstrapServer
    $flapContainer = $brokerMap[$flapBrokerID].DockerContainer

    $cycles = @()
    for ($cycle = 1; $cycle -le $FlapCycles; $cycle++) {
        docker stop $flapContainer | Out-Null
        $stoppedContainers += $flapContainer
        $degradedStates = @(Wait-ForTopicISR -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $probeTopic -ExpectedISRCount 2 -BrokerId $flapBrokerID -BrokerExpectation "absent")
        $degradedProbe = Test-KafkaProbeProduce -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $probeTopic -Cycle $cycle -Phase "degraded"

        docker start $flapContainer | Out-Null
        $stoppedContainers = @($stoppedContainers | Where-Object { $_ -ne $flapContainer })
        Wait-ForKafkaContainersHealthy
        $restoredStates = @(Wait-ForTopicISR -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $probeTopic -ExpectedISRCount 3 -BrokerId $flapBrokerID -BrokerExpectation "present")
        $restoredProbe = Test-KafkaProbeProduce -ExecContainer $adminExecContainer -BootstrapServer $adminBootstrapServer -Topic $probeTopic -Cycle $cycle -Phase "restored"

        $cycles += [ordered]@{
            cycle = $cycle
            stopped_broker_id = $flapBrokerID
            stopped_container = $flapContainer
            admin_broker_id = $adminBrokerID
            degraded_topic_state = $degradedStates
            degraded_probe = $degradedProbe
            restored_topic_state = $restoredStates
            restored_probe = $restoredProbe
        }
    }

    $summary = [ordered]@{
        run_name = $RunName
        git_commit = (git rev-parse HEAD)
        git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
        completed_at = (Get-Date).ToString("o")
        kafka_brokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094"
        probe_topic = $probeTopic
        topic_replication_factor = 3
        topic_min_insync_replicas = 2
        controller_broker_id = $controllerBrokerID
        flapped_broker_id = $flapBrokerID
        flap_cycles = $FlapCycles
        stable_checks = $StableChecks
        cycles = $cycles
    }
    $summaryPath = Join-Path $resultDir "kafka-isr-flapping-summary.json"
    $summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

    Write-Host "kafka_isr_flapping_summary=$summaryPath"
    Write-Host "kafka_isr_flapping_cycles=$FlapCycles"
    Write-Host "kafka_isr_flapped_broker_id=$flapBrokerID"
}
finally {
    foreach ($container in @($stoppedContainers | Select-Object -Unique)) {
        docker start $container *> $null
    }
    Pop-Location
}
