param(
    [int[]]$BatchSizes = @(100, 500, 1000),
    [int[]]$RelayWorkers = @(8, 12, 16),
    [int[]]$VUs = @(1200, 1600),
    [int]$PGMaxConns = 64,
    [int]$PGMinConns = 0,
    [string]$Duration = "30s",
    [string]$StatsWait = "20s",
    [int]$ConversationCount = 1000,
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = $(if ($env:NEXUSIM_KAFKA_BROKERS) { $env:NEXUSIM_KAFKA_BROKERS } else { "localhost:9092" }),
    [string]$KafkaTopic = $(if ($env:NEXUSIM_KAFKA_TOPIC) { $env:NEXUSIM_KAFKA_TOPIC } else { "conversation.timeline.events" }),
    [string]$GrpcAddr = "127.0.0.1:10495",
    [string]$ServiceDebugAddr = "127.0.0.1:10600",
    [string]$RelayDebugAddr = "127.0.0.1:10700",
    [bool]$PublishBatchEnabled = $true,
    [string]$PollInterval = "200ms",
    [string]$FailureBackoff = "1s",
    [switch]$BackpressureEnabled,
    [int]$BackpressureMinAvailableConns = 8,
    [switch]$RetryOverloaded,
    [int]$MaxRetries = 2,
    [string]$RetryJitter = "100ms",
    [string]$ResultRoot = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($PGDSN)) {
    $PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
}

if (-not $ResultRoot) {
    $ResultRoot = Join-Path "H:\NexusIM\loadtest-results" ("outbox-batch-worker-matrix-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
}

. .\tools\go-env.ps1
New-Item -ItemType Directory -Force bin, logs, $ResultRoot | Out-Null

if (-not $SkipBuild) {
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
}

function Get-SummaryValue($summary, $name) {
    $property = $summary.PSObject.Properties[$name]
    if ($null -eq $property -or $null -eq $property.Value) {
        return $null
    }
    return $property.Value
}

$matrix = @()

foreach ($batchSize in $BatchSizes) {
    foreach ($workerCount in $RelayWorkers) {
        $comboName = "batch-$batchSize-workers-$workerCount"
        $comboRoot = Join-Path $ResultRoot $comboName
        New-Item -ItemType Directory -Force $comboRoot | Out-Null

        Write-Host "Starting outbox batch/worker matrix combo batch_size=$batchSize workers=$workerCount vus=$($VUs -join ',')"

        $args = @{
            PGMaxConns = @($PGMaxConns)
            PGMinConns = $PGMinConns
            VUs = $VUs
            Duration = $Duration
            StatsWait = $StatsWait
            ConversationCount = $ConversationCount
            PGDSN = $PGDSN
            KafkaBrokers = $KafkaBrokers
            KafkaTopic = $KafkaTopic
            GrpcAddr = $GrpcAddr
            ServiceDebugAddr = $ServiceDebugAddr
            RelayDebugAddr = $RelayDebugAddr
            RelayWorkers = $workerCount
            BatchSize = $batchSize
            PublishBatchEnabled = $PublishBatchEnabled
            PollInterval = $PollInterval
            FailureBackoff = $FailureBackoff
            BackpressureMinAvailableConns = $BackpressureMinAvailableConns
            MaxRetries = $MaxRetries
            RetryJitter = $RetryJitter
            ResultRoot = $comboRoot
            SkipBuild = $true
        }
        if ($BackpressureEnabled) {
            $args.BackpressureEnabled = $true
        }
        if ($RetryOverloaded) {
            $args.RetryOverloaded = $true
        }

        .\loadtest\sendmessage\run-local-pgpool-gradient.ps1 @args

        $summaryPaths = Get-ChildItem -Path $comboRoot -Filter sendmessage-summary.json -Recurse | Sort-Object FullName
        foreach ($summaryPath in $summaryPaths) {
            $summary = Get-Content -Raw $summaryPath.FullName | ConvertFrom-Json
            $relativeSummaryPath = (Resolve-Path -Path $summaryPath.FullName -Relative) -replace '^\.\\', ''
            $matrix += [PSCustomObject]@{
                batch_size = $batchSize
                relay_workers = $workerCount
                publish_batch_enabled = $PublishBatchEnabled
                result_root = $comboName
                run = Split-Path (Split-Path $summaryPath.FullName -Parent) -Leaf
                commit = Get-SummaryValue $summary "commit"
                git_dirty = Get-SummaryValue $summary "git_dirty"
                vus = Get-SummaryValue $summary "vus"
                request_count = Get-SummaryValue $summary "request_count"
                logical_success_rate = Get-SummaryValue $summary "logical_success_rate"
                accepted_rps = Get-SummaryValue $summary "accepted_rps"
                overload_rate = Get-SummaryValue $summary "overload_rate"
                success_p99_ms = Get-SummaryValue $summary "success_p99_ms"
                error_p99_ms = Get-SummaryValue $summary "error_p99_ms"
                outbox_pending_count = Get-SummaryValue $summary "outbox_pending_count"
                outbox_published_count = Get-SummaryValue $summary "outbox_published_count"
                outbox_dlq_count = Get-SummaryValue $summary "outbox_dlq_count"
                outbox_process_ready_latency_ms = Get-SummaryValue $summary "outbox_process_ready_latency_ms"
                outbox_process_ready_active_latency_ms = Get-SummaryValue $summary "outbox_process_ready_active_latency_ms"
                outbox_process_ready_idle_latency_ms = Get-SummaryValue $summary "outbox_process_ready_idle_latency_ms"
                outbox_fetched_per_call = Get-SummaryValue $summary "outbox_fetched_per_call"
                kafka_publish_call_latency_ms = Get-SummaryValue $summary "kafka_publish_call_latency_ms"
                kafka_publish_records_per_call = Get-SummaryValue $summary "kafka_publish_records_per_call"
                kafka_publish_record_latency_estimate_ms = Get-SummaryValue $summary "kafka_publish_record_latency_estimate_ms"
                summary_path = $relativeSummaryPath
            }
        }

        $matrixPath = Join-Path $ResultRoot "outbox-batch-worker-matrix-summary.json"
        $matrix | ConvertTo-Json -Depth 6 | Set-Content -Path $matrixPath -Encoding UTF8
    }
}

$matrixPath = Join-Path $ResultRoot "outbox-batch-worker-matrix-summary.json"
$matrix | ConvertTo-Json -Depth 6 | Set-Content -Path $matrixPath -Encoding UTF8
$matrix | Format-Table -AutoSize
Write-Host "Outbox batch/worker matrix summary written to $matrixPath"
