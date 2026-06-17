param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$KafkaAdminBootstrap = "localhost:9092",
    [string]$RedisClusterAddrs = "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005",
    [int]$MessageCount = 32,
    [string]$ReceiverDeviceIds = "push-device-1,push-device-2",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if ($MessageCount -lt 1) {
    throw "-MessageCount must be positive"
}

if (-not $RunName) {
    $RunName = "redis-cluster-capacity-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
$resultDir = Join-Path $ResultRoot $RunName
$redisKeyPrefix = "nexusim:push:$safeRunName"

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
        "migrations\postgres\message",
        "migrations\postgres\conversation",
        "migrations\postgres\delivery"
    )
    foreach ($dir in $migrationSets) {
        foreach ($migration in Get-ChildItem -LiteralPath (Join-Path $repo $dir) -Filter "*.sql" | Sort-Object Name) {
            Apply-PostgresMigration -Path $migration.FullName -Name ("redis-cluster-capacity-" + $migration.Name)
        }
    }
}

& .\tools\local-down-redis-cluster-ha.ps1 | Out-Null
docker compose -f deploy/local/docker-compose.yml up -d postgres kafka | Out-Null
& .\tools\local-up-redis-cluster-ha.ps1 -ClusterAddrs $RedisClusterAddrs
Apply-CoreMigrations

$runArgs = @{
    Scenario = "full"
    RouteBackend = "redis"
    RedisMode = "cluster"
    RedisClusterAddrs = $RedisClusterAddrs
    RedisKeyPrefix = $redisKeyPrefix
    TenantId = "tenant-$safeRunName"
    ConversationId = "conv-$safeRunName"
    ReceiverDeviceIds = $ReceiverDeviceIds
    MessageCount = $MessageCount
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

$summary = [ordered]@{
    run_name = $RunName
    git_commit = (git rev-parse HEAD)
    git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
    redis_mode = "cluster"
    redis_cluster_addrs = $RedisClusterAddrs
    receiver_device_ids = $ReceiverDeviceIds
    message_count = $MessageCount
    pushgateway_summary = Join-Path $resultDir "pushgateway-summary.json"
    completed_at = (Get-Date).ToString("o")
}

$summaryPath = Join-Path $resultDir "redis-cluster-capacity-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
Write-Host "redis_cluster_capacity_summary=$summaryPath"
