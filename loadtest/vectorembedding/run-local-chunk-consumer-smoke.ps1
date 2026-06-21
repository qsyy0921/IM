param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KnowledgeGrpcAddr = "",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$KnowledgeEventsTopic = "",
    [string]$ConsumerGroup = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "vector-chunk-consumer-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if (-not $KnowledgeEventsTopic) {
    $KnowledgeEventsTopic = "im.knowledge.events.$RunName"
}
if (-not $ConsumerGroup) {
    $ConsumerGroup = "nexusim-vector-chunk-smoke-$RunName"
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\knowledge-ingestion-service.exe") ./services/knowledge-ingestion-service/cmd/knowledge-ingestion-service
    go build -o (Join-Path $repoRoot "bin\vector-index-service.exe") ./services/vector-index-service/cmd/vector-index-service
    go build -o (Join-Path $repoRoot "bin\vector-embedding-smoke.exe") ./loadtest/vectorembedding
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Ensure-KafkaTopic {
    param([string]$Topic)
    docker exec $KafkaExecContainer kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Topic `
        --partitions 1 `
        --replication-factor 1 | Out-Null
}

function Wait-Tcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 200
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port = 0
    )
    $stdout = Join-Path $logDir "$Name.out.log"
    $stderr = Join-Path $logDir "$Name.err.log"
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $FilePath
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
    $outSub = Register-ObjectEvent -InputObject $process -EventName OutputDataReceived -Action {
        if ($EventArgs.Data) {
            Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
        }
    } -MessageData $stdout
    $errSub = Register-ObjectEvent -InputObject $process -EventName ErrorDataReceived -Action {
        if ($EventArgs.Data) {
            Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
        }
    } -MessageData $stderr
    $process.BeginOutputReadLine()
    $process.BeginErrorReadLine()

    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return [pscustomobject]@{ Process = $process; Subscriptions = @($outSub, $errSub) }
}

if (-not $KnowledgeGrpcAddr) {
    $KnowledgeGrpcAddr = "127.0.0.1:" + (Get-FreeTcpPort)
}
$knowledgeGrpcPort = [int]($KnowledgeGrpcAddr.Split(":")[-1])

$processes = @()
try {
    Ensure-KafkaTopic -Topic $KnowledgeEventsTopic

    $processes += Start-NexusProcess -Name "knowledge-ingestion-grpc" -FilePath (Join-Path $repoRoot "bin\knowledge-ingestion-service.exe") -Port $knowledgeGrpcPort -Env @{
        NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE = "grpc"
        NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR = $KnowledgeGrpcAddr
        NEXUSIM_KNOWLEDGE_INGESTION_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    $runner = Join-Path $repoRoot "bin\vector-embedding-smoke.exe"
    & $runner `
        --phase prepare `
        --pg-dsn $PgDsn `
        --knowledge-target $KnowledgeGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName
    if ($LASTEXITCODE -ne 0) {
        throw "vector chunk consumer smoke prepare failed with exit code $LASTEXITCODE"
    }

    $summaryPath = Join-Path $resultDir "vector-embedding-producer-summary.json"
    $prepareSummary = Get-Content -Path $summaryPath -Raw | ConvertFrom-Json

    $processes += Start-NexusProcess -Name "knowledge-ingestion-outbox-relay" -FilePath (Join-Path $repoRoot "bin\knowledge-ingestion-service.exe") -Env @{
        NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE = "outbox-relay"
        NEXUSIM_KNOWLEDGE_INGESTION_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_KNOWLEDGE_EVENTS_TOPIC = $KnowledgeEventsTopic
        NEXUSIM_KNOWLEDGE_OUTBOX_BATCH_SIZE = "100"
        NEXUSIM_KNOWLEDGE_OUTBOX_POLL_INTERVAL = "200ms"
        NEXUSIM_KNOWLEDGE_OUTBOX_ERROR_BACKOFF = "200ms"
    }

    $processes += Start-NexusProcess -Name "vector-index-chunk-consumer" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "chunk-consumer"
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_VECTOR_CHUNK_TOPIC = $KnowledgeEventsTopic
        NEXUSIM_VECTOR_CHUNK_CONSUMER_GROUP = $ConsumerGroup
        NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR = $KnowledgeGrpcAddr
        NEXUSIM_VECTOR_CHUNK_KNOWLEDGE_PAGE_SIZE = "10"
        NEXUSIM_VECTOR_EMBEDDING_MODEL_REF = $prepareSummary.embedding_model_ref
        NEXUSIM_VECTOR_EMBEDDING_DIMENSION = [string]$prepareSummary.embedding_dimension
        NEXUSIM_VECTOR_EMBEDDING_TRACE_ID = "trace-$RunName"
        NEXUSIM_VECTOR_CHUNK_ERROR_BACKOFF = "200ms"
    }

    & $runner `
        --phase verify-queue `
        --pg-dsn $PgDsn `
        --result-root $ResultRoot `
        --run-name $RunName `
        --tenant-id $prepareSummary.tenant_id `
        --source-id $prepareSummary.knowledge_source_id `
        --document-id $prepareSummary.document_id `
        --expected-count $prepareSummary.chunk_count `
        --embedding-model $prepareSummary.embedding_model_ref `
        --wait-timeout "20s"
    if ($LASTEXITCODE -ne 0) {
        throw "vector chunk consumer smoke verify-queue failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "knowledge_grpc_addr=$KnowledgeGrpcAddr"
    Write-Host "knowledge_events_topic=$KnowledgeEventsTopic"
    Write-Host "consumer_group=$ConsumerGroup"
    Write-Host "kafka_brokers=$KafkaBrokers"
    Write-Host "knowledge_source_id=$($prepareSummary.knowledge_source_id)"
    Write-Host "document_id=$($prepareSummary.document_id)"
    Write-Host "result_dir=$resultDir"
} finally {
    foreach ($entry in $processes) {
        if ($entry.Process -and -not $entry.Process.HasExited) {
            $entry.Process.Kill()
            $entry.Process.WaitForExit(5000) | Out-Null
        }
        foreach ($sub in $entry.Subscriptions) {
            Unregister-Event -SubscriptionId $sub.Id -ErrorAction SilentlyContinue
            Remove-Job -Id $sub.Id -Force -ErrorAction SilentlyContinue
        }
    }
}
