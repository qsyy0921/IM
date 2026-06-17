param(
    [Parameter(Mandatory = $true)]
    [string]$Service,

    [Parameter(Mandatory = $true)]
    [string]$Mode,

    [string]$OutputPath = "",
    [string]$OutputEnv = "",
    [switch]$DryRun,
    [string]$DryRunEnv = "",
    [string]$ReasonFilePath = "",
    [string]$ReasonFileEnv = "",
    [string[]]$Env = @(),
    [string]$PlanOutputPath = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

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

$outputEnvSet = Convert-ToSet @($serviceSpec.output_envs)
$dryRunEnvSet = Convert-ToSet @($serviceSpec.dry_run_envs)
$reasonFileEnvSet = Convert-ToSet @($serviceSpec.reason_file_envs)

$environment = [ordered]@{}
$environment[[string]$serviceSpec.mode_env] = $Mode

if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
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

if (-not [string]::IsNullOrWhiteSpace($ReasonFilePath)) {
    $selectedReasonFileEnv = $ReasonFileEnv
    if ([string]::IsNullOrWhiteSpace($selectedReasonFileEnv)) {
        $reasonFileEnvValues = Convert-ToStringList @($serviceSpec.reason_file_envs)
        if ($reasonFileEnvValues.Count -eq 1) {
            $selectedReasonFileEnv = $reasonFileEnvValues[0]
        } else {
            throw "ReasonFilePath requires ReasonFileEnv when ${Service} has zero or multiple reason-file envs."
        }
    }
    if (-not $reasonFileEnvSet.ContainsKey($selectedReasonFileEnv)) {
        throw "Unsupported reason-file env for ${Service}: $selectedReasonFileEnv"
    }
    Assert-LowSensitiveRepairAdHocEnv -Key $selectedReasonFileEnv -Value $ReasonFilePath
    $environment[$selectedReasonFileEnv] = $ReasonFilePath
}

if ([string]::IsNullOrWhiteSpace($ReasonFilePath) -and -not [string]::IsNullOrWhiteSpace($ReasonFileEnv)) {
    throw "ReasonFileEnv requires ReasonFilePath."
}

foreach ($entry in @($Env)) {
    if ([string]::IsNullOrWhiteSpace($entry)) {
        continue
    }
    $parts = $entry.Split("=", 2)
    if ($parts.Count -ne 2 -or [string]::IsNullOrWhiteSpace($parts[0])) {
        throw "Env entries must use KEY=VALUE format: $entry"
    }
    if ($parts[0] -match "_REASON$") {
        throw "Refusing to write raw operator reason into repair operator plan: $($parts[0]). Use -ReasonFilePath and -ReasonFileEnv instead."
    }
    Assert-LowSensitiveRepairAdHocEnv -Key $parts[0] -Value $parts[1]
    if ($environment.Contains($parts[0])) {
        throw "Env entry duplicates a catalog-managed environment key. Use the dedicated parameter for: $($parts[0])"
    }
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
    Assert-ExternalRepairOutputPath -Value $PlanOutputPath -FieldName "PlanOutputPath"
    $parent = Split-Path -Parent $PlanOutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $PlanOutputPath -Encoding UTF8
} else {
    $json
}
