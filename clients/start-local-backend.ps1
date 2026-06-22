param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$BindHost = "127.0.0.1",
    [string]$ClientHost = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$script = Join-Path $repo "loadtest\clientweb\run-local-dev.ps1"

$arguments = @{
    PgDsn = $PgDsn
    KafkaBrokers = $KafkaBrokers
    ResultRoot = $ResultRoot
    BindHost = $BindHost
    ClientHost = $ClientHost
}

if ($SkipBuild) {
    $arguments.SkipBuild = $true
}

$displayHost = $BindHost
if (-not [string]::IsNullOrWhiteSpace($ClientHost)) {
    $displayHost = $ClientHost
}

Write-Host "Starting NexusIM local client backend..."
Write-Host "API: http://${displayHost}:8080"
Write-Host "WebSocket: ws://${displayHost}:8088/ws"

& $script @arguments
