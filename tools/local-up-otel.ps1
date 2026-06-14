$ErrorActionPreference = "Stop"

docker compose -f deploy/local/docker-compose.otel.yml up -d

$deadline = (Get-Date).AddSeconds(60)
$ready = $false
do {
    Start-Sleep -Seconds 1
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:13133/" -TimeoutSec 2
        $ready = ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
    } catch {
        $ready = $false
    }
} while (-not $ready -and (Get-Date) -lt $deadline)

if (-not $ready) {
    docker logs --tail 80 nexusim-otel-collector 2>$null
    throw "OpenTelemetry collector did not become ready before timeout."
}

Write-Host "otel_collector_otlp_grpc=127.0.0.1:4317"
Write-Host "otel_collector_otlp_http=http://127.0.0.1:4318"
Write-Host "otel_collector_health=http://127.0.0.1:13133"
