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

$lastStatusError = $null
for ($attempt = 1; $attempt -le 30; $attempt++) {
    try {
        $status = Invoke-RestMethod -Method Get -Uri "$statusUrl/status" -TimeoutSec 10
        $connectorRunning = $status.connector.state -eq "RUNNING"
        $tasksRunning = @($status.tasks | Where-Object { $_.state -ne "RUNNING" }).Count -eq 0
        if ($connectorRunning -and $tasksRunning) {
            $status | ConvertTo-Json -Depth 20
            return
        }
        if ($status.connector.state -eq "FAILED" -or @($status.tasks | Where-Object { $_.state -eq "FAILED" }).Count -gt 0) {
            $status | ConvertTo-Json -Depth 20
            throw "connector $($definition.name) entered FAILED state"
        }
    } catch {
        $lastStatusError = $_
        $response = $_.Exception.Response
        if (-not $response) {
            throw
        }
        if ($response -and [int]$response.StatusCode -ne 404) {
            throw
        }
        Start-Sleep -Seconds 1
    }
}

throw "connector $($definition.name) was created or updated, but status was not available within 30s: $($lastStatusError.Exception.Message)"
