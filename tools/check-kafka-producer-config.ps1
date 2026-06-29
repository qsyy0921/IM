param(
    [string]$SummaryPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"

$expectedProducerFiles = @(
    "services\admin-service\internal\infrastructure\kafka\producer.go",
    "services\agent-service\internal\infrastructure\kafka\producer.go",
    "services\contacts-service\internal\infrastructure\kafka\producer.go",
    "services\delivery-service\internal\infrastructure\kafka\producer.go",
    "services\identity-service\internal\infrastructure\kafka\producer.go",
    "services\knowledge-ingestion-service\internal\infrastructure\kafka\producer.go",
    "services\media-service\internal\infrastructure\kafka\producer.go",
    "services\message-service\internal\infrastructure\kafka\producer.go",
    "services\notification-service\internal\infrastructure\kafka\producer.go",
    "services\policy-service\internal\infrastructure\kafka\producer.go",
    "services\receipt-service\internal\infrastructure\kafka\producer.go",
    "services\vector-index-service\internal\infrastructure\kafka\producer.go",
    "services\workflow-service\internal\infrastructure\kafka\producer.go"
)

$requiredSnippets = @(
    "RequiredAcks:           kafkago.RequireAll",
    "AllowAutoTopicCreation: false",
    "kafkaProducerMaxAttempts     = 5",
    "kafkaProducerWriteBackoffMin = 100 * time.Millisecond",
    "kafkaProducerWriteBackoffMax = time.Second",
    "MaxAttempts:            kafkaProducerMaxAttempts",
    "WriteBackoffMin:        kafkaProducerWriteBackoffMin",
    "WriteBackoffMax:        kafkaProducerWriteBackoffMax",
    "kafka-go does not expose Kafka's enable.idempotence producer flag",
    "Production hardening must revisit the client"
)

$violations = [System.Collections.Generic.List[string]]::new()
$producerRows = @()

foreach ($relativePath in $expectedProducerFiles) {
    $path = Join-Path $repoRoot $relativePath
    if (-not (Test-Path -LiteralPath $path)) {
        $violations.Add("${relativePath}: missing expected Kafka producer")
        continue
    }
    $content = Get-Content -LiteralPath $path -Raw
    foreach ($snippet in $requiredSnippets) {
        if (-not $content.Contains($snippet)) {
            $violations.Add("${relativePath}: missing required snippet [$snippet]")
        }
    }
    $hasLiteralBatchSize = $content.Contains("BatchSize:              100")
    $hasConfiguredBatchSize = $content.Contains("config.BatchSize = 100") -and $content.Contains("BatchSize:              config.BatchSize")
    if (-not ($hasLiteralBatchSize -or $hasConfiguredBatchSize)) {
        $violations.Add("${relativePath}: missing batch size guardrail [100 default or explicit config default]")
    }
    $hasLiteralBatchTimeout = $content.Contains("BatchTimeout:           10 * time.Millisecond")
    $hasConfiguredBatchTimeout = $content.Contains("config.BatchTimeout = 10 * time.Millisecond") -and $content.Contains("BatchTimeout:           config.BatchTimeout")
    if (-not ($hasLiteralBatchTimeout -or $hasConfiguredBatchTimeout)) {
        $violations.Add("${relativePath}: missing batch timeout guardrail [10ms default or explicit config default]")
    }

    if ($content.Length -gt 0) {
        $service = if ($relativePath -match "^services\\([^\\]+)\\") { $Matches[1] } else { "" }
        $producerRows += [pscustomobject]@{
            service = $service
            path = $relativePath
            required_acks = "RequireAll"
            allow_auto_topic_creation = $false
            max_attempts = 5
            write_backoff_min = "100ms"
            write_backoff_max = "1s"
            batch_size = 100
            batch_timeout = "10ms"
            idempotent_producer_flag_supported = $false
            idempotency_boundary = "outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer"
        }
    }
}

$actualProducerFiles = Get-ChildItem -LiteralPath $servicesRoot -Recurse -Filter "producer.go" -File |
    Where-Object { $_.FullName -match "\\internal\\infrastructure\\kafka\\producer\.go$" } |
    ForEach-Object {
        $_.FullName.Substring($repoRoot.Length + 1)
    } |
    Sort-Object

$expectedSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($relativePath in $expectedProducerFiles) {
    [void]$expectedSet.Add($relativePath)
}
foreach ($relativePath in $actualProducerFiles) {
    if (-not $expectedSet.Contains($relativePath)) {
        $violations.Add("${relativePath}: new Kafka producer must be added to check-kafka-producer-config.ps1")
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

if ($SummaryPath.Trim().Length -gt 0 -or $MarkdownPath.Trim().Length -gt 0) {
    $producerRows = @($producerRows | Sort-Object service)
    $summary = [pscustomobject]@{
        created_at = (Get-Date).ToUniversalTime().ToString("o")
        scope = "Kafka producer first-stage config snapshot; not proof of idempotent or transactional producer semantics"
        producer_count = $producerRows.Count
        required_acks = "RequireAll"
        allow_auto_topic_creation = $false
        max_attempts = 5
        write_backoff_min = "100ms"
        write_backoff_max = "1s"
        batch_size = 100
        batch_timeout = "10ms"
        idempotent_producer_flag_supported = $false
        producers = $producerRows
    }

    if ($SummaryPath.Trim().Length -gt 0) {
        $summaryFullPath = [System.IO.Path]::GetFullPath($SummaryPath)
        $summaryDir = Split-Path -Parent $summaryFullPath
        if ($summaryDir -and -not (Test-Path -LiteralPath $summaryDir)) {
            New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
        }
        $summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryFullPath -Encoding UTF8
        Write-Host "OK   Kafka producer config summary written: $summaryFullPath"
    }

    if ($MarkdownPath.Trim().Length -gt 0) {
        $markdownFullPath = [System.IO.Path]::GetFullPath($MarkdownPath)
        $markdownDir = Split-Path -Parent $markdownFullPath
        if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
            New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
        }

        $markdown = @()
        $markdown += "# Kafka Producer Config Snapshot"
        $markdown += ""
        $markdown += "- Created at: $($summary.created_at)"
        $markdown += "- Scope: $($summary.scope)"
        $markdown += "- Producers: $($summary.producer_count)"
        $markdown += "- Required acks: $($summary.required_acks)"
        $markdown += "- Auto topic creation: disabled"
        $markdown += "- Attempts/backoff: $($summary.max_attempts) attempts, $($summary.write_backoff_min)-$($summary.write_backoff_max)"
        $markdown += "- Idempotent producer flag supported: false"
        $markdown += ""
        $markdown += "| Service | Path | Batch | Boundary |"
        $markdown += "| --- | --- | ---: | --- |"
        foreach ($row in $producerRows) {
            $markdown += "| $($row.service) | $($row.path) | $($row.batch_size) / $($row.batch_timeout) | $($row.idempotency_boundary) |"
        }
        $markdown += ""
        $markdown += "This snapshot proves current first-stage producer guardrails only. Exactly-once, idempotent, or transactional Kafka producer semantics remain a separate production hardening decision."

        $markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8
        Write-Host "OK   Kafka producer config markdown written: $markdownFullPath"
    }
}

Write-Host "OK   Kafka producer retry/ack guardrails"
