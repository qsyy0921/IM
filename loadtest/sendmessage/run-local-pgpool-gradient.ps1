param(
    [int[]]$PGMaxConns = @(16, 32, 64, 96),
    [int]$PGMinConns = 0,
    [int[]]$VUs = @(1200, 1600, 2000),
    [string]$Duration = "60s",
    [string]$StatsWait = "30s",
    [int]$ConversationCount = 1000,
    [string]$PGDSN = $env:NEXUSIM_PG_DSN,
    [string]$KafkaBrokers = $(if ($env:NEXUSIM_KAFKA_BROKERS) { $env:NEXUSIM_KAFKA_BROKERS } else { "localhost:9092" }),
    [string]$KafkaTopic = $(if ($env:NEXUSIM_KAFKA_TOPIC) { $env:NEXUSIM_KAFKA_TOPIC } else { "conversation.timeline.events" }),
    [string]$GrpcAddr = "127.0.0.1:10495",
    [string]$ServiceDebugAddr = "127.0.0.1:10600",
    [string]$RelayDebugAddr = "127.0.0.1:10700",
    [int]$RelayWorkers = 8,
    [int]$BatchSize = 500,
    [bool]$PublishBatchEnabled = $true,
    [string]$PollInterval = "200ms",
    [string]$FailureBackoff = "1s",
    [string]$ResultRoot = "",
    [switch]$BackpressureEnabled,
    [int]$BackpressureMinAvailableConns = 0,
    [switch]$AdaptiveLimitEnabled,
    [int]$AdaptiveMaxInFlight = 0,
    [int]$AdaptiveMinAvailableConns = 8,
    [int]$AdaptiveReleaseAvailableConns = 12,
    [string]$AdaptiveMaxPoolAcquireP95 = "250ms",
    [long]$AdaptiveMaxOutboxPending = 20000,
    [long]$AdaptiveReleaseOutboxPending = 10000,
    [string]$AdaptiveMaxRelayActiveP95 = "200ms",
    [double]$AdaptiveMinOutboxFetchedPerCall = 5,
    [double]$AdaptiveMinKafkaRecordsPerCall = 10,
    [string]$AdaptiveSampleInterval = "1s",
    [string]$AdaptiveRetryBaseDelay = "500ms",
    [string]$AdaptiveRetryMaxDelay = "2s",
    [switch]$RetryOverloaded,
    [int]$MaxRetries = 0,
    [string]$RetryJitter = "0s",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$nexusIMRepoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $nexusIMRepoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $nexusIMRepoRoot -Name "ResultRoot"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($PGDSN)) {
    $PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
}

if (-not $ResultRoot) {
    $ResultRoot = Join-Path "H:\NexusIM\loadtest-results" ("pgpool-gradient-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
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

foreach ($pgMax in $PGMaxConns) {
    foreach ($vu in $VUs) {
        $backpressureLabel = if ($BackpressureEnabled) { "bpon" } else { "bpoff" }
        $adaptiveLabel = if ($AdaptiveLimitEnabled) { "adapton" } else { "adaptoff" }
        $publishBatchLabel = if ($PublishBatchEnabled) { "pbatchon" } else { "pbatchoff" }
        $inFlightLabel = if ($AdaptiveLimitEnabled -and $AdaptiveMaxInFlight -gt 0) { "inflight-$AdaptiveMaxInFlight" } else { "inflight-off" }
        $runName = "$backpressureLabel-$adaptiveLabel-$publishBatchLabel-$inFlightLabel-pgmax-$pgMax-vu-$vu-" + (Get-Date -Format "yyyyMMdd-HHmmss")
        $resultDir = Join-Path $ResultRoot $runName
        $serviceOut = Join-Path (Get-Location) "logs\message-service-grpc-$runName.out.log"
        $serviceErr = Join-Path (Get-Location) "logs\message-service-grpc-$runName.err.log"
        $relayOut = Join-Path (Get-Location) "logs\message-service-relay-$runName.out.log"
        $relayErr = Join-Path (Get-Location) "logs\message-service-relay-$runName.err.log"

        Write-Host "Starting PG pool run pg_max=$pgMax pg_min=$PGMinConns vus=$vu duration=$Duration publish_batch=$PublishBatchEnabled adaptive=$AdaptiveLimitEnabled max_in_flight=$AdaptiveMaxInFlight"

        $env:NEXUSIM_PG_DSN = $PGDSN
        $env:NEXUSIM_PG_MAX_CONNS = [string]$pgMax
        if ($PGMinConns -gt 0) {
            $env:NEXUSIM_PG_MIN_CONNS = [string]$PGMinConns
        } else {
            Remove-Item Env:\NEXUSIM_PG_MIN_CONNS -ErrorAction SilentlyContinue
        }
        if ($BackpressureEnabled) {
            $env:NEXUSIM_PG_BACKPRESSURE_ENABLED = "true"
            $env:NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS = [string]$BackpressureMinAvailableConns
        } else {
            Remove-Item Env:\NEXUSIM_PG_BACKPRESSURE_ENABLED -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS -ErrorAction SilentlyContinue
        }
        if ($AdaptiveLimitEnabled) {
            $env:NEXUSIM_ADAPTIVE_LIMIT_ENABLED = "true"
            if ($AdaptiveMaxInFlight -gt 0) {
                $env:NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT = [string]$AdaptiveMaxInFlight
            } else {
                Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT -ErrorAction SilentlyContinue
            }
            $env:NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS = [string]$AdaptiveMinAvailableConns
            $env:NEXUSIM_ADAPTIVE_RELEASE_AVAILABLE_CONNS = [string]$AdaptiveReleaseAvailableConns
            $env:NEXUSIM_ADAPTIVE_MAX_POOL_ACQUIRE_P95 = $AdaptiveMaxPoolAcquireP95
            $env:NEXUSIM_ADAPTIVE_MAX_OUTBOX_PENDING = [string]$AdaptiveMaxOutboxPending
            $env:NEXUSIM_ADAPTIVE_RELEASE_OUTBOX_PENDING = [string]$AdaptiveReleaseOutboxPending
            $env:NEXUSIM_ADAPTIVE_MAX_RELAY_ACTIVE_P95 = $AdaptiveMaxRelayActiveP95
            $env:NEXUSIM_ADAPTIVE_MIN_OUTBOX_FETCHED_PER_CALL = [string]$AdaptiveMinOutboxFetchedPerCall
            $env:NEXUSIM_ADAPTIVE_MIN_KAFKA_RECORDS_PER_CALL = [string]$AdaptiveMinKafkaRecordsPerCall
            $env:NEXUSIM_ADAPTIVE_SAMPLE_INTERVAL = $AdaptiveSampleInterval
            $env:NEXUSIM_ADAPTIVE_RETRY_BASE_DELAY = $AdaptiveRetryBaseDelay
            $env:NEXUSIM_ADAPTIVE_RETRY_MAX_DELAY = $AdaptiveRetryMaxDelay
            $env:NEXUSIM_ADAPTIVE_RELAY_METRICS_URL = $relayMetricsURL
        } else {
            Remove-Item Env:\NEXUSIM_ADAPTIVE_LIMIT_ENABLED -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RELEASE_AVAILABLE_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_POOL_ACQUIRE_P95 -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_OUTBOX_PENDING -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RELEASE_OUTBOX_PENDING -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_RELAY_ACTIVE_P95 -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MIN_OUTBOX_FETCHED_PER_CALL -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MIN_KAFKA_RECORDS_PER_CALL -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_SAMPLE_INTERVAL -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RETRY_BASE_DELAY -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RETRY_MAX_DELAY -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RELAY_METRICS_URL -ErrorAction SilentlyContinue
        }

        $env:NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
        $env:NEXUSIM_GRPC_ADDR = $GrpcAddr
        $env:NEXUSIM_DEBUG_ADDR = $ServiceDebugAddr
        $grpc = Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $serviceOut -RedirectStandardError $serviceErr

        $env:NEXUSIM_MESSAGE_SERVICE_MODE = "outbox-relay"
        $env:NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        $env:NEXUSIM_KAFKA_TOPIC = $KafkaTopic
        $env:NEXUSIM_OUTBOX_WORKERS = [string]$RelayWorkers
        $env:NEXUSIM_OUTBOX_BATCH_SIZE = [string]$BatchSize
        $env:NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED = [string]$PublishBatchEnabled
        $env:NEXUSIM_OUTBOX_POLL_INTERVAL = $PollInterval
        $env:NEXUSIM_OUTBOX_FAILURE_BACKOFF = $FailureBackoff
        $env:NEXUSIM_DEBUG_ADDR = $RelayDebugAddr
        $relay = Start-Process -FilePath $messageService -WindowStyle Hidden -PassThru -RedirectStandardOutput $relayOut -RedirectStandardError $relayErr

        try {
            Start-Sleep -Seconds 2
            $loadtestArgs = @(
                "--target=$GrpcAddr",
                "--vus=$vu",
                "--duration=$Duration",
                "--stats-wait=$StatsWait",
                "--conversation-count=$ConversationCount",
                "--pg-dsn=$PGDSN",
                "--service-metrics-url=$serviceMetricsURL",
                "--relay-metrics-url=$relayMetricsURL",
                "--result-dir=$resultDir"
            )
            if ($RetryOverloaded) {
                $loadtestArgs += "--retry-overloaded"
                $loadtestArgs += "--max-retries=$MaxRetries"
                $loadtestArgs += "--retry-jitter=$RetryJitter"
            }
            & $loadtest @loadtestArgs
        } finally {
            foreach ($process in @($grpc, $relay)) {
                if ($process -and -not $process.HasExited) {
                    Stop-Process -Id $process.Id -Force
                    $process.WaitForExit()
                }
            }
            Remove-Item Env:\NEXUSIM_PG_MAX_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_PG_MIN_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_PG_BACKPRESSURE_ENABLED -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_OUTBOX_PUBLISH_BATCH_ENABLED -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_LIMIT_ENABLED -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_IN_FLIGHT -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MIN_AVAILABLE_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RELEASE_AVAILABLE_CONNS -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_POOL_ACQUIRE_P95 -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_OUTBOX_PENDING -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RELEASE_OUTBOX_PENDING -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MAX_RELAY_ACTIVE_P95 -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MIN_OUTBOX_FETCHED_PER_CALL -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_MIN_KAFKA_RECORDS_PER_CALL -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_SAMPLE_INTERVAL -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RETRY_BASE_DELAY -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RETRY_MAX_DELAY -ErrorAction SilentlyContinue
            Remove-Item Env:\NEXUSIM_ADAPTIVE_RELAY_METRICS_URL -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "PG pool gradient results written to $ResultRoot"
