param(
    [Parameter(Mandatory = $true)]
    [string]$ManifestPath,

    [string]$ExternalProofRef = "",
    [string]$ExpectedProvider = "",
    [string]$ExpectedProofHash = "",
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

function Assert-AllowedProperties {
    param(
        [pscustomobject]$Object,
        [string[]]$AllowedProperties,
        [string]$Path
    )

    $allowed = @{}
    foreach ($propertyName in $AllowedProperties) {
        $allowed[$propertyName] = $true
    }

    foreach ($property in $Object.PSObject.Properties) {
        $name = [string]$property.Name
        if (-not $allowed.ContainsKey($name)) {
            throw "Unsupported or sensitive manifest field at ${Path}: $name"
        }
    }
}

function Get-RequiredStringProperty {
    param(
        [pscustomobject]$Object,
        [string]$Name,
        [string]$Path
    )

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        throw "Missing required manifest field at ${Path}: $Name"
    }
    $value = [string]$property.Value
    $value = $value.Trim()
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "Empty required manifest field at ${Path}: $Name"
    }
    return $value
}

function Read-ManifestEntries {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Manifest file not found: $Path"
    }

    $jsonText = Get-Content -LiteralPath $Path -Raw
    if ([string]::IsNullOrWhiteSpace($jsonText)) {
        throw "Manifest file is empty: $Path"
    }

    try {
        $document = $jsonText | ConvertFrom-Json
    }
    catch {
        throw "Manifest is not valid JSON: $Path"
    }

    if ($document -is [System.Array]) {
        return @($document)
    }

    if ($document -is [pscustomobject]) {
        Assert-AllowedProperties -Object $document -AllowedProperties @("proofs") -Path "$"
        $proofsProperty = $document.PSObject.Properties["proofs"]
        if ($null -eq $proofsProperty) {
            throw "Manifest envelope must contain proofs array."
        }
        return @($proofsProperty.Value)
    }

    throw "Manifest root must be a proofs envelope or an array."
}

function Normalize-ProofEntry {
    param(
        [object]$Entry,
        [int]$Index
    )

    if (-not ($Entry -is [pscustomobject])) {
        throw "Manifest proof entry at index $Index must be an object."
    }

    $path = "`$.proofs[$Index]"
    Assert-AllowedProperties -Object $Entry -AllowedProperties @(
        "external_proof_ref",
        "provider",
        "proof_hash",
        "status"
    ) -Path $path

    $externalProofRef = Get-RequiredStringProperty -Object $Entry -Name "external_proof_ref" -Path $path
    $provider = Get-RequiredStringProperty -Object $Entry -Name "provider" -Path $path
    $proofHash = Get-RequiredStringProperty -Object $Entry -Name "proof_hash" -Path $path
    $status = (Get-RequiredStringProperty -Object $Entry -Name "status" -Path $path).ToUpperInvariant()

    if ($status -ne "VERIFIED" -and $status -ne "REVOKED") {
        throw "Manifest proof entry ${externalProofRef} has unsupported status: $status"
    }
    if (-not $proofHash.StartsWith("sha256:", [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Manifest proof entry ${externalProofRef} proof_hash must start with sha256:."
    }
    if ($proofHash.Length -le "sha256:".Length + 3) {
        throw "Manifest proof entry ${externalProofRef} proof_hash is too short."
    }

    return [pscustomobject]@{
        external_proof_ref = $externalProofRef
        provider = $provider
        proof_hash = $proofHash
        status = $status
    }
}

$resolvedManifestPath = (Resolve-Path -LiteralPath $ManifestPath).Path
$entries = @(Read-ManifestEntries -Path $resolvedManifestPath)
if ($entries.Count -eq 0) {
    throw "Manifest must contain at least one proof entry."
}

$seenRefs = @{}
$normalizedEntries = [System.Collections.Generic.List[object]]::new()
$verifiedCount = 0
$revokedCount = 0

for ($index = 0; $index -lt $entries.Count; $index++) {
    $entry = Normalize-ProofEntry -Entry $entries[$index] -Index $index
    if ($seenRefs.ContainsKey($entry.external_proof_ref)) {
        throw "Manifest contains duplicate external_proof_ref: $($entry.external_proof_ref)"
    }
    $seenRefs[$entry.external_proof_ref] = $true
    if ($entry.status -eq "VERIFIED") {
        $verifiedCount++
    }
    elseif ($entry.status -eq "REVOKED") {
        $revokedCount++
    }
    $normalizedEntries.Add($entry)
}

$selectedEntry = $null
$ExternalProofRef = $ExternalProofRef.Trim()
if ($ExternalProofRef -ne "") {
    foreach ($entry in $normalizedEntries) {
        if ($entry.external_proof_ref -eq $ExternalProofRef) {
            $selectedEntry = $entry
            break
        }
    }
    if ($null -eq $selectedEntry) {
        throw "Selected external_proof_ref not found in manifest: $ExternalProofRef"
    }
    if ($selectedEntry.status -ne "VERIFIED") {
        throw "Selected external_proof_ref is not VERIFIED: $ExternalProofRef"
    }
}

$ExpectedProvider = $ExpectedProvider.Trim()
if ($ExpectedProvider -ne "") {
    if ($null -eq $selectedEntry) {
        throw "-ExpectedProvider requires -ExternalProofRef."
    }
    if ($selectedEntry.provider -ne $ExpectedProvider) {
        throw "Selected external_proof_ref provider does not match expected provider."
    }
}

$ExpectedProofHash = $ExpectedProofHash.Trim()
if ($ExpectedProofHash -ne "") {
    if ($null -eq $selectedEntry) {
        throw "-ExpectedProofHash requires -ExternalProofRef."
    }
    if ($selectedEntry.proof_hash -ne $ExpectedProofHash) {
        throw "Selected external_proof_ref proof_hash does not match expected proof_hash."
    }
}

$summary = [ordered]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    valid = $true
    manifest_path = $resolvedManifestPath
    proof_count = $normalizedEntries.Count
    verified_count = $verifiedCount
    revoked_count = $revokedCount
    selected_external_proof_ref_present = ($null -ne $selectedEntry)
    selected_status = $(if ($null -ne $selectedEntry) { $selectedEntry.status } else { "" })
    selected_provider = $(if ($null -ne $selectedEntry) { $selectedEntry.provider } else { "" })
    note = "validated low-sensitive compliance proof manifest; proof bodies and unknown fields are not allowed"
}

$summaryJson = $summary | ConvertTo-Json -Depth 4
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $outputParent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($outputParent)) {
        New-Item -ItemType Directory -Force -Path $outputParent | Out-Null
    }
    Set-Content -LiteralPath $OutputPath -Value $summaryJson -Encoding UTF8
}
else {
    Write-Output $summaryJson
}
