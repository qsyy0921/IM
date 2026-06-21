$ErrorActionPreference = "Stop"

$checker = Join-Path $PSScriptRoot "check-kafka-producer-config.ps1"
if (-not (Test-Path -LiteralPath $checker -PathType Leaf)) {
    throw "Missing Kafka producer config checker: $checker"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-producer-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $summaryPath = Join-Path $tempRoot "kafka-producer-summary.json"
    $markdownPath = Join-Path $tempRoot "kafka-producer-summary.md"

    & $checker -SummaryPath $summaryPath -MarkdownPath $markdownPath
    if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
        throw "Kafka producer summary JSON was not written."
    }
    if (-not (Test-Path -LiteralPath $markdownPath -PathType Leaf)) {
        throw "Kafka producer summary markdown was not written."
    }

    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    $expectedServices = @(
        "admin-service",
        "agent-service",
        "contacts-service",
        "delivery-service",
        "identity-service",
        "knowledge-ingestion-service",
        "media-service",
        "message-service",
        "notification-service",
        "policy-service",
        "receipt-service",
        "vector-index-service"
    )
    if ($summary.producer_count -ne $expectedServices.Count) {
        throw "Kafka producer summary must include $($expectedServices.Count) producer files."
    }
    if ($summary.required_acks -ne "RequireAll" -or $summary.allow_auto_topic_creation -ne $false) {
        throw "Kafka producer summary has incorrect acks or auto-topic settings."
    }
    if ($summary.max_attempts -ne 5 -or $summary.write_backoff_min -ne "100ms" -or $summary.write_backoff_max -ne "1s") {
        throw "Kafka producer summary has incorrect retry/backoff settings."
    }
    if ($summary.idempotent_producer_flag_supported -ne $false -or $summary.scope -notmatch "not proof of idempotent") {
        throw "Kafka producer summary must preserve the idempotent producer boundary."
    }

    $services = @($summary.producers | ForEach-Object { [string]$_.service } | Sort-Object)
    $diff = Compare-Object -ReferenceObject $expectedServices -DifferenceObject $services
    if ($diff) {
        $diffText = ($diff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
        throw "Kafka producer summary service list mismatch: $diffText"
    }

    foreach ($row in $summary.producers) {
        if ($row.idempotent_producer_flag_supported -ne $false -or $row.idempotency_boundary -notmatch "outbox/event_id") {
            throw "Kafka producer summary row missing idempotency boundary."
        }
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("# Kafka Producer Config Snapshot") -or -not $markdown.Contains("Exactly-once")) {
        throw "Kafka producer summary markdown missing expected boundary text."
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   Kafka producer config summary self-test"
