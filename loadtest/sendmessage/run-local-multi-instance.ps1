param(
    [int[]]$Instances = @(1, 2, 4),
    [int]$VUs = 1200,
    [string]$Duration = "60s",
    [string]$StatsWait = "30s",
    [int]$ConversationCount = 1000,
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = $(if ($env:NEXUSIM_KAFKA_BROKERS) { $env:NEXUSIM_KAFKA_BROKERS } else { "localhost:9092" }),
    [string]$KafkaTopic = $(if ($env:NEXUSIM_KAFKA_TOPIC) { $env:NEXUSIM_KAFKA_TOPIC } else { "conversation.timeline.events" }),
    [int]$BaseGrpcPort = 10495,
    [int]$BaseServiceDebugPort = 10600,
    [string]$RelayDebugAddr = "127.0.0.1:10700",
    [int]$PGMaxConns = 64,
    [int]$PGMinConns = 0,
    [int]$RelayWorkers = 8,
    [int]$BatchSize = 500,
    [string]$PollInterval = "200ms",
    [string]$FailureBackoff = "1s",
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
    $ResultRoot = Join-Path "loadtest\results" ("multi-instance-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
}

. .\tools\go-env.ps1
New-Item -ItemType Directory -Force bin, logs, $ResultRoot | Out-Null

if (-not $SkipBuild) {
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
}

$messageService = Join-Path (Get-Location) "bin\message-service.exe"
$loadtest = Join-Path (Get-Location) "bin\sendmessage-loadtest.exe"
$relayMetricsURL = "http://$RelayDebugAddr/debug/metrics"

foreach ($instanceCount in $Instances) {
    $runName = "instances-$instanceCount-vu-$VUs-" + (Get-Date -Format "yyyyMMdd-HHmmss")
    $resultDir = Join-Path $ResultRoot $runName
    $processes = @()
    $targets = @()
    $serviceMetrics = @()

    Write-Host "Starting multi-instance run instances=$instanceCount vus=$VUs duration=$Duration pg_max=$PGMaxConns"

    $env:NEXUSIM_PG_DSN = $PGDSN
    $env:NEXUSIM_PG_MAX_CONNS = [string]$PGMaxConns
    if ($PGMinConns -gt 0) {
        $env:NEXUSIM_PG_MIN_CONNS = [string]$PGMinConns
    } else {
        Remove-Item Env:\NEXUSIM_PG_MIN_CONNS -ErrorAction SilentlyContinue
    }

    for ($i = 0; $i -lt $instanceCount; $i++) {
        $grpcAddr = "127.0.0.1:" + ($BaseGrpcPort + $i)
        $debugAddr = "127.0.0.1:" + ($BaseServiceDebugPort + $i)
        $serviceOut = Join-Path (Get-Location) "logs\message-service-grpc-$runName-$i.out.log"
        $serviceErr = Join-Path (Get-Location) "logs\message-service-grpc-$runName-$i.err.log"

        $env:NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        $env:NEXUSIM_GRPC_ADDR = $grpcAddr
        $env:NEXUSIM_DEBUG_ADDR = $debugAddr
        $processes += Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $serviceOut -RedirectStandardError $serviceErr
        $targets += $grpcAddr
        $serviceMetrics += "http://$debugAddr/debug/metrics"
    }

    $relayOut = Join-Path (Get-Location) "logs\message-service-relay-$runName.out.log"
    $relayErr = Join-Path (Get-Location) "logs\message-service-relay-$runName.err.log"
    $env:NEXUSIM_MESSAGE_SERVICE_MODE = "outbox-relay"
    $env:NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
    $env:NEXUSIM_KAFKA_TOPIC = $KafkaTopic
    $env:NEXUSIM_OUTBOX_WORKERS = [string]$RelayWorkers
    $env:NEXUSIM_OUTBOX_BATCH_SIZE = [string]$BatchSize
    $env:NEXUSIM_OUTBOX_POLL_INTERVAL = $PollInterval
    $env:NEXUSIM_OUTBOX_FAILURE_BACKOFF = $FailureBackoff
    $env:NEXUSIM_DEBUG_ADDR = $RelayDebugAddr
    $processes += Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $relayOut -RedirectStandardError $relayErr

    try {
        Start-Sleep -Seconds 2
        $targetCSV = $targets -join ","
        $serviceMetricsCSV = $serviceMetrics -join ","
        & $loadtest `
            --target=$targetCSV `
            --vus=$VUs `
            --duration=$Duration `
            --stats-wait=$StatsWait `
            --conversation-count=$ConversationCount `
            --pg-dsn=$PGDSN `
            --service-metrics-url=$serviceMetricsCSV `
            --relay-metrics-url=$relayMetricsURL `
            --result-dir=$resultDir
    } finally {
        foreach ($process in $processes) {
            if ($process -and -not $process.HasExited) {
                Stop-Process -Id $process.Id -Force
                $process.WaitForExit()
            }
        }
        Remove-Item Env:\NEXUSIM_PG_MAX_CONNS -ErrorAction SilentlyContinue
        Remove-Item Env:\NEXUSIM_PG_MIN_CONNS -ErrorAction SilentlyContinue
    }
}

Write-Host "Multi-instance results written to $ResultRoot"
