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
    $passed = (
        [string]$snapshot.state -eq "Stable" -and
        [int]$snapshot.member_count -eq $expectedMembers -and
        $consumerIDs.Count -eq $expectedMembers -and
        [int]$snapshot.assigned_partition_count -eq 3
    )
    Assert-Condition -Condition $passed -Message "Consumer churn transition failed validation: cycle=$($transition.cycle) action=$($transition.action)"
    $transitionSummaries += [pscustomobject]@{
        cycle = [int]$transition.cycle
        action = [string]$transition.action
        expected_members = $expectedMembers
        member_count = [int]$snapshot.member_count
        assigned_partition_count = [int]$snapshot.assigned_partition_count
        passed = $passed
    }
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
$markdown += ""
$markdown += "| Cycle | Action | Expected members | Assigned partitions |"
$markdown += "| ---: | --- | ---: | ---: |"
foreach ($transition in $summary.transitions) {
    $markdown += "| $($transition.cycle) | $($transition.action) | $($transition.member_count) | $($transition.assigned_partition_count) |"
}
$markdown += ""
$markdown += "This validates a local push-gateway delivery-consumer churn observation: consumers repeatedly leave and rejoin the same group, and Kafka returns the group to Stable with all three partitions assigned after every transition. It is not a production rebalance storm SLO, capacity, or long-duration partition churn proof."

$markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

Write-Host "OK   Kafka consumer churn summary written: $summaryFullPath"
Write-Host "OK   Kafka consumer churn markdown written: $markdownFullPath"
