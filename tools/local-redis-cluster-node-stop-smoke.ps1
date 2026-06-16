param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$KafkaAdminBootstrap = "localhost:9092",
    [string]$RedisClusterAddrs = "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "redis-cluster-node-stop-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$safeRunName = $RunName -replace '[^a-zA-Z0-9_-]', '-'
$resultDir = Join-Path $ResultRoot $RunName
$redisKeyPrefix = "nexusim:push:$safeRunName"
$tenantID = "tenant-$safeRunName"
$receiverUserID = "push-user-1"
$routeKey = "${redisKeyPrefix}:route:{user:${tenantID}:${receiverUserID}}:user"

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
            Apply-PostgresMigration -Path $migration.FullName -Name ("redis-cluster-node-stop-" + $migration.Name)
        }
    }
}

$faultScript = Join-Path $resultDir "redis-cluster-stop-route-slot-owner.ps1"
$restoreScript = Join-Path $resultDir "redis-cluster-restore-route-slot-owner.ps1"
$statePath = Join-Path $resultDir "redis-cluster-stopped-port.txt"

@"
`$ErrorActionPreference = "Stop"
`$routeKey = '$routeKey'
`$statePath = '$statePath'
`$slotRaw = @(docker exec nexusim-redis-cluster redis-cli -p 7000 cluster keyslot `$routeKey)
if (`$slotRaw.Count -eq 0) { throw "redis-cli cluster keyslot returned no output for `$routeKey" }
`$slot = [int]`$slotRaw[0].Trim()
`$nodes = @(docker exec nexusim-redis-cluster redis-cli -p 7000 cluster nodes)
`$ownerPort = 0
foreach (`$line in `$nodes) {
    `$parts = `$line -split '\s+'
    if (`$parts.Length -lt 9) { continue }
    if (`$parts[2] -notmatch 'master') { continue }
    for (`$i = 8; `$i -lt `$parts.Length; `$i++) {
        `$range = `$parts[`$i]
        if (`$range -match '^\d+$') {
            if ([int]`$range -eq `$slot) { `$ownerPort = [int]((`$parts[1] -split ':')[1] -split '@')[0] }
        } elseif (`$range -match '^(\d+)-(\d+)$') {
            if (`$slot -ge [int]`$Matches[1] -and `$slot -le [int]`$Matches[2]) { `$ownerPort = [int]((`$parts[1] -split ':')[1] -split '@')[0] }
        }
        if (`$ownerPort -gt 0) { break }
    }
    if (`$ownerPort -gt 0) { break }
}
if (`$ownerPort -eq 0) { throw "could not locate Redis Cluster owner for slot `$slot key `$routeKey" }
Set-Content -LiteralPath `$statePath -Value `$ownerPort -Encoding ASCII
docker exec nexusim-redis-cluster redis-cli -p `$ownerPort shutdown nosave 2>`$null
`$deadline = (Get-Date).AddSeconds(10)
do {
    Start-Sleep -Milliseconds 250
    cmd /c "docker exec nexusim-redis-cluster redis-cli -p `$ownerPort ping >nul 2>nul"
    `$stopped = `$LASTEXITCODE -ne 0
} while (-not `$stopped -and (Get-Date) -lt `$deadline)
if (-not `$stopped) { throw "redis cluster node `$ownerPort did not stop before timeout" }
Write-Output "redis_cluster_fault_key=`$routeKey"
Write-Output "redis_cluster_fault_slot=`$slot"
Write-Output "redis_cluster_stopped_port=`$ownerPort"
"@ | Set-Content -LiteralPath $faultScript -Encoding UTF8

@"
`$ErrorActionPreference = "Stop"
`$statePath = '$statePath'
if (-not (Test-Path -LiteralPath `$statePath)) {
    Write-Output "redis_cluster_restore_skipped=no_stopped_port"
    exit 0
}
`$port = [int](Get-Content -LiteralPath `$statePath -Raw)
docker restart nexusim-redis-cluster | Out-Null

foreach (`$probePort in @(7000, 7001, 7002)) {
    `$deadline = (Get-Date).AddSeconds(30)
    do {
        Start-Sleep -Milliseconds 500
        cmd /c "docker exec nexusim-redis-cluster redis-cli -p `$probePort ping >nul 2>nul"
        `$ready = `$LASTEXITCODE -eq 0
    } while (-not `$ready -and (Get-Date) -lt `$deadline)
    if (-not `$ready) { throw "redis cluster node `$probePort did not restart" }
}

`$deadline = (Get-Date).AddSeconds(30)
do {
    Start-Sleep -Milliseconds 500
    `$clusterInfo = @(docker exec nexusim-redis-cluster redis-cli -p 7000 cluster info 2>`$null)
    `$clusterOK = `$clusterInfo | Where-Object { `$_ -match '^cluster_state:ok' }
} while (-not `$clusterOK -and (Get-Date) -lt `$deadline)
if (-not `$clusterOK) { throw "redis cluster did not return to cluster_state:ok after restarting container" }
Remove-Item -LiteralPath `$statePath -Force
Write-Output "redis_cluster_restored_port=`$port"
"@ | Set-Content -LiteralPath $restoreScript -Encoding UTF8

& .\tools\local-down-redis-cluster.ps1 | Out-Null
docker compose -f deploy/local/docker-compose.yml up -d postgres kafka | Out-Null
& .\tools\local-up-redis-cluster.ps1 -ClusterAddrs $RedisClusterAddrs
Apply-CoreMigrations

$runArgs = @{
    Scenario = "redis-cluster-node-stop"
    RouteBackend = "redis"
    RedisMode = "cluster"
    RedisClusterAddrs = $RedisClusterAddrs
    RedisKeyPrefix = $redisKeyPrefix
    TenantId = $tenantID
    ConversationId = "conv-$safeRunName"
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    KafkaBrokers = $KafkaBrokers
    KafkaExecContainer = $KafkaExecContainer
    KafkaAdminBootstrap = $KafkaAdminBootstrap
    ResultRoot = $ResultRoot
    RunName = $RunName
    RedisFaultCommand = "& '$faultScript'"
    RedisRestoreCommand = "& '$restoreScript'"
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
    redis_route_key = $routeKey
    fault_script = $faultScript
    restore_script = $restoreScript
    pushgateway_summary = Join-Path $resultDir "pushgateway-summary.json"
    completed_at = (Get-Date).ToString("o")
}

$summaryPath = Join-Path $resultDir "redis-cluster-node-stop-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
Write-Host "redis_cluster_node_stop_summary=$summaryPath"
