$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "service-registry.ps1")

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

$currentImplementedServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Stages @("core")
$currentFoundationServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Stages @("foundation-active")
$currentProductServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Stages @("product-active")
$futureServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Stages @("future")

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

$allowedServiceDirs = @(Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Active)
$actualServiceDirs = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | ForEach-Object { $_.Name })
$unexpectedServices = @($actualServiceDirs | Where-Object { $_ -notin $allowedServiceDirs } | Sort-Object)
if ($unexpectedServices.Count -gt 0) {
    throw "Unexpected service directories before ADR/stage switch: $($unexpectedServices -join ', '). Future services must not appear before the current phase explicitly allows them."
}

foreach ($service in $currentFoundationServices) {
    if (-not $currentBrief.Contains($service)) {
        throw "current-brief.md must list active foundation service: $service"
    }
    if (-not $remainingGoals.Contains($service)) {
        throw "remaining-goals.md must keep active foundation service backlog: $service"
    }
    if (-not (Test-Path -LiteralPath (Join-Path $servicesRoot $service) -PathType Container)) {
        throw "Missing active foundation service directory: services\$service"
    }
    $briefPath = Join-Path $serviceBriefRoot "$service.md"
    if (-not (Test-Path -LiteralPath $briefPath -PathType Leaf)) {
        throw "Missing active foundation service brief: docs\runbook\service-briefs\$service.md"
    }
    $brief = Read-UTF8Text -Path $briefPath
    switch ($service) {
        "search-service" {
            if (-not ($brief.Contains("search_service.proto") -and $brief.Contains("projection usecase skeleton"))) {
                throw "Active search-service brief must state first implementation slice: docs\runbook\service-briefs\$service.md"
            }
        }
        "memory-service" {
            if (-not ($brief.Contains("memory_service.proto") -and $brief.Contains("projection usecase"))) {
                throw "Active memory-service brief must state first implementation slice: docs\runbook\service-briefs\$service.md"
            }
        }
        default {
            if (-not $brief.Contains("foundation-active")) {
                throw "Active foundation service brief must state foundation-active status: docs\runbook\service-briefs\$service.md"
            }
        }
    }
}

foreach ($service in $currentProductServices) {
    if (-not $currentBrief.Contains($service)) {
        throw "current-brief.md must list active product service: $service"
    }
    if (-not $remainingGoals.Contains($service)) {
        throw "remaining-goals.md must keep active product service backlog: $service"
    }
    if (-not (Test-Path -LiteralPath (Join-Path $servicesRoot $service) -PathType Container)) {
        throw "Missing active product service directory: services\$service"
    }
    $briefPath = Join-Path $serviceBriefRoot "$service.md"
    if (-not (Test-Path -LiteralPath $briefPath -PathType Leaf)) {
        throw "Missing active product service brief: docs\runbook\service-briefs\$service.md"
    }
    $brief = Read-UTF8Text -Path $briefPath
    if (-not ($brief.Contains("product-active") -or $brief.Contains("stage-switch review passed"))) {
        throw "Active product service brief must state product-active or stage-switch review status: docs\runbook\service-briefs\$service.md"
    }
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
