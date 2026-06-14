$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.yml up -d redis kafka schema-registry kafka-ui
docker compose -f deploy/local/docker-compose.postgres-ha.yml up -d

$deadline = (Get-Date).AddSeconds(180)
do {
    Start-Sleep -Seconds 2
    $health = docker inspect -f "{{.State.Health.Status}}" nexusim-pgpool 2>$null
} while ($health -ne "healthy" -and (Get-Date) -lt $deadline)

if ($health -ne "healthy") {
    throw "PostgreSQL HA pgpool did not become healthy before timeout."
}

$nodes = @(
    docker exec nexusim-pgpool env PGPASSWORD=nexusim psql `
        -U nexusim `
        -h 127.0.0.1 `
        -p 5432 `
        -d postgres `
        -At `
        -c "show pool_nodes;"
)

if ($LASTEXITCODE -ne 0 -or $nodes.Count -lt 1) {
    throw "Failed to query pgpool backend nodes."
}

$primaryLine = $nodes | Where-Object { $_ -match "\|primary\|" } | Select-Object -First 1
if (-not $primaryLine) {
    throw "pgpool did not report a primary backend."
}

$parts = $primaryLine -split "\|"
$primaryHost = $parts[1].Trim()
$primaryPort = $parts[2].Trim()

Write-Host "postgres_ha_pgpool=127.0.0.1:15432"
Write-Host "postgres_ha_primary=$primaryHost`:$primaryPort"
Write-Host "postgres_ha_config=OK"
