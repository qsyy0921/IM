param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,

    [Parameter(Mandatory = $true)]
    [string]$RequestPath,

    [Parameter(Mandatory = $true)]
    [string]$DecisionPath,

    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($path in @($PlanPath, $RequestPath, $DecisionPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair approval chain file: $path"
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$catalogPath = Join-Path $repoRoot "docs\runbook\repair-operators.catalog.json"
if (-not (Test-Path -LiteralPath $catalogPath -PathType Leaf)) {
    throw "Missing repair operator catalog: $catalogPath"
}

function Get-Utf8Hash {
    param([string]$Text)
    return Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Text))
}

$planRaw = Get-Content -LiteralPath $PlanPath -Raw
$requestRaw = Get-Content -LiteralPath $RequestPath -Raw
$decisionRaw = Get-Content -LiteralPath $DecisionPath -Raw

$plan = $planRaw | ConvertFrom-Json
$request = $requestRaw | ConvertFrom-Json
$decision = $decisionRaw | ConvertFrom-Json
$catalog = Get-Content -LiteralPath $catalogPath -Raw | ConvertFrom-Json

if ($plan.schema_version -ne 1) {
    throw "Unsupported repair operator plan schema_version: $($plan.schema_version)"
}
if ($request.schema_version -ne 1) {
    throw "Unsupported repair approval request schema_version: $($request.schema_version)"
}
if ($decision.schema_version -ne 1) {
    throw "Unsupported repair approval decision schema_version: $($decision.schema_version)"
}
if ($catalog.schema_version -ne 1) {
    throw "Unsupported repair operator catalog schema_version: $($catalog.schema_version)"
}
if ($plan.executes -ne $false -or $request.executes -ne $false -or $decision.executes -ne $false) {
    throw "Repair approval chain only accepts non-executing artifacts."
}
if ($request.status -ne "PENDING") {
    throw "Repair approval request must be PENDING."
}
if ($decision.status -ne "APPROVED") {
    throw "Repair approval decision must be APPROVED."
}
Assert-LowSensitiveRepairIdentifier -Value ([string]$request.approval_id) -FieldName "approval_id"
Assert-LowSensitiveRepairIdentifier -Value ([string]$decision.decision_id) -FieldName "decision_id"

$planHash = Get-Utf8Hash -Text $planRaw
$requestHash = Get-Utf8Hash -Text $requestRaw

if ([string]$request.plan_sha256 -ne $planHash) {
    throw "Repair approval request plan hash does not match plan file."
}
if ([string]$decision.plan_sha256 -ne $planHash) {
    throw "Repair approval decision plan hash does not match plan file."
}
if ([string]$decision.request_sha256 -ne $requestHash) {
    throw "Repair approval decision request hash does not match request file."
}
if ([string]$decision.approval_id -ne [string]$request.approval_id) {
    throw "Repair approval decision approval_id does not match request."
}

foreach ($field in @("service", "mode", "command")) {
    $planValue = [string]$plan.$field
    $requestValue = [string]$request.$field
    $decisionValue = [string]$decision.$field
    if ([string]::IsNullOrWhiteSpace($planValue) -or
        $requestValue -ne $planValue -or
        $decisionValue -ne $planValue) {
        throw "Repair approval chain field mismatch: $field"
    }
}

$serviceSpec = @($catalog.services) | Where-Object { [string]$_.service -eq [string]$plan.service } | Select-Object -First 1
if ($null -eq $serviceSpec) {
    throw "Repair approval chain service is not present in catalog: $($plan.service)"
}

$modeSet = @{}
foreach ($catalogMode in @($serviceSpec.modes)) {
    $modeSet[[string]$catalogMode] = $true
}
if (-not $modeSet.ContainsKey([string]$plan.mode)) {
    throw "Repair approval chain mode is not present in catalog for $($plan.service): $($plan.mode)"
}

$expectedCommand = "go run .\services\$($plan.service)\cmd\$($plan.service)"
if ([string]$plan.command -ne $expectedCommand) {
    throw "Repair approval chain command does not match catalog-derived service command."
}

$modeEnv = [string]$serviceSpec.mode_env
if ([string]::IsNullOrWhiteSpace($modeEnv)) {
    throw "Repair operator catalog is missing mode_env for $($plan.service)."
}
if ($null -eq $plan.environment -or [string]$plan.environment.$modeEnv -ne [string]$plan.mode) {
    throw "Repair approval chain plan environment does not set catalog mode env to the selected mode."
}

$summary = [ordered]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    valid = $true
    approval_id = [string]$request.approval_id
    decision_id = [string]$decision.decision_id
    service = [string]$plan.service
    mode = [string]$plan.mode
    command = [string]$plan.command
    mode_env = $modeEnv
    plan_sha256 = $planHash
    request_sha256 = $requestHash
    decision_sha256 = Get-Utf8Hash -Text $decisionRaw
    executes = $false
    note = "Approval chain validation only. It does not execute the operator or copy env values, reasons, or business data."
}

$json = $summary | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
