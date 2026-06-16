$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$remainingGoalsPath = Join-Path $repoRoot "docs\runbook\remaining-goals.md"
$developmentProgressPath = Join-Path $repoRoot "docs\runbook\development-progress.md"
$deliveryServiceBriefPath = Join-Path $repoRoot "docs\runbook\service-briefs\delivery-service.md"
$pushGatewayBriefPath = Join-Path $repoRoot "docs\runbook\service-briefs\push-gateway.md"
$distributedLocalPath = Join-Path $repoRoot "docs\runbook\distributed-local.md"

foreach ($path in @($remainingGoalsPath, $developmentProgressPath, $deliveryServiceBriefPath, $pushGatewayBriefPath, $distributedLocalPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing runbook file: $path"
    }
}

$remainingGoals = Get-Content -LiteralPath $remainingGoalsPath -Raw
$developmentProgress = Get-Content -LiteralPath $developmentProgressPath -Raw
$deliveryServiceBrief = Get-Content -LiteralPath $deliveryServiceBriefPath -Raw
$pushGatewayBrief = Get-Content -LiteralPath $pushGatewayBriefPath -Raw
$distributedLocal = Get-Content -LiteralPath $distributedLocalPath -Raw

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

$pushGatewayCrossInstanceResumeComplete = $distributedLocal.Contains("Win-Mac Docker cross-instance resume smoke") -and
    $distributedLocal.Contains("redis_resume_replay_count=1 / redis_resume_miss_count=0") -and
    $pushGatewayBrief.Contains("Redis route") -and
    $pushGatewayBrief.Contains("resume buffer")

if ($pushGatewayCrossInstanceResumeComplete -and $remainingGoals.Contains("跨实例 resume 强化")) {
    throw "remaining-goals.md still lists generic push-gateway cross-instance resume hardening even though Redis-backed cross-instance resume has smoke evidence. Keep remaining work specific to miss/permission-denied/buffer-gap, HA, or capacity."
}

Write-Host "OK   runbook consistency guardrails"
