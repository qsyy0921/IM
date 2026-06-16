param(
    [string]$ClusterAddrs = "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002"
)

$ErrorActionPreference = "Stop"

cmd /c "docker rm -f nexusim-redis-cluster >nul 2>nul"

docker compose `
    -f deploy/local/docker-compose.redis-cluster.yml `
    up -d redis-cluster

$ports = @(7000, 7001, 7002)
$deadline = (Get-Date).AddSeconds(60)
foreach ($port in $ports) {
    do {
        Start-Sleep -Seconds 1
        $ping = @(docker exec nexusim-redis-cluster redis-cli -p $port ping 2>$null)
        $ready = $ping.Count -gt 0 -and $ping[0].Trim() -eq "PONG"
    } while (-not $ready -and (Get-Date) -lt $deadline)
    if (-not $ready) {
        throw "Redis Cluster node on port $port did not become ready."
    }
}

$clusterInfo = @(docker exec nexusim-redis-cluster redis-cli -p 7000 cluster info 2>$null)
$clusterOK = $clusterInfo | Where-Object { $_ -match '^cluster_state:ok' }
if (-not $clusterOK) {
    $addrs = $ClusterAddrs.Split(",") | ForEach-Object { $_.Trim() } | Where-Object { $_ }
    if ($addrs.Count -lt 3) {
        throw "Redis Cluster requires at least 3 node addresses."
    }
    docker exec nexusim-redis-cluster redis-cli --cluster create $addrs --cluster-replicas 0 --cluster-yes | Out-Null
}

$deadline = (Get-Date).AddSeconds(60)
do {
    Start-Sleep -Seconds 1
    $clusterInfo = @(docker exec nexusim-redis-cluster redis-cli -p 7000 cluster info 2>$null)
    $clusterOK = $clusterInfo | Where-Object { $_ -match '^cluster_state:ok' }
} while (-not $clusterOK -and (Get-Date) -lt $deadline)

if (-not $clusterOK) {
    throw "Redis Cluster did not reach cluster_state:ok before timeout."
}

foreach ($port in $ports) {
    $tcp = Test-NetConnection -ComputerName "127.0.0.1" -Port $port -WarningAction SilentlyContinue
    if (-not $tcp.TcpTestSucceeded) {
        throw "Redis Cluster node 127.0.0.1:$port is not reachable from host."
    }
}

$nodes = @(docker exec nexusim-redis-cluster redis-cli -p 7000 cluster nodes)
Write-Host "redis_cluster_addrs=$ClusterAddrs"
Write-Host "redis_cluster_node_lines=$($nodes.Count)"
Write-Host "redis_cluster_config=OK"
