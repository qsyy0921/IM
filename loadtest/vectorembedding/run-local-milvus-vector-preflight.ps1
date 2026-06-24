param(
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
    $RunName = "milvus-vector-preflight-" + (Get-Date -Format "yyyyMMdd-HHmmss")
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
    --phase preflight-milvus-vector `
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
    throw "Milvus vector preflight failed with exit code $LASTEXITCODE"
}
