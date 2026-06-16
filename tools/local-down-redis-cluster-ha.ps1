docker compose `
    -f deploy/local/docker-compose.redis-cluster-ha.yml `
    down -v

cmd /c "docker rm -f nexusim-redis-cluster-ha >nul 2>nul"
