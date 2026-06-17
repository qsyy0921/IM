param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$KafkaAdminBootstrap = "localhost:9092",
    [string]$RedisClusterAddrs = "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "redis-cluster-failover-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
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
            Apply-PostgresMigration -Path $migration.FullName -Name ("redis-cluster-failover-" + $migration.Name)
        }
    }
}

$faultScript = Join-Path $resultDir "redis-cluster-stop-route-slot-master-and-wait-failover.ps1"

@"
`$ErrorActionPreference = "Stop"
`$routeKey = '$routeKey'
`$ports = @(7000, 7001, 7002, 7003, 7004, 7005)

function Invoke-Redis {
    param(
        [int]`$Port,
        [string[]]`$RedisArgs
    )
    `$oldErrorActionPreference = `$ErrorActionPreference
    `$ErrorActionPreference = "Continue"
    try {
        `$output = @(& docker exec nexusim-redis-cluster-ha redis-cli -p `$Port @RedisArgs 2>`$null)
        `$exitCode = `$LASTEXITCODE
    } finally {
        `$ErrorActionPreference = `$oldErrorActionPreference
    }
    if (`$exitCode -ne 0) { return @() }
    return `$output
}

function Get-ClusterNodes {
    foreach (`$probePort in `$ports) {
        `$nodes = @(Invoke-Redis -Port `$probePort -RedisArgs @("cluster", "nodes"))
        if (`$nodes.Count -gt 0) { return `$nodes }
    }
    return @()
}

function Find-SlotMasterPort {
    param([int]`$Slot)
    `$nodes = @(Get-ClusterNodes)
    foreach (`$line in `$nodes) {
        `$parts = `$line -split '\s+'
        if (`$parts.Length -lt 9) { continue }
        if (`$parts[2] -notmatch 'master') { continue }
        if (`$parts[2] -match 'fail') { continue }
        for (`$i = 8; `$i -lt `$parts.Length; `$i++) {
            `$range = `$parts[`$i]
            if (`$range -match '^\d+$') {
                if ([int]`$range -eq `$Slot) { return [int]((`$parts[1] -split ':')[1] -split '@')[0] }
            } elseif (`$range -match '^(\d+)-(\d+)$') {
                if (`$Slot -ge [int]`$Matches[1] -and `$Slot -le [int]`$Matches[2]) { return [int]((`$parts[1] -split ':')[1] -split '@')[0] }
            }
        }
    }
    return 0
}

`$slotRaw = @(Invoke-Redis -Port 7000 -RedisArgs @("cluster", "keyslot", `$routeKey))
if (`$slotRaw.Count -eq 0) { throw "redis-cli cluster keyslot returned no output for `$routeKey" }
`$slot = [int]`$slotRaw[0].Trim()
`$ownerPort = Find-SlotMasterPort -Slot `$slot
if (`$ownerPort -eq 0) { throw "could not locate Redis Cluster master for slot `$slot key `$routeKey" }

docker exec nexusim-redis-cluster-ha redis-cli -p `$ownerPort shutdown nosave 2>`$null
`$deadline = (Get-Date).AddSeconds(15)
do {
    Start-Sleep -Milliseconds 250
    cmd /c "docker exec nexusim-redis-cluster-ha redis-cli -p `$ownerPort ping >nul 2>nul"
    `$stopped = `$LASTEXITCODE -ne 0
} while (-not `$stopped -and (Get-Date) -lt `$deadline)
if (-not `$stopped) { throw "redis cluster master `$ownerPort did not stop before timeout" }

`$deadline = (Get-Date).AddSeconds(90)
`$newOwnerPort = 0
do {
    Start-Sleep -Seconds 1
    `$newOwnerPort = Find-SlotMasterPort -Slot `$slot
    `$clusterOK = `$false
    foreach (`$probePort in `$ports | Where-Object { `$_ -ne `$ownerPort }) {
        `$clusterInfo = @(Invoke-Redis -Port `$probePort -RedisArgs @("cluster", "info"))
        if (`$clusterInfo | Where-Object { `$_ -match '^cluster_state:ok' }) {
            `$clusterOK = `$true
            break
        }
    }
} while ((-not `$clusterOK -or `$newOwnerPort -eq 0 -or `$newOwnerPort -eq `$ownerPort) -and (Get-Date) -lt `$deadline)

if (-not `$clusterOK) { throw "redis cluster did not return to cluster_state:ok after stopping master `$ownerPort" }
if (`$newOwnerPort -eq 0 -or `$newOwnerPort -eq `$ownerPort) { throw "redis cluster slot `$slot was not promoted to a new master after stopping `$ownerPort" }

Write-Output "redis_cluster_fault_key=`$routeKey"
Write-Output "redis_cluster_fault_slot=`$slot"
Write-Output "redis_cluster_stopped_master_port=`$ownerPort"
Write-Output "redis_cluster_promoted_master_port=`$newOwnerPort"
"@ | Set-Content -LiteralPath $faultScript -Encoding UTF8

& .\tools\local-down-redis-cluster-ha.ps1 | Out-Null
docker compose -f deploy/local/docker-compose.yml up -d postgres kafka | Out-Null
& .\tools\local-up-redis-cluster-ha.ps1 -ClusterAddrs $RedisClusterAddrs
Apply-CoreMigrations

$runArgs = @{
    Scenario = "redis-cluster-failover"
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
    pushgateway_summary = Join-Path $resultDir "pushgateway-summary.json"
    completed_at = (Get-Date).ToString("o")
}

$summaryPath = Join-Path $resultDir "redis-cluster-failover-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
Write-Host "redis_cluster_failover_summary=$summaryPath"
