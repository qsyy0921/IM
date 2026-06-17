param(
    [int[]]$CpuCores = @(1, 2, 4),
    [string[]]$MemoryLimits = @("128MiB", "256MiB", "512MiB"),
    [int[]]$VUSteps = @(20, 50, 100),
    [int]$OutboxWorkers = 8,
    [string]$Duration = "30s",
    [string]$StatsWait = "15s",
    [int]$ConversationCount = 1000,
    [double]$MinSuccessRate = 0.99,
    [double]$MaxP99MS = 1000,
    [int]$MaxPending = 1000,
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = $(if ($env:NEXUSIM_KAFKA_BROKERS) { $env:NEXUSIM_KAFKA_BROKERS } else { "localhost:9092" }),
    [string]$KafkaTopic = $(if ($env:NEXUSIM_KAFKA_TOPIC) { $env:NEXUSIM_KAFKA_TOPIC } else { "conversation.timeline.events" }),
    [string]$GrpcAddr = "127.0.0.1:10496",
    [string]$ServiceDebugAddr = "127.0.0.1:10498",
    [string]$RelayDebugAddr = "127.0.0.1:10500",
    [int]$BatchSize = 500,
    [string]$PollInterval = "200ms",
    [string]$FailureBackoff = "1s",
    [string]$ResultRoot = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $ResultRoot) {
    $ResultRoot = Join-Path "H:\NexusIM\loadtest-results" ("resource-matrix-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
}

$nexusIMRepoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $nexusIMRepoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $nexusIMRepoRoot -Name "ResultRoot"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($PGDSN)) {
    $PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
}

. .\tools\go-env.ps1
New-Item -ItemType Directory -Force bin, logs, $ResultRoot | Out-Null

if (-not $SkipBuild) {
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
}

$messageService = Join-Path (Get-Location) "bin\message-service.exe"
$loadtest = Join-Path (Get-Location) "bin\sendmessage-loadtest.exe"
$serviceMetricsURL = "http://$ServiceDebugAddr/debug/metrics"
$relayMetricsURL = "http://$RelayDebugAddr/debug/metrics"
$matrix = @()

foreach ($cpu in $CpuCores) {
    foreach ($memory in $MemoryLimits) {
        $maxPassingVU = 0
        foreach ($vu in $VUSteps) {
            $safeMemory = $memory -replace '[^A-Za-z0-9]', ''
            $runName = "cpu-$cpu-mem-$safeMemory-vu-$vu-" + (Get-Date -Format "yyyyMMdd-HHmmss")
            $resultDir = Join-Path $ResultRoot $runName
            $serviceOut = Join-Path (Get-Location) "logs\message-service-grpc-$runName.out.log"
            $serviceErr = Join-Path (Get-Location) "logs\message-service-grpc-$runName.err.log"
            $relayOut = Join-Path (Get-Location) "logs\message-service-relay-$runName.out.log"
            $relayErr = Join-Path (Get-Location) "logs\message-service-relay-$runName.err.log"

            Write-Host "Starting resource run cpu=$cpu memory=$memory vu=$vu workers=$OutboxWorkers duration=$Duration stats_wait=$StatsWait"

            $env:GOMAXPROCS = [string]$cpu
            $env:GOMEMLIMIT = $memory
            $env:NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
            $env:NEXUSIM_PG_DSN = $PGDSN
            $env:NEXUSIM_GRPC_ADDR = $GrpcAddr
            $env:NEXUSIM_DEBUG_ADDR = $ServiceDebugAddr
            $grpc = Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $serviceOut -RedirectStandardError $serviceErr

            $env:GOMAXPROCS = [string]$cpu
            $env:GOMEMLIMIT = $memory
            $env:NEXUSIM_MESSAGE_SERVICE_MODE = "outbox-relay"
            $env:NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
            $env:NEXUSIM_KAFKA_TOPIC = $KafkaTopic
            $env:NEXUSIM_OUTBOX_WORKERS = [string]$OutboxWorkers
            $env:NEXUSIM_OUTBOX_BATCH_SIZE = [string]$BatchSize
            $env:NEXUSIM_OUTBOX_POLL_INTERVAL = $PollInterval
            $env:NEXUSIM_OUTBOX_FAILURE_BACKOFF = $FailureBackoff
            $env:NEXUSIM_DEBUG_ADDR = $RelayDebugAddr
            $relay = Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $relayOut -RedirectStandardError $relayErr

            try {
                Start-Sleep -Seconds 2
                & $loadtest `
                    --target=$GrpcAddr `
                    --vus=$vu `
                    --duration=$Duration `
                    --stats-wait=$StatsWait `
                    --conversation-count=$ConversationCount `
                    --pg-dsn=$PGDSN `
                    --service-metrics-url=$serviceMetricsURL `
                    --relay-metrics-url=$relayMetricsURL `
                    --result-dir=$resultDir
            } finally {
                foreach ($process in @($grpc, $relay)) {
                    if ($process -and -not $process.HasExited) {
                        Stop-Process -Id $process.Id -Force
                        $process.WaitForExit()
                    }
                }
                Remove-Item Env:\GOMAXPROCS -ErrorAction SilentlyContinue
                Remove-Item Env:\GOMEMLIMIT -ErrorAction SilentlyContinue
            }

            $summaryPath = Join-Path $resultDir "sendmessage-summary.json"
            $summary = Get-Content -Raw $summaryPath | ConvertFrom-Json
            $passed = (
                [double]$summary.success_rate -ge $MinSuccessRate -and
                [double]$summary.p99_ms -le $MaxP99MS -and
                [int]$summary.outbox_pending_count -le $MaxPending
            )
            if ($passed) {
                $maxPassingVU = $vu
            }

            $matrix += [PSCustomObject]@{
                cpu_cores = $cpu
                memory_limit = $memory
                outbox_workers = $OutboxWorkers
                vus = $vu
                passed = $passed
                success_rate = [double]$summary.success_rate
                p95_ms = [double]$summary.p95_ms
                p99_ms = [double]$summary.p99_ms
                request_count = [int64]$summary.request_count
                outbox_pending_count = [int]$summary.outbox_pending_count
                seq_alloc_avg_ms = [double]$summary.conversation_seq_alloc_latency_ms
                kafka_publish_avg_ms = [double]$summary.kafka_publish_latency_ms
                result_file = $summary.result_file
            }

            if (-not $passed) {
                Write-Host "Stopping cpu=$cpu memory=$memory after vu=$vu failed threshold"
                break
            }
        }

        $matrix += [PSCustomObject]@{
            cpu_cores = $cpu
            memory_limit = $memory
            outbox_workers = $OutboxWorkers
            vus = "max_pass"
            passed = $true
            success_rate = $null
            p95_ms = $null
            p99_ms = $null
            request_count = $null
            outbox_pending_count = $null
            seq_alloc_avg_ms = $null
            kafka_publish_avg_ms = $null
            max_passing_vus = $maxPassingVU
            result_file = $null
        }
    }
}

$matrixPath = Join-Path $ResultRoot "resource-matrix-summary.json"
$matrix | ConvertTo-Json -Depth 4 | Set-Content -Path $matrixPath -Encoding UTF8
$matrix | Format-Table -AutoSize
Write-Host "Resource matrix summary written to $matrixPath"
