$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-kafka-consumer-rebalance-smoke.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing Kafka consumer rebalance summarizer: $summarizer"
}

function Write-KafkaConsumerRebalanceFixture {
    param(
        [string]$Directory,
        [bool]$AfterStopSingleMember = $true
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $summary = [ordered]@{
        run_name = "kafka-consumer-rebalance-selftest"
        git_commit = "selftest"
        git_dirty = $false
        completed_at = "2026-06-16T00:00:00Z"
        topic = "im.delivery.events"
        consumer_group = "nexusim-push-rebalance-selftest"
        before_stop = [ordered]@{
            state = "Stable"
            member_count = 2
            consumer_ids = @("consumer-a", "consumer-b")
            assigned_partition_count = 3
            assignments = @(
                [ordered]@{ partition = 0; consumer_id = "consumer-a" },
                [ordered]@{ partition = 1; consumer_id = "consumer-b" },
                [ordered]@{ partition = 2; consumer_id = "consumer-a" }
            )
        }
        after_stop = [ordered]@{
            state = "Stable"
            member_count = if ($AfterStopSingleMember) { 1 } else { 2 }
            consumer_ids = if ($AfterStopSingleMember) { @("consumer-b") } else { @("consumer-a", "consumer-b") }
            assigned_partition_count = 3
            assignments = @(
                [ordered]@{ partition = 0; consumer_id = "consumer-b" },
                [ordered]@{ partition = 1; consumer_id = "consumer-b" },
                [ordered]@{ partition = 2; consumer_id = "consumer-b" }
            )
        }
    }
    $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Directory "kafka-consumer-rebalance-summary.json") -Encoding UTF8
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-consumer-rebalance-check-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodDir = Join-Path $tempRoot "good"
    Write-KafkaConsumerRebalanceFixture -Directory $goodDir -AfterStopSingleMember $true
    $jsonPath = Join-Path $goodDir "rebalance-summary.json"
    $markdownPath = Join-Path $goodDir "rebalance-summary.md"
    $goodResult = Invoke-Summarizer -RunDir $goodDir -OutputPath $jsonPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL Kafka consumer rebalance fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if (-not $summary.passed -or $summary.before_stop.member_count -ne 2 -or $summary.after_stop.member_count -ne 1) {
        Write-Host "FAIL Kafka consumer rebalance summary produced wrong member counts." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("# Kafka Consumer Rebalance Smoke Summary") -or -not $markdown.Contains("Before stop: state=Stable, members=2") -or -not $markdown.Contains("After stop: state=Stable, members=1")) {
        Write-Host "FAIL Kafka consumer rebalance markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badDir = Join-Path $tempRoot "bad"
    Write-KafkaConsumerRebalanceFixture -Directory $badDir -AfterStopSingleMember $false
    $badResult = Invoke-Summarizer -RunDir $badDir -OutputPath (Join-Path $badDir "rebalance-summary.json") -MarkdownPath (Join-Path $badDir "rebalance-summary.md")
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL Kafka consumer rebalance fixture with two members after stop should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("one member after stopping one consumer")) {
        Write-Host "FAIL Kafka consumer rebalance bad fixture did not report member-count boundary." -ForegroundColor Red
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

Write-Host "OK   Kafka consumer rebalance summary self-test"
