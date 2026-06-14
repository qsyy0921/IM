$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.kafka-ha.yml down
