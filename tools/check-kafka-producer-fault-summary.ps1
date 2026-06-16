$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-kafka-producer-fault-observation.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing Kafka producer fault summarizer: $summarizer"
}

function Write-KafkaProducerFaultFixture {
    param(
        [string]$Directory,
        [bool]$MissingAcked = $false
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $missingIDs = if ($MissingAcked) { @("fault-selftest-000002") } else { @() }
    [ordered]@{
        run_name = "kafka-producer-fault-selftest"
        git_commit = "selftest"
        git_dirty = $false
        completed_at = "2026-06-16T00:00:00Z"
        scope = "local kafka-go producer in-flight broker-fault observation; not an exactly-once proof"
        topic = "nexusim.kafka.producer.fault.selftest"
        stopped_broker_id = "1"
        producer_attempted = 3
        producer_acked = 3
        producer_failed = 0
        consumed_total = if ($MissingAcked) { 2 } else { 4 }
        consumed_unique = if ($MissingAcked) { 2 } else { 3 }
        duplicate_count = if ($MissingAcked) { 0 } else { 1 }
        missing_acked_count = $missingIDs.Count
        missing_acked_ids = $missingIDs
        unacked_observed_count = 0
        unacked_observed_ids = @()
    } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Directory "kafka-producer-fault-observation-summary.json") -Encoding UTF8
}

function Invoke-Summarizer {
    param(
        [string]$RunDir,
        [string]$OutputPath,
        [string]$MarkdownPath
    )

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $summarizer `
            -RunDir $RunDir `
            -OutputPath $OutputPath `
            -MarkdownPath $MarkdownPath 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
            Output = (($output | Out-String).Trim())
        }
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-producer-fault-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodDir = Join-Path $tempRoot "good"
    Write-KafkaProducerFaultFixture -Directory $goodDir -MissingAcked $false
    $jsonPath = Join-Path $goodDir "summary.json"
    $markdownPath = Join-Path $goodDir "summary.md"
    $goodResult = Invoke-Summarizer -RunDir $goodDir -OutputPath $jsonPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL Kafka producer fault summary fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if (-not $summary.passed -or $summary.duplicate_count -ne 1 -or $summary.missing_acked_count -ne 0) {
        Write-Host "FAIL Kafka producer fault summary produced wrong result flags." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Kafka Producer Fault Observation Summary") -or -not $markdown.Contains("does not prove exactly-once")) {
        Write-Host "FAIL Kafka producer fault markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badDir = Join-Path $tempRoot "bad"
    Write-KafkaProducerFaultFixture -Directory $badDir -MissingAcked $true
    $badResult = Invoke-Summarizer -RunDir $badDir -OutputPath (Join-Path $badDir "summary.json") -MarkdownPath (Join-Path $badDir "summary.md")
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL Kafka producer fault fixture with missing acked records should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("missed acknowledged records")) {
        Write-Host "FAIL Kafka producer fault bad fixture did not report missing acked records." -ForegroundColor Red
        if ($badResult.Output) {
            Write-Host $badResult.Output -ForegroundColor Red
        }
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   Kafka producer fault summary self-test"
