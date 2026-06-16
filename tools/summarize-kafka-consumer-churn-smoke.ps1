param(
    [Parameter(Mandatory = $true)]
    [string]$RunDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$runPath = [System.IO.Path]::GetFullPath($RunDir)
if (-not (Test-Path -LiteralPath $runPath -PathType Container)) {
    throw "Kafka consumer churn run directory does not exist: $runPath"
}
if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $runPath "kafka-consumer-churn-report-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $runPath "kafka-consumer-churn-report.md"
}

$sourceSummaryPath = Join-Path $runPath "kafka-consumer-churn-summary.json"
if (-not (Test-Path -LiteralPath $sourceSummaryPath -PathType Leaf)) {
    throw "Kafka consumer churn summary is missing: $sourceSummaryPath"
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
$initial = $source.initial
$transitions = @(Convert-ToArray -Value $source.transitions)
$probeBatches = @(Convert-ToArray -Value $source.probe_batches)
$probeMessagesPerTransition = [int]$source.probe_messages_per_transition

Assert-Condition -Condition ([bool]$source.git_dirty -eq $false) -Message "Kafka consumer churn smoke must be run from a clean worktree."
Assert-Condition -Condition ([string]$source.topic -eq "im.delivery.events") -Message "Kafka consumer churn smoke must target im.delivery.events."
Assert-Condition -Condition ([string]$initial.state -eq "Stable") -Message "Initial consumer group state must be Stable."
Assert-Condition -Condition ([int]$initial.member_count -eq 2) -Message "Initial consumer group must have two members."
Assert-Condition -Condition ([int]$initial.assigned_partition_count -eq 3) -Message "Initial consumer group must assign all three partitions."
Assert-Condition -Condition ($transitions.Count -gt 0) -Message "Consumer churn smoke has no transitions."

$transitionSummaries = @()
foreach ($transition in $transitions) {
    $snapshot = $transition.snapshot
    $expectedMembers = [int]$transition.expected_members
    $consumerIDs = @(Convert-ToArray -Value $snapshot.consumer_ids)
    $postProbeSnapshot = $transition.post_probe_snapshot
    $probe = $transition.probe
    $passed = (
        [string]$snapshot.state -eq "Stable" -and
        [int]$snapshot.member_count -eq $expectedMembers -and
        $consumerIDs.Count -eq $expectedMembers -and
        [int]$snapshot.assigned_partition_count -eq 3
    )
    Assert-Condition -Condition $passed -Message "Consumer churn transition failed validation: cycle=$($transition.cycle) action=$($transition.action)"
    if ($probeMessagesPerTransition -gt 0) {
        Assert-Condition -Condition ($null -ne $probe) -Message "Consumer churn transition is missing probe summary: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([int]$probe.Attempted -eq $probeMessagesPerTransition) -Message "Probe attempted count mismatch: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([int]$probe.Acked -eq $probeMessagesPerTransition) -Message "Probe acked count mismatch: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([int]$probe.Failed -eq 0) -Message "Probe had failed writes: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ($null -ne $postProbeSnapshot) -Message "Consumer churn transition is missing post-probe snapshot: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([string]$postProbeSnapshot.state -eq "Stable") -Message "Post-probe consumer group state must be Stable: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([int]$postProbeSnapshot.member_count -eq $expectedMembers) -Message "Post-probe member count mismatch: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([int]$postProbeSnapshot.assigned_partition_count -eq 3) -Message "Post-probe assigned partition count mismatch: cycle=$($transition.cycle) action=$($transition.action)"
        Assert-Condition -Condition ([int64]$postProbeSnapshot.total_lag -eq 0) -Message "Post-probe consumer lag must be zero: cycle=$($transition.cycle) action=$($transition.action)"
    }
    $transitionSummaries += [pscustomobject]@{
        cycle = [int]$transition.cycle
        action = [string]$transition.action
        expected_members = $expectedMembers
        member_count = [int]$snapshot.member_count
        assigned_partition_count = [int]$snapshot.assigned_partition_count
        total_lag = [int64]$snapshot.total_lag
        probe_attempted = if ($null -ne $probe) { [int]$probe.Attempted } else { 0 }
        probe_acked = if ($null -ne $probe) { [int]$probe.Acked } else { 0 }
        post_probe_lag = if ($null -ne $postProbeSnapshot) { [int64]$postProbeSnapshot.total_lag } else { 0 }
        passed = $passed
    }
}

if ($probeMessagesPerTransition -gt 0) {
    Assert-Condition -Condition ($probeBatches.Count -eq $transitionSummaries.Count) -Message "Probe batch count must match transition count."
}

$summary = [pscustomobject]@{
    run_name = [string]$source.run_name
    created_at = [string]$source.completed_at
    summarized_at = (Get-Date).ToUniversalTime().ToString("o")
    git_commit = [string]$source.git_commit
    git_dirty = [bool]$source.git_dirty
    scope = "local Kafka consumer group churn observation; not a production rebalance storm SLO proof"
    source_summary_path = $sourceSummaryPath
    passed = $true
    topic = [string]$source.topic
    consumer_group = [string]$source.consumer_group
    churn_cycles = [int]$source.churn_cycles
    transition_count = $transitionSummaries.Count
    probe_messages_per_transition = $probeMessagesPerTransition
    probe_batch_count = $probeBatches.Count
    probe_attempted = (($transitionSummaries | Measure-Object -Property probe_attempted -Sum).Sum)
    probe_acked = (($transitionSummaries | Measure-Object -Property probe_acked -Sum).Sum)
    initial_member_count = [int]$initial.member_count
    initial_assigned_partition_count = [int]$initial.assigned_partition_count
    transitions = $transitionSummaries
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
$markdown += "# Kafka Consumer Churn Smoke Summary"
$markdown += ""
$markdown += "- Run: $($summary.run_name)"
$markdown += "- Commit: $($summary.git_commit)"
$markdown += "- Git dirty: $($summary.git_dirty)"
$markdown += "- Scope: $($summary.scope)"
$markdown += "- Result: passed"
$markdown += "- Topic: $($summary.topic)"
$markdown += "- Consumer group: $($summary.consumer_group)"
$markdown += "- Churn cycles: $($summary.churn_cycles)"
$markdown += "- Transition count: $($summary.transition_count)"
$markdown += "- Probe messages per transition: $($summary.probe_messages_per_transition)"
$markdown += "- Probe writes acked: $($summary.probe_acked) / $($summary.probe_attempted)"
$markdown += ""
$markdown += "| Cycle | Action | Expected members | Assigned partitions | Probe acked | Post-probe lag |"
$markdown += "| ---: | --- | ---: | ---: | ---: | ---: |"
foreach ($transition in $summary.transitions) {
    $markdown += "| $($transition.cycle) | $($transition.action) | $($transition.member_count) | $($transition.assigned_partition_count) | $($transition.probe_acked) | $($transition.post_probe_lag) |"
}
$markdown += ""
$markdown += "This validates a local push-gateway delivery-consumer churn observation: consumers repeatedly leave and rejoin the same group, Kafka returns the group to Stable with all three partitions assigned after every transition, and optional probe delivery events are consumed to zero lag after each transition. It is not a production rebalance storm SLO, capacity, or long-duration partition churn proof."

$markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

Write-Host "OK   Kafka consumer churn summary written: $summaryFullPath"
Write-Host "OK   Kafka consumer churn markdown written: $markdownFullPath"
