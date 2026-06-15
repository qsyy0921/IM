$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.prometheus.yml up -d

$deadline = (Get-Date).AddSeconds(60)
$ready = $false
do {
    Start-Sleep -Seconds 1
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:19090/-/ready" -TimeoutSec 2
        $ready = ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
    } catch {
        $ready = $false
    }
} while (-not $ready -and (Get-Date) -lt $deadline)

if (-not $ready) {
    docker logs --tail 80 nexusim-prometheus 2>$null
    throw "Prometheus did not become ready before timeout."
}

Write-Host "prometheus_url=http://127.0.0.1:19090"
Write-Host "api_gateway_scrape_target=host.docker.internal:11904"
Write-Host "identity_service_scrape_target=host.docker.internal:11905"
Write-Host "message_service_scrape_target=host.docker.internal:11910"
Write-Host "conversation_service_scrape_target=host.docker.internal:11911"
Write-Host "delivery_service_scrape_target=host.docker.internal:11912"
Write-Host "push_gateway_scrape_target=host.docker.internal:11913"
Write-Host "receipt_service_scrape_target=host.docker.internal:11914"
