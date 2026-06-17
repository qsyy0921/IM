param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [int]$MessageCount = 120,
    [int]$StopAfterSeconds = 2,
    [int]$BrokerDownSeconds = 6,
    [int]$ConsumeTimeoutSeconds = 45
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "kafka-producer-fault-observation-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ($MessageCount -lt 1) {
    throw "MessageCount must be >= 1."
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force -Path $resultDir | Out-Null

$brokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094"
$topic = "nexusim.kafka.producer.fault." + (Get-Date -Format "yyyyMMddHHmmss")
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

function Read-JsonFile {
    param([string]$Path)
    return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

Push-Location $repoRoot
$stoppedContainers = @()
try {
    & .\tools\local-up-kafka-ha.ps1
    New-KafkaProbeTopic -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092" -Topic $topic

    $controllerBrokerID = Get-KafkaControllerBrokerId -ExecContainer "nexusim-kafka-ha-0" -BootstrapServer "kafka-ha-0:29092"
    $stopBrokerID = @($brokerMap.Keys | Where-Object { $_ -ne $controllerBrokerID } | Sort-Object)[0]
    $stopContainer = $brokerMap[$stopBrokerID].DockerContainer

    $producerSummaryPath = Join-Path $resultDir "producer-summary.json"
    $producerOut = Join-Path $resultDir "producer.out.log"
    $producerErr = Join-Path $resultDir "producer.err.log"
    $producer = Start-Process -FilePath "go" `
        -ArgumentList @(
            "run", ".\tools\kafka-producer-fault-probe",
            "-mode", "produce",
            "-brokers", $brokers,
            "-topic", $topic,
            "-run", $RunName,
            "-count", ([string]$MessageCount),
            "-interval", "50ms",
            "-output", $producerSummaryPath
        ) `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $producerOut `
        -RedirectStandardError $producerErr

    Start-Sleep -Seconds $StopAfterSeconds
    docker stop $stopContainer | Out-Null
    $stoppedContainers += $stopContainer
    Start-Sleep -Seconds $BrokerDownSeconds
    docker start $stopContainer | Out-Null
    $stoppedContainers = @($stoppedContainers | Where-Object { $_ -ne $stopContainer })
    Wait-ForKafkaContainersHealthy

    $producer.WaitForExit()
    $producer.Refresh()
    $producerExitCode = $producer.ExitCode
    if ($null -eq $producerExitCode -and (Test-Path -LiteralPath $producerSummaryPath -PathType Leaf)) {
        $producerExitCode = 0
    }
    if ($producerExitCode -ne 0) {
        $stderr = if (Test-Path -LiteralPath $producerErr) { Get-Content -LiteralPath $producerErr -Raw } else { "" }
        throw "Kafka producer fault probe failed with exit code ${producerExitCode}: $stderr"
    }

    $consumerSummaryPath = Join-Path $resultDir "consumer-summary.json"
    & go run .\tools\kafka-producer-fault-probe `
        -mode consume `
        -brokers $brokers `
        -topic $topic `
        -run $RunName `
        -timeout "$($ConsumeTimeoutSeconds)s" `
        -idle-timeout "3s" `
        -output $consumerSummaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "Kafka producer fault consumer probe failed."
    }

    $producerSummary = Read-JsonFile -Path $producerSummaryPath
    $consumerSummary = Read-JsonFile -Path $consumerSummaryPath
    $ackedIDs = @{}
    foreach ($attempt in @($producerSummary.attempts)) {
        if ([bool]$attempt.acked) {
            $ackedIDs[[string]$attempt.id] = $true
        }
    }
    $missingAcked = @()
    foreach ($id in @($ackedIDs.Keys | Sort-Object)) {
        if ($null -eq $consumerSummary.occurrence_by_id.$id) {
            $missingAcked += $id
        }
    }
    $unackedObserved = @()
    foreach ($property in $consumerSummary.occurrence_by_id.PSObject.Properties) {
        if (-not $ackedIDs.ContainsKey($property.Name)) {
            $unackedObserved += $property.Name
        }
    }

    $summary = [ordered]@{
        run_name = $RunName
        git_commit = (git rev-parse HEAD)
        git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
        completed_at = (Get-Date).ToString("o")
        scope = "local kafka-go producer in-flight broker-fault observation; not an exactly-once proof"
        kafka_brokers = $brokers
        topic = $topic
        topic_replication_factor = 3
        topic_min_insync_replicas = 2
        stopped_broker_id = $stopBrokerID
        stopped_container = $stopContainer
        message_count = $MessageCount
        stop_after_seconds = $StopAfterSeconds
        broker_down_seconds = $BrokerDownSeconds
        producer_summary_path = $producerSummaryPath
        consumer_summary_path = $consumerSummaryPath
        producer_attempted = [int]$producerSummary.attempted
        producer_acked = [int]$producerSummary.acked
        producer_failed = [int]$producerSummary.failed
        consumed_total = [int]$consumerSummary.observed_total
        consumed_unique = [int]$consumerSummary.observed_unique
        duplicate_count = [int]$consumerSummary.duplicate_count
        missing_acked_count = $missingAcked.Count
        missing_acked_ids = $missingAcked
        unacked_observed_count = $unackedObserved.Count
        unacked_observed_ids = $unackedObserved
    }
    $summaryPath = Join-Path $resultDir "kafka-producer-fault-observation-summary.json"
    $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

    Write-Host "kafka_producer_fault_summary=$summaryPath"
    Write-Host "producer_acked=$($summary.producer_acked)"
    Write-Host "producer_failed=$($summary.producer_failed)"
    Write-Host "consumed_unique=$($summary.consumed_unique)"
    Write-Host "duplicate_count=$($summary.duplicate_count)"
    Write-Host "missing_acked_count=$($summary.missing_acked_count)"
    Write-Host "unacked_observed_count=$($summary.unacked_observed_count)"
}
finally {
    foreach ($container in @($stoppedContainers | Select-Object -Unique)) {
        docker start $container *> $null
    }
    Pop-Location
}
