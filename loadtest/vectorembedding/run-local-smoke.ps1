param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KnowledgeGrpcAddr = "",
    [string]$ModelGatewayGrpcAddr = "",
    [string]$VectorGrpcAddr = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "vector-embedding-producer-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\knowledge-ingestion-service.exe") ./services/knowledge-ingestion-service/cmd/knowledge-ingestion-service
    go build -o (Join-Path $repoRoot "bin\model-gateway.exe") ./services/model-gateway/cmd/model-gateway
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
if (-not $ModelGatewayGrpcAddr) {
    $ModelGatewayGrpcAddr = "127.0.0.1:" + (Get-FreeTcpPort)
}
if (-not $VectorGrpcAddr) {
    $VectorGrpcAddr = "127.0.0.1:" + (Get-FreeTcpPort)
}
$knowledgeGrpcPort = [int]($KnowledgeGrpcAddr.Split(":")[-1])
$modelGatewayGrpcPort = [int]($ModelGatewayGrpcAddr.Split(":")[-1])
$vectorGrpcPort = [int]($VectorGrpcAddr.Split(":")[-1])

$processes = @()
try {
    $processes += Start-NexusProcess -Name "knowledge-ingestion-grpc" -FilePath (Join-Path $repoRoot "bin\knowledge-ingestion-service.exe") -Port $knowledgeGrpcPort -Env @{
        NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE = "grpc"
        NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR = $KnowledgeGrpcAddr
        NEXUSIM_KNOWLEDGE_INGESTION_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "model-gateway-grpc" -FilePath (Join-Path $repoRoot "bin\model-gateway.exe") -Port $modelGatewayGrpcPort -Env @{
        NEXUSIM_MODEL_GATEWAY_MODE = "grpc"
        NEXUSIM_MODEL_GATEWAY_GRPC_ADDR = $ModelGatewayGrpcAddr
        NEXUSIM_MODEL_GATEWAY_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    $processes += Start-NexusProcess -Name "vector-index-grpc" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Port $vectorGrpcPort -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "grpc"
        NEXUSIM_VECTOR_INDEX_GRPC_ADDR = $VectorGrpcAddr
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
    }

    $runner = Join-Path $repoRoot "bin\vector-embedding-smoke.exe"
    & $runner `
        --phase prepare `
        --pg-dsn $PgDsn `
        --knowledge-target $KnowledgeGrpcAddr `
        --vector-target $VectorGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName
    if ($LASTEXITCODE -ne 0) {
        throw "vector embedding smoke prepare failed with exit code $LASTEXITCODE"
    }

    $summaryPath = Join-Path $resultDir "vector-embedding-producer-summary.json"
    $prepareSummary = Get-Content -Path $summaryPath -Raw | ConvertFrom-Json

    $processes += Start-NexusProcess -Name "vector-index-embedding-producer" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "embedding-producer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
        NEXUSIM_VECTOR_EMBEDDING_PRODUCER_SOURCE = "knowledge"
        NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR = $KnowledgeGrpcAddr
        NEXUSIM_VECTOR_EMBEDDING_TENANT_ID = $prepareSummary.tenant_id
        NEXUSIM_VECTOR_EMBEDDING_SOURCE_ID = $prepareSummary.knowledge_source_id
        NEXUSIM_VECTOR_EMBEDDING_DOCUMENT_ID = $prepareSummary.document_id
        NEXUSIM_VECTOR_EMBEDDING_KNOWLEDGE_PAGE_SIZE = "10"
        NEXUSIM_VECTOR_EMBEDDING_MODEL_REF = $prepareSummary.embedding_model_ref
        NEXUSIM_VECTOR_EMBEDDING_DIMENSION = [string]$prepareSummary.embedding_dimension
        NEXUSIM_VECTOR_EMBEDDING_BATCH_SIZE = "10"
        NEXUSIM_VECTOR_EMBEDDING_POLL_INTERVAL = "200ms"
        NEXUSIM_VECTOR_EMBEDDING_ERROR_BACKOFF = "200ms"
        NEXUSIM_VECTOR_EMBEDDING_TRACE_ID = "trace-$RunName"
    }

    $processes += Start-NexusProcess -Name "vector-index-embedding-worker" -FilePath (Join-Path $repoRoot "bin\vector-index-service.exe") -Env @{
        NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "embedding-worker"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_VECTOR_INDEX_DEBUG_ADDR = ""
        NEXUSIM_VECTOR_EMBEDDING_SOURCE = "postgres"
        NEXUSIM_MODEL_GATEWAY_GRPC_ADDR = $ModelGatewayGrpcAddr
        NEXUSIM_VECTOR_EMBEDDING_TENANT_ID = $prepareSummary.tenant_id
        NEXUSIM_VECTOR_EMBEDDING_CLAIM_TIMEOUT = "30s"
        NEXUSIM_VECTOR_EMBEDDING_BATCH_SIZE = "10"
        NEXUSIM_VECTOR_EMBEDDING_POLL_INTERVAL = "200ms"
        NEXUSIM_VECTOR_EMBEDDING_ERROR_BACKOFF = "200ms"
        NEXUSIM_VECTOR_EMBEDDING_TRACE_ID = "trace-$RunName"
    }

    & $runner `
        --phase verify `
        --pg-dsn $PgDsn `
        --knowledge-target $KnowledgeGrpcAddr `
        --vector-target $VectorGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName `
        --tenant-id $prepareSummary.tenant_id `
        --source-id $prepareSummary.knowledge_source_id `
        --document-id $prepareSummary.document_id `
        --visibility-scope $prepareSummary.visibility_scope `
        --policy-version $prepareSummary.policy_version `
        --expected-count $prepareSummary.chunk_count `
        --wait-timeout "20s"
    if ($LASTEXITCODE -ne 0) {
        throw "vector embedding smoke verify failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "knowledge_grpc_addr=$KnowledgeGrpcAddr"
    Write-Host "model_gateway_grpc_addr=$ModelGatewayGrpcAddr"
    Write-Host "vector_grpc_addr=$VectorGrpcAddr"
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
