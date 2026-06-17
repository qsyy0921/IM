param(
    [Parameter(Mandatory = $true)]
    [string]$BundlePath,

    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $BundlePath -PathType Leaf)) {
    throw "Missing repair audit bundle file: $BundlePath"
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

function Assert-RequiredString {
    param(
        [object]$Value,
        [string]$Name,
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace([string]$Value)) {
        throw "Repair audit bundle is missing $Name`: $Path"
    }
}

$bundleRaw = Get-Content -LiteralPath $BundlePath -Raw
$bundle = $bundleRaw | ConvertFrom-Json

if ($bundle.schema_version -ne 1) {
    throw "Unsupported repair audit bundle schema_version: $($bundle.schema_version)"
}
Assert-RequiredString $bundle.bundle_id "bundle_id" $BundlePath
Assert-RequiredString $bundle.generated_by "generated_by" $BundlePath

$files = @($bundle.files)
if ($files.Count -eq 0) {
    throw "Repair audit bundle must include at least one evidence file."
}
if ([int]$bundle.file_count -ne $files.Count) {
    throw "Repair audit bundle file_count does not match files length."
}

$seenHashes = @{}
$kindCounts = @{}
foreach ($file in $files) {
    Assert-RequiredString $file.path "file.path" $BundlePath
    Assert-RequiredString $file.sha256 "file.sha256" $BundlePath
    Assert-RequiredString $file.kind "file.kind" $BundlePath

    $hash = [string]$file.sha256
    if ($seenHashes.ContainsKey($hash)) {
        throw "Duplicate evidence hash in repair audit bundle: $hash"
    }
    $seenHashes[$hash] = $true

    $evidencePath = [string]$file.path
    if (-not (Test-Path -LiteralPath $evidencePath -PathType Leaf)) {
        throw "Repair audit bundle references missing evidence file: $evidencePath"
    }
    $evidenceRaw = Get-Content -LiteralPath $evidencePath -Raw
    $actualHash = Get-Sha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($evidenceRaw))
    if ($actualHash -ne $hash) {
        throw "Repair audit bundle evidence hash mismatch: $evidencePath"
    }

    try {
        $evidenceDocument = $evidenceRaw | ConvertFrom-Json
    } catch {
        throw "Repair audit bundle evidence must be JSON: $evidencePath"
    }
    if ($evidenceDocument.schema_version -ne [int]$file.schema_version) {
        throw "Repair audit bundle evidence schema_version mismatch: $evidencePath"
    }

    $kind = [string]$file.kind
    if (-not $kindCounts.ContainsKey($kind)) {
        $kindCounts[$kind] = 0
    }
    $kindCounts[$kind]++
}

$summaryKinds = @($bundle.kind_summary)
foreach ($summaryKind in $summaryKinds) {
    Assert-RequiredString $summaryKind.kind "kind_summary.kind" $BundlePath
    $kind = [string]$summaryKind.kind
    $expectedCount = if ($kindCounts.ContainsKey($kind)) { [int]$kindCounts[$kind] } else { 0 }
    if ([int]$summaryKind.count -ne $expectedCount) {
        throw "Repair audit bundle kind_summary count mismatch for kind: $kind"
    }
}

$result = [ordered]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    bundle_id = [string]$bundle.bundle_id
    file_count = $files.Count
    kind_count = $kindCounts.Count
    valid = $true
    note = "Repair audit bundle validation only. It verifies evidence hashes and does not copy evidence bodies, environment values, reasons, or business data."
}

$json = $result | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
