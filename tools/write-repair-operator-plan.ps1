param(
    [Parameter(Mandatory = $true)]
    [string]$Service,

    [Parameter(Mandatory = $true)]
    [string]$Mode,

    [string]$OutputPath = "",
    [string]$OutputEnv = "",
    [switch]$DryRun,
    [string]$DryRunEnv = "",
    [string[]]$Env = @(),
    [string]$PlanOutputPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$catalogPath = Join-Path $repoRoot "docs\runbook\repair-operators.catalog.json"

if (-not (Test-Path -LiteralPath $catalogPath -PathType Leaf)) {
    throw "Missing repair operator catalog: $catalogPath"
}

$catalog = Get-Content -LiteralPath $catalogPath -Raw | ConvertFrom-Json
if ($catalog.schema_version -ne 1) {
    throw "Unsupported repair operator catalog schema_version: $($catalog.schema_version)"
}

$serviceSpec = @($catalog.services) | Where-Object { [string]$_.service -eq $Service } | Select-Object -First 1
if ($null -eq $serviceSpec) {
    throw "Unknown repair operator service: $Service"
}

$modeSet = @{}
foreach ($catalogMode in @($serviceSpec.modes)) {
    $modeSet[[string]$catalogMode] = $true
}
if (-not $modeSet.ContainsKey($Mode)) {
    throw "Unsupported repair operator mode for ${Service}: $Mode"
}

function Convert-ToSet {
    param([object[]]$Values)

    $set = @{}
    foreach ($value in @($Values)) {
        $stringValue = [string]$value
        if (-not [string]::IsNullOrWhiteSpace($stringValue)) {
            $set[$stringValue] = $true
        }
    }
    return $set
}

function Convert-ToStringList {
    param([object[]]$Values)

    $list = [System.Collections.Generic.List[string]]::new()
    foreach ($value in @($Values)) {
        $stringValue = [string]$value
        if (-not [string]::IsNullOrWhiteSpace($stringValue)) {
            $list.Add($stringValue)
        }
    }
    return ,$list
}

function Assert-LowSensitiveAdHocEnv {
    param(
        [string]$Key,
        [string]$Value
    )

    if ($Key -notmatch "^[A-Z][A-Z0-9_]*$") {
        throw "Env key must be an uppercase environment variable name: $Key"
    }

    $sensitiveKeyPattern = "(?i)(PASSWORD|PASSWD|SECRET|TOKEN|BEARER|PRIVATE|CREDENTIAL|API[_-]?KEY|ACCESS[_-]?KEY|REFRESH|SESSION|COOKIE)"
    if ($Key -match $sensitiveKeyPattern) {
        throw "Refusing to write potentially sensitive Env key into repair operator plan: $Key"
    }

    $sensitiveValuePattern = "(?i)(bearer\s+\S+|password\s*=|secret\s*=|token\s*=|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.)"
    if ($Value -match $sensitiveValuePattern) {
        throw "Refusing to write potentially sensitive Env value into repair operator plan: $Key"
    }
}

$outputEnvSet = Convert-ToSet @($serviceSpec.output_envs)
$dryRunEnvSet = Convert-ToSet @($serviceSpec.dry_run_envs)

$environment = [ordered]@{}
$environment[[string]$serviceSpec.mode_env] = $Mode

if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    if ([string]::IsNullOrWhiteSpace($OutputEnv)) {
        throw "OutputPath requires OutputEnv. Choose one of the service output_envs from the catalog."
    }
    if (-not $outputEnvSet.ContainsKey($OutputEnv)) {
        throw "Unsupported output env for ${Service}: $OutputEnv"
    }
    $environment[$OutputEnv] = $OutputPath
}

if ($DryRun) {
    $selectedDryRunEnv = $DryRunEnv
    if ([string]::IsNullOrWhiteSpace($selectedDryRunEnv)) {
        $dryRunEnvValues = Convert-ToStringList @($serviceSpec.dry_run_envs)
        if ($dryRunEnvValues.Count -eq 1) {
            $selectedDryRunEnv = $dryRunEnvValues[0]
        } else {
            throw "DryRun requires DryRunEnv when ${Service} has zero or multiple dry-run envs."
        }
    }
    if (-not $dryRunEnvSet.ContainsKey($selectedDryRunEnv)) {
        throw "Unsupported dry-run env for ${Service}: $selectedDryRunEnv"
    }
    $environment[$selectedDryRunEnv] = "true"
}

foreach ($entry in @($Env)) {
    if ([string]::IsNullOrWhiteSpace($entry)) {
        continue
    }
    $parts = $entry.Split("=", 2)
    if ($parts.Count -ne 2 -or [string]::IsNullOrWhiteSpace($parts[0])) {
        throw "Env entries must use KEY=VALUE format: $entry"
    }
    Assert-LowSensitiveAdHocEnv -Key $parts[0] -Value $parts[1]
    $environment[$parts[0]] = $parts[1]
}

$relativeCommand = ".\services\$Service\cmd\$Service"
$plan = [ordered]@{
    schema_version = 1
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    service = $Service
    mode = $Mode
    command = "go run $relativeCommand"
    environment = $environment
    catalog = "docs/runbook/repair-operators.catalog.json"
    dry_run_requested = [bool]$DryRun
    executes = $false
    note = "Plan only. This script does not execute the operator or read service data."
}

$json = $plan | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($PlanOutputPath)) {
    $parent = Split-Path -Parent $PlanOutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $PlanOutputPath -Encoding UTF8
} else {
    $json
}
