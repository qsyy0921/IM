param(
    [Parameter(Mandatory = $true)]
    [string]$RunDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$runPath = [System.IO.Path]::GetFullPath($RunDir)
if (-not (Test-Path -LiteralPath $runPath -PathType Container)) {
    throw "Kafka consumer rebalance run directory does not exist: $runPath"
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $runPath "kafka-consumer-rebalance-report-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $runPath "kafka-consumer-rebalance-report.md"
}

$sourceSummaryPath = Join-Path $runPath "kafka-consumer-rebalance-summary.json"
if (-not (Test-Path -LiteralPath $sourceSummaryPath -PathType Leaf)) {
    throw "Kafka consumer rebalance summary is missing: $sourceSummaryPath"
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
$before = $source.before_stop
$after = $source.after_stop

$beforeConsumers = @(Convert-ToArray -Value $before.consumer_ids)
$afterConsumers = @(Convert-ToArray -Value $after.consumer_ids)

Assert-Condition -Condition ([bool]$source.git_dirty -eq $false) -Message "Kafka consumer rebalance smoke must be run from a clean worktree."
Assert-Condition -Condition ([string]$source.topic -eq "im.delivery.events") -Message "Kafka consumer rebalance smoke must target im.delivery.events."
Assert-Condition -Condition ([string]$before.state -eq "Stable") -Message "Consumer group must be Stable before stopping one consumer."
Assert-Condition -Condition ([int]$before.member_count -eq 2) -Message "Consumer group must have two members before stopping one consumer."
Assert-Condition -Condition ($beforeConsumers.Count -eq 2) -Message "Consumer group must expose two consumer ids before stopping one consumer."
Assert-Condition -Condition ([int]$before.assigned_partition_count -ge 2) -Message "Consumer group must assign partitions before stopping one consumer."
Assert-Condition -Condition ([string]$after.state -eq "Stable") -Message "Consumer group must be Stable after stopping one consumer."
Assert-Condition -Condition ([int]$after.member_count -eq 1) -Message "Consumer group must have one member after stopping one consumer."
Assert-Condition -Condition ($afterConsumers.Count -eq 1) -Message "Consumer group must expose one consumer id after stopping one consumer."
Assert-Condition -Condition ([int]$after.assigned_partition_count -ge [int]$before.assigned_partition_count) -Message "Remaining consumer must own the assigned partitions after rebalance."

$summary = [pscustomobject]@{
    run_name = [string]$source.run_name
    created_at = [string]$source.completed_at
    summarized_at = (Get-Date).ToUniversalTime().ToString("o")
    git_commit = [string]$source.git_commit
    git_dirty = [bool]$source.git_dirty
    scope = "local Kafka consumer group rebalance observation; not a production rebalance SLO proof"
    source_summary_path = $sourceSummaryPath
    passed = $true
    topic = [string]$source.topic
    consumer_group = [string]$source.consumer_group
    before_stop = $before
    after_stop = $after
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
$markdown += "# Kafka Consumer Rebalance Smoke Summary"
$markdown += ""
$markdown += "- Run: $($summary.run_name)"
$markdown += "- Commit: $($summary.git_commit)"
$markdown += "- Git dirty: $($summary.git_dirty)"
$markdown += "- Scope: $($summary.scope)"
$markdown += "- Result: passed"
$markdown += "- Topic: $($summary.topic)"
$markdown += "- Consumer group: $($summary.consumer_group)"
$markdown += "- Before stop: state=$($before.state), members=$($before.member_count), assigned_partitions=$($before.assigned_partition_count)"
$markdown += "- After stop: state=$($after.state), members=$($after.member_count), assigned_partitions=$($after.assigned_partition_count)"
$markdown += ""
$markdown += "## Interpretation"
$markdown += ""
$markdown += "This validates a local push-gateway delivery-consumer group rebalance observation: two consumers join the same group, one process is stopped, and Kafka reassigns the delivery topic partitions to the remaining consumer. It is not a sustained rebalance storm, capacity, or production SLO proof."

$markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

Write-Host "OK   Kafka consumer rebalance summary written: $summaryFullPath"
Write-Host "OK   Kafka consumer rebalance markdown written: $markdownFullPath"
