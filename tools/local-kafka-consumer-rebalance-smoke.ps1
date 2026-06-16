param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KafkaBrokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094",
    [string]$KafkaExecContainer = "nexusim-kafka-ha-0",
    [string]$KafkaAdminBootstrap = "kafka-ha-0:29092",
    [int]$KafkaTopicReplicationFactor = 3,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-consumer-rebalance-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

$topic = "im.delivery.events"
$consumerGroup = "nexusim-push-rebalance-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")

function Invoke-KafkaExec {
    param([string[]]$Arguments)

    $output = @(& docker exec $KafkaExecContainer @Arguments 2>&1 | ForEach-Object { $_.ToString() })
    if ($LASTEXITCODE -ne 0) {
        throw "Kafka command failed: $($Arguments -join ' ')`n$($output -join "`n")"
    }
    return $output
}

function Ensure-KafkaTopic {
    Invoke-KafkaExec -Arguments @(
        "kafka-topics",
        "--bootstrap-server", $KafkaAdminBootstrap,
        "--create",
        "--if-not-exists",
        "--topic", $topic,
        "--partitions", "3",
        "--replication-factor", ([string]$KafkaTopicReplicationFactor)
    ) | Out-Null
}

function Get-ConsumerGroupSnapshot {
    $stateLines = @(Invoke-KafkaExec -Arguments @(
        "kafka-consumer-groups",
        "--bootstrap-server", $KafkaAdminBootstrap,
        "--group", $consumerGroup,
        "--describe",
        "--state"
    ))
    $describeLines = @(Invoke-KafkaExec -Arguments @(
        "kafka-consumer-groups",
        "--bootstrap-server", $KafkaAdminBootstrap,
        "--group", $consumerGroup,
        "--describe"
    ))

    $state = ""
    $memberCount = 0
    $escapedGroup = [regex]::Escape($consumerGroup)
    foreach ($line in $stateLines) {
        $trimmed = $line.Trim()
        if ($trimmed.Length -eq 0 -or $trimmed.StartsWith("GROUP")) {
            continue
        }
        if ($trimmed -match "^$escapedGroup\s+.+\s+(?<state>Stable|PreparingRebalance|CompletingRebalance|Empty|Dead|Unknown)\s+(?<members>\d+)\s*$") {
            $state = $Matches["state"]
            $memberCount = [int]$Matches["members"]
            break
        }
    }

    $assignments = @()
    $consumerIDs = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    $escapedTopic = [regex]::Escape($topic)
    foreach ($line in $describeLines) {
        $trimmed = $line.Trim()
        if ($trimmed.Length -eq 0 -or $trimmed.StartsWith("GROUP")) {
            continue
        }
        $match = [regex]::Match($trimmed, "^$escapedGroup\s+$escapedTopic\s+(?<partition>\d+)\s+(?<current>\S+)\s+(?<end>\S+)\s+(?<lag>\S+)\s+(?<consumer>.+?)\s+(?<host>/\S+)\s+(?<client>.+)$")
        if (-not $match.Success) {
            continue
        }
        $consumerID = $match.Groups["consumer"].Value.Trim()
        if ($consumerID -eq "-" -or [string]::IsNullOrWhiteSpace($consumerID)) {
            continue
        }
        [void]$consumerIDs.Add($consumerID)
        $assignments += [ordered]@{
            partition = [int]$match.Groups["partition"].Value
            current_offset = $match.Groups["current"].Value
            log_end_offset = $match.Groups["end"].Value
            lag = $match.Groups["lag"].Value
            consumer_id = $consumerID
            host = $match.Groups["host"].Value
            client_id = $match.Groups["client"].Value.Trim()
        }
    }

    return [ordered]@{
        state = $state
        member_count = $memberCount
        consumer_ids = @($consumerIDs | Sort-Object)
        assigned_partition_count = $assignments.Count
        assignments = $assignments
        state_output = ($stateLines -join "`n")
        describe_output = ($describeLines -join "`n")
    }
}

function Wait-ConsumerGroupStable {
    param(
        [int]$ExpectedMembers,
        [int]$TimeoutSeconds = 90
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $stableChecks = 0
    $last = $null
    do {
        Start-Sleep -Seconds 2
        try {
            $last = Get-ConsumerGroupSnapshot
            if ($last.state -eq "Stable" -and [int]$last.member_count -eq $ExpectedMembers -and @($last.consumer_ids).Count -eq $ExpectedMembers) {
                $stableChecks++
            } else {
                $stableChecks = 0
            }
        } catch {
            $stableChecks = 0
            $last = [ordered]@{ state = "error"; member_count = 0; error = $_.Exception.Message }
        }
    } while ($stableChecks -lt 2 -and (Get-Date) -lt $deadline)

    if ($stableChecks -lt 2) {
        throw "Consumer group did not become stable with expected members=$ExpectedMembers; last=$($last | ConvertTo-Json -Depth 6 -Compress)"
    }
    return $last
}

function Start-PushConsumer {
    param(
        [string]$Name,
        [string]$DebugAddr
    )

    $env:NEXUSIM_PUSH_GATEWAY_MODE = "delivery-consumer"
    $env:NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
    $env:NEXUSIM_DELIVERY_EVENTS_TOPIC = $topic
    $env:NEXUSIM_PUSH_CONSUMER_GROUP = $consumerGroup
    $env:NEXUSIM_PUSH_ROUTE_BACKEND = "memory"
    $env:NEXUSIM_PUSH_DEBUG_ADDR = $DebugAddr
    $env:NEXUSIM_PUSH_DELIVERY_CONSUMER_ERROR_BACKOFF = "200ms"

    $pushGateway = Join-Path $repoRoot "bin\push-gateway.exe"
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    return Start-Process -FilePath $pushGateway `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
}

Push-Location $repoRoot
$processes = @()
try {
    & .\tools\local-up-kafka-ha.ps1

    if (-not $SkipBuild) {
        go build -o bin\push-gateway.exe ./services/push-gateway/cmd/push-gateway
        if ($LASTEXITCODE -ne 0) {
            throw "go build push-gateway failed"
        }
    }

    Ensure-KafkaTopic

    $processes += Start-PushConsumer -Name "push-consumer-a" -DebugAddr "127.0.0.1:11710"
    $processes += Start-PushConsumer -Name "push-consumer-b" -DebugAddr "127.0.0.1:11711"

    $beforeStop = Wait-ConsumerGroupStable -ExpectedMembers 2

    $stoppedProcessID = $processes[0].Id
    Stop-Process -Id $stoppedProcessID -Force
    $processes = @($processes | Where-Object { $_.Id -ne $stoppedProcessID })

    $afterStop = Wait-ConsumerGroupStable -ExpectedMembers 1

    $summary = [ordered]@{
        run_name = $RunName
        git_commit = (git rev-parse HEAD)
        git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
        completed_at = (Get-Date).ToString("o")
        kafka_brokers = $KafkaBrokers
        kafka_admin_bootstrap = $KafkaAdminBootstrap
        topic = $topic
        topic_replication_factor = $KafkaTopicReplicationFactor
        consumer_group = $consumerGroup
        before_stop = $beforeStop
        after_stop = $afterStop
        log_dir = $logDir
    }
    $summaryPath = Join-Path $resultDir "kafka-consumer-rebalance-summary.json"
    $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
    Write-Host "kafka_consumer_rebalance_summary=$summaryPath"
    Write-Host "before_members=$($beforeStop.member_count)"
    Write-Host "after_members=$($afterStop.member_count)"
}
finally {
    foreach ($proc in $processes) {
        try {
            if ($proc -and -not $proc.HasExited) {
                Stop-Process -Id $proc.Id -Force
            }
        } catch {
        }
    }
    Pop-Location
}
