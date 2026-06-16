param(
    [Parameter(Mandatory = $true)]
    [string]$RunDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$runPath = [System.IO.Path]::GetFullPath($RunDir)
if (-not (Test-Path -LiteralPath $runPath -PathType Container)) {
    throw "Kafka ISR flapping run directory does not exist: $runPath"
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $runPath "kafka-isr-flapping-report-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $runPath "kafka-isr-flapping-report.md"
}

$sourceSummaryPath = Join-Path $runPath "kafka-isr-flapping-summary.json"
if (-not (Test-Path -LiteralPath $sourceSummaryPath -PathType Leaf)) {
    throw "Kafka ISR flapping summary is missing: $sourceSummaryPath"
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

function Test-TopicState {
    param(
        [object[]]$States,
        [int]$ExpectedISRCount,
        [string]$BrokerID,
        [string]$BrokerExpectation
    )

    $rows = @(Convert-ToArray -Value $States)
    $badRows = @($rows | Where-Object {
        $brokerInISR = @($_.isr) -contains $BrokerID
        $brokerCheckFailed = (
            ($BrokerExpectation -eq "absent" -and $brokerInISR) -or
            ($BrokerExpectation -eq "present" -and -not $brokerInISR)
        )
        [int]$_.replica_count -ne 3 -or [int]$_.isr_count -ne $ExpectedISRCount -or $brokerCheckFailed
    })
    return [pscustomobject]@{
        partition_count = $rows.Count
        expected_isr_count = $ExpectedISRCount
        broker_expectation = $BrokerExpectation
        passed = ($rows.Count -gt 0 -and $badRows.Count -eq 0)
        rows = $rows
    }
}

function Test-ProbeAccepted {
    param([object]$Probe)
    return ($null -ne $Probe -and [bool]$Probe.accepted -and -not [bool]$Probe.contains_not_enough_replicas)
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
$cycles = @(Convert-ToArray -Value $source.cycles)
Assert-Condition -Condition ($cycles.Count -ge 1) -Message "Kafka ISR flapping summary has no cycles."

$cycleSummaries = @()
$allPassed = $true
foreach ($cycle in $cycles) {
    $brokerID = [string]$cycle.stopped_broker_id
    $degradedState = Test-TopicState -States (Convert-ToArray -Value $cycle.degraded_topic_state) -ExpectedISRCount 2 -BrokerID $brokerID -BrokerExpectation "absent"
    $restoredState = Test-TopicState -States (Convert-ToArray -Value $cycle.restored_topic_state) -ExpectedISRCount 3 -BrokerID $brokerID -BrokerExpectation "present"
    $degradedProbeAccepted = Test-ProbeAccepted -Probe $cycle.degraded_probe
    $restoredProbeAccepted = Test-ProbeAccepted -Probe $cycle.restored_probe
    $cyclePassed = $degradedState.passed -and $restoredState.passed -and $degradedProbeAccepted -and $restoredProbeAccepted
    $allPassed = $allPassed -and $cyclePassed

    $cycleSummaries += [pscustomobject]@{
        cycle = [int]$cycle.cycle
        stopped_broker_id = $brokerID
        degraded_state = $degradedState
        degraded_probe_accepted = $degradedProbeAccepted
        restored_state = $restoredState
        restored_probe_accepted = $restoredProbeAccepted
        passed = $cyclePassed
    }
}

Assert-Condition -Condition $allPassed -Message "At least one Kafka ISR flapping cycle failed validation."

$summary = [pscustomobject]@{
    run_name = [string]$source.run_name
    created_at = [string]$source.completed_at
    summarized_at = (Get-Date).ToUniversalTime().ToString("o")
    git_commit = [string]$source.git_commit
    git_dirty = [bool]$source.git_dirty
    scope = "local Kafka KRaft repeated ISR flapping observation; not a production Kafka HA or rebalance-storm proof"
    source_summary_path = $sourceSummaryPath
    passed = $allPassed
    flap_cycles = $cycles.Count
    flapped_broker_id = [string]$source.flapped_broker_id
    topic_replication_factor = [int]$source.topic_replication_factor
    topic_min_insync_replicas = [int]$source.topic_min_insync_replicas
    cycles = $cycleSummaries
}

$summaryFullPath = [System.IO.Path]::GetFullPath($OutputPath)
$summaryDir = Split-Path -Parent $summaryFullPath
if ($summaryDir -and -not (Test-Path -LiteralPath $summaryDir)) {
    New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
}
$summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $summaryFullPath -Encoding UTF8

$markdownFullPath = [System.IO.Path]::GetFullPath($MarkdownPath)
$markdownDir = Split-Path -Parent $markdownFullPath
if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
    New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
}

$markdown = @()
$markdown += "# Kafka ISR Flapping Smoke Summary"
$markdown += ""
$markdown += "- Run: $($summary.run_name)"
$markdown += "- Commit: $($summary.git_commit)"
$markdown += "- Git dirty: $($summary.git_dirty)"
$markdown += "- Scope: $($summary.scope)"
$markdown += "- Overall result: $($summary.passed)"
$markdown += "- Flap cycles: $($summary.flap_cycles)"
$markdown += "- Flapped broker id: $($summary.flapped_broker_id)"
$markdown += "- Topic RF / min ISR: $($summary.topic_replication_factor) / $($summary.topic_min_insync_replicas)"
$markdown += ""
$markdown += "| Cycle | Degraded ISR OK | Degraded produce OK | Restored ISR OK | Restored produce OK |"
$markdown += "| ---: | --- | --- | --- | --- |"
foreach ($cycle in $summary.cycles) {
    $markdown += "| $($cycle.cycle) | $($cycle.degraded_state.passed) | $($cycle.degraded_probe_accepted) | $($cycle.restored_state.passed) | $($cycle.restored_probe_accepted) |"
}
$markdown += ""
$markdown += "This summary validates repeated local broker stop/start ISR shrink-and-restore behavior for a replicated probe topic. It is not a production Kafka HA proof, capacity benchmark, rebalance storm test, disk-loss test, or exactly-once producer proof."

$markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

Write-Host "OK   Kafka ISR flapping summary written: $summaryFullPath"
Write-Host "OK   Kafka ISR flapping markdown written: $markdownFullPath"
