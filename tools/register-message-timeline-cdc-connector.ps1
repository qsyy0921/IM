param(
    [string]$ConnectUrl = "http://localhost:18083",
    [string]$ConnectorFile = "deploy/local/debezium/message-timeline-connector.json"
)

$ErrorActionPreference = "Stop"

$path = Resolve-Path -LiteralPath $ConnectorFile
$body = Get-Content -LiteralPath $path -Raw
$definition = $body | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace($definition.name)) {
    throw "Connector file is missing name"
}

$headers = @{ "Content-Type" = "application/json" }
$base = $ConnectUrl.TrimEnd("/")
$statusUrl = "$base/connectors/$($definition.name)"

try {
    Invoke-RestMethod -Method Get -Uri $statusUrl -TimeoutSec 10 | Out-Null
    Invoke-RestMethod -Method Put -Uri "$statusUrl/config" -Headers $headers -Body (($definition.config | ConvertTo-Json -Depth 20)) -TimeoutSec 30 | Out-Null
    Write-Output "updated connector $($definition.name)"
} catch {
    $response = $_.Exception.Response
    if (-not $response) {
        throw
    }
    if ($response -and [int]$response.StatusCode -ne 404) {
        throw
    }
    Invoke-RestMethod -Method Post -Uri "$base/connectors" -Headers $headers -Body $body -TimeoutSec 30 | Out-Null
    Write-Output "created connector $($definition.name)"
}

$status = Invoke-RestMethod -Method Get -Uri "$statusUrl/status" -TimeoutSec 10
$status | ConvertTo-Json -Depth 20
