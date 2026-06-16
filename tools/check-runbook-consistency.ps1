$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$remainingGoalsPath = Join-Path $repoRoot "docs\runbook\remaining-goals.md"
$developmentProgressPath = Join-Path $repoRoot "docs\runbook\development-progress.md"
$deliveryServiceBriefPath = Join-Path $repoRoot "docs\runbook\service-briefs\delivery-service.md"
$pushGatewayBriefPath = Join-Path $repoRoot "docs\runbook\service-briefs\push-gateway.md"

foreach ($path in @($remainingGoalsPath, $developmentProgressPath, $deliveryServiceBriefPath, $pushGatewayBriefPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing runbook file: $path"
    }
}

$remainingGoals = Get-Content -LiteralPath $remainingGoalsPath -Raw
$developmentProgress = Get-Content -LiteralPath $developmentProgressPath -Raw
$deliveryServiceBrief = Get-Content -LiteralPath $deliveryServiceBriefPath -Raw
$pushGatewayBrief = Get-Content -LiteralPath $pushGatewayBriefPath -Raw

$pushGatewayNetworkPartitionComplete = $developmentProgress.Contains("Redis Sentinel network-partition fallback smoke") -and
    $pushGatewayBrief.Contains("network-partition fallback")

if ($pushGatewayNetworkPartitionComplete -and $remainingGoals.Contains("Redis 网络分区组合 smoke")) {
    throw "remaining-goals.md still lists push-gateway Redis network-partition smoke even though progress and push-gateway brief mark it complete."
}

$deliveryHideCrossDeviceComplete = $deliveryServiceBrief.Contains("delivery.inbox_item.hidden.v1") -and
    $pushGatewayBrief.Contains("delivery.hide")

if ($deliveryHideCrossDeviceComplete -and $remainingGoals.Contains("隐藏项跨设备提示")) {
    throw "remaining-goals.md still lists delivery hidden-item cross-device prompt even though delivery and push-gateway briefs mark it complete."
}

Write-Host "OK   runbook consistency guardrails"
