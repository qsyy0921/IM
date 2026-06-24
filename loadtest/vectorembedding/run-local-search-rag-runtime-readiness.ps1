param(
    [string]$ProviderReadiness = "pgvector,opensearch-vector",
    [string]$PgVectorDsn = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable",
    [string]$PgVectorTable = "vector_embedding_items",
    [string]$OpenSearchEndpoint = "http://127.0.0.1:9200",
    [string]$OpenSearchIndex = "nexusim-vector-items",
    [string]$OpenSearchVectorField = "embedding_vector",
    [int]$OpenSearchVectorDimension = 8,
    [string]$RequestTimeout = "5s",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$StartPgVector,
    [switch]$StartOpenSearch,
    [switch]$PrepareOpenSearchVectorIndex,
    [switch]$AllowPull,
    [int]$DockerTimeoutSeconds = 30,
    [int]$WaitTimeoutSeconds = 90,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
. (Join-Path $repoRoot "tools\docker-runtime.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "search-rag-provider-runtime-readiness-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

function Wait-OpenSearchRoot {
    param(
        [string]$Endpoint,
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $Endpoint -Method Get -TimeoutSec 2
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
                Write-Host "opensearch_ready=$Endpoint"
                return
            }
        } catch {
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for OpenSearch endpoint $Endpoint"
}

function Ensure-OpenSearchVectorIndex {
    param(
        [string]$Endpoint,
        [string]$Index,
        [string]$VectorField,
        [int]$Dimension
    )

    if ($Dimension -le 0) {
        throw "OpenSearchVectorDimension must be greater than zero."
    }
    if ($Endpoint -match "[\?#]") {
        throw "OpenSearchEndpoint must not include query or fragment."
    }

    $properties = @{
        source_ref_hash = @{ type = "keyword" }
        source_service = @{ type = "keyword" }
        source_type = @{ type = "keyword" }
        visibility_version = @{ type = "keyword" }
        tombstone_status = @{ type = "keyword" }
    }
    $properties[$VectorField] = @{
        type = "knn_vector"
        dimension = $Dimension
        method = @{
            name = "hnsw"
            engine = "lucene"
            space_type = "l2"
        }
    }

    $body = @{
        settings = @{
            index = @{
                knn = $true
                number_of_shards = 1
                number_of_replicas = 0
            }
        }
        mappings = @{
            dynamic = "strict"
            properties = $properties
        }
    } | ConvertTo-Json -Depth 20

    $target = ($Endpoint.TrimEnd("/") + "/" + $Index)
    try {
        $response = Invoke-WebRequest `
            -UseBasicParsing `
            -Uri $target `
            -Method Put `
            -ContentType "application/json" `
            -Body $body `
            -TimeoutSec 10
        if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
            Write-Host "opensearch_vector_index_created=$Index"
            return
        }
        throw "OpenSearch create index returned status $($response.StatusCode)"
    } catch {
        $message = $_.Exception.Message
        if ($message -match "resource_already_exists_exception") {
            Write-Host "opensearch_vector_index_exists=$Index"
            return
        }
        throw
    }
}

$needsDocker = $StartPgVector -or $StartOpenSearch
if ($needsDocker) {
    Assert-NexusIMDockerEngine -TimeoutSeconds $DockerTimeoutSeconds
}

if ($StartPgVector) {
    $image = "pgvector/pgvector:pg16"
    Ensure-NexusIMDockerImage `
        -Image $image `
        -TimeoutSeconds $DockerTimeoutSeconds `
        -AllowPull:$AllowPull
    Invoke-NexusIMDockerComposeUp `
        -ComposeFiles @(
            (Join-Path $repoRoot "deploy\local\docker-compose.yml"),
            (Join-Path $repoRoot "deploy\local\docker-compose.pgvector.yml")
        ) `
        -Profiles @("pgvector") `
        -Services @("pgvector") `
        -TimeoutSeconds $DockerTimeoutSeconds
    Wait-NexusIMTcp -HostName "127.0.0.1" -Port 15432 -TimeoutSeconds $WaitTimeoutSeconds
}

if ($StartOpenSearch) {
    $image = "opensearchproject/opensearch:3.3.2"
    Ensure-NexusIMDockerImage `
        -Image $image `
        -TimeoutSeconds $DockerTimeoutSeconds `
        -AllowPull:$AllowPull
    Invoke-NexusIMDockerComposeUp `
        -ComposeFiles @((Join-Path $repoRoot "deploy\local\docker-compose.opensearch.yml")) `
        -Profiles @("search-rag") `
        -Services @("opensearch") `
        -TimeoutSeconds $DockerTimeoutSeconds
    Wait-NexusIMTcp -HostName "127.0.0.1" -Port 9200 -TimeoutSeconds $WaitTimeoutSeconds
    Wait-OpenSearchRoot -Endpoint $OpenSearchEndpoint -TimeoutSeconds $WaitTimeoutSeconds
}

if ($PrepareOpenSearchVectorIndex) {
    Ensure-OpenSearchVectorIndex `
        -Endpoint $OpenSearchEndpoint `
        -Index $OpenSearchIndex `
        -VectorField $OpenSearchVectorField `
        -Dimension $OpenSearchVectorDimension
}

$readinessArgs = @{
    ProviderReadiness = $ProviderReadiness
    PgVectorDsn = $PgVectorDsn
    PgVectorTable = $PgVectorTable
    OpenSearchEndpoint = $OpenSearchEndpoint
    OpenSearchIndex = $OpenSearchIndex
    OpenSearchVectorField = $OpenSearchVectorField
    OpenSearchVectorDimension = $OpenSearchVectorDimension
    RequestTimeout = $RequestTimeout
    ResultRoot = $ResultRoot
    RunName = $RunName
}
if ($SkipBuild) {
    $readinessArgs.SkipBuild = $true
}

& (Join-Path $PSScriptRoot "run-local-provider-readiness.ps1") @readinessArgs
if ($LASTEXITCODE -ne 0) {
    throw "search-rag provider runtime readiness failed with exit code $LASTEXITCODE"
}
