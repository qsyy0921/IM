docker compose `
    -f deploy/local/docker-compose.yml `
    -f deploy/local/docker-compose.redis-sentinel.yml `
    down -v

$containers = @(
    "nexusim-redis-ha-master",
    "nexusim-redis-ha-replica-1",
    "nexusim-redis-ha-replica-2",
    "nexusim-redis-sentinel-1",
    "nexusim-redis-sentinel-2",
    "nexusim-redis-sentinel-3"
)
foreach ($container in $containers) {
    cmd /c "docker rm -f $container >nul 2>nul"
}
