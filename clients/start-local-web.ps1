param(
    [string]$ApiBaseURL = "http://127.0.0.1:8080",
    [string]$PushWebSocketURL = "ws://127.0.0.1:8088/ws",
    [string]$DeviceID = "nexusim-web-dev"
)

$ErrorActionPreference = "Stop"

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$clientsRoot = Join-Path $repo "clients"

$env:VITE_NEXUSIM_API_BASE = $ApiBaseURL
$env:VITE_NEXUSIM_WS_URL = $PushWebSocketURL
$env:VITE_NEXUSIM_DEVICE_ID = $DeviceID

Write-Host "Starting NexusIM Web client..."
Write-Host "API: $ApiBaseURL"
Write-Host "WebSocket: $PushWebSocketURL"
Write-Host "Device: $DeviceID"
Write-Host ""
Write-Host "Start the backend separately with:"
Write-Host ".\clients\start-local-backend.ps1"
Write-Host ""

npm --prefix $clientsRoot run dev:web
