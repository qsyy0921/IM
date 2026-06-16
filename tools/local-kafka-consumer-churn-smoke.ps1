param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KafkaBrokers = "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094",
    [string]$KafkaExecContainer = "nexusim-kafka-ha-0",
    [string]$KafkaAdminBootstrap = "kafka-ha-0:29092",
    [int]$KafkaTopicReplicationFactor = 3,
    [int]$ChurnCycles = 3,
    [int]$ProbeMessagesPerTransition = 0,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "kafka-consumer-churn-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ($ChurnCycles -lt 1) {
    throw "ChurnCycles must be >= 1."
}
if ($ProbeMessagesPerTransition -lt 0) {
    throw "ProbeMessagesPerTransition must be >= 0."
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
    $totalLag = [int64]0
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
        $lagText = $match.Groups["lag"].Value
        $lagValue = [int64]0
        if ([int64]::TryParse($lagText, [ref]$lagValue)) {
            $totalLag += $lagValue
        }
        [void]$consumerIDs.Add($consumerID)
        $assignments += [ordered]@{
            partition = [int]$match.Groups["partition"].Value
            current_offset = $match.Groups["current"].Value
            log_end_offset = $match.Groups["end"].Value
            lag = $lagText
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
        total_lag = $totalLag
        assignments = $assignments
        state_output = ($stateLines -join "`n")
        describe_output = ($describeLines -join "`n")
    }
}

function Wait-ConsumerGroupStable {
    param(
        [int]$ExpectedMembers,
        [switch]$RequireZeroLag,
        [int]$TimeoutSeconds = 90
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $stableChecks = 0
    $last = $null
    do {
        Start-Sleep -Seconds 2
        try {
            $last = Get-ConsumerGroupSnapshot
            $lagOK = (-not $RequireZeroLag) -or ([int64]$last.total_lag -eq 0)
            if ($last.state -eq "Stable" -and [int]$last.member_count -eq $ExpectedMembers -and @($last.consumer_ids).Count -eq $ExpectedMembers -and [int]$last.assigned_partition_count -eq 3 -and $lagOK) {
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
        throw "Consumer group did not become stable with expected members=$ExpectedMembers zero_lag=$RequireZeroLag; last=$($last | ConvertTo-Json -Depth 6 -Compress)"
    }
    return $last
}

function Invoke-ProbeProduce {
    param(
        [string]$BatchRunName,
        [int]$Count
    )

    $probeTool = Join-Path $repoRoot "bin\kafka-delivery-event-probe.exe"
    $outputPath = Join-Path $logDir "$BatchRunName-probe-summary.json"
    & $probeTool `
        -brokers $KafkaBrokers `
        -topic $topic `
        -run $BatchRunName `
        -count $Count `
        -output $outputPath
    if ($LASTEXITCODE -ne 0) {
        throw "kafka-delivery-event-probe failed for batch $BatchRunName"
    }
    return Get-Content -LiteralPath $outputPath -Raw | ConvertFrom-Json
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
        go build -o bin\kafka-delivery-event-probe.exe ./tools/kafka-delivery-event-probe
        if ($LASTEXITCODE -ne 0) {
            throw "go build kafka-delivery-event-probe failed"
        }
    } elseif ($ProbeMessagesPerTransition -gt 0 -and -not (Test-Path -LiteralPath (Join-Path $repoRoot "bin\kafka-delivery-event-probe.exe") -PathType Leaf)) {
        throw "ProbeMessagesPerTransition requires bin\kafka-delivery-event-probe.exe when SkipBuild is set."
    }

    Ensure-KafkaTopic

    $consumerA = Start-PushConsumer -Name "push-consumer-a"
    $consumerB = Start-PushConsumer -Name "push-consumer-b"

    $initial = Wait-ConsumerGroupStable -ExpectedMembers 2
    $transitions = @()
    $probeBatches = @()

    function Complete-Transition {
        param(
            [int]$Cycle,
            [string]$Action,
            [int]$ExpectedMembers,
            [object]$Snapshot
        )

        $probe = $null
        $postProbeSnapshot = $null
        if ($ProbeMessagesPerTransition -gt 0) {
            $batchName = "$RunName-cycle$Cycle-$Action"
            $probe = Invoke-ProbeProduce -BatchRunName $batchName -Count $ProbeMessagesPerTransition
            $postProbeSnapshot = Wait-ConsumerGroupStable -ExpectedMembers $ExpectedMembers -RequireZeroLag
            $script:probeBatches += [ordered]@{
                cycle = $Cycle
                action = $Action
                run_name = $batchName
                attempted = [int]$probe.Attempted
                acked = [int]$probe.Acked
                failed = [int]$probe.Failed
                summary_path = (Join-Path $logDir "$batchName-probe-summary.json")
            }
        }

        $script:transitions += [ordered]@{
            cycle = $Cycle
            action = $Action
            expected_members = $ExpectedMembers
            snapshot = $Snapshot
            probe = $probe
            post_probe_snapshot = $postProbeSnapshot
        }
    }

    for ($cycle = 1; $cycle -le $ChurnCycles; $cycle++) {
        Stop-PushConsumer -Process $consumerA
        $afterStopA = Wait-ConsumerGroupStable -ExpectedMembers 1
        Complete-Transition -Cycle $cycle -Action "stop_a" -ExpectedMembers 1 -Snapshot $afterStopA

        $consumerA = Start-PushConsumer -Name "push-consumer-a"
        $afterStartA = Wait-ConsumerGroupStable -ExpectedMembers 2
        Complete-Transition -Cycle $cycle -Action "start_a" -ExpectedMembers 2 -Snapshot $afterStartA

        Stop-PushConsumer -Process $consumerB
        $afterStopB = Wait-ConsumerGroupStable -ExpectedMembers 1
        Complete-Transition -Cycle $cycle -Action "stop_b" -ExpectedMembers 1 -Snapshot $afterStopB

        $consumerB = Start-PushConsumer -Name "push-consumer-b"
        $afterStartB = Wait-ConsumerGroupStable -ExpectedMembers 2
        Complete-Transition -Cycle $cycle -Action "start_b" -ExpectedMembers 2 -Snapshot $afterStartB
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
        probe_messages_per_transition = $ProbeMessagesPerTransition
        initial = $initial
        transitions = $transitions
        probe_batches = $probeBatches
        log_dir = $logDir
    }
    $summaryPath = Join-Path $resultDir "kafka-consumer-churn-summary.json"
    $summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
    Write-Host "kafka_consumer_churn_summary=$summaryPath"
    Write-Host "churn_cycles=$ChurnCycles"
    Write-Host "transition_count=$($transitions.Count)"
    Write-Host "probe_messages_per_transition=$ProbeMessagesPerTransition"
}
finally {
    Stop-PushConsumer -Process $consumerA
    Stop-PushConsumer -Process $consumerB
    Pop-Location
}
