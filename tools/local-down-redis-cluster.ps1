docker compose `
    -f deploy/local/docker-compose.redis-cluster.yml `
    down -v

cmd /c "docker rm -f nexusim-redis-cluster >nul 2>nul"
