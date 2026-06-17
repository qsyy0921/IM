$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$catalogPath = Join-Path $repoRoot "docs\runbook\repair-operators.catalog.json"
$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"

foreach ($path in @($catalogPath, $plannerPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair catalog plannable dependency: $path"
    }
}

$catalog = Get-Content -LiteralPath $catalogPath -Raw | ConvertFrom-Json
if ($catalog.schema_version -ne 1) {
    throw "Unsupported repair operator catalog schema_version: $($catalog.schema_version)"
}

$plannedCount = 0
foreach ($serviceSpec in @($catalog.services)) {
    $service = [string]$serviceSpec.service
    $modeEnv = [string]$serviceSpec.mode_env
    if ([string]::IsNullOrWhiteSpace($service) -or [string]::IsNullOrWhiteSpace($modeEnv)) {
        throw "Repair operator catalog service entry must include service and mode_env."
    }

    foreach ($mode in @($serviceSpec.modes)) {
        $mode = [string]$mode
        if ([string]::IsNullOrWhiteSpace($mode)) {
            throw "Repair operator catalog service ${service} contains an empty mode."
        }

        $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service $service `
            -Mode $mode
        if ($LASTEXITCODE -ne 0) {
            throw "write-repair-operator-plan.ps1 failed for catalog mode ${service}/${mode}"
        }

        $plan = $planJson | ConvertFrom-Json
        $expectedCommand = "go run .\services\$service\cmd\$service"
        if ($plan.schema_version -ne 1 -or
            $plan.executes -ne $false -or
            $plan.service -ne $service -or
            $plan.mode -ne $mode -or
            $plan.command -ne $expectedCommand -or
            $plan.catalog -ne "docs/runbook/repair-operators.catalog.json" -or
            $plan.dry_run_requested -ne $false) {
            throw "repair operator plan has unexpected fields for catalog mode ${service}/${mode}"
        }
        if ($null -eq $plan.environment -or [string]$plan.environment.$modeEnv -ne $mode) {
            throw "repair operator plan missing catalog mode env for ${service}/${mode}: $modeEnv"
        }
        $plannedCount++
    }
}

if ($plannedCount -le 0) {
    throw "Repair operator catalog plannable check found no modes."
}

Write-Host "OK   repair operator catalog plannable guardrails ($plannedCount modes)"
