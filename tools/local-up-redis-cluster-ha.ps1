param(
    [string]$ClusterAddrs = "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005"
)

$ErrorActionPreference = "Stop"

cmd /c "docker rm -f nexusim-redis-cluster-ha >nul 2>nul"

docker compose `
    -f deploy/local/docker-compose.redis-cluster-ha.yml `
    up -d redis-cluster-ha

$ports = @(7000, 7001, 7002, 7003, 7004, 7005)
$deadline = (Get-Date).AddSeconds(60)
foreach ($port in $ports) {
    do {
        Start-Sleep -Seconds 1
        $ping = @(docker exec nexusim-redis-cluster-ha redis-cli -p $port ping 2>$null)
        $ready = $ping.Count -gt 0 -and $ping[0].Trim() -eq "PONG"
    } while (-not $ready -and (Get-Date) -lt $deadline)
    if (-not $ready) {
        throw "Redis Cluster HA node on port $port did not become ready."
    }
}

$clusterInfo = @(docker exec nexusim-redis-cluster-ha redis-cli -p 7000 cluster info 2>$null)
$clusterOK = $clusterInfo | Where-Object { $_ -match '^cluster_state:ok' }
if (-not $clusterOK) {
    $addrs = $ClusterAddrs.Split(",") | ForEach-Object { $_.Trim() } | Where-Object { $_ }
    if ($addrs.Count -lt 6) {
        throw "Redis Cluster HA requires at least 6 node addresses."
    }
    docker exec nexusim-redis-cluster-ha redis-cli --cluster create $addrs --cluster-replicas 1 --cluster-yes | Out-Null
}

$deadline = (Get-Date).AddSeconds(90)
do {
    Start-Sleep -Seconds 1
    $clusterInfo = @(docker exec nexusim-redis-cluster-ha redis-cli -p 7000 cluster info 2>$null)
    $clusterOK = $clusterInfo | Where-Object { $_ -match '^cluster_state:ok' }
} while (-not $clusterOK -and (Get-Date) -lt $deadline)

if (-not $clusterOK) {
    throw "Redis Cluster HA did not reach cluster_state:ok before timeout."
}

foreach ($port in $ports) {
    $tcp = Test-NetConnection -ComputerName "127.0.0.1" -Port $port -WarningAction SilentlyContinue
    if (-not $tcp.TcpTestSucceeded) {
        throw "Redis Cluster HA node 127.0.0.1:$port is not reachable from host."
    }
}

$nodes = @(docker exec nexusim-redis-cluster-ha redis-cli -p 7000 cluster nodes)
$masters = $nodes | Where-Object { $_ -match '\bmaster\b' -and $_ -notmatch '\bfail\b' }
$replicas = $nodes | Where-Object { $_ -match '\bslave\b' -and $_ -notmatch '\bfail\b' }
Write-Host "redis_cluster_ha_addrs=$ClusterAddrs"
Write-Host "redis_cluster_ha_node_lines=$($nodes.Count)"
Write-Host "redis_cluster_ha_masters=$($masters.Count)"
Write-Host "redis_cluster_ha_replicas=$($replicas.Count)"
Write-Host "redis_cluster_ha_config=OK"
