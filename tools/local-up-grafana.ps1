param(
    [switch]$AllowImagePull
)

$ErrorActionPreference = "Stop"

$image = if ($env:NEXUSIM_GRAFANA_IMAGE) { $env:NEXUSIM_GRAFANA_IMAGE } else { "grafana/grafana-oss:11.2.0" }
$previousErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = "Continue"
docker image inspect $image > $null 2> $null
$inspectExitCode = $LASTEXITCODE
$ErrorActionPreference = $previousErrorActionPreference
if ($inspectExitCode -ne 0 -and -not $AllowImagePull) {
    throw "Missing Docker image $image. Pull it explicitly or rerun with -AllowImagePull to let docker compose pull it."
}

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
Write-Host "prometheus_datasource=http://host.docker.internal:19091"
