param(
    [Parameter(Mandatory = $true)]
    [string]$RequestPath,

    [Parameter(Mandatory = $true)]
    [ValidateSet("APPROVED", "REJECTED")]
    [string]$Decision,

    [Parameter(Mandatory = $true)]
    [string]$DecidedBy,

    [string]$OutputPath = "",
    [string]$DecisionID = "",
    [string]$ReasonFile = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $RequestPath -PathType Leaf)) {
    throw "Missing repair approval request file: $RequestPath"
}
$requestRaw = Get-Content -LiteralPath $RequestPath -Raw
$request = $requestRaw | ConvertFrom-Json
if ($request.schema_version -ne 1) {
    throw "Unsupported repair approval request schema_version: $($request.schema_version)"
}
if ($request.status -ne "PENDING") {
    throw "Repair approval decision only accepts PENDING approval requests."
}
if ($request.executes -ne $false) {
    throw "Repair approval decision only accepts non-executing approval requests."
}
if ([string]::IsNullOrWhiteSpace([string]$request.approval_id) -or
    [string]::IsNullOrWhiteSpace([string]$request.plan_sha256) -or
    [string]::IsNullOrWhiteSpace([string]$request.service) -or
    [string]::IsNullOrWhiteSpace([string]$request.mode)) {
    throw "Repair approval request is missing required identity fields."
}

if ([string]::IsNullOrWhiteSpace($DecisionID)) {
    $DecisionID = "decision-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $DecidedBy -FieldName "DecidedBy"
Assert-LowSensitiveRepairIdentifier -Value ([string]$request.approval_id) -FieldName "approval_id"
Assert-LowSensitiveRepairIdentifier -Value $DecisionID -FieldName "DecisionID"

$requestBytes = [System.Text.Encoding]::UTF8.GetBytes($requestRaw)
$requestHash = Get-RepairSha256Hex -Bytes $requestBytes

$reasonPresent = $false
$reasonHash = ""
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    $reasonSummary = Read-RepairReasonFileSummary -Path $ReasonFile -MissingMessage "Missing repair approval decision reason file"
    $reasonPresent = [bool]$reasonSummary.Present
    $reasonHash = [string]$reasonSummary.Sha256
}

$decisionDocument = [ordered]@{
    schema_version = 1
    decision_id = $DecisionID
    approval_id = [string]$request.approval_id
    status = $Decision
    decided_at = (Get-Date).ToUniversalTime().ToString("o")
    decided_by = $DecidedBy
    service = [string]$request.service
    mode = [string]$request.mode
    command = [string]$request.command
    request_path = $RequestPath
    request_sha256 = $requestHash
    plan_sha256 = [string]$request.plan_sha256
    reason_present = $reasonPresent
    reason_sha256 = $reasonHash
    executes = $false
    note = "Approval decision only. It redacts decision reason text and does not execute the operator."
}

$json = $decisionDocument | ConvertTo-Json -Depth 8
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
