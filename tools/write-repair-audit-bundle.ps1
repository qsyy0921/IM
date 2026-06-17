param(
    [Parameter(Mandatory = $true)]
    [string[]]$EvidencePath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$BundleID = "",
    [string]$ReasonFile = ""
)

$ErrorActionPreference = "Stop"

$expandedEvidencePaths = @()
foreach ($pathEntry in $EvidencePath) {
    foreach ($pathPart in ([string]$pathEntry -split ",")) {
        if (-not [string]::IsNullOrWhiteSpace($pathPart)) {
            $expandedEvidencePaths += $pathPart.Trim()
        }
    }
}

if ($expandedEvidencePaths.Count -eq 0) {
    throw "At least one repair audit evidence file is required."
}
if ([string]::IsNullOrWhiteSpace($BundleID)) {
    $BundleID = "repair-audit-bundle-" + [System.Guid]::NewGuid().ToString("N")
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

function Get-RepairEvidenceKind {
    param([object]$Document)

    if ($Document.batch_id -and $Document.valid -eq $true) {
        return "repair_batch_validation"
    }
    if ($Document.batch_id -and $Document.items -and $null -ne $Document.executed_count) {
        return "repair_batch_invocation"
    }
    if ($Document.batch_id -and $Document.items -and $Document.executes -eq $false) {
        return "repair_batch_manifest"
    }
    if ($Document.approval_id -and $Document.decision_id -and $null -ne $Document.executed) {
        return "approved_repair_invocation"
    }
    if ($Document.decision_id -and $Document.approval_id -and $Document.status) {
        return "repair_approval_decision"
    }
    if ($Document.approval_id -and $Document.status -eq "PENDING") {
        return "repair_approval_request"
    }
    if ($Document.service -and $Document.mode -and $Document.environment) {
        return "repair_operator_plan"
    }
    return "unknown_json"
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

Assert-LowSensitiveActor -Value $GeneratedBy -FieldName "GeneratedBy"

$reasonPresent = $false
$reasonHash = ""
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    if (-not (Test-Path -LiteralPath $ReasonFile -PathType Leaf)) {
        throw "Missing repair audit bundle reason file: $ReasonFile"
    }
    $reasonBytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $ReasonFile))
    $reasonPresent = $reasonBytes.Length -gt 0
    if ($reasonPresent) {
        $reasonHash = Get-Sha256Hex -Bytes $reasonBytes
    }
}

$seenHashes = @{}
$files = @()
$kindCounts = @{}
$index = 0

foreach ($evidencePath in $expandedEvidencePaths) {
    if (-not (Test-Path -LiteralPath $evidencePath -PathType Leaf)) {
        throw "Missing repair audit evidence file: $evidencePath"
    }
    $resolvedPath = [string](Resolve-Path -LiteralPath $evidencePath)
    $raw = Get-Content -LiteralPath $resolvedPath -Raw
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($raw)
    $hash = Get-Sha256Hex -Bytes $bytes
    if ($seenHashes.ContainsKey($hash)) {
        throw "Duplicate repair audit evidence content: $resolvedPath"
    }
    $seenHashes[$hash] = $true

    try {
        $document = $raw | ConvertFrom-Json
    } catch {
        throw "Repair audit evidence must be JSON: $resolvedPath"
    }
    if ($document.schema_version -ne 1) {
        throw "Unsupported repair audit evidence schema_version in $resolvedPath`: $($document.schema_version)"
    }

    $kind = Get-RepairEvidenceKind -Document $document
    if (-not $kindCounts.ContainsKey($kind)) {
        $kindCounts[$kind] = 0
    }
    $kindCounts[$kind]++

    $files += [ordered]@{
        index = $index
        path = $resolvedPath
        sha256 = $hash
        kind = $kind
        schema_version = [int]$document.schema_version
        service = [string]$document.service
        mode = [string]$document.mode
        batch_id = [string]$document.batch_id
        approval_id = [string]$document.approval_id
        decision_id = [string]$document.decision_id
        item_count = if ($null -ne $document.item_count) { [int]$document.item_count } else { $null }
        executes = if ($null -ne $document.executes) { [bool]$document.executes } else { $null }
        executed = if ($null -ne $document.executed) { [bool]$document.executed } else { $null }
        execute_requested = if ($null -ne $document.execute_requested) { [bool]$document.execute_requested } else { $null }
        valid = if ($null -ne $document.valid) { [bool]$document.valid } else { $null }
    }
    $index++
}

$kindSummary = @()
foreach ($key in @($kindCounts.Keys | Sort-Object)) {
    $kindSummary += [ordered]@{
        kind = [string]$key
        count = [int]$kindCounts[$key]
    }
}

$bundle = [ordered]@{
    schema_version = 1
    bundle_id = $BundleID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    file_count = $files.Count
    reason_present = $reasonPresent
    reason_sha256 = $reasonHash
    kind_summary = $kindSummary
    files = $files
    note = "Repair audit bundle manifest only. It stores file hashes and low-sensitive metadata, not evidence bodies, environment values, reasons, or business data."
}

$json = $bundle | ConvertTo-Json -Depth 10
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
