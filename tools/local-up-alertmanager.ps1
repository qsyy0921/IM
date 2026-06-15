param(
    [switch]$AllowImagePull
)

$ErrorActionPreference = "Stop"

$image = if ($env:NEXUSIM_ALERTMANAGER_IMAGE) { $env:NEXUSIM_ALERTMANAGER_IMAGE } else { "prom/alertmanager:v0.27.0" }
$previousErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = "Continue"
docker image inspect $image > $null 2> $null
$inspectExitCode = $LASTEXITCODE
$ErrorActionPreference = $previousErrorActionPreference
if ($inspectExitCode -ne 0 -and -not $AllowImagePull) {
    throw "Missing Docker image $image. Pull it explicitly or rerun with -AllowImagePull to let docker compose pull it."
}

docker compose -f deploy/local/docker-compose.alertmanager.yml up -d

$deadline = (Get-Date).AddSeconds(60)
$ready = $false
do {
    Start-Sleep -Seconds 1
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:19093/-/ready" -TimeoutSec 2
        $ready = ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500)
    } catch {
        $ready = $false
    }
} while (-not $ready -and (Get-Date) -lt $deadline)

if (-not $ready) {
    docker logs --tail 80 nexusim-alertmanager 2>$null
    throw "Alertmanager did not become ready before timeout."
}

Write-Host "alertmanager_url=http://127.0.0.1:19093"
Write-Host "prometheus_alertmanager_target=host.docker.internal:19093"
