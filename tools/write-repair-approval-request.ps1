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

function Get-Sha256Hex {
    param([byte[]]$Bytes)

    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash($Bytes)
    } finally {
        $sha.Dispose()
    }
    return -join ($hash | ForEach-Object { $_.ToString("x2") })
}

function Assert-LowSensitiveActor {
    param(
        [string]$Value,
        [string]$FieldName
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        throw "$FieldName is required."
    }
    if ($Value.Length -gt 64 -or $Value -notmatch "^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$") {
        throw "$FieldName must be a low-sensitive operator id using letters, digits, dot, underscore, or dash."
    }
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ)") {
        throw "$FieldName must be a low-sensitive operator id, not a credential-like value."
    }
}

Assert-LowSensitiveActor -Value $RequestedBy -FieldName "RequestedBy"

$planBytes = [System.Text.Encoding]::UTF8.GetBytes($planRaw)
$planHash = Get-Sha256Hex -Bytes $planBytes

$reasonPresent = $false
$reasonHash = ""
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    if (-not (Test-Path -LiteralPath $ReasonFile -PathType Leaf)) {
        throw "Missing repair approval reason file: $ReasonFile"
    }
    $reasonBytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $ReasonFile))
    $reasonPresent = $reasonBytes.Length -gt 0
    if ($reasonPresent) {
        $reasonHash = Get-Sha256Hex -Bytes $reasonBytes
    }
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
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
