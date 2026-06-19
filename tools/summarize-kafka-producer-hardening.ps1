param(
    [Parameter(Mandatory = $true)]
    [string]$ISRRunDir,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$isrRunPath = [System.IO.Path]::GetFullPath($ISRRunDir)
if (-not (Test-Path -LiteralPath $isrRunPath -PathType Container)) {
    throw "Kafka ISR observation run directory does not exist: $isrRunPath"
}

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $isrRunPath "kafka-producer-hardening-summary.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $isrRunPath "kafka-producer-hardening-summary.md"
}

$producerChecker = Join-Path $PSScriptRoot "check-kafka-producer-config.ps1"
$isrSummarizer = Join-Path $PSScriptRoot "summarize-kafka-isr-observation-smoke.ps1"
if (-not (Test-Path -LiteralPath $producerChecker -PathType Leaf)) {
    throw "Missing Kafka producer config checker: $producerChecker"
}
if (-not (Test-Path -LiteralPath $isrSummarizer -PathType Leaf)) {
    throw "Missing Kafka ISR observation summarizer: $isrSummarizer"
}

function Read-JsonFile {
    param([string]$Path)

    return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-producer-hardening-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $producerSummaryPath = Join-Path $tempRoot "producer-config-summary.json"
    $producerMarkdownPath = Join-Path $tempRoot "producer-config-summary.md"
    $isrSummaryPath = Join-Path $tempRoot "isr-summary.json"
    $isrMarkdownPath = Join-Path $tempRoot "isr-summary.md"

    & $producerChecker -SummaryPath $producerSummaryPath -MarkdownPath $producerMarkdownPath
    & $isrSummarizer -RunDir $isrRunPath -OutputPath $isrSummaryPath -MarkdownPath $isrMarkdownPath

    $producer = Read-JsonFile -Path $producerSummaryPath
    $isr = Read-JsonFile -Path $isrSummaryPath

    Assert-Condition -Condition ($producer.producer_count -eq 7) -Message "Kafka producer config must cover 7 producer packages."
    Assert-Condition -Condition ($producer.required_acks -eq "RequireAll") -Message "Kafka producer config must require acks=all."
    Assert-Condition -Condition ($producer.allow_auto_topic_creation -eq $false) -Message "Kafka producer config must disable auto topic creation."
    Assert-Condition -Condition ($producer.max_attempts -eq 5) -Message "Kafka producer config must keep bounded MaxAttempts=5."
    Assert-Condition -Condition ($producer.write_backoff_min -eq "100ms" -and $producer.write_backoff_max -eq "1s") -Message "Kafka producer config must keep bounded write backoff."
    Assert-Condition -Condition ($producer.idempotent_producer_flag_supported -eq $false) -Message "Current kafka-go producer must not be reported as idempotent."
    Assert-Condition -Condition ([bool]$isr.passed) -Message "Kafka ISR observation summary must pass."
    Assert-Condition -Condition ([bool]$isr.one_broker_down_producer_probe_accepted) -Message "One-broker-down producer probe must be accepted."
    Assert-Condition -Condition ([bool]$isr.two_broker_down_producer_probe_rejected_not_enough_replicas) -Message "Two-broker-down producer probe must fail with NOT_ENOUGH_REPLICAS."

    $assessment = [pscustomobject]@{
        client = "segmentio/kafka-go"
        idempotent_producer_flag_supported = $false
        kafka_transactions_used = $false
        exactly_once_claimed = $false
        first_stage_boundary = "acks=all + bounded retry/backoff + outbox/event_id idempotency; no Kafka transactional producer"
        production_decision_needed = "Evaluate a producer client with explicit idempotence/transactions before claiming exactly-once producer semantics."
    }

    $summary = [pscustomobject]@{
        created_at = (Get-Date).ToUniversalTime().ToString("o")
        scope = "Kafka producer hardening evaluation; combines static producer guardrails with a local ISR fault observation"
        passed = $true
        producer_config = $producer
        isr_observation = $isr
        idempotence_assessment = $assessment
    }

    $summaryFullPath = [System.IO.Path]::GetFullPath($OutputPath)
    $summaryDir = Split-Path -Parent $summaryFullPath
    if ($summaryDir -and -not (Test-Path -LiteralPath $summaryDir)) {
        New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
    }
    $summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $summaryFullPath -Encoding UTF8

    $markdownFullPath = [System.IO.Path]::GetFullPath($MarkdownPath)
    $markdownDir = Split-Path -Parent $markdownFullPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $markdown = @()
    $markdown += "# Kafka Producer Hardening Evaluation"
    $markdown += ""
    $markdown += "- Created at: $($summary.created_at)"
    $markdown += "- Scope: $($summary.scope)"
    $markdown += "- Result: passed"
    $markdown += "- Producer packages covered: $($producer.producer_count)"
    $markdown += "- Producer guardrails: ``acks=all``, auto topic creation disabled, $($producer.max_attempts) attempts, $($producer.write_backoff_min)-$($producer.write_backoff_max) write backoff"
    $markdown += "- One-broker-down producer probe accepted: $($isr.one_broker_down_producer_probe_accepted)"
    $markdown += "- Two-broker-down write rejected with `NOT_ENOUGH_REPLICAS`: $($isr.two_broker_down_producer_probe_rejected_not_enough_replicas)"
    $markdown += "- Current producer client: $($assessment.client)"
    $markdown += "- Idempotent producer flag supported: false"
    $markdown += "- Exactly-once / transactional producer claimed: false"
    $markdown += ""
    $markdown += "## Producer Packages"
    $markdown += ""
    $markdown += "| Service | Path | Attempts | Backoff | Boundary |"
    $markdown += "| --- | --- | ---: | --- | --- |"
    foreach ($row in $producer.producers) {
        $markdown += "| $($row.service) | $($row.path) | $($row.max_attempts) | $($producer.write_backoff_min)-$($producer.write_backoff_max) | $($row.idempotency_boundary) |"
    }
    $markdown += ""
    $markdown += "## Interpretation"
    $markdown += ""
    $markdown += "This evaluation proves the current first-stage Kafka producer guardrails and local ISR fault boundary. It does not prove idempotent, exactly-once, or transactional Kafka producer semantics. The reliable business boundary remains outbox rows plus event-id idempotency and downstream idempotent consumers."
    $markdown += ""
    $markdown += "Before claiming production exactly-once Kafka producer behavior, NexusIM must evaluate or adopt a producer client with explicit idempotence and transaction support, then add real broker-fault and duplicate-produce verification for that client."

    $markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8

    Write-Host "OK   Kafka producer hardening summary written: $summaryFullPath"
    Write-Host "OK   Kafka producer hardening markdown written: $markdownFullPath"
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}
