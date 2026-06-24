param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PgVectorDsn = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$KnowledgeGrpcAddr = "",
    [string]$ModelGatewayGrpcAddr = "",
    [string]$VectorGrpcAddr = "",
    [string]$PgVectorTable = "vector_embedding_items",
    [string]$RequestTimeout = "5s",
    [switch]$StartPgVector,
    [switch]$AllowPull,
    [int]$DockerTimeoutSeconds = 30,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
. (Join-Path $repoRoot "tools\docker-runtime.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "vector-pgvector-provider-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

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
    Assert-NexusIMDockerEngine -TimeoutSeconds $DockerTimeoutSeconds
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
    Wait-Tcp -HostName "127.0.0.1" -Port 15432
}

. (Join-Path $repoRoot "tools\go-env.ps1")
if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\vector-embedding-smoke.exe") ./loadtest/vectorembedding
    if ($LASTEXITCODE -ne 0) {
        throw "failed to build vector embedding smoke runner"
    }
}

$runner = Join-Path $repoRoot "bin\vector-embedding-smoke.exe"
$runnerArgs = @(
    "--phase", "preflight-pgvector",
    "--pgvector-dsn", $PgVectorDsn,
    "--pgvector-table", $PgVectorTable,
    "--request-timeout", $RequestTimeout,
    "--result-root", $ResultRoot,
    "--run-name", $RunName
)
& $runner @runnerArgs
if ($LASTEXITCODE -ne 0) {
    throw "vector pgvector preflight failed with exit code $LASTEXITCODE"
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
