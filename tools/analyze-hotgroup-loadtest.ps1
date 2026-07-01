param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string[]]$RunNamePattern = @("hotgroup-*"),
    [int]$Latest = 0,
    [string]$OutputPath = "",
    [switch]$RequireCleanCommit
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

function Get-SummaryFiles {
    param(
        [string]$Root,
        [string[]]$Patterns
    )

    if (-not (Test-Path -LiteralPath $Root -PathType Container)) {
        throw "ResultRoot does not exist: $Root"
    }

    $seen = @{}
    $files = @()
    foreach ($pattern in $Patterns) {
        Get-ChildItem -LiteralPath $Root -Directory -Filter $pattern |
            ForEach-Object {
                $summaryPath = Join-Path $_.FullName "hotgroup-summary.json"
                if ((Test-Path -LiteralPath $summaryPath -PathType Leaf) -and -not $seen.ContainsKey($summaryPath)) {
                    $seen[$summaryPath] = $true
                    $files += [pscustomobject]@{
                        RunDirectory = $_.FullName
                        SummaryPath = $summaryPath
                        LastWriteTime = $_.LastWriteTime
                    }
                }
            }
    }

    return $files
}

function Convert-HotGroupSummary {
    param([object]$File)

    $summary = Get-Content -LiteralPath $File.SummaryPath -Raw | ConvertFrom-Json
    $push = Get-PropertyValue -Object $summary -Name "push"
    $send = Get-PropertyValue -Object $summary -Name "send"
    $receiver = Get-PropertyValue -Object $summary -Name "receiver"
    $postgres = Get-PropertyValue -Object $summary -Name "postgres"
    $subscriberSignals = @()
    if ($null -ne $push) {
        $subscriberSignals = @(Get-PropertyValue -Object $push -Name "subscriber_signals")
    }

    $lastSignalMs = 0.0
    $firstSignalMs = 0.0
    $completedSubscribers = 0
    $subscriberErrors = 0
    foreach ($subscriber in $subscriberSignals) {
        $lastSignalMs = [Math]::Max($lastSignalMs, (Convert-ToDoubleOrDefault (Get-PropertyValue -Object $subscriber -Name "last_signal_after_ms")))
        $firstSignalMs = [Math]::Max($firstSignalMs, (Convert-ToDoubleOrDefault (Get-PropertyValue -Object $subscriber -Name "first_signal_after_ms")))
        if ([bool](Get-PropertyValue -Object $subscriber -Name "completed")) {
            $completedSubscribers++
        }
        $errorText = [string](Get-PropertyValue -Object $subscriber -Name "error")
        if ($errorText.Trim().Length -gt 0) {
            $subscriberErrors++
        }
    }

    $signalCount = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "conversation_signal_count")
    $signalDrainSeconds = 0.0
    $signalDrainRate = 0.0
    if ($lastSignalMs -gt 0) {
        $signalDrainSeconds = $lastSignalMs / 1000.0
        $signalDrainRate = [double]$signalCount / $signalDrainSeconds
    }
    $sendStartedAt = [string](Get-PropertyValue -Object $send -Name "started_at")
    $sendFinishedAt = [string](Get-PropertyValue -Object $send -Name "finished_at")
    $sendDurationSeconds = 0.0
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
    $achievedSendRate = 0.0
    if ($sendDurationSeconds -gt 0) {
        $achievedSendRate = [double]$sendSuccess / $sendDurationSeconds
    }
    $sendConcurrencyValue = Get-PropertyValue -Object $summary -Name "send_concurrency"
    $sendConcurrency = Convert-ToInt64OrDefault $sendConcurrencyValue
    if ($null -eq $sendConcurrencyValue) {
        $sendConcurrency = 1
    }

    $metricsWindowPath = Join-Path $File.RunDirectory "hotgroup-prometheus-window.json"
    $hasMetricsWindow = Test-Path -LiteralPath $metricsWindowPath -PathType Leaf

    return [pscustomobject]@{
        run_name = [string](Get-PropertyValue -Object $summary -Name "run_name")
        result_dir = $File.RunDirectory
        summary_path = $File.SummaryPath
        metrics_window_path = $metricsWindowPath
        has_metrics_window = $hasMetricsWindow
        last_write_time = $File.LastWriteTime
        commit = [string](Get-PropertyValue -Object $summary -Name "commit")
        git_dirty = [bool](Get-PropertyValue -Object $summary -Name "git_dirty")
        success = [bool](Get-PropertyValue -Object $summary -Name "success")
        error = [string](Get-PropertyValue -Object $summary -Name "error")
        group_size = Convert-ToInt64OrDefault (Get-PropertyValue -Object $summary -Name "group_size")
        message_count = Convert-ToInt64OrDefault (Get-PropertyValue -Object $summary -Name "message_count")
        message_rate = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $summary -Name "message_rate")
        sender_count = Convert-ToInt64OrDefault (Get-PropertyValue -Object $summary -Name "sender_count")
        send_concurrency = $sendConcurrency
        fanout_mode = [string](Get-PropertyValue -Object $summary -Name "actual_fanout_mode")
        expected_fanout_mode = [string](Get-PropertyValue -Object $summary -Name "expected_fanout_mode")
        send_success = $sendSuccess
        send_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $send -Name "error_count")
        send_duration_seconds = $sendDurationSeconds
        achieved_send_rate = $achievedSendRate
        send_p95_ms = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $send -Name "latency_p95_ms")
        send_p99_ms = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $send -Name "latency_p99_ms")
        max_seq = Convert-ToInt64OrDefault (Get-PropertyValue -Object $send -Name "max_seq")
        subscriber_count = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "subscriber_count")
        subscribe_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "subscribe_error_count")
        signal_count = $signalCount
        max_conversation_seq = Convert-ToInt64OrDefault (Get-PropertyValue -Object $push -Name "max_conversation_seq")
        signal_first_max_ms = $firstSignalMs
        signal_slowest_ms = $lastSignalMs
        signal_drain_seconds = $signalDrainSeconds
        signal_drain_rate = $signalDrainRate
        completed_subscribers = $completedSubscribers
        subscriber_errors = $subscriberErrors
        sampled_receivers = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "sampled_receivers")
        pull_success = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "pull_success_count")
        pull_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "pull_error_count")
        ack_success = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "ack_success_count")
        ack_errors = Convert-ToInt64OrDefault (Get-PropertyValue -Object $receiver -Name "ack_error_count")
        pull_p95_ms = Convert-ToDoubleOrDefault (Get-PropertyValue -Object $receiver -Name "pull_latency_p95_ms")
        conversation_mode = [string](Get-PropertyValue -Object $postgres -Name "conversation_mode")
        postgres_fanout_mode = [string](Get-PropertyValue -Object $postgres -Name "fanout_mode")
        message_log_count = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "message_log_count")
        delivery_timeline_rows = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_timeline_rows")
        user_inbox_rows = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "user_inbox_rows")
        delivery_outbox_rows = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_outbox_rows")
        message_outbox_pending = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "message_outbox_pending")
        delivery_outbox_pending = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_outbox_pending")
        message_outbox_dlq = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "message_outbox_dlq")
        delivery_outbox_dlq = Convert-ToInt64OrDefault (Get-PropertyValue -Object $postgres -Name "delivery_outbox_dlq")
    }
}

function Get-BottleneckClassification {
    param([object]$Run)

    $evidence = New-Object System.Collections.Generic.List[string]
    $next = New-Object System.Collections.Generic.List[string]
    $class = "undetermined"

    if (-not $Run.success) {
        $class = "failed-run"
        if ($Run.error.Trim().Length -gt 0) {
            $evidence.Add("run error: " + $Run.error)
        }
        $next.Add("read the run error and classify the failure before comparing QPS")
        return [pscustomobject]@{ Class = $class; Evidence = @($evidence); Next = @($next) }
    }

    if ($Run.git_dirty -and $RequireCleanCommit) {
        $class = "untrusted-dirty-run"
        $evidence.Add("git_dirty=true")
        $next.Add("rerun from a clean commit before using the data as baseline")
        return [pscustomobject]@{ Class = $class; Evidence = @($evidence); Next = @($next) }
    }

    if ($Run.send_errors -gt 0) {
        $class = "send-path"
        $evidence.Add("send_errors=$($Run.send_errors)")
        $next.Add("inspect api/message/conversation error distribution and sender concurrency")
    }
    elseif ($Run.send_p99_ms -ge 200) {
        $class = "send-path-latency"
        $evidence.Add("send_p99_ms=$(Format-Number $Run.send_p99_ms)")
        $next.Add("profile SendMessage, sequencer allocation, message log write and message outbox insert")
    }
    elseif ($Run.message_outbox_pending -gt 0 -or $Run.message_outbox_dlq -gt 0) {
        $class = "message-outbox-relay"
        $evidence.Add("message_outbox_pending=$($Run.message_outbox_pending), message_outbox_dlq=$($Run.message_outbox_dlq)")
        $next.Add("inspect message outbox ready query, worker sharding, publish batch and mark-published batch")
    }
    elseif ($Run.delivery_outbox_pending -gt 0 -or $Run.delivery_outbox_dlq -gt 0) {
        $class = "delivery-outbox-relay"
        $evidence.Add("delivery_outbox_pending=$($Run.delivery_outbox_pending), delivery_outbox_dlq=$($Run.delivery_outbox_dlq)")
        $next.Add("inspect delivery outbox frontier query, worker count, Kafka publish latency and DLQ blockers")
    }
    elseif ($Run.pull_errors -gt 0 -or $Run.ack_errors -gt 0) {
        $class = "receiver-pull-ack"
        $evidence.Add("pull_errors=$($Run.pull_errors), ack_errors=$($Run.ack_errors)")
        $next.Add("inspect delivery-service PullInbox / AckDelivery latency and cursor write path")
    }
    elseif ($Run.pull_p95_ms -ge 200) {
        $class = "receiver-pull-latency"
        $evidence.Add("pull_p95_ms=$(Format-Number $Run.pull_p95_ms)")
        $next.Add("inspect PullInbox query plan, cursor indexes and receiver sampling pressure")
    }
    elseif ($Run.subscribe_errors -gt 0 -or $Run.subscriber_errors -gt 0) {
        $class = "push-subscribe-or-read-errors"
        $evidence.Add("subscribe_errors=$($Run.subscribe_errors), subscriber_errors=$($Run.subscriber_errors)")
        $next.Add("inspect push-gateway subscribe path, websocket writer errors and runner read errors")
    }
    elseif ($Run.subscriber_count -gt 0 -and $Run.completed_subscribers -lt $Run.subscriber_count) {
        $class = "push-signal-incomplete"
        $evidence.Add("completed_subscribers=$($Run.completed_subscribers)/$($Run.subscriber_count)")
        $next.Add("increase push writer metrics coverage and split receiver load across machines")
    }
    elseif ($Run.signal_count -gt 0 -and $Run.signal_drain_seconds -gt 60) {
        $class = "online-signal-drain"
        $evidence.Add("signals=$($Run.signal_count), slowest_drain_s=$(Format-Number $Run.signal_drain_seconds), drain_rate=$(Format-Number $Run.signal_drain_rate) signals/s")
        $next.Add("raise subscriber count and total signal volume; inspect push writer flush, session queues, Redis route and runner read capacity")
    }
    elseif ($Run.signal_count -gt 0) {
        $class = "no-backend-bottleneck-yet"
        $evidence.Add("outbox pending=0, send_p99_ms=$(Format-Number $Run.send_p99_ms), pull_p95_ms=$(Format-Number $Run.pull_p95_ms), signals=$($Run.signal_count)")
        $next.Add("increase hot group count, subscriber_count, total messages or use multi-runner load")
    }
    else {
        $class = "insufficient-observability"
        $evidence.Add("summary lacks push signal or lag fields")
        $next.Add("add Prometheus/debug metrics or run with conversation subscribers enabled")
    }

    return [pscustomobject]@{
        Class = $class
        Evidence = @($evidence)
        Next = @($next)
    }
}

function Write-AnalysisMarkdown {
    param(
        [object[]]$Runs,
        [string]$Path,
        [string[]]$Patterns
    )

    $generatedAt = (Get-Date).ToString("yyyy-MM-dd HH:mm:ss zzz")
    $builder = New-Object System.Text.StringBuilder
    [void]$builder.AppendLine("# Hot Group Loadtest Analysis")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("Generated by tools/analyze-hotgroup-loadtest.ps1 at $generatedAt.")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("Scope: offline analysis of existing hotgroup-summary.json files. This report does not claim production capacity or SLO.")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- result_root: $ResultRoot")
    [void]$builder.AppendLine("- run_patterns: $($Patterns -join ', ')")
    [void]$builder.AppendLine("- run_count: $($Runs.Count)")
    [void]$builder.AppendLine("")

    if ($Runs.Count -eq 0) {
        [void]$builder.AppendLine("No matching hotgroup summaries found.")
        Set-Content -LiteralPath $Path -Value ($builder.ToString().TrimEnd()) -Encoding UTF8
        return
    }

    $commits = @($Runs | Select-Object -ExpandProperty commit -Unique | Where-Object { $_ })
    $dirtyCount = @($Runs | Where-Object { $_.git_dirty }).Count
    $successCount = @($Runs | Where-Object { $_.success }).Count
    $maxRateRun = @($Runs | Sort-Object -Property message_rate, message_count -Descending)[0]
    $maxSignalRun = @($Runs | Sort-Object -Property signal_count -Descending)[0]
    $latestRun = @($Runs | Sort-Object -Property last_write_time -Descending)[0]

    [void]$builder.AppendLine("## Executive Summary")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- commits: $($commits -join ', ')")
    [void]$builder.AppendLine("- clean_runs: $($Runs.Count - $dirtyCount) / $($Runs.Count)")
    [void]$builder.AppendLine("- successful_runs: $successCount / $($Runs.Count)")
    [void]$builder.AppendLine("- highest_target_message_rate: $($maxRateRun.message_rate) msg/s in $($maxRateRun.run_name)")
    [void]$builder.AppendLine("- highest_signal_count: $($maxSignalRun.signal_count) signals in $($maxSignalRun.run_name)")
    [void]$builder.AppendLine("- latest_run: $($latestRun.run_name)")
    [void]$builder.AppendLine("")

    [void]$builder.AppendLine("## Run Matrix")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("| run | commit | clean | ok | fanout | group | target msg/s | achieved msg/s | messages | senders | send concurrency | subs | send p95 | send p99 | pull p95 | signals | slowest drain s | drain signals/s | msg pending | delivery pending | metrics window | bottleneck |")
    [void]$builder.AppendLine("| --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |")

    foreach ($run in $Runs) {
        $classification = Get-BottleneckClassification -Run $run
        [void]$builder.AppendLine((
            "| {0} | {1} | {2} | {3} | {4} | {5} | {6} | {7} | {8} | {9} | {10} | {11} | {12} | {13} | {14} | {15} | {16} | {17} | {18} | {19} | {20} | {21} |" -f
            (Escape-MarkdownCell $run.run_name),
            (Escape-MarkdownCell $run.commit),
            (-not $run.git_dirty),
            $run.success,
            (Escape-MarkdownCell $run.fanout_mode),
            $run.group_size,
            (Format-Number $run.message_rate),
            (Format-Number $run.achieved_send_rate),
            $run.message_count,
            $run.sender_count,
            $run.send_concurrency,
            $run.subscriber_count,
            (Format-Number $run.send_p95_ms),
            (Format-Number $run.send_p99_ms),
            (Format-Number $run.pull_p95_ms),
            $run.signal_count,
            (Format-Number $run.signal_drain_seconds),
            (Format-Number $run.signal_drain_rate),
            $run.message_outbox_pending,
            $run.delivery_outbox_pending,
            $run.has_metrics_window,
            (Escape-MarkdownCell $classification.Class)
        ))
    }
    [void]$builder.AppendLine("")

    [void]$builder.AppendLine("## Bottleneck Evidence")
    [void]$builder.AppendLine("")
    foreach ($run in $Runs) {
        $classification = Get-BottleneckClassification -Run $run
        [void]$builder.AppendLine("### $($run.run_name)")
        [void]$builder.AppendLine("")
        [void]$builder.AppendLine("- classification: $($classification.Class)")
        foreach ($item in $classification.Evidence) {
            [void]$builder.AppendLine("- evidence: $item")
        }
        foreach ($item in $classification.Next) {
            [void]$builder.AppendLine("- next_strategy: $item")
        }
        [void]$builder.AppendLine("- result_dir: $($run.result_dir)")
        if ($run.has_metrics_window) {
            [void]$builder.AppendLine("- metrics_window: $($run.metrics_window_path)")
        }
        else {
            [void]$builder.AppendLine("- metrics_window: missing")
        }
        [void]$builder.AppendLine("")
    }

    $latestClassification = Get-BottleneckClassification -Run $latestRun
    [void]$builder.AppendLine("## Recommended Next Step")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- current_bottleneck: $($latestClassification.Class)")
    foreach ($item in $latestClassification.Evidence) {
        [void]$builder.AppendLine("- evidence: $item")
    }
    foreach ($item in $latestClassification.Next) {
        [void]$builder.AppendLine("- strategy: $item")
    }
    if ($latestRun.has_metrics_window) {
        [void]$builder.AppendLine("- required_before_next_claim: metrics window is captured for the latest run; keep capturing a window for each subsequent pressure step.")
    }
    else {
        [void]$builder.AppendLine("- required_before_next_claim: capture Prometheus / Grafana or debug metrics time window and bind it to the run name.")
    }
    [void]$builder.AppendLine("")

    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    Set-Content -LiteralPath $Path -Value ($builder.ToString().TrimEnd()) -Encoding UTF8
}

$files = Get-SummaryFiles -Root $ResultRoot -Patterns $RunNamePattern
if ($Latest -gt 0) {
    $files = @($files | Sort-Object LastWriteTime -Descending | Select-Object -First $Latest)
}

$runs = @($files | ForEach-Object { Convert-HotGroupSummary -File $_ } | Sort-Object message_rate, message_count, run_name)
if ($RequireCleanCommit) {
    $dirty = @($runs | Where-Object { $_.git_dirty })
    if ($dirty.Count -gt 0) {
        throw "Found dirty hotgroup runs: $($dirty.run_name -join ', ')"
    }
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path (Split-Path -Parent $PSScriptRoot) ("docs\runbook\loadtest\hotgroup\hotgroup-analysis-" + (Get-Date).ToString("yyyyMMdd-HHmmss") + ".md")
}

Write-AnalysisMarkdown -Runs $runs -Path $OutputPath -Patterns $RunNamePattern
Write-Host "Wrote hotgroup analysis: $OutputPath"
