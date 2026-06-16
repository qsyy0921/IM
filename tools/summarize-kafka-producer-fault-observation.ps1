param(
    [Parameter(Mandatory = $true)]
    [string]$RunDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$runPath = [System.IO.Path]::GetFullPath($RunDir)
if (-not (Test-Path -LiteralPath $runPath -PathType Container)) {
    throw "Kafka producer fault run directory does not exist: $runPath"
}
if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $runPath "kafka-producer-fault-report-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $runPath "kafka-producer-fault-report.md"
}

$sourceSummaryPath = Join-Path $runPath "kafka-producer-fault-observation-summary.json"
if (-not (Test-Path -LiteralPath $sourceSummaryPath -PathType Leaf)) {
    throw "Kafka producer fault observation summary is missing: $sourceSummaryPath"
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
$missingAcked = @(Convert-ToArray -Value $source.missing_acked_ids)
$unackedObserved = @(Convert-ToArray -Value $source.unacked_observed_ids)

$acked = [int]$source.producer_acked
$attempted = [int]$source.producer_attempted
$consumedUnique = [int]$source.consumed_unique
$duplicateCount = [int]$source.duplicate_count
$missingAckedCount = [int]$source.missing_acked_count
$unackedObservedCount = [int]$source.unacked_observed_count

$passed = (
    $attempted -gt 0 -and
    $acked -gt 0 -and
    $consumedUnique -ge $acked -and
    $missingAckedCount -eq 0
)

Assert-Condition -Condition ($attempted -gt 0) -Message "Producer fault observation attempted zero records."
Assert-Condition -Condition ($acked -gt 0) -Message "Producer fault observation had no acknowledged records."
Assert-Condition -Condition ($missingAckedCount -eq 0) -Message "Producer fault observation missed acknowledged records in consumed topic."

$summary = [pscustomobject]@{
    run_name = [string]$source.run_name
    created_at = [string]$source.completed_at
    summarized_at = (Get-Date).ToUniversalTime().ToString("o")
    git_commit = [string]$source.git_commit
    git_dirty = [bool]$source.git_dirty
    scope = "local kafka-go producer in-flight broker-fault observation; not an exactly-once producer proof"
    source_summary_path = $sourceSummaryPath
    passed = $passed
    topic = [string]$source.topic
    stopped_broker_id = [string]$source.stopped_broker_id
    producer_attempted = $attempted
    producer_acked = $acked
    producer_failed = [int]$source.producer_failed
    consumed_total = [int]$source.consumed_total
    consumed_unique = $consumedUnique
    duplicate_count = $duplicateCount
    missing_acked_count = $missingAckedCount
    unacked_observed_count = $unackedObservedCount
    missing_acked_ids = $missingAcked
    unacked_observed_ids = $unackedObserved
    interpretation = "Acknowledged records were present in the consumed topic. Duplicate and unacked-observed counts are observations, not guarantees."
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
$markdown += "# Kafka Producer Fault Observation Summary"
$markdown += ""
$markdown += "- Run: $($summary.run_name)"
$markdown += "- Commit: $($summary.git_commit)"
$markdown += "- Git dirty: $($summary.git_dirty)"
$markdown += "- Scope: $($summary.scope)"
$markdown += "- Result: $($summary.passed)"
$markdown += "- Topic: $($summary.topic)"
$markdown += "- Stopped broker id: $($summary.stopped_broker_id)"
$markdown += "- Producer attempted / acked / failed: $($summary.producer_attempted) / $($summary.producer_acked) / $($summary.producer_failed)"
$markdown += "- Consumed total / unique: $($summary.consumed_total) / $($summary.consumed_unique)"
$markdown += "- Duplicate count: $($summary.duplicate_count)"
$markdown += "- Missing acknowledged records: $($summary.missing_acked_count)"
$markdown += "- Unacknowledged records observed: $($summary.unacked_observed_count)"
$markdown += ""
$markdown += 'This observation uses the project `kafka-go` writer settings with `acks=all`, bounded retry/backoff, and no idempotent or transactional producer claim. A clean run with zero duplicates does not prove exactly-once behavior; it only records what happened in this local broker-fault window.'

$markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

Write-Host "OK   Kafka producer fault summary written: $summaryFullPath"
Write-Host "OK   Kafka producer fault markdown written: $markdownFullPath"
