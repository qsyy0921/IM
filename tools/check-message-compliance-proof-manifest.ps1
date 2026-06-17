$ErrorActionPreference = "Stop"

$validatorPath = Join-Path $PSScriptRoot "validate-message-compliance-proof-manifest.ps1"
if (-not (Test-Path -LiteralPath $validatorPath -PathType Leaf)) {
    throw "Missing validator: $validatorPath"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-message-proof-manifest-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

function Write-Manifest {
    param(
        [string]$Name,
        [string]$Content
    )

    $path = Join-Path $tempRoot $Name
    Set-Content -LiteralPath $path -Value $Content -Encoding UTF8
    return $path
}

function Invoke-ValidatorExpectSuccess {
    param(
        [string]$ManifestPath,
        [string]$ExternalProofRef = "",
        [string]$ExpectedProvider = "",
        [string]$ExpectedProofHash = ""
    )

    $outputPath = Join-Path $tempRoot ([System.IO.Path]::GetRandomFileName() + ".json")
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        $validatorPath,
        "-ManifestPath",
        $ManifestPath,
        "-OutputPath",
        $outputPath
    )
    if ($ExternalProofRef -ne "") {
        $args += @("-ExternalProofRef", $ExternalProofRef)
    }
    if ($ExpectedProvider -ne "") {
        $args += @("-ExpectedProvider", $ExpectedProvider)
    }
    if ($ExpectedProofHash -ne "") {
        $args += @("-ExpectedProofHash", $ExpectedProofHash)
    }

    & powershell @args
    if ($LASTEXITCODE -ne 0) {
        throw "Validator failed for expected-success manifest: $ManifestPath"
    }
    if (-not (Test-Path -LiteralPath $outputPath -PathType Leaf)) {
        throw "Validator did not write output summary: $outputPath"
    }
    $summary = Get-Content -LiteralPath $outputPath -Raw | ConvertFrom-Json
    if ($summary.schema_version -ne 1 -or $summary.valid -ne $true) {
        throw "Validator wrote invalid summary for expected-success manifest."
    }
    return $summary
}

function Invoke-ValidatorExpectFailure {
    param(
        [string]$ManifestPath,
        [string]$ExternalProofRef = "",
        [string]$ExpectedProvider = "",
        [string]$ExpectedProofHash = ""
    )

    $args = @(
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        $validatorPath,
        "-ManifestPath",
        $ManifestPath
    )
    if ($ExternalProofRef -ne "") {
        $args += @("-ExternalProofRef", $ExternalProofRef)
    }
    if ($ExpectedProvider -ne "") {
        $args += @("-ExpectedProvider", $ExpectedProvider)
    }
    if ($ExpectedProofHash -ne "") {
        $args += @("-ExpectedProofHash", $ExpectedProofHash)
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell @args *> $null
        $exitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }

    if ($exitCode -eq 0) {
        throw "Validator succeeded for expected-failure manifest: $ManifestPath"
    }
}

try {
    $goodEnvelope = Write-Manifest "good-envelope.json" @'
{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "VERIFIED"
    },
    {
      "external_proof_ref": "proof://case/b",
      "provider": "legal-archive",
      "proof_hash": "sha256:def456",
      "status": "REVOKED"
    }
  ]
}
'@
    $summary = Invoke-ValidatorExpectSuccess -ManifestPath $goodEnvelope -ExternalProofRef "proof://case/a" -ExpectedProvider "legal-archive" -ExpectedProofHash "sha256:abc123"
    if ($summary.proof_count -ne 2 -or $summary.verified_count -ne 1 -or $summary.revoked_count -ne 1) {
        throw "Validator summary counts are incorrect for envelope manifest."
    }

    $goodArray = Write-Manifest "good-array.json" @'
[
  {
    "external_proof_ref": "proof://case/c",
    "provider": "legal-archive",
    "proof_hash": "sha256:ghi789",
    "status": "VERIFIED"
  }
]
'@
    Invoke-ValidatorExpectSuccess -ManifestPath $goodArray -ExternalProofRef "proof://case/c" | Out-Null

    $duplicateRef = Write-Manifest "duplicate-ref.json" @'
{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "VERIFIED"
    },
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:def456",
      "status": "VERIFIED"
    }
  ]
}
'@
    Invoke-ValidatorExpectFailure -ManifestPath $duplicateRef

    $rawBody = Write-Manifest "raw-body.json" @'
{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "VERIFIED",
      "proof_body": "raw legal proof text must not be embedded"
    }
  ]
}
'@
    Invoke-ValidatorExpectFailure -ManifestPath $rawBody

    $revokedSelected = Write-Manifest "revoked-selected.json" @'
{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "REVOKED"
    }
  ]
}
'@
    Invoke-ValidatorExpectFailure -ManifestPath $revokedSelected -ExternalProofRef "proof://case/a"

    Invoke-ValidatorExpectFailure -ManifestPath $goodEnvelope -ExternalProofRef "proof://case/a" -ExpectedProvider "other-provider"
    Invoke-ValidatorExpectFailure -ManifestPath $goodEnvelope -ExternalProofRef "proof://case/a" -ExpectedProofHash "sha256:other"

    Write-Host "OK   message compliance proof manifest self-test"
}
finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
