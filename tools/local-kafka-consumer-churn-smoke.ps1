param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KafkaBrokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094",
    [string]$KafkaExecContainer = "nexusim-kafka-ha-0",
    [string]$KafkaAdminBootstrap = "kafka-ha-0:29092",
    [int]$KafkaTopicReplicationFactor = 3,
    [int]$ChurnCycles = 3,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-consumer-churn-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ($ChurnCycles -lt 1) {
    throw "ChurnCycles must be >= 1."
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null

$topic = "im.delivery.events"
$consumerGroup = "nexusim-push-churn-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
$nextDebugPort = 11820

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
            if ($last.state -eq "Stable" -and [int]$last.member_count -eq $ExpectedMembers -and @($last.consumer_ids).Count -eq $ExpectedMembers -and [int]$last.assigned_partition_count -eq 3) {
                $stableChecks++
            } else {
                $stableChecks = 0
            }
        } catch {
            $stableChecks = 0
            $last = [ordered]@{ state = "error"; member_count = 0; assigned_partition_count = 0; error = $_.Exception.Message }
        }
    } while ($stableChecks -lt 2 -and (Get-Date) -lt $deadline)

    if ($stableChecks -lt 2) {
        throw "Consumer group did not become stable with expected members=$ExpectedMembers; last=$($last | ConvertTo-Json -Depth 6 -Compress)"
    }
    return $last
}

function Start-PushConsumer {
    param([string]$Name)

    $script:nextDebugPort++
    $debugAddr = "127.0.0.1:$script:nextDebugPort"
    $env:NEXUSIM_PUSH_GATEWAY_MODE = "delivery-consumer"
    $env:NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
    $env:NEXUSIM_DELIVERY_EVENTS_TOPIC = $topic
    $env:NEXUSIM_PUSH_CONSUMER_GROUP = $consumerGroup
    $env:NEXUSIM_PUSH_ROUTE_BACKEND = "memory"
    $env:NEXUSIM_PUSH_DEBUG_ADDR = $debugAddr
    $env:NEXUSIM_PUSH_DELIVERY_CONSUMER_ERROR_BACKOFF = "200ms"

    $pushGateway = Join-Path $repoRoot "bin\push-gateway.exe"
    $stamp = Get-Date -Format "HHmmssfff"
    $out = Join-Path $logDir "$Name-$stamp.out.log"
    $err = Join-Path $logDir "$Name-$stamp.err.log"
    return Start-Process -FilePath $pushGateway `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
}

function Stop-PushConsumer {
    param([System.Diagnostics.Process]$Process)

    if ($Process -and -not $Process.HasExited) {
        Stop-Process -Id $Process.Id -Force
    }
}

Push-Location $repoRoot
$consumerA = $null
$consumerB = $null
try {
    & .\tools\local-up-kafka-ha.ps1

    if (-not $SkipBuild) {
        go build -o bin\push-gateway.exe ./services/push-gateway/cmd/push-gateway
        if ($LASTEXITCODE -ne 0) {
            throw "go build push-gateway failed"
        }
    }

    Ensure-KafkaTopic

    $consumerA = Start-PushConsumer -Name "push-consumer-a"
    $consumerB = Start-PushConsumer -Name "push-consumer-b"

    $initial = Wait-ConsumerGroupStable -ExpectedMembers 2
    $transitions = @()

    for ($cycle = 1; $cycle -le $ChurnCycles; $cycle++) {
        Stop-PushConsumer -Process $consumerA
        $afterStopA = Wait-ConsumerGroupStable -ExpectedMembers 1
        $transitions += [ordered]@{
            cycle = $cycle
            action = "stop_a"
            expected_members = 1
            snapshot = $afterStopA
        }

        $consumerA = Start-PushConsumer -Name "push-consumer-a"
        $afterStartA = Wait-ConsumerGroupStable -ExpectedMembers 2
        $transitions += [ordered]@{
            cycle = $cycle
            action = "start_a"
            expected_members = 2
            snapshot = $afterStartA
        }

        Stop-PushConsumer -Process $consumerB
        $afterStopB = Wait-ConsumerGroupStable -ExpectedMembers 1
        $transitions += [ordered]@{
            cycle = $cycle
            action = "stop_b"
            expected_members = 1
            snapshot = $afterStopB
        }

        $consumerB = Start-PushConsumer -Name "push-consumer-b"
        $afterStartB = Wait-ConsumerGroupStable -ExpectedMembers 2
        $transitions += [ordered]@{
            cycle = $cycle
            action = "start_b"
            expected_members = 2
            snapshot = $afterStartB
        }
    }

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
        churn_cycles = $ChurnCycles
        initial = $initial
        transitions = $transitions
        log_dir = $logDir
    }
    $summaryPath = Join-Path $resultDir "kafka-consumer-churn-summary.json"
    $summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
    Write-Host "kafka_consumer_churn_summary=$summaryPath"
    Write-Host "churn_cycles=$ChurnCycles"
    Write-Host "transition_count=$($transitions.Count)"
}
finally {
    Stop-PushConsumer -Process $consumerA
    Stop-PushConsumer -Process $consumerB
    Pop-Location
}
