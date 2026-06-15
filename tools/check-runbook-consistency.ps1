$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$remainingGoalsPath = Join-Path $repoRoot "docs\runbook\remaining-goals.md"
$developmentProgressPath = Join-Path $repoRoot "docs\runbook\development-progress.md"
$pushGatewayBriefPath = Join-Path $repoRoot "docs\runbook\service-briefs\push-gateway.md"

foreach ($path in @($remainingGoalsPath, $developmentProgressPath, $pushGatewayBriefPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing runbook file: $path"
    }
}

$remainingGoals = Get-Content -LiteralPath $remainingGoalsPath -Raw
$developmentProgress = Get-Content -LiteralPath $developmentProgressPath -Raw
$pushGatewayBrief = Get-Content -LiteralPath $pushGatewayBriefPath -Raw

$pushGatewayNetworkPartitionComplete = $developmentProgress.Contains("Redis Sentinel network-partition fallback smoke") -and
    $pushGatewayBrief.Contains("network-partition fallback")

if ($pushGatewayNetworkPartitionComplete -and $remainingGoals.Contains("Redis 网络分区组合 smoke")) {
    throw "remaining-goals.md still lists push-gateway Redis network-partition smoke even though progress and push-gateway brief mark it complete."
}

Write-Host "OK   runbook consistency guardrails"
