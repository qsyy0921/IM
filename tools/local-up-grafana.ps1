$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.grafana.yml up -d

$deadline = (Get-Date).AddSeconds(60)
$ready = $false
do {
    Start-Sleep -Seconds 1
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:13000/api/health" -TimeoutSec 2
        $ready = ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
    } catch {
        $ready = $false
    }
} while (-not $ready -and (Get-Date) -lt $deadline)

if (-not $ready) {
    docker logs --tail 80 nexusim-grafana 2>$null
    throw "Grafana did not become ready before timeout."
}

Write-Host "grafana_url=http://127.0.0.1:13000"
Write-Host "grafana_login=admin / nexusim"
Write-Host "prometheus_datasource=http://host.docker.internal:19090"
