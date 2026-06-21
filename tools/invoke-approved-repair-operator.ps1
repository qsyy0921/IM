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

function Get-PlanEnvironmentValue {
    param(
        [object]$Environment,
        [string]$Name
    )

    if ($null -eq $Environment -or $null -eq $Environment.PSObject.Properties[$Name]) {
        return ""
    }
    return ([string]$Environment.PSObject.Properties[$Name].Value).Trim()
}

function Invoke-WorkflowInstructionManifestPreflight {
    param(
        [object]$Plan
    )

    if ([string]$Plan.service -ne "workflow-service" -or [string]$Plan.mode -ne "compensation-instruction-import") {
        return $null
    }

    $tenantID = Get-PlanEnvironmentValue -Environment $Plan.environment -Name "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID"
    $manifestPath = Get-PlanEnvironmentValue -Environment $Plan.environment -Name "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE"
    Assert-LowSensitiveRepairIdentifier -Value $tenantID -FieldName "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID"
    if ([string]::IsNullOrWhiteSpace($manifestPath)) {
        throw "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE is required for workflow-service compensation-instruction-import."
    }
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "Missing workflow compensation instruction manifest: $manifestPath"
    }

    $manifestValidatorPath = Join-Path $PSScriptRoot "validate-workflow-compensation-instruction-manifest.ps1"
    if (-not (Test-Path -LiteralPath $manifestValidatorPath -PathType Leaf)) {
        throw "Missing workflow compensation instruction manifest validator: $manifestValidatorPath"
    }

    $manifestSummaryRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $manifestValidatorPath `
        -ManifestPath $manifestPath
    if ($LASTEXITCODE -ne 0) {
        throw "Workflow compensation instruction manifest validation failed."
    }
    $manifestSummary = ($manifestSummaryRaw -join "`n") | ConvertFrom-Json
    return [ordered]@{
        name = "workflow_compensation_instruction_manifest"
        valid = $true
        instruction_count = [int]$manifestSummary.instruction_count
        manifest_sha256 = [string]$manifestSummary.manifest_sha256
        manifest_path_sha256 = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($manifestPath))
        note = "Validated workflow compensation instruction manifest before approved invocation. Path and environment values are redacted."
    }
}

$preflightChecks = @()
$workflowInstructionPreflight = Invoke-WorkflowInstructionManifestPreflight -Plan $plan
if ($null -ne $workflowInstructionPreflight) {
    $preflightChecks += $workflowInstructionPreflight
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
    preflight_checks = $preflightChecks
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
