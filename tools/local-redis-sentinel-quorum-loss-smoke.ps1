param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$KafkaAdminBootstrap = "localhost:9092",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "redis-sentinel-quorum-loss-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = (Resolve-Path $Path).Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "${PostgresExecContainer}:$containerPath" | Out-Null
    docker exec $PostgresExecContainer psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "failed to apply migration $Name"
    }
}

function Apply-CoreMigrations {
    $migrationSets = @(
        "migrations\\postgres\\message",
        "migrations\\postgres\\conversation",
        "migrations\\postgres\\delivery"
    )
    foreach ($dir in $migrationSets) {
        foreach ($migration in Get-ChildItem -LiteralPath (Join-Path $repo $dir) -Filter "*.sql" | Sort-Object Name) {
            Apply-PostgresMigration -Path $migration.FullName -Name ("redis-quorum-" + $migration.Name)
        }
    }
}

& .\tools\local-down-redis-sentinel.ps1 | Out-Null
docker compose -f deploy/local/docker-compose.yml up -d postgres kafka | Out-Null
& .\tools\local-up-redis-sentinel.ps1
Apply-CoreMigrations

$summary = [ordered]@{
    run_name = $RunName
    git_commit = (git rev-parse HEAD)
    git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
    pg_dsn = $PgDsn
    postgres_exec_container = $PostgresExecContainer
    kafka_brokers = $KafkaBrokers
    redis_mode = "sentinel"
    redis_sentinel_addrs = "127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381"
    redis_sentinel_master_name = "mymaster"
}

$runArgs = @{
    Scenario = "redis-sentinel-quorum-loss"
    RouteBackend = "redis"
    RedisMode = "sentinel"
    RedisSentinelAddrs = "127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381"
    RedisSentinelMasterName = "mymaster"
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    KafkaBrokers = $KafkaBrokers
    KafkaExecContainer = $KafkaExecContainer
    KafkaAdminBootstrap = $KafkaAdminBootstrap
    ResultRoot = $ResultRoot
    RunName = $RunName
}
if ($SkipBuild) {
    $runArgs.SkipBuild = $true
}

& .\loadtest\pushgateway\run-local-smoke.ps1 @runArgs

$summary.pushgateway_summary = Join-Path $resultDir "pushgateway-summary.json"
$summary.completed_at = (Get-Date).ToString("o")

$summaryPath = Join-Path $resultDir "redis-sentinel-quorum-loss-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

Write-Host "redis_sentinel_quorum_loss_summary=$summaryPath"
