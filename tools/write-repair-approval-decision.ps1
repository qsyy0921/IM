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

Assert-LowSensitiveActor -Value $DecidedBy -FieldName "DecidedBy"

$requestBytes = [System.Text.Encoding]::UTF8.GetBytes($requestRaw)
$requestHash = Get-Sha256Hex -Bytes $requestBytes

$reasonPresent = $false
$reasonHash = ""
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    if (-not (Test-Path -LiteralPath $ReasonFile -PathType Leaf)) {
        throw "Missing repair approval decision reason file: $ReasonFile"
    }
    $reasonBytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $ReasonFile))
    $reasonPresent = $reasonBytes.Length -gt 0
    if ($reasonPresent) {
        $reasonHash = Get-Sha256Hex -Bytes $reasonBytes
    }
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
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
