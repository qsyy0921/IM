param(
    [int[]]$Workers = @(4, 8, 16),
    [int]$VUs = 100,
    [string]$Duration = "60s",
    [string]$StatsWait = "30s",
    [int]$ConversationCount = 1000,
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

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($PGDSN)) {
    $PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
}

if (-not $ResultRoot) {
    $ResultRoot = Join-Path "loadtest\results" ("gradient-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
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

foreach ($worker in $Workers) {
    $runName = "workers-$worker-" + (Get-Date -Format "yyyyMMdd-HHmmss")
    $resultDir = Join-Path $ResultRoot $runName
    $serviceOut = Join-Path (Get-Location) "logs\message-service-grpc-$runName.out.log"
    $serviceErr = Join-Path (Get-Location) "logs\message-service-grpc-$runName.err.log"
    $relayOut = Join-Path (Get-Location) "logs\message-service-relay-$runName.out.log"
    $relayErr = Join-Path (Get-Location) "logs\message-service-relay-$runName.err.log"

    Write-Host "Starting gradient run workers=$worker vus=$VUs duration=$Duration stats_wait=$StatsWait"

    $env:NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
    $env:NEXUSIM_PG_DSN = $PGDSN
    $env:NEXUSIM_GRPC_ADDR = $GrpcAddr
    $env:NEXUSIM_DEBUG_ADDR = $ServiceDebugAddr
    $grpc = Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $serviceOut -RedirectStandardError $serviceErr

    $env:NEXUSIM_MESSAGE_SERVICE_MODE = "outbox-relay"
    $env:NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
    $env:NEXUSIM_KAFKA_TOPIC = $KafkaTopic
    $env:NEXUSIM_OUTBOX_WORKERS = [string]$worker
    $env:NEXUSIM_OUTBOX_BATCH_SIZE = [string]$BatchSize
    $env:NEXUSIM_OUTBOX_POLL_INTERVAL = $PollInterval
    $env:NEXUSIM_OUTBOX_FAILURE_BACKOFF = $FailureBackoff
    $env:NEXUSIM_DEBUG_ADDR = $RelayDebugAddr
    $relay = Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $relayOut -RedirectStandardError $relayErr

    try {
        Start-Sleep -Seconds 2
        & $loadtest `
            --target=$GrpcAddr `
            --vus=$VUs `
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
    }
}

Write-Host "Gradient results written to $ResultRoot"
