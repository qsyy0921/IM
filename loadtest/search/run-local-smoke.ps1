param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$Topic = "",
    [string]$ConsumerGroup = "",
    [string]$SearchGrpcAddr = "127.0.0.1:10570",
    [int]$TopicReplicationFactor = 1,
    [switch]$UseOpenSearchBackend,
    [string]$OpenSearchEndpoint = "http://127.0.0.1:9200",
    [string]$OpenSearchIndex = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "search-service-projection-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
if (-not $Topic) {
    $Topic = "conversation.timeline.search." + (Get-Date -Format "yyyyMMddHHmmss")
}
if (-not $ConsumerGroup) {
    $ConsumerGroup = "nexusim-search-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
}
if ($UseOpenSearchBackend -and -not $OpenSearchIndex) {
    $OpenSearchIndex = ("nexusim-search-smoke-" + $safeRunName).ToLowerInvariant()
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\search-service.exe") ./services/search-service/cmd/search-service
    go build -o (Join-Path $repoRoot "bin\search-smoke.exe") ./loadtest/search
}

function Ensure-KafkaTopic {
    param([string]$Name)
    docker exec $KafkaExecContainer kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Name `
        --partitions 1 `
        --replication-factor $TopicReplicationFactor | Out-Null
}

function Start-SearchProcess {
    param(
        [string]$Name,
        [hashtable]$Env
    )

    $exe = Join-Path $repoRoot "bin\search-service.exe"
    $stdout = Join-Path $logDir "$Name.out.log"
    $stderr = Join-Path $logDir "$Name.err.log"
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $exe
    $psi.WorkingDirectory = $repoRoot
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    foreach ($key in $Env.Keys) {
        $psi.Environment[$key] = [string]$Env[$key]
    }

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $psi
    $null = $process.Start()
    Register-ObjectEvent `
        -InputObject $process `
        -EventName OutputDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stdout | Out-Null
    Register-ObjectEvent `
        -InputObject $process `
        -EventName ErrorDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stderr | Out-Null
    $process.BeginOutputReadLine()
    $process.BeginErrorReadLine()
    return $process
}

$processes = @()
try {
    # Create the topic before starting the consumer; kafka-go readers may miss
    # a topic created after group startup in local smoke runs.
    Ensure-KafkaTopic -Name $Topic

    $processes += Start-SearchProcess -Name "search-grpc" -Env @{
        NEXUSIM_SEARCH_SERVICE_MODE = "grpc"
        NEXUSIM_SEARCH_GRPC_ADDR = $SearchGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_SEARCH_DEBUG_ADDR = ""
        NEXUSIM_SEARCH_BACKEND = $(if ($UseOpenSearchBackend) { "opensearch" } else { "postgres" })
        NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT = $(if ($UseOpenSearchBackend) { $OpenSearchEndpoint } else { "" })
        NEXUSIM_SEARCH_OPENSEARCH_INDEX = $(if ($UseOpenSearchBackend) { $OpenSearchIndex } else { "" })
    }
    Start-Sleep -Milliseconds 500

    $processes += Start-SearchProcess -Name "search-timeline-consumer" -Env @{
        NEXUSIM_SEARCH_SERVICE_MODE = "timeline-consumer"
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_TIMELINE_TOPIC = $Topic
        NEXUSIM_SEARCH_CONSUMER_GROUP = $ConsumerGroup
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_SEARCH_DEBUG_ADDR = ""
    }
    Start-Sleep -Seconds 2

    $runner = Join-Path $repoRoot "bin\search-smoke.exe"
    $searchBackend = if ($UseOpenSearchBackend) { "opensearch" } else { "postgres" }
    & $runner `
        --pg-dsn $PgDsn `
        --kafka-brokers $KafkaBrokers `
        --topic $Topic `
        --consumer-group $ConsumerGroup `
        --search-target $SearchGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName `
        --search-backend $searchBackend `
        --opensearch-endpoint $OpenSearchEndpoint `
        --opensearch-index $OpenSearchIndex `
        --ensure-topic=false
    if ($LASTEXITCODE -ne 0) {
        throw "search smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "topic=$Topic"
    Write-Host "consumer_group=$ConsumerGroup"
    if ($UseOpenSearchBackend) {
        Write-Host "search_backend=opensearch"
        Write-Host "opensearch_endpoint=$OpenSearchEndpoint"
        Write-Host "opensearch_index=$OpenSearchIndex"
    }
    Write-Host "result_dir=$resultDir"
}
finally {
    foreach ($process in $processes) {
        if ($process -and -not $process.HasExited) {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
        }
        if ($process) {
            $process.Dispose()
        }
    }
    Get-EventSubscriber |
        Where-Object { $_.SourceObject -is [System.Diagnostics.Process] } |
        Unregister-Event -ErrorAction SilentlyContinue
}
