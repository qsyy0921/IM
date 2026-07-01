param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [Parameter(Mandatory = $true)]
    [string]$CoordinatorRunName,
    [Parameter(Mandatory = $true)]
    [string]$ShardRunNamePattern,
    [string]$BaselineRunName = "",
    [string]$BaselineShardRunNamePattern = "",
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

function Get-PropertyValue {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object) {
        return $null
    }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }
    return $property.Value
}

function Convert-ToDoubleOrDefault {
    param(
        [object]$Value,
        [double]$Default = 0
    )

    if ($null -eq $Value) {
        return $Default
    }
    try {
        return [double]$Value
    }
    catch {
        return $Default
    }
}

function Convert-ToInt64OrDefault {
    param(
        [object]$Value,
        [int64]$Default = 0
    )

    if ($null -eq $Value) {
        return $Default
    }
    try {
        return [int64]$Value
    }
    catch {
        return $Default
    }
}

function Format-Number {
    param(
        [object]$Value,
        [string]$Format = "0.###"
    )

    if ($null -eq $Value) {
        return ""
    }
    try {
        return ([double]$Value).ToString($Format, [System.Globalization.CultureInfo]::InvariantCulture)
    }
    catch {
        return [string]$Value
    }
}

function Escape-MarkdownCell {
    param([object]$Value)

    if ($null -eq $Value) {
        return ""
    }
    return ([string]$Value).Replace("|", "/").Replace([string][char]0x60, "'")
}

function Read-HotGroupSummary {
    param([string]$RunName)

    $summaryPath = Join-Path (Join-Path $ResultRoot $RunName) "hotgroup-summary.json"
    if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
        throw "hotgroup summary not found: $summaryPath"
    }
    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    return [pscustomobject]@{
        RunName = $RunName
        ResultDir = (Join-Path $ResultRoot $RunName)
        SummaryPath = $summaryPath
        Summary = $summary
    }
}

function Get-ShardSummaries {
    param([string]$Pattern)

    $items = @(
        Get-ChildItem -LiteralPath $ResultRoot -Directory -Filter $Pattern |
            Sort-Object Name |
            ForEach-Object {
                Read-HotGroupSummary -RunName $_.Name
            }
    )
    if ($items.Count -eq 0) {
        throw "no shard summaries matched pattern: $Pattern"
    }
    return $items
}

function Get-SendSummary {
    param([object]$Run)

    $summary = $Run.Summary
    $send = Get-PropertyValue -Object $summary -Name "send"
    $receiver = Get-PropertyValue -Object $summary -Name "receiver"
    $postgres = Get-PropertyValue -Object $summary -Name "postgres"
    $sendStartedAt = [string](Get-PropertyValue -Object $send -Name "started_at")
    $sendFinishedAt = [string](Get-PropertyValue -Object $send -Name "finished_at")
    $sendDurationSeconds = 0.0
    $achievedSendRate = 0.0
    if ($sendStartedAt.Trim().Length -gt 0 -and $sendFinishedAt.Trim().Length -gt 0) {
        try {
            $started = [datetime]$sendStartedAt
            $finished = [datetime]$sendFinishedAt
            $sendDurationSeconds = ($finished.ToUniversalTime() - $started.ToUniversalTime()).TotalSeconds
        }
        catch {
            $sendDurationSeconds = 0.0
        }
    }
    $sendSuccess = Convert-ToInt64OrDefault (Get-PropertyValue -Object $send -Name "success_count")
    if ($sendDurationSeconds -gt 0) {
        $achievedSendRate = [double]$sendSuccess / $sendDurationSeconds
    }
    return [pscustomobject]@{
        run_name = [string](Get-PropertyValue -Object $summary -Name "run_name")
        commit = [string](Get-PropertyValue -Object $summary -Name "commit")
        git_dirty = [bool](Get-PropertyValue -Object $summary -Name "git_dirty")
        success = [bool](Get-PropertyValue -Object $summary -Name "success")
        group_size = Convert-ToInt64OrDefault (Get-PropertyValue -Object $summary -Name "group_size")
        message_count = Convert-ToInt64OrDefault (Get-PropertyValue -Object $summary -Name "message_count")
        message_rate = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $summary -Name "message_rate")
        sender_count = Convert-ToInt64OrDefault (Get-PropertyValue -Object $summary -Name "sender_count")
        fanout_mode = [string](Get-PropertyValue -Object $summary -Name "actual_fanout_mode")
        send_success = $sendSuccess
        send_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $send -Name "error_count")
        send_started_at = $sendStartedAt
        send_finished_at = $sendFinishedAt
        send_duration_seconds = $sendDurationSeconds
        achieved_send_rate = $achievedSendRate
        send_p95_ms = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $send -Name "latency_p95_ms")
        send_p99_ms = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $send -Name "latency_p99_ms")
        pull_success = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "pull_success_count")
        pull_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "pull_error_count")
        ack_success = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "ack_success_count")
        ack_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "ack_error_count")
        pull_p95_ms = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $receiver -Name "pull_latency_p95_ms")
        delivery_timeline_rows = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_timeline_rows")
        user_inbox_rows = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "user_inbox_rows")
        message_outbox_pending = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "message_outbox_pending")
        delivery_outbox_pending = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_outbox_pending")
        message_outbox_dlq = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "message_outbox_dlq")
        delivery_outbox_dlq = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_outbox_dlq")
    }
}

function Get-SignalSummary {
    param([object[]]$Runs)

    $subscriberCount = 0
    $signalCount = 0
    $completedCount = 0
    $subscriberErrors = 0
    $subscribeErrors = 0
    $minFirst = $null
    $maxLast = $null
    $maxSubscriberSpan = 0.0
    $shardRows = @()

    foreach ($run in $Runs) {
        $summary = $run.Summary
        $push = Get-PropertyValue -Object $summary -Name "push"
        $signals = @(Get-PropertyValue -Object $push -Name "subscriber_signals")
        $runSignalCount = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "conversation_signal_count")
        $runSubscriberCount = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "subscriber_count")
        $runSubscribeErrors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "subscribe_error_count")
        $runMinFirst = $null
        $runMaxLast = $null
        $runCompleted = 0
        $runSubscriberErrors = 0
        $runMaxSpan = 0.0

        foreach ($subscriber in $signals) {
            $first = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $subscriber -Name "first_signal_after_ms")
            $last = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $subscriber -Name "last_signal_after_ms")
            if ($first -gt 0 -and ($null -eq $minFirst -or $first -lt $minFirst)) {
                $minFirst = $first
            }
            if ($first -gt 0 -and ($null -eq $runMinFirst -or $first -lt $runMinFirst)) {
                $runMinFirst = $first
            }
            if ($last -gt 0 -and ($null -eq $maxLast -or $last -gt $maxLast)) {
                $maxLast = $last
            }
            if ($last -gt 0 -and ($null -eq $runMaxLast -or $last -gt $runMaxLast)) {
                $runMaxLast = $last
            }
            $span = $last - $first
            if ($span -gt $runMaxSpan) {
                $runMaxSpan = $span
            }
            if ($span -gt $maxSubscriberSpan) {
                $maxSubscriberSpan = $span
            }
            if ([bool](Get-PropertyValue -Object $subscriber -Name "completed")) {
                $completedCount++
                $runCompleted++
            }
            $errorText = [string](Get-PropertyValue -Object $subscriber -Name "error")
            if ($errorText.Trim().Length -gt 0) {
                $subscriberErrors++
                $runSubscriberErrors++
            }
        }

        $subscriberCount += $runSubscriberCount
        $signalCount += $runSignalCount
        $subscribeErrors += $runSubscribeErrors
        $runGlobalSpan = 0.0
        if ($null -ne $runMinFirst -and $null -ne $runMaxLast -and $runMaxLast -gt $runMinFirst) {
            $runGlobalSpan = $runMaxLast - $runMinFirst
        }
        $shardRows += [pscustomobject]@{
            run_name = [string](Get-PropertyValue -Object $summary -Name "run_name")
            subscriber_count = $runSubscriberCount
            signal_count = $runSignalCount
            completed_subscribers = $runCompleted
            subscribe_errors = $runSubscribeErrors
            subscriber_errors = $runSubscriberErrors
            first_signal_min_ms = $runMinFirst
            last_signal_max_ms = $runMaxLast
            signal_span_seconds = $runGlobalSpan / 1000.0
            signal_span_rate = if ($runGlobalSpan -gt 0) { [double]$runSignalCount / ($runGlobalSpan / 1000.0) } else { 0 }
            max_subscriber_span_seconds = $runMaxSpan / 1000.0
        }
    }

    $globalSpan = 0.0
    if ($null -ne $minFirst -and $null -ne $maxLast -and $maxLast -gt $minFirst) {
        $globalSpan = $maxLast - $minFirst
    }
    return [pscustomobject]@{
        subscriber_count = $subscriberCount
        signal_count = $signalCount
        completed_subscribers = $completedCount
        subscribe_errors = $subscribeErrors
        subscriber_errors = $subscriberErrors
        first_signal_min_ms = $minFirst
        last_signal_max_ms = $maxLast
        signal_span_seconds = $globalSpan / 1000.0
        signal_span_rate = if ($globalSpan -gt 0) { [double]$signalCount / ($globalSpan / 1000.0) } else { 0 }
        max_subscriber_span_seconds = $maxSubscriberSpan / 1000.0
        shard_rows = $shardRows
    }
}

function Write-MultirunnerReport {
    param(
        [object]$Coordinator,
        [object[]]$Shards,
        [object]$SendSummary,
        [object]$SignalSummary,
        [object]$BaselineSignalSummary,
        [string]$Path
    )

    $builder = New-Object System.Text.StringBuilder
    $generatedAt = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss zzz")
    [void]$builder.AppendLine("# Hot Group Multi-Runner Signal Drain Analysis")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("Generated by tools/analyze-hotgroup-multirunner.ps1 at $generatedAt.")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("Scope: low-sensitive analysis that combines one full coordinator run with multiple subscriber-only shards. This is not a production SLO.")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Run Identity")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- coordinator_run: $CoordinatorRunName")
    [void]$builder.AppendLine("- shard_pattern: $ShardRunNamePattern")
    [void]$builder.AppendLine("- shard_count: $($Shards.Count)")
    [void]$builder.AppendLine("- result_root: $ResultRoot")
    [void]$builder.AppendLine("- commit: $($SendSummary.commit)")
    [void]$builder.AppendLine("- git_dirty: $($SendSummary.git_dirty)")
    if ($BaselineRunName.Trim().Length -gt 0) {
        [void]$builder.AppendLine("- baseline_run: $BaselineRunName")
    }
    if ($BaselineShardRunNamePattern.Trim().Length -gt 0) {
        [void]$builder.AppendLine("- baseline_shard_pattern: $BaselineShardRunNamePattern")
    }
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Coordinator")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("| field | value |")
    [void]$builder.AppendLine("| --- | ---: |")
    [void]$builder.AppendLine("| success | $($SendSummary.success) |")
    [void]$builder.AppendLine("| group_size | $($SendSummary.group_size) |")
    [void]$builder.AppendLine("| fanout_mode | $($SendSummary.fanout_mode) |")
    [void]$builder.AppendLine("| message_count | $($SendSummary.message_count) |")
    [void]$builder.AppendLine("| target_message_rate | $(Format-Number $SendSummary.message_rate) |")
    [void]$builder.AppendLine("| sender_count | $($SendSummary.sender_count) |")
    [void]$builder.AppendLine("| send_success | $($SendSummary.send_success) |")
    [void]$builder.AppendLine("| send_errors | $($SendSummary.send_errors) |")
    [void]$builder.AppendLine("| send_duration_seconds | $(Format-Number $SendSummary.send_duration_seconds) |")
    [void]$builder.AppendLine("| achieved_send_rate | $(Format-Number $SendSummary.achieved_send_rate) |")
    [void]$builder.AppendLine("| send_p95_ms | $(Format-Number $SendSummary.send_p95_ms) |")
    [void]$builder.AppendLine("| send_p99_ms | $(Format-Number $SendSummary.send_p99_ms) |")
    [void]$builder.AppendLine("| pull_success | $($SendSummary.pull_success) |")
    [void]$builder.AppendLine("| pull_errors | $($SendSummary.pull_errors) |")
    [void]$builder.AppendLine("| ack_success | $($SendSummary.ack_success) |")
    [void]$builder.AppendLine("| ack_errors | $($SendSummary.ack_errors) |")
    [void]$builder.AppendLine("| pull_p95_ms | $(Format-Number $SendSummary.pull_p95_ms) |")
    [void]$builder.AppendLine("| delivery_timeline_rows | $($SendSummary.delivery_timeline_rows) |")
    [void]$builder.AppendLine("| user_inbox_rows | $($SendSummary.user_inbox_rows) |")
    [void]$builder.AppendLine("| message_outbox_pending | $($SendSummary.message_outbox_pending) |")
    [void]$builder.AppendLine("| delivery_outbox_pending | $($SendSummary.delivery_outbox_pending) |")
    [void]$builder.AppendLine("| message_outbox_dlq | $($SendSummary.message_outbox_dlq) |")
    [void]$builder.AppendLine("| delivery_outbox_dlq | $($SendSummary.delivery_outbox_dlq) |")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Subscriber Drain")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("| field | value |")
    [void]$builder.AppendLine("| --- | ---: |")
    [void]$builder.AppendLine("| subscriber_count | $($SignalSummary.subscriber_count) |")
    [void]$builder.AppendLine("| completed_subscribers | $($SignalSummary.completed_subscribers) |")
    [void]$builder.AppendLine("| subscribe_errors | $($SignalSummary.subscribe_errors) |")
    [void]$builder.AppendLine("| subscriber_errors | $($SignalSummary.subscriber_errors) |")
    [void]$builder.AppendLine("| conversation_signal_count | $($SignalSummary.signal_count) |")
    [void]$builder.AppendLine("| first_signal_min_ms | $(Format-Number $SignalSummary.first_signal_min_ms) |")
    [void]$builder.AppendLine("| last_signal_max_ms | $(Format-Number $SignalSummary.last_signal_max_ms) |")
    [void]$builder.AppendLine("| signal_span_seconds | $(Format-Number $SignalSummary.signal_span_seconds) |")
    [void]$builder.AppendLine("| signal_span_rate | $(Format-Number $SignalSummary.signal_span_rate) |")
    [void]$builder.AppendLine("| max_subscriber_span_seconds | $(Format-Number $SignalSummary.max_subscriber_span_seconds) |")
    [void]$builder.AppendLine("")
    if ($null -ne $BaselineSignalSummary) {
        $delta = $SignalSummary.signal_span_rate - $BaselineSignalSummary.signal_span_rate
        $ratio = 0.0
        $signalCountRatio = 0.0
        $spanRatio = 0.0
        if ($BaselineSignalSummary.signal_span_rate -gt 0) {
            $ratio = $SignalSummary.signal_span_rate / $BaselineSignalSummary.signal_span_rate
        }
        if ($BaselineSignalSummary.signal_count -gt 0) {
            $signalCountRatio = [double]$SignalSummary.signal_count / [double]$BaselineSignalSummary.signal_count
        }
        if ($BaselineSignalSummary.signal_span_seconds -gt 0) {
            $spanRatio = [double]$SignalSummary.signal_span_seconds / [double]$BaselineSignalSummary.signal_span_seconds
        }
        [void]$builder.AppendLine("## Baseline Comparison")
        [void]$builder.AppendLine("")
        [void]$builder.AppendLine("| run | subscribers | signals | signal span s | span rate signals/s |")
        [void]$builder.AppendLine("| --- | ---: | ---: | ---: | ---: |")
        [void]$builder.AppendLine("| baseline $BaselineRunName | $($BaselineSignalSummary.subscriber_count) | $($BaselineSignalSummary.signal_count) | $(Format-Number $BaselineSignalSummary.signal_span_seconds) | $(Format-Number $BaselineSignalSummary.signal_span_rate) |")
        [void]$builder.AppendLine("| multi-runner $CoordinatorRunName + shards | $($SignalSummary.subscriber_count) | $($SignalSummary.signal_count) | $(Format-Number $SignalSummary.signal_span_seconds) | $(Format-Number $SignalSummary.signal_span_rate) |")
        [void]$builder.AppendLine("")
        [void]$builder.AppendLine("- delta_signals_per_second: $(Format-Number $delta)")
        [void]$builder.AppendLine("- ratio_vs_baseline: $(Format-Number $ratio)")
        [void]$builder.AppendLine("- signal_count_ratio_vs_baseline: $(Format-Number $signalCountRatio)")
        [void]$builder.AppendLine("- signal_span_ratio_vs_baseline: $(Format-Number $spanRatio)")
        [void]$builder.AppendLine("")
    }
    [void]$builder.AppendLine("## Shards")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("| shard | subscribers | completed | signals | errors | signal span s | span rate |")
    [void]$builder.AppendLine("| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
    foreach ($row in $SignalSummary.shard_rows) {
        [void]$builder.AppendLine("| $(Escape-MarkdownCell $row.run_name) | $($row.subscriber_count) | $($row.completed_subscribers) | $($row.signal_count) | $($row.subscriber_errors) | $(Format-Number $row.signal_span_seconds) | $(Format-Number $row.signal_span_rate) |")
    }
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Bottleneck Judgment")
    [void]$builder.AppendLine("")
    if ($SignalSummary.subscribe_errors -eq 0 -and $SignalSummary.subscriber_errors -eq 0 -and $SendSummary.message_outbox_pending -eq 0 -and $SendSummary.delivery_outbox_pending -eq 0) {
        $signalVolumeReducedWithoutSpanImprovement = $false
        if ($null -ne $BaselineSignalSummary -and $BaselineSignalSummary.signal_count -gt 0 -and $BaselineSignalSummary.signal_span_seconds -gt 0) {
            $signalCountRatioForJudgment = [double]$SignalSummary.signal_count / [double]$BaselineSignalSummary.signal_count
            $spanRatioForJudgment = [double]$SignalSummary.signal_span_seconds / [double]$BaselineSignalSummary.signal_span_seconds
            $signalVolumeReducedWithoutSpanImprovement = ($signalCountRatioForJudgment -lt 0.8 -and $spanRatioForJudgment -gt 0.9)
        }

        if ($signalVolumeReducedWithoutSpanImprovement) {
            [void]$builder.AppendLine("- current_bottleneck: signal-volume-reduced-without-drain-improvement")
            [void]$builder.AppendLine("- evidence: coordinator send / PullInbox / ACK succeeded, message_outbox and delivery_outbox ended with zero pending, all subscriber shards completed.")
            [void]$builder.AppendLine("- evidence: emitted conversation signal count fell materially versus baseline, but first-to-last signal span stayed nearly flat; the stronger pull-first cadence reduced online frame volume but did not reduce end-to-end drain time in this run.")
            if ($SendSummary.message_rate -gt 0 -and $SendSummary.achieved_send_rate -gt 0 -and ($SendSummary.achieved_send_rate / $SendSummary.message_rate) -lt 0.5) {
                [void]$builder.AppendLine("- evidence: achieved SendMessage rate was far below the target rate, so sender pacing / setup / client-side throttling must be measured before interpreting signal drain as a server capacity ceiling.")
            }
            [void]$builder.AppendLine("- evidence: do not report this as a throughput improvement. Treat it as load-reduction evidence plus a new diagnostic clue.")
            [void]$builder.AppendLine("- next_strategy: inspect actual SendMessage generation duration, delivery_outbox signal production cadence, Kafka publish / consume cadence, and push event pacing before further increasing sample_every or subscriber count.")
        }
        else {
            [void]$builder.AppendLine("- current_bottleneck: online-signal-drain")
            [void]$builder.AppendLine("- evidence: coordinator send / PullInbox / ACK succeeded, message_outbox and delivery_outbox ended with zero pending, all subscriber shards completed.")
            [void]$builder.AppendLine("- evidence: multi-runner signal span rate did not materially exceed the previous single-runner 400 subscriber baseline, so the immediate limit is not just one Go process doing JSON decode/accounting.")
            [void]$builder.AppendLine("- next_strategy: inspect push-gateway conversation signal writer path, WebSocket flush cadence, Redis subscriber fanout, network throughput and per-connection write scheduling before increasing subscriber count again.")
        }
    }
    else {
        [void]$builder.AppendLine("- current_bottleneck: failed-or-incomplete-run")
        [void]$builder.AppendLine("- evidence: errors or pending rows exist; inspect coordinator and shard raw summaries before using this run as a baseline.")
    }
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Raw Artifacts")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- coordinator_result_dir: $($Coordinator.ResultDir)")
    foreach ($shard in $Shards) {
        [void]$builder.AppendLine("- shard_result_dir: $($shard.ResultDir)")
    }

    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    Set-Content -LiteralPath $Path -Value ($builder.ToString().TrimEnd()) -Encoding UTF8
}

$coordinator = Read-HotGroupSummary -RunName $CoordinatorRunName
$shards = Get-ShardSummaries -Pattern $ShardRunNamePattern
$sendSummary = Get-SendSummary -Run $coordinator
$signalSummary = Get-SignalSummary -Runs $shards
$baselineSignalSummary = $null
if ($BaselineShardRunNamePattern.Trim().Length -gt 0) {
    $baselineShards = Get-ShardSummaries -Pattern $BaselineShardRunNamePattern
    $baselineSignalSummary = Get-SignalSummary -Runs $baselineShards
}
elseif ($BaselineRunName.Trim().Length -gt 0) {
    $baseline = Read-HotGroupSummary -RunName $BaselineRunName
    $baselineSignalSummary = Get-SignalSummary -Runs @($baseline)
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path (Split-Path -Parent $PSScriptRoot) ("docs\runbook\loadtest\hotgroup\hotgroup-multirunner-analysis-" + (Get-Date).ToString("yyyyMMdd-HHmmss") + ".md")
}

Write-MultirunnerReport -Coordinator $coordinator -Shards $shards -SendSummary $sendSummary -SignalSummary $signalSummary -BaselineSignalSummary $baselineSignalSummary -Path $OutputPath
Write-Host "Wrote hotgroup multirunner analysis: $OutputPath"
