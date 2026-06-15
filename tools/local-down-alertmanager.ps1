$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.alertmanager.yml down
