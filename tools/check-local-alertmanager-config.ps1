$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$composePath = Join-Path $repoRoot "deploy\local\docker-compose.alertmanager.yml"
$configPath = Join-Path $repoRoot "deploy\local\alertmanager.yml"
$prometheusPath = Join-Path $repoRoot "deploy\local\prometheus.yml"
$runbookPath = Join-Path $repoRoot "docs\runbook\observability-local.md"
$upScriptPath = Join-Path $PSScriptRoot "local-up-alertmanager.ps1"
$downScriptPath = Join-Path $PSScriptRoot "local-down-alertmanager.ps1"

foreach ($path in @($composePath, $configPath, $prometheusPath, $runbookPath, $upScriptPath, $downScriptPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing local Alertmanager file: $path"
    }
}

$compose = Get-Content -LiteralPath $composePath -Raw
$config = Get-Content -LiteralPath $configPath -Raw
$prometheus = Get-Content -LiteralPath $prometheusPath -Raw
$runbook = Get-Content -LiteralPath $runbookPath -Raw
$upScript = Get-Content -LiteralPath $upScriptPath -Raw

if ($compose -notmatch "prom/alertmanager:v0\.27\.0") {
    throw "Alertmanager compose must use the pinned default prom/alertmanager:v0.27.0 image."
}
if ($compose -notmatch "19093:9093") {
    throw "Alertmanager compose must expose host port 19093 to avoid existing local ports."
}
if ($compose -notmatch "alertmanager\.yml:/etc/alertmanager/alertmanager\.yml:ro") {
    throw "Alertmanager compose must mount local alertmanager.yml read-only."
}
if ($config -notmatch "(?ms)^route:\s*\r?\n\s*receiver:\s*local-null") {
    throw "Alertmanager config must route to the local-null receiver."
}
if ($config -notmatch "(?ms)^receivers:\s*\r?\n\s*-\s*name:\s*local-null") {
    throw "Alertmanager config must define local-null receiver."
}
if ($config -match "webhook_configs|email_configs|pagerduty_configs|slack_configs|msteams_configs|opsgenie_configs") {
    throw "Local Alertmanager config must not define real external notification receivers."
}
if ($prometheus -notmatch "(?ms)^alerting:\s*\r?\n\s*alertmanagers:") {
    throw "Prometheus config must declare an alertmanager target section."
}
if ($prometheus -notmatch "host\.docker\.internal:19093") {
    throw "Prometheus config must target local Alertmanager through host.docker.internal:19093."
}
if ($upScript -notmatch "NEXUSIM_ALERTMANAGER_IMAGE" -or $upScript -notmatch "AllowImagePull") {
    throw "local-up-alertmanager.ps1 must support explicit image override and AllowImagePull."
}
if ($runbook -notmatch "local-up-alertmanager\.ps1" -or $runbook -notmatch "local-null") {
    throw "observability runbook must document local Alertmanager startup and null receiver boundary."
}

Write-Host "OK   local Alertmanager config"
