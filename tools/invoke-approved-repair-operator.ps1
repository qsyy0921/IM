param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,

    [Parameter(Mandatory = $true)]
    [string]$RequestPath,

    [Parameter(Mandatory = $true)]
    [string]$DecisionPath,

    [string]$OutputPath = "",
    [switch]$Execute,
    [switch]$AllowMutating
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

$validatorPath = Join-Path $PSScriptRoot "validate-repair-approval-chain.ps1"
if (-not (Test-Path -LiteralPath $validatorPath -PathType Leaf)) {
    throw "Missing repair approval chain validator: $validatorPath"
}
if (-not (Test-Path -LiteralPath $PlanPath -PathType Leaf)) {
    throw "Missing repair operator plan file: $PlanPath"
}

$validationJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
    -PlanPath $PlanPath `
    -RequestPath $RequestPath `
    -DecisionPath $DecisionPath
if ($LASTEXITCODE -ne 0) {
    throw "Repair approval chain validation failed."
}
$validation = $validationJson | ConvertFrom-Json

$plan = Get-Content -LiteralPath $PlanPath -Raw | ConvertFrom-Json
$environmentKeys = @()
if ($plan.environment) {
    $environmentKeys = @($plan.environment.PSObject.Properties.Name | Sort-Object)
}

$dryRunKeys = @($environmentKeys | Where-Object { $_ -like "*_DRY_RUN" })
$hasDryRunTrue = $false
foreach ($key in $dryRunKeys) {
    if ([string]$plan.environment.$key -eq "true") {
        $hasDryRunTrue = $true
        break
    }
}

if ($Execute -and -not $hasDryRunTrue -and -not $AllowMutating) {
    throw "Refusing to execute a repair operator plan without a true dry-run env. Pass -AllowMutating only after explicit operator approval."
}

$exitCode = $null
if ($Execute) {
    $oldValues = @{}
    foreach ($key in $environmentKeys) {
        $oldValues[$key] = [System.Environment]::GetEnvironmentVariable($key, "Process")
        [System.Environment]::SetEnvironmentVariable($key, [string]$plan.environment.$key, "Process")
    }
    try {
        $service = [string]$plan.service
        $servicePath = Join-Path (Split-Path -Parent $PSScriptRoot) "services\$service\cmd\$service"
        & go run $servicePath
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            throw "Approved repair operator exited with code $exitCode."
        }
    } finally {
        foreach ($key in $environmentKeys) {
            [System.Environment]::SetEnvironmentVariable($key, $oldValues[$key], "Process")
        }
    }
}

$summary = [ordered]@{
    schema_version = 1
    prepared_at = (Get-Date).ToUniversalTime().ToString("o")
    approval_id = [string]$validation.approval_id
    decision_id = [string]$validation.decision_id
    service = [string]$validation.service
    mode = [string]$validation.mode
    command = [string]$validation.command
    mode_env = [string]$validation.mode_env
    plan_path = [string](Resolve-Path -LiteralPath $PlanPath)
    request_path = [string](Resolve-Path -LiteralPath $RequestPath)
    decision_path = [string](Resolve-Path -LiteralPath $DecisionPath)
    plan_sha256 = [string]$validation.plan_sha256
    request_sha256 = [string]$validation.request_sha256
    decision_sha256 = [string]$validation.decision_sha256
    environment_keys = $environmentKeys
    dry_run_env_keys = $dryRunKeys
    dry_run_requested = [bool]$hasDryRunTrue
    execute_requested = [bool]$Execute
    mutating_execution_allowed = [bool]$AllowMutating
    executed = [bool]$Execute
    exit_code = $exitCode
    note = "Approved repair invocation summary. Environment values, reasons, and business data are redacted."
}

$json = $summary | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
