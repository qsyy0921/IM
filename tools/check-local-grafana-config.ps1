$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.grafana.yml"
$datasourcePath = Join-Path $repoRoot "deploy\local\grafana-datasources.yml"
$providerPath = Join-Path $repoRoot "deploy\local\grafana-dashboards.yml"
$dashboardPath = Join-Path $repoRoot "deploy\local\grafana\dashboards\api-gateway-observability.json"

foreach ($path in @($composePath, $datasourcePath, $providerPath, $dashboardPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing local Grafana config file: $path"
    }
}

$compose = Get-Content -LiteralPath $composePath -Raw
$datasource = Get-Content -LiteralPath $datasourcePath -Raw
$provider = Get-Content -LiteralPath $providerPath -Raw
$dashboardRaw = Get-Content -LiteralPath $dashboardPath -Raw

if ($compose -notmatch "13000:3000") {
    throw "Grafana compose must expose host port 13000 to avoid existing local service ports."
}
if ($datasource -notmatch "http://host\.docker\.internal:19090") {
    throw "Grafana datasource must point at the local Prometheus host port through host.docker.internal:19090."
}
if ($provider -notmatch "/var/lib/grafana/dashboards") {
    throw "Grafana dashboard provider must load dashboards from /var/lib/grafana/dashboards."
}

try {
    $dashboard = $dashboardRaw | ConvertFrom-Json
} catch {
    throw "Grafana api-gateway dashboard is not valid JSON: $($_.Exception.Message)"
}

if ($dashboard.uid -ne "nexusim-api-gateway") {
    throw "Grafana api-gateway dashboard uid mismatch."
}
if (-not $dashboard.panels -or $dashboard.panels.Count -lt 5) {
    throw "Grafana api-gateway dashboard should include core observability panels."
}

$requiredMetrics = @(
    "nexusim_api_gateway_grpc_requests_total",
    "nexusim_api_gateway_grpc_errors_total",
    "nexusim_api_gateway_grpc_exposure_requests_total",
    "nexusim_api_gateway_grpc_legacy_descriptor_last_seen_unix_milliseconds",
    "nexusim_api_gateway_grpc_latency_avg_milliseconds",
    "nexusim_api_gateway_rate_limit_limited_total",
    "nexusim_api_gateway_auth_jwks_refresh_failures_total",
    "nexusim_api_gateway_otel_traces_enabled"
)

foreach ($metric in $requiredMetrics) {
    if ($dashboardRaw -notmatch [regex]::Escape($metric)) {
        throw "Grafana api-gateway dashboard missing metric: $metric"
    }
}

$forbiddenLabels = @("tenant_id", "user_id", "device_id", "session_id", "request_id", "trace_id", "gateway-token")
foreach ($label in $forbiddenLabels) {
    if ($dashboardRaw -match [regex]::Escape($label)) {
        throw "Grafana api-gateway dashboard must not reference sensitive or high-cardinality field: $label"
    }
}

Write-Host "OK   local Grafana config"
