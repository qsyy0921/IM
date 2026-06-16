$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-kafka-isr-observation-smoke.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing Kafka ISR observation summarizer: $summarizer"
}

function Write-PushGatewaySummary {
    param(
        [string]$Path,
        [bool]$Success
    )

    $dir = Split-Path -Parent $Path
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    [pscustomobject]@{
        success = $Success
        delivery_notify_seq = 2
        pull_inbox_item_count = 1
        delivery_outbox_published = 2
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Write-KafkaISRFixture {
    param(
        [string]$Directory,
        [bool]$TwoBrokerDownRejected = $true
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $beforePath = Join-Path $Directory "before\pushgateway-summary.json"
    $onePath = Join-Path $Directory "one\pushgateway-summary.json"
    $restorePath = Join-Path $Directory "restore\pushgateway-summary.json"
    Write-PushGatewaySummary -Path $beforePath -Success $true
    Write-PushGatewaySummary -Path $onePath -Success $true
    Write-PushGatewaySummary -Path $restorePath -Success $true

    $summary = [ordered]@{
        run_name = "kafka-isr-summary-selftest"
        git_commit = "selftest"
        git_dirty = $false
        completed_at = "2026-06-16T00:00:00Z"
        before_summary = $beforePath
        one_broker_down_summary = $onePath
        after_restore_summary = $restorePath
        first_stopped_broker_id = "1"
        second_stopped_broker_id = "3"
        remaining_broker_id_after_two_stops = "2"
        delivery_topic_after_one_broker_stop = @(
            [ordered]@{
                partition = 0
                leader = 2
                replicas = @("1", "2", "3")
                isr = @("2", "3")
                replica_count = 3
                isr_count = 2
            },
            [ordered]@{
                partition = 1
                leader = 3
                replicas = @("1", "2", "3")
                isr = @("2", "3")
                replica_count = 3
                isr_count = 2
            }
        )
        probe_produce_after_one_broker_stop = [ordered]@{
            exit_code = 0
            accepted = $true
            contains_not_enough_replicas = $false
            output = ""
        }
        probe_produce_after_two_broker_stops = [ordered]@{
            exit_code = if ($TwoBrokerDownRejected) { 1 } else { 0 }
            accepted = (-not $TwoBrokerDownRejected)
            contains_not_enough_replicas = $TwoBrokerDownRejected
            output = if ($TwoBrokerDownRejected) { "NOT_ENOUGH_REPLICAS" } else { "" }
        }
    }
    $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $Directory "kafka-isr-observation-summary.json") -Encoding UTF8
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-isr-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodDir = Join-Path $tempRoot "good"
    Write-KafkaISRFixture -Directory $goodDir -TwoBrokerDownRejected $true
    $jsonPath = Join-Path $goodDir "summary.json"
    $markdownPath = Join-Path $goodDir "summary.md"
    $goodResult = Invoke-Summarizer -RunDir $goodDir -OutputPath $jsonPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL Kafka ISR observation summary fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if (-not $summary.passed -or -not $summary.one_broker_down_producer_probe_accepted -or -not $summary.two_broker_down_producer_probe_rejected_not_enough_replicas) {
        Write-Host "FAIL Kafka ISR observation summary produced wrong result flags." -ForegroundColor Red
        exit 1
    }
    if ($summary.delivery_topic_after_one_broker_stop.partition_count -ne 2) {
        Write-Host "FAIL Kafka ISR observation summary produced wrong partition count." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("One broker down: passed") -or -not $markdown.Contains("NOT_ENOUGH_REPLICAS") -or -not $markdown.Contains("not a production Kafka HA proof")) {
        Write-Host "FAIL Kafka ISR observation markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badDir = Join-Path $tempRoot "bad"
    Write-KafkaISRFixture -Directory $badDir -TwoBrokerDownRejected $false
    $badResult = Invoke-Summarizer -RunDir $badDir -OutputPath (Join-Path $badDir "summary.json") -MarkdownPath (Join-Path $badDir "summary.md")
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL Kafka ISR observation fixture without two-broker rejection should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("Two-broker-down producer probe")) {
        Write-Host "FAIL Kafka ISR observation bad fixture did not report producer probe boundary." -ForegroundColor Red
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

Write-Host "OK   Kafka ISR observation summary self-test"
