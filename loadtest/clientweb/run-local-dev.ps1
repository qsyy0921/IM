param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$BindHost = "127.0.0.1",
    [string]$ClientHost = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ClientHost)) {
    $ClientHost = $BindHost
}

$nexusIMRepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $nexusIMRepoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $nexusIMRepoRoot -Name "ResultRoot"

$smokeArgs = @{
    PgDsn = $PgDsn
    KafkaBrokers = $KafkaBrokers
    ResultRoot = $ResultRoot
    BindHost = $BindHost
    ClientHost = $ClientHost
    BffPort = 8080
    PushPort = 8088
    RunName = "client-local-dev"
    ClientTenantId = "tenant-client-local"
    ClientConversationId = "conv-client-local"
    ClientSenderUserId = "user-b"
    ClientSenderPassword = "ClientWebSenderPassw0rd!"
    ClientReceiverUserId = "user-a"
    ClientReceiverPassword = "ClientWebReceiverPassw0rd!"
    KeepAlive = $true
}

if ($SkipBuild) {
    $smokeArgs.SkipBuild = $true
}

& (Join-Path $PSScriptRoot "run-local-smoke.ps1") @smokeArgs
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "NexusIM local client backend is running."
Write-Host "API: http://${ClientHost}:8080"
Write-Host "WebSocket: ws://${ClientHost}:8088/ws"
Write-Host "Login user: user-a"
Write-Host "Login password: ClientWebReceiverPassw0rd!"
Write-Host "Conversation: conv-client-local"
