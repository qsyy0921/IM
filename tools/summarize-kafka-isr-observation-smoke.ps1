param(
    [Parameter(Mandatory = $true)]
    [string]$RunDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$runPath = [System.IO.Path]::GetFullPath($RunDir)
if (-not (Test-Path -LiteralPath $runPath -PathType Container)) {
    throw "Kafka ISR observation run directory does not exist: $runPath"
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $runPath "kafka-isr-observation-report-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $runPath "kafka-isr-observation-report.md"
}

$sourceSummaryPath = Join-Path $runPath "kafka-isr-observation-summary.json"
if (-not (Test-Path -LiteralPath $sourceSummaryPath -PathType Leaf)) {
    throw "Kafka ISR observation summary is missing: $sourceSummaryPath"
}

function Read-JsonFile {
    param([string]$Path)

    return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

function Convert-ToArray {
    param([object]$Value)

    if ($null -eq $Value) {
        return @()
    }
    if ($Value -is [System.Array]) {
        return @($Value)
    }
    return @($Value)
}

function Get-SmokeSuccess {
    param([string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path) -or -not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return [pscustomobject]@{
            path = $Path
            present = $false
            success = $false
        }
    }

    $summary = Read-JsonFile -Path $Path
    $success = $false
    if ($null -ne $summary.success) {
        $success = [bool]$summary.success
    }
    return [pscustomobject]@{
        path = $Path
        present = $true
        success = $success
    }
}

function Test-ProbeAccepted {
    param([object]$Probe)

    return ($null -ne $Probe -and [bool]$Probe.accepted)
}

function Test-NotEnoughReplicas {
    param([object]$Probe)

    return ($null -ne $Probe -and [bool]$Probe.contains_not_enough_replicas)
}

function Get-TopicStateSummary {
    param([object[]]$States)

    $rows = @(Convert-ToArray -Value $States)
    $badRows = @($rows | Where-Object {
        [int]$_.replica_count -ne 3 -or [int]$_.isr_count -ne 2
    })
    return [pscustomobject]@{
        partition_count = $rows.Count
        all_replica_count_3 = ($rows.Count -gt 0 -and $badRows.Count -eq 0)
        all_isr_count_2 = ($rows.Count -gt 0 -and $badRows.Count -eq 0)
        rows = $rows
    }
}

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

$source = Read-JsonFile -Path $sourceSummaryPath

$beforeSmoke = Get-SmokeSuccess -Path ([string]$source.before_summary)
$oneBrokerDownSmoke = Get-SmokeSuccess -Path ([string]$source.one_broker_down_summary)
$restoreSmoke = Get-SmokeSuccess -Path ([string]$source.after_restore_summary)

$deliveryOneDown = Get-TopicStateSummary -States (Convert-ToArray -Value $source.delivery_topic_after_one_broker_stop)
$probeOneDownAccepted = Test-ProbeAccepted -Probe $source.probe_produce_after_one_broker_stop
$probeTwoDownRejected = (-not (Test-ProbeAccepted -Probe $source.probe_produce_after_two_broker_stops)) -and (Test-NotEnoughReplicas -Probe $source.probe_produce_after_two_broker_stops)

$passed = (
    $beforeSmoke.success -and
    $oneBrokerDownSmoke.success -and
    $restoreSmoke.success -and
    $deliveryOneDown.all_replica_count_3 -and
    $deliveryOneDown.all_isr_count_2 -and
    $probeOneDownAccepted -and
    $probeTwoDownRejected
)

Assert-Condition -Condition $beforeSmoke.success -Message "Baseline distributed smoke did not pass or summary is missing."
Assert-Condition -Condition $oneBrokerDownSmoke.success -Message "One-broker-down distributed smoke did not pass or summary is missing."
Assert-Condition -Condition $restoreSmoke.success -Message "Restore distributed smoke did not pass or summary is missing."
Assert-Condition -Condition $deliveryOneDown.all_replica_count_3 -Message "One-broker-down delivery topic did not keep replica_count=3 for all partitions."
Assert-Condition -Condition $deliveryOneDown.all_isr_count_2 -Message "One-broker-down delivery topic did not reach isr_count=2 for all partitions."
Assert-Condition -Condition $probeOneDownAccepted -Message "One-broker-down producer probe was not accepted."
Assert-Condition -Condition $probeTwoDownRejected -Message "Two-broker-down producer probe was not rejected with NOT_ENOUGH_REPLICAS."

$summary = [pscustomobject]@{
    run_name = [string]$source.run_name
    created_at = [string]$source.completed_at
    summarized_at = (Get-Date).ToUniversalTime().ToString("o")
    git_commit = [string]$source.git_commit
    git_dirty = [bool]$source.git_dirty
    scope = "local Kafka KRaft ISR observation summary; not a production Kafka HA proof"
    source_summary_path = $sourceSummaryPath
    passed = $passed
    before_smoke = $beforeSmoke
    one_broker_down_smoke = $oneBrokerDownSmoke
    restore_smoke = $restoreSmoke
    first_stopped_broker_id = [string]$source.first_stopped_broker_id
    second_stopped_broker_id = [string]$source.second_stopped_broker_id
    remaining_broker_id_after_two_stops = [string]$source.remaining_broker_id_after_two_stops
    delivery_topic_after_one_broker_stop = $deliveryOneDown
    one_broker_down_producer_probe_accepted = $probeOneDownAccepted
    two_broker_down_producer_probe_rejected_not_enough_replicas = $probeTwoDownRejected
}

$summaryFullPath = [System.IO.Path]::GetFullPath($OutputPath)
$summaryDir = Split-Path -Parent $summaryFullPath
if ($summaryDir -and -not (Test-Path -LiteralPath $summaryDir)) {
    New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
}
$summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryFullPath -Encoding UTF8

$markdownFullPath = [System.IO.Path]::GetFullPath($MarkdownPath)
$markdownDir = Split-Path -Parent $markdownFullPath
if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
    New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
}

$markdown = @()
$markdown += "# Kafka ISR Observation Smoke Summary"
$markdown += ""
$markdown += "- Run: $($summary.run_name)"
$markdown += "- Commit: $($summary.git_commit)"
$markdown += "- Git dirty: $($summary.git_dirty)"
$markdown += "- Scope: $($summary.scope)"
$markdown += "- Overall result: $($summary.passed)"
$markdown += "- Baseline smoke: $($summary.before_smoke.success)"
$markdown += "- One broker down: passed"
$markdown += "- Restore smoke: $($summary.restore_smoke.success)"
$markdown += "- Delivery topic partitions after one broker stop: $($summary.delivery_topic_after_one_broker_stop.partition_count)"
$markdown += "- One-broker-down producer probe accepted: $($summary.one_broker_down_producer_probe_accepted)"
$markdown += "- Two-broker-down write rejected with NOT_ENOUGH_REPLICAS: $($summary.two_broker_down_producer_probe_rejected_not_enough_replicas)"
$markdown += ""
$markdown += "| Partition | Leader | Replicas | ISR |"
$markdown += "| ---: | ---: | --- | --- |"
foreach ($row in $summary.delivery_topic_after_one_broker_stop.rows) {
    $replicas = @(Convert-ToArray -Value $row.replicas) -join ","
    $isr = @(Convert-ToArray -Value $row.isr) -join ","
    $markdown += "| $($row.partition) | $($row.leader) | $replicas | $isr |"
}
$markdown += ""
$markdown += "This summary validates a local one-broker-down ISR observation and a two-broker-down NOT_ENOUGH_REPLICAS boundary. It is not a production Kafka HA proof, capacity benchmark, or sustained flapping test."

$markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

Write-Host "OK   Kafka ISR observation summary written: $summaryFullPath"
Write-Host "OK   Kafka ISR observation markdown written: $markdownFullPath"
