param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PgVectorDsn = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KnowledgeGrpcAddr = "",
    [string]$ModelGatewayGrpcAddr = "",
    [string]$VectorGrpcAddr = "",
    [string]$PgVectorTable = "vector_embedding_items",
    [switch]$StartPgVector,
    [switch]$AllowPull,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

function Wait-Tcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 40
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
        Start-Sleep -Milliseconds 250
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

if ($StartPgVector) {
    $image = "pgvector/pgvector:pg16"
    $imageId = docker image ls $image -q
    if (-not $imageId) {
        if (-not $AllowPull) {
            throw "Missing Docker image $image. Pull manually or rerun with -AllowPull; this script does not pull by default."
        }
        docker pull $image
        if ($LASTEXITCODE -ne 0) {
            throw "failed to pull $image"
        }
    }
    $baseCompose = Join-Path $repoRoot "deploy\local\docker-compose.yml"
    $pgVectorCompose = Join-Path $repoRoot "deploy\local\docker-compose.pgvector.yml"
    docker compose `
        -f $baseCompose `
        -f $pgVectorCompose `
        --profile pgvector `
        up -d pgvector
    if ($LASTEXITCODE -ne 0) {
        throw "failed to start pgvector compose profile"
    }
    Wait-Tcp -HostName "127.0.0.1" -Port 15432
}

$args = @(
    "-PgDsn", $PgDsn,
    "-PgVectorDsn", $PgVectorDsn,
    "-PgVectorTable", $PgVectorTable,
    "-ResultRoot", $ResultRoot,
    "-VectorProviderBackend", "pgvector"
)
if ($RunName) {
    $args += @("-RunName", $RunName)
}
if ($KnowledgeGrpcAddr) {
    $args += @("-KnowledgeGrpcAddr", $KnowledgeGrpcAddr)
}
if ($ModelGatewayGrpcAddr) {
    $args += @("-ModelGatewayGrpcAddr", $ModelGatewayGrpcAddr)
}
if ($VectorGrpcAddr) {
    $args += @("-VectorGrpcAddr", $VectorGrpcAddr)
}
if ($SkipBuild) {
    $args += "-SkipBuild"
}

& (Join-Path $PSScriptRoot "run-local-smoke.ps1") @args
if ($LASTEXITCODE -ne 0) {
    throw "vector pgvector smoke failed with exit code $LASTEXITCODE"
}
