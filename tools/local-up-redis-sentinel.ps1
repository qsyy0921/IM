$ErrorActionPreference = "Stop"

docker compose `
    -f deploy/local/docker-compose.yml `
    -f deploy/local/docker-compose.redis-sentinel.yml `
    up -d redis-ha-master redis-ha-replica-1 redis-ha-replica-2 redis-sentinel-1 redis-sentinel-2 redis-sentinel-3
