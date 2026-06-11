param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ConversationTarget = "127.0.0.1:10496",
    [string]$MessageTarget = "127.0.0.1:10495",
    [string]$DeliveryTarget = "127.0.0.1:10497",
    [string]$ReceiptTarget = "127.0.0.1:10499",
    [string]$PushUrl = "ws://127.0.0.1:10498",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$TenantId = "",
    [string]$ConversationId = "",
    [string]$SenderUserId = "demo-sender",
    [string]$ReceiverUserId = "demo-receiver",
    [string]$ReceiverDeviceId = "demo-device-1",
    [ValidateSet("mock", "hmac")]
    [string]$PushAuthMode = "mock",
    [string]$PushAuthHmacSecret = "",
    [switch]$NoCleanup,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

. .\tools\go-env.ps1

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "e2e-demo-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if ([string]::IsNullOrWhiteSpace($TenantId)) {
    $TenantId = "tenant-$RunName"
}
if ([string]::IsNullOrWhiteSpace($ConversationId)) {
    $ConversationId = "conv-$RunName"
}

$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

if (-not $SkipBuild) {
    go build -o .\bin\nexusim-e2e-demo.exe .\loadtest\demo
}

$args = @(
    "--pg-dsn", $PgDsn,
    "--conversation-target", $ConversationTarget,
    "--message-target", $MessageTarget,
    "--delivery-target", $DeliveryTarget,
    "--receipt-target", $ReceiptTarget,
    "--push-url", $PushUrl,
    "--result-dir", $resultDir,
    "--tenant-id", $TenantId,
    "--conversation-id", $ConversationId,
    "--sender-user-id", $SenderUserId,
    "--receiver-user-id", $ReceiverUserId,
    "--receiver-device-id", $ReceiverDeviceId,
    "--push-auth-mode", $PushAuthMode
)

if ($PushAuthMode -eq "hmac") {
    if ([string]::IsNullOrWhiteSpace($PushAuthHmacSecret)) {
        throw "-PushAuthHmacSecret is required when -PushAuthMode hmac"
    }
    $args += @("--push-auth-hmac-secret", $PushAuthHmacSecret)
}
if ($NoCleanup) {
    $args += @("--cleanup=false")
}

& .\bin\nexusim-e2e-demo.exe @args

Write-Host "Summary: $resultDir\e2e-demo-summary.json"
