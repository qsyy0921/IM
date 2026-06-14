$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.kafka-ha.yml up -d

$containers = @(
    "nexusim-kafka-ha-0",
    "nexusim-kafka-ha-1",
    "nexusim-kafka-ha-2"
)

$deadline = (Get-Date).AddSeconds(180)
foreach ($container in $containers) {
    do {
        Start-Sleep -Seconds 2
        $health = docker inspect -f "{{.State.Health.Status}}" $container 2>$null
    } while ($health -ne "healthy" -and (Get-Date) -lt $deadline)
    if ($health -ne "healthy") {
        throw "Kafka HA broker $container did not become healthy before timeout."
    }
}

Write-Host "kafka_ha_bootstrap=127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094"
Write-Host "kafka_ha_admin_exec=nexusim-kafka-ha-0"
Write-Host "kafka_ha_admin_bootstrap=kafka-ha-0:29092"
Write-Host "kafka_ha_replication_factor=3"
