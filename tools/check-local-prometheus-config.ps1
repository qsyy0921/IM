$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.prometheus.yml"
$configPath = Join-Path $repoRoot "deploy\local\prometheus.yml"
$rulesPath = Join-Path $repoRoot "deploy\local\prometheus-api-gateway-alerts.yml"

foreach ($path in @($composePath, $configPath, $rulesPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing local Prometheus config file: $path"
    }
}

$compose = Get-Content -LiteralPath $composePath -Raw
$config = Get-Content -LiteralPath $configPath -Raw
$rules = Get-Content -LiteralPath $rulesPath -Raw

if ($compose -notmatch "19090:9090") {
    throw "Prometheus compose must expose host port 19090 to avoid existing local service ports."
}
if ($config -notmatch "metrics_path:\s*/metrics") {
    throw "Prometheus config must scrape the api-gateway /metrics endpoint."
}
if ($config -notmatch "host\.docker\.internal:11904") {
    throw "Prometheus config must target the local api-gateway debug endpoint through host.docker.internal:11904."
}

$requiredAlerts = @(
    "NexusIMApiGatewayGrpcErrors",
    "NexusIMApiGatewayLegacyDescriptorTraffic",
    "NexusIMApiGatewayRateLimitRedisErrors",
    "NexusIMApiGatewayTenantQuotaSnapshotStale",
    "NexusIMApiGatewayJwksRefreshFailures",
    "NexusIMApiGatewayOtlpEndpointMissing"
)

foreach ($alert in $requiredAlerts) {
    if ($rules -notmatch [regex]::Escape($alert)) {
        throw "Prometheus api-gateway rules missing alert: $alert"
    }
}

Write-Host "OK   local Prometheus config"
