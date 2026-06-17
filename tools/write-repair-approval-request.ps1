param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,

    [Parameter(Mandatory = $true)]
    [string]$RequestedBy,

    [string]$OutputPath = "",
    [string]$ApprovalID = "",
    [string]$ReasonFile = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $PlanPath -PathType Leaf)) {
    throw "Missing repair operator plan file: $PlanPath"
}
$planRaw = Get-Content -LiteralPath $PlanPath -Raw
$plan = $planRaw | ConvertFrom-Json
if ($plan.schema_version -ne 1) {
    throw "Unsupported repair operator plan schema_version: $($plan.schema_version)"
}
if ($plan.executes -ne $false) {
    throw "Repair operator approval request only accepts non-executing plan files."
}
if ([string]::IsNullOrWhiteSpace([string]$plan.service) -or [string]::IsNullOrWhiteSpace([string]$plan.mode)) {
    throw "Repair operator plan must include service and mode."
}

if ([string]::IsNullOrWhiteSpace($ApprovalID)) {
    $ApprovalID = "approval-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $RequestedBy -FieldName "RequestedBy"
Assert-LowSensitiveRepairIdentifier -Value $ApprovalID -FieldName "ApprovalID"

$planBytes = [System.Text.Encoding]::UTF8.GetBytes($planRaw)
$planHash = Get-RepairSha256Hex -Bytes $planBytes

$reasonPresent = $false
$reasonHash = ""
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    $reasonSummary = Read-RepairReasonFileSummary -Path $ReasonFile -MissingMessage "Missing repair approval reason file"
    $reasonPresent = [bool]$reasonSummary.Present
    $reasonHash = [string]$reasonSummary.Sha256
}

$environmentKeys = @()
if ($plan.environment) {
    $environmentKeys = @($plan.environment.PSObject.Properties.Name | Sort-Object)
}

$request = [ordered]@{
    schema_version = 1
    approval_id = $ApprovalID
    status = "PENDING"
    requested_at = (Get-Date).ToUniversalTime().ToString("o")
    requested_by = $RequestedBy
    service = [string]$plan.service
    mode = [string]$plan.mode
    command = [string]$plan.command
    plan_path = $PlanPath
    plan_sha256 = $planHash
    dry_run_requested = [bool]$plan.dry_run_requested
    environment_keys = $environmentKeys
    reason_present = $reasonPresent
    reason_sha256 = $reasonHash
    executes = $false
    note = "Approval request only. It redacts environment values and does not execute the operator."
}

$json = $request | ConvertTo-Json -Depth 8
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
