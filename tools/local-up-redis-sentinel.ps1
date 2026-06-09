param(
    [string]$AnnounceIP = "172.31.50.1"
)

$ErrorActionPreference = "Stop"

$env:NEXUSIM_REDIS_SENTINEL_ANNOUNCE_IP = $AnnounceIP

docker compose `
    -f deploy/local/docker-compose.yml `
    -f deploy/local/docker-compose.redis-sentinel.yml `
    up -d redis-ha-master redis-ha-replica-1 redis-ha-replica-2 redis-sentinel-1 redis-sentinel-2 redis-sentinel-3

$deadline = (Get-Date).AddSeconds(60)
$master = @()
do {
    Start-Sleep -Seconds 2
    $master = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>$null)
} while (($master.Count -lt 2 -or [string]::IsNullOrWhiteSpace($master[0]) -or [string]::IsNullOrWhiteSpace($master[1])) -and (Get-Date) -lt $deadline)

if ($master.Count -lt 2) {
    throw "Redis Sentinel did not report mymaster before timeout."
}

$masterHost = $master[0].Trim()
$masterPort = [int]$master[1].Trim()
$tcp = Test-NetConnection -ComputerName $masterHost -Port $masterPort -WarningAction SilentlyContinue
if (-not $tcp.TcpTestSucceeded) {
    throw "Redis Sentinel reported master $masterHost`:$masterPort, but host TCP connection failed."
}

$ping = docker exec nexusim-redis-sentinel-1 redis-cli -h $masterHost -p $masterPort ping
if (($ping | Select-Object -First 1) -ne "PONG") {
    throw "Redis Sentinel reported master $masterHost`:$masterPort, but Redis PING failed: $ping"
}

$sentinels = @(docker exec nexusim-redis-sentinel-1 redis-cli -p 26379 SENTINEL sentinels mymaster)
Write-Host "redis_sentinel_master=$masterHost`:$masterPort"
Write-Host "redis_sentinel_peer_output_lines=$($sentinels.Count)"
Write-Host "redis_sentinel_config=OK"
