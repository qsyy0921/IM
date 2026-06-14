$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.postgres-ha.yml down
