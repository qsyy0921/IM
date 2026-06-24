param(
    [string]$ProviderReadiness = "pgvector,opensearch-vector",
    [string]$PgVectorDsn = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable",
    [string]$PgVectorTable = "vector_embedding_items",
    [string]$OpenSearchEndpoint = "http://127.0.0.1:9200",
    [string]$OpenSearchIndex = "nexusim-vector-items",
    [string]$OpenSearchVectorField = "embedding_vector",
    [int]$OpenSearchVectorDimension = 8,
    [string]$MilvusEndpoint = "http://127.0.0.1:19530",
    [string]$MilvusToken = "",
    [string]$MilvusDatabase = "_default",
    [string]$MilvusCollection = "nexusim_vector_items",
    [string]$MilvusVectorField = "embedding_vector",
    [int]$MilvusVectorDimension = 8,
    [string]$RequestTimeout = "5s",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "vector-provider-readiness-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

. (Join-Path $repoRoot "tools\go-env.ps1")
if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\vector-embedding-smoke.exe") ./loadtest/vectorembedding
    if ($LASTEXITCODE -ne 0) {
        throw "failed to build vector embedding smoke runner"
    }
}

$runner = Join-Path $repoRoot "bin\vector-embedding-smoke.exe"
& $runner `
    --phase preflight-provider-readiness `
    --provider-readiness $ProviderReadiness `
    --pgvector-dsn $PgVectorDsn `
    --pgvector-table $PgVectorTable `
    --opensearch-vector-endpoint $OpenSearchEndpoint `
    --opensearch-vector-index $OpenSearchIndex `
    --opensearch-vector-field $OpenSearchVectorField `
    --opensearch-vector-dimension $OpenSearchVectorDimension `
    --milvus-endpoint $MilvusEndpoint `
    --milvus-token $MilvusToken `
    --milvus-database $MilvusDatabase `
    --milvus-collection $MilvusCollection `
    --milvus-vector-field $MilvusVectorField `
    --milvus-vector-dimension $MilvusVectorDimension `
    --request-timeout $RequestTimeout `
    --result-root $ResultRoot `
    --run-name $RunName
if ($LASTEXITCODE -ne 0) {
    throw "vector provider readiness failed with exit code $LASTEXITCODE"
}
