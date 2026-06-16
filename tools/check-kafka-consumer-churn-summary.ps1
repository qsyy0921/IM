$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-kafka-consumer-churn-smoke.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing Kafka consumer churn summarizer: $summarizer"
}

function Write-KafkaConsumerChurnFixture {
    param(
        [string]$Directory,
        [bool]$BadTransition = $false,
        [bool]$BadProbeLag = $false
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $transitions = @()
    $probeBatches = @()
    foreach ($item in @(
        @{ Cycle = 1; Action = "stop_a"; Expected = 1 },
        @{ Cycle = 1; Action = "start_a"; Expected = 2 },
        @{ Cycle = 1; Action = "stop_b"; Expected = 1 },
        @{ Cycle = 1; Action = "start_b"; Expected = 2 }
    )) {
        $memberCount = if ($BadTransition -and $item.Action -eq "start_b") { 1 } else { $item.Expected }
        $consumerIDs = if ($memberCount -eq 1) { @("consumer-a") } else { @("consumer-a", "consumer-b") }
        $postProbeLag = if ($BadProbeLag -and $item.Action -eq "start_b") { 1 } else { 0 }
        $probeRunName = "kafka-consumer-churn-selftest-cycle$($item.Cycle)-$($item.Action)"
        $transitions += [ordered]@{
            cycle = $item.Cycle
            action = $item.Action
            expected_members = $item.Expected
            snapshot = [ordered]@{
                state = "Stable"
                member_count = $memberCount
                consumer_ids = $consumerIDs
                assigned_partition_count = 3
                total_lag = 0
            }
            probe = [ordered]@{
                attempted = 2
                acked = 2
                failed = 0
            }
            post_probe_snapshot = [ordered]@{
                state = "Stable"
                member_count = $memberCount
                consumer_ids = $consumerIDs
                assigned_partition_count = 3
                total_lag = $postProbeLag
            }
        }
        $probeBatches += [ordered]@{
            cycle = $item.Cycle
            action = $item.Action
            run_name = $probeRunName
            attempted = 2
            acked = 2
            failed = 0
        }
    }

    [ordered]@{
        run_name = "kafka-consumer-churn-selftest"
        git_commit = "selftest"
        git_dirty = $false
        completed_at = "2026-06-16T00:00:00Z"
        topic = "im.delivery.events"
        consumer_group = "nexusim-push-churn-selftest"
        churn_cycles = 1
        probe_messages_per_transition = 2
        initial = [ordered]@{
            state = "Stable"
            member_count = 2
            consumer_ids = @("consumer-a", "consumer-b")
            assigned_partition_count = 3
            total_lag = 0
        }
        transitions = $transitions
        probe_batches = $probeBatches
    } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Directory "kafka-consumer-churn-summary.json") -Encoding UTF8
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-consumer-churn-check-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodDir = Join-Path $tempRoot "good"
    Write-KafkaConsumerChurnFixture -Directory $goodDir -BadTransition $false
    $jsonPath = Join-Path $goodDir "churn-summary.json"
    $markdownPath = Join-Path $goodDir "churn-summary.md"
    $goodResult = Invoke-Summarizer -RunDir $goodDir -OutputPath $jsonPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL Kafka consumer churn fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if (-not $summary.passed -or $summary.transition_count -ne 4 -or $summary.probe_acked -ne 8) {
        Write-Host "FAIL Kafka consumer churn summary produced wrong pass/transition flags." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("# Kafka Consumer Churn Smoke Summary") -or -not $markdown.Contains("not a production rebalance storm SLO")) {
        Write-Host "FAIL Kafka consumer churn markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badDir = Join-Path $tempRoot "bad"
    Write-KafkaConsumerChurnFixture -Directory $badDir -BadTransition $true
    $badResult = Invoke-Summarizer -RunDir $badDir -OutputPath (Join-Path $badDir "churn-summary.json") -MarkdownPath (Join-Path $badDir "churn-summary.md")
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL Kafka consumer churn fixture with bad transition should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("Consumer churn transition failed validation")) {
        Write-Host "FAIL Kafka consumer churn bad fixture did not report transition failure." -ForegroundColor Red
        if ($badResult.Output) {
            Write-Host $badResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $badProbeDir = Join-Path $tempRoot "bad-probe"
    Write-KafkaConsumerChurnFixture -Directory $badProbeDir -BadProbeLag $true
    $badProbeResult = Invoke-Summarizer -RunDir $badProbeDir -OutputPath (Join-Path $badProbeDir "churn-summary.json") -MarkdownPath (Join-Path $badProbeDir "churn-summary.md")
    if ($badProbeResult.ExitCode -eq 0) {
        Write-Host "FAIL Kafka consumer churn fixture with post-probe lag should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badProbeResult.Output.Contains("Post-probe consumer lag must be zero")) {
        Write-Host "FAIL Kafka consumer churn bad probe fixture did not report lag failure." -ForegroundColor Red
        if ($badProbeResult.Output) {
            Write-Host $badProbeResult.Output -ForegroundColor Red
        }
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   Kafka consumer churn summary self-test"
