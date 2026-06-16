$ErrorActionPreference = "Stop"

$summarizer = Join-Path $PSScriptRoot "summarize-kafka-isr-flapping-smoke.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $summarizer -PathType Leaf)) {
    throw "Missing Kafka ISR flapping summarizer: $summarizer"
}

function Write-KafkaISRFlappingFixture {
    param(
        [string]$Directory,
        [bool]$SecondCycleRestored = $true
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $cycles = @()
    foreach ($cycle in @(1, 2)) {
        $restoredIsr = if ($cycle -eq 2 -and -not $SecondCycleRestored) { @("2", "3") } else { @("1", "2", "3") }
        $restoredIsrCount = $restoredIsr.Count
        $cycles += [ordered]@{
            cycle = $cycle
            stopped_broker_id = "1"
            stopped_container = "nexusim-kafka-ha-0"
            admin_broker_id = "2"
            degraded_topic_state = @(
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
            degraded_probe = [ordered]@{
                exit_code = 0
                accepted = $true
                contains_not_enough_replicas = $false
                output = ""
            }
            restored_topic_state = @(
                [ordered]@{
                    partition = 0
                    leader = 2
                    replicas = @("1", "2", "3")
                    isr = $restoredIsr
                    replica_count = 3
                    isr_count = $restoredIsrCount
                },
                [ordered]@{
                    partition = 1
                    leader = 3
                    replicas = @("1", "2", "3")
                    isr = $restoredIsr
                    replica_count = 3
                    isr_count = $restoredIsrCount
                }
            )
            restored_probe = [ordered]@{
                exit_code = 0
                accepted = $true
                contains_not_enough_replicas = $false
                output = ""
            }
        }
    }

    [ordered]@{
        run_name = "kafka-isr-flapping-summary-selftest"
        git_commit = "selftest"
        git_dirty = $false
        completed_at = "2026-06-16T00:00:00Z"
        probe_topic = "nexusim.kafka.isr.flap.selftest"
        topic_replication_factor = 3
        topic_min_insync_replicas = 2
        controller_broker_id = "3"
        flapped_broker_id = "1"
        flap_cycles = 2
        stable_checks = 1
        cycles = $cycles
    } | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $Directory "kafka-isr-flapping-summary.json") -Encoding UTF8
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-kafka-isr-flapping-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $goodDir = Join-Path $tempRoot "good"
    Write-KafkaISRFlappingFixture -Directory $goodDir -SecondCycleRestored $true
    $jsonPath = Join-Path $goodDir "summary.json"
    $markdownPath = Join-Path $goodDir "summary.md"
    $goodResult = Invoke-Summarizer -RunDir $goodDir -OutputPath $jsonPath -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL Kafka ISR flapping summary fixture should pass." -ForegroundColor Red
        if ($goodResult.Output) {
            Write-Host $goodResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $summary = Get-Content -LiteralPath $jsonPath -Raw | ConvertFrom-Json
    if (-not $summary.passed -or $summary.flap_cycles -ne 2) {
        Write-Host "FAIL Kafka ISR flapping summary produced wrong pass/cycle flags." -ForegroundColor Red
        exit 1
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Kafka ISR Flapping Smoke Summary") -or -not $markdown.Contains("not a production Kafka HA proof")) {
        Write-Host "FAIL Kafka ISR flapping markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badDir = Join-Path $tempRoot "bad"
    Write-KafkaISRFlappingFixture -Directory $badDir -SecondCycleRestored $false
    $badResult = Invoke-Summarizer -RunDir $badDir -OutputPath (Join-Path $badDir "summary.json") -MarkdownPath (Join-Path $badDir "summary.md")
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL Kafka ISR flapping fixture with unrestored ISR should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("At least one Kafka ISR flapping cycle failed validation")) {
        Write-Host "FAIL Kafka ISR flapping bad fixture did not report cycle failure." -ForegroundColor Red
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

Write-Host "OK   Kafka ISR flapping summary self-test"
