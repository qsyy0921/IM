$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"

$expectedProducerFiles = @(
    "services\contacts-service\internal\infrastructure\kafka\producer.go",
    "services\delivery-service\internal\infrastructure\kafka\producer.go",
    "services\identity-service\internal\infrastructure\kafka\producer.go",
    "services\message-service\internal\infrastructure\kafka\producer.go",
    "services\policy-service\internal\infrastructure\kafka\producer.go",
    "services\receipt-service\internal\infrastructure\kafka\producer.go"
)

$requiredSnippets = @(
    "RequiredAcks:           kafkago.RequireAll",
    "AllowAutoTopicCreation: false",
    "kafkaProducerMaxAttempts     = 5",
    "kafkaProducerWriteBackoffMin = 100 * time.Millisecond",
    "kafkaProducerWriteBackoffMax = time.Second",
    "MaxAttempts:            kafkaProducerMaxAttempts",
    "WriteBackoffMin:        kafkaProducerWriteBackoffMin",
    "WriteBackoffMax:        kafkaProducerWriteBackoffMax"
)

$violations = [System.Collections.Generic.List[string]]::new()

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

Write-Host "OK   Kafka producer retry/ack guardrails"
