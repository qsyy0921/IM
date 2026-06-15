$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$currentBriefPath = Join-Path $repoRoot "docs\runbook\current-brief.md"
$remainingGoalsPath = Join-Path $repoRoot "docs\runbook\remaining-goals.md"
$serviceBriefRoot = Join-Path $repoRoot "docs\runbook\service-briefs"

foreach ($path in @($servicesRoot, $currentBriefPath, $remainingGoalsPath, $serviceBriefRoot)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing path for future service boundary check: $path"
    }
}

function Read-UTF8Text {
    param([string]$Path)

    return [System.IO.File]::ReadAllText($Path, [System.Text.Encoding]::UTF8)
}

$currentImplementedServices = @(
    "api-gateway",
    "identity-service",
    "message-service",
    "conversation-service",
    "delivery-service",
    "push-gateway",
    "receipt-service",
    "contacts-service",
    "policy-service"
)

$futureServices = @(
    "search-service",
    "memory-service",
    "media-service",
    "notification-service",
    "audit-service",
    "admin-service",
    "retrieval-gateway",
    "rag-service",
    "summary-service",
    "agent-service",
    "ai-eval-service"
)

$currentBrief = Read-UTF8Text -Path $currentBriefPath
$remainingGoals = Read-UTF8Text -Path $remainingGoalsPath

foreach ($service in $currentImplementedServices) {
    if (-not $currentBrief.Contains($service)) {
        throw "current-brief.md must list implemented service: $service"
    }
    if (-not (Test-Path -LiteralPath (Join-Path $servicesRoot $service) -PathType Container)) {
        throw "Missing implemented service directory: services\$service"
    }
}

$actualServiceDirs = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | ForEach-Object { $_.Name })
$unexpectedServices = @($actualServiceDirs | Where-Object { $_ -notin $currentImplementedServices } | Sort-Object)
if ($unexpectedServices.Count -gt 0) {
    throw "Unexpected service directories before stage switch: $($unexpectedServices -join ', '). Current phase must clean the 9 existing services before new service code."
}

foreach ($futureService in $futureServices) {
    if (-not $remainingGoals.Contains($futureService)) {
        throw "remaining-goals.md must keep future service listed until stage switch: $futureService"
    }

    $futureDir = Join-Path $servicesRoot $futureService
    if (Test-Path -LiteralPath $futureDir -PathType Container) {
        throw "Future service already has implementation directory before stage switch: services\$futureService"
    }

    $briefPath = Join-Path $serviceBriefRoot "$futureService.md"
    if (Test-Path -LiteralPath $briefPath -PathType Leaf) {
        $brief = Read-UTF8Text -Path $briefPath
        $isExplicitFutureBrief = $brief.Contains("SDD v0.1 draft")
        if (-not $isExplicitFutureBrief) {
            throw "Future service brief must stay explicitly marked future/draft before stage switch: docs\runbook\service-briefs\$futureService.md"
        }
    }
}

foreach ($futureMarker in @("search-service", "RAG", "agent")) {
    if (-not ($currentBrief.Contains($futureMarker))) {
        throw "current-brief.md must keep future-service phase marker: $futureMarker"
    }
}

Write-Host "OK   future service boundary guardrails"
