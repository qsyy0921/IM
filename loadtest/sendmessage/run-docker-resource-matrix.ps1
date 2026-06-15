param(
    [double[]]$CpuLimits = @(1, 2, 4),
    [string[]]$MemoryLimits = @("256m", "512m", "1g"),
    [int[]]$VUSteps = @(20, 50, 100),
    [int]$OutboxWorkers = 8,
    [string]$Duration = "30s",
    [string]$StatsWait = "15s",
    [int]$ConversationCount = 1000,
    [double]$MinSuccessRate = 0.99,
    [double]$MaxP99MS = 1000,
    [int]$MaxPending = 1000,
    [string]$DockerNetwork = "nexusim-local_default",
    [string]$PGDSN = "postgres://nexusim:nexusim@postgres:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "kafka:29092",
    [string]$KafkaTopic = "conversation.timeline.events",
    [int]$PGMaxConns = 0,
    [int]$PGMinConns = 0,
    [int]$BatchSize = 500,
    [string]$PollInterval = "200ms",
    [string]$FailureBackoff = "1s",
    [string]$ResultRoot = "",
    [string]$MessageImage = "nexusim/message-service:local",
    [string]$LoadtestImage = "nexusim/sendmessage-loadtest:local",
    [switch]$SkipImageBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

if (-not $ResultRoot) {
    $ResultRoot = Join-Path "H:\NexusIM\loadtest-results" ("docker-resource-matrix-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
}

. .\tools\go-env.ps1
New-Item -ItemType Directory -Force bin\linux, logs, $ResultRoot | Out-Null

$commitShort = ((git rev-parse --short HEAD) -join "").Trim()
$commitFull = ((git rev-parse HEAD) -join "").Trim()
$gitStatus = ((git status --short) -join "`n").Trim()
$gitStatusForEnv = $gitStatus -replace "(`r`n|`n|`r)", " | "
$gitDirty = if ($gitStatus) { "true" } else { "false" }

if (-not $SkipImageBuild) {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    try {
        go build -o bin\linux\message-service ./services/message-service/cmd/message-service
        go build -o bin\linux\sendmessage-loadtest ./loadtest/sendmessage
    } finally {
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    }
    docker build -f deploy/docker/message-service.runtime.Dockerfile -t $MessageImage .
    docker build -f deploy/docker/sendmessage-loadtest.runtime.Dockerfile -t $LoadtestImage .
}

$resultAbs = (Resolve-Path $ResultRoot).Path
$matrix = @()

foreach ($cpu in $CpuLimits) {
    foreach ($memory in $MemoryLimits) {
        $maxPassingVU = 0
        foreach ($vu in $VUSteps) {
            $safeCPU = ([string]$cpu) -replace '[^A-Za-z0-9]', ''
            $safeMemory = $memory -replace '[^A-Za-z0-9]', ''
            $runName = "cpu-$safeCPU-mem-$safeMemory-vu-$vu-" + (Get-Date -Format "yyyyMMdd-HHmmss")
            $containerPrefix = "nexusim-$runName"
            $grpcName = "$containerPrefix-grpc"
            $relayName = "$containerPrefix-relay"
            $containerResultDir = "/results/$runName"
            $hostSummaryPath = Join-Path $ResultRoot (Join-Path $runName "sendmessage-summary.json")

            Write-Host "Starting Docker resource run cpu=$cpu memory=$memory vu=$vu workers=$OutboxWorkers"

            $gomaxprocs = [math]::Max(1, [int][math]::Ceiling($cpu))
            docker run -d --rm `
                --name $grpcName `
                --network $DockerNetwork `
                --cpus $cpu `
                --memory $memory `
                -e GOMAXPROCS=$gomaxprocs `
                -e NEXUSIM_MESSAGE_SERVICE_MODE=grpc `
                -e NEXUSIM_PG_DSN=$PGDSN `
                -e NEXUSIM_PG_MAX_CONNS=$PGMaxConns `
                -e NEXUSIM_PG_MIN_CONNS=$PGMinConns `
                -e NEXUSIM_GRPC_ADDR=0.0.0.0:10495 `
                -e NEXUSIM_DEBUG_ADDR=0.0.0.0:10497 `
                $MessageImage | Out-Null

            docker run -d --rm `
                --name $relayName `
                --network $DockerNetwork `
                --cpus $cpu `
                --memory $memory `
                -e GOMAXPROCS=$gomaxprocs `
                -e NEXUSIM_MESSAGE_SERVICE_MODE=outbox-relay `
                -e NEXUSIM_PG_DSN=$PGDSN `
                -e NEXUSIM_PG_MAX_CONNS=$PGMaxConns `
                -e NEXUSIM_PG_MIN_CONNS=$PGMinConns `
                -e NEXUSIM_KAFKA_BROKERS=$KafkaBrokers `
                -e NEXUSIM_KAFKA_TOPIC=$KafkaTopic `
                -e NEXUSIM_OUTBOX_WORKERS=$OutboxWorkers `
                -e NEXUSIM_OUTBOX_BATCH_SIZE=$BatchSize `
                -e NEXUSIM_OUTBOX_POLL_INTERVAL=$PollInterval `
                -e NEXUSIM_OUTBOX_FAILURE_BACKOFF=$FailureBackoff `
                -e NEXUSIM_DEBUG_ADDR=0.0.0.0:10500 `
                $MessageImage | Out-Null

            try {
                Start-Sleep -Seconds 2
                docker run --rm `
                    --network $DockerNetwork `
                    -v "${resultAbs}:/results" `
                    -e NEXUSIM_COMMIT=$commitShort `
                    -e NEXUSIM_COMMIT_FULL=$commitFull `
                    -e NEXUSIM_GIT_DIRTY=$gitDirty `
                    -e "NEXUSIM_GIT_STATUS_SHORT=$gitStatusForEnv" `
                    $LoadtestImage `
                    --target="$grpcName`:10495" `
                    --vus=$vu `
                    --duration=$Duration `
                    --stats-wait=$StatsWait `
                    --conversation-count=$ConversationCount `
                    --pg-dsn=$PGDSN `
                    --service-metrics-url="http://$grpcName`:10497/debug/metrics" `
                    --relay-metrics-url="http://$relayName`:10500/debug/metrics" `
                    --result-dir=$containerResultDir
            } finally {
                docker rm -f $grpcName $relayName 2>$null | Out-Null
            }

            $summary = Get-Content -Raw $hostSummaryPath | ConvertFrom-Json
            $passed = (
                [double]$summary.success_rate -ge $MinSuccessRate -and
                [double]$summary.p99_ms -le $MaxP99MS -and
                [int]$summary.outbox_pending_count -le $MaxPending
            )
            if ($passed) {
                $maxPassingVU = $vu
            }

            $matrix += [PSCustomObject]@{
                cpu_limit = $cpu
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
            cpu_limit = $cpu
            memory_limit = $memory
            outbox_workers = $OutboxWorkers
            vus = "max_pass"
            passed = $true
            max_passing_vus = $maxPassingVU
        }
    }
}

$matrixPath = Join-Path $ResultRoot "docker-resource-matrix-summary.json"
$matrix | ConvertTo-Json -Depth 4 | Set-Content -Path $matrixPath -Encoding UTF8
$matrix | Format-Table -AutoSize
Write-Host "Docker resource matrix summary written to $matrixPath"
