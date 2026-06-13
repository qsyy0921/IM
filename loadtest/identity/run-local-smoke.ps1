param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$IdentityTlsCaFile = "",
    [string]$IdentityTlsServerName = "",
    [string]$IdentityTlsClientCertFile = "",
    [string]$IdentityTlsClientKeyFile = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "identity-challenge-delivery-outbox-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$webhookFile = Join-Path $resultDir "webhook-notification.json"
$topicSuffix = Get-Date -Format "yyyyMMdd-HHmmss"
$tenantId = "tenant-identity-outbox-smoke-$topicSuffix"
$userId = "identity-user"
$password = "IdentitySmokePassw0rd!"
$destination = "identity-user-$topicSuffix@example.com"
$gatewayTokenSecret = "identity-smoke-gateway-secret-$topicSuffix"
$deliveryTokenKey = "identity-smoke-delivery-token-key-$topicSuffix"
$webhookBearerToken = "identity-smoke-webhook-token-$topicSuffix"

New-Item -ItemType Directory -Force $resultDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null
if (Test-Path -LiteralPath $webhookFile) {
    Remove-Item -LiteralPath $webhookFile -Force
}

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\identity-service.exe ./services/identity-service/cmd/identity-service
    go build -o bin\identity-loadtest.exe ./loadtest/identity
}

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = Resolve-Path $Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "nexusim-postgres:$containerPath" | Out-Null
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
}

function Wait-Tcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 200
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port = 0,
        [string[]]$ArgumentList = @()
    )
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $startParams = @{
        FilePath = $FilePath
        WindowStyle = "Hidden"
        PassThru = $true
        RedirectStandardOutput = $out
        RedirectStandardError = $err
    }
    if ($ArgumentList.Count -gt 0) {
        $startParams.ArgumentList = $ArgumentList
    }
    $proc = Start-Process @startParams
    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
}

function Assert-Summary {
    param([string]$Path)
    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if (-not $summary.success) {
        throw "identity smoke failed: $($summary.error)"
    }
    if ($summary.request_verification_challenge.dev_challenge_token_set) {
        throw "identity smoke unexpectedly used dev challenge token"
    }
    if (-not $summary.webhook.received -or -not $summary.webhook.token_set) {
        throw "identity smoke did not receive webhook token"
    }
    if (-not $summary.webhook.authorization_ok) {
        throw "identity webhook authorization did not match"
    }
    if ($summary.challenge_delivery_outbox.pending -ne 0 -or $summary.challenge_delivery_outbox.dlq -ne 0 -or $summary.challenge_delivery_outbox.delivered -lt 1) {
        throw "identity challenge delivery outbox did not drain: pending=$($summary.challenge_delivery_outbox.pending) delivered=$($summary.challenge_delivery_outbox.delivered) dlq=$($summary.challenge_delivery_outbox.dlq)"
    }
    if ($summary.challenge_row.status -ne "CONSUMED" -or $summary.challenge_row.delivery_status -ne "DELIVERED") {
        throw "identity challenge row did not reach consumed/delivered: status=$($summary.challenge_row.status) delivery=$($summary.challenge_row.delivery_status)"
    }
}

$processes = @()
try {
    Get-ChildItem -Path "migrations\postgres\identity" -Filter "*.sql" |
        Sort-Object Name |
        ForEach-Object {
            Apply-PostgresMigration -Path $_.FullName -Name $_.Name
        }

    $identityService = Join-Path $repo "bin\identity-service.exe"
    $runner = Join-Path $repo "bin\identity-loadtest.exe"
    $identityGrpcPort = Get-FreeTcpPort
    $webhookPort = Get-FreeTcpPort
    $identityGrpcAddr = "127.0.0.1:$identityGrpcPort"
    $webhookAddr = "127.0.0.1:$webhookPort"
    $webhookURL = "http://$webhookAddr/challenge"

    $processes += Start-NexusProcess -Name "identity-webhook" -FilePath $runner -Port $webhookPort -ArgumentList @(
        "--mode", "webhook",
        "--webhook-listen", $webhookAddr,
        "--webhook-file", $webhookFile,
        "--webhook-bearer-token", $webhookBearerToken
    ) -Env @{}

    $processes += Start-NexusProcess -Name "identity-grpc" -FilePath $identityService -Port $identityGrpcPort -Env @{
        NEXUSIM_IDENTITY_SERVICE_MODE = "grpc"
        NEXUSIM_IDENTITY_GRPC_ADDR = $identityGrpcAddr
        NEXUSIM_IDENTITY_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET = $gatewayTokenSecret
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MODE = "outbox"
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY = $deliveryTokenKey
        NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN = "false"
    }

    $processes += Start-NexusProcess -Name "identity-challenge-delivery-worker" -FilePath $identityService -Env @{
        NEXUSIM_IDENTITY_SERVICE_MODE = "challenge-delivery-worker"
        NEXUSIM_IDENTITY_DEBUG_ADDR = ""
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_URL = $webhookURL
        NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_BEARER_TOKEN = $webhookBearerToken
        NEXUSIM_IDENTITY_CHALLENGE_WEBHOOK_TIMEOUT = "5s"
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY = $deliveryTokenKey
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_BATCH_SIZE = "10"
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_POLL_INTERVAL = "200ms"
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_RETRY_BASE_DELAY = "200ms"
        NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_MAX_ATTEMPTS = "3"
    }

    $runnerArgs = @(
        "--mode", "client",
        "--target", $identityGrpcAddr,
        "--pg-dsn", $PgDsn,
        "--tenant-id", $tenantId,
        "--user-id", $userId,
        "--password", $password,
        "--destination", $destination,
        "--webhook-file", $webhookFile,
        "--webhook-bearer-token", $webhookBearerToken,
        "--cleanup",
        "--wait-timeout", "20s",
        "--result-dir", $resultDir
    )
    if (-not [string]::IsNullOrWhiteSpace($IdentityTlsCaFile)) {
        $runnerArgs += @("--identity-tls-ca-file", $IdentityTlsCaFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($IdentityTlsServerName)) {
        $runnerArgs += @("--identity-tls-server-name", $IdentityTlsServerName)
    }
    if (-not [string]::IsNullOrWhiteSpace($IdentityTlsClientCertFile)) {
        $runnerArgs += @("--identity-tls-client-cert-file", $IdentityTlsClientCertFile)
    }
    if (-not [string]::IsNullOrWhiteSpace($IdentityTlsClientKeyFile)) {
        $runnerArgs += @("--identity-tls-client-key-file", $IdentityTlsClientKeyFile)
    }

    & $runner @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "identity smoke runner failed with exit code $LASTEXITCODE"
    }

    Assert-Summary -Path (Join-Path $resultDir "identity-summary.json")
} finally {
    foreach ($proc in $processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "result_dir=$resultDir"
Write-Host "identity_grpc_addr=127.0.0.1:$identityGrpcPort"
Write-Host "identity_webhook_url=http://127.0.0.1:$webhookPort/challenge"
