param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [string]$InstructionID = "",

    [Parameter(Mandatory = $true)]
    [string]$WorkflowID,

    [string]$PayloadRefHash = "",
    [string]$PayloadRefFile = "",

    [Parameter(Mandatory = $true)]
    [string]$Environment,

    [Parameter(Mandatory = $true)]
    [string]$ConfigKind,

    [Parameter(Mandatory = $true)]
    [string]$BundleKey,

    [Parameter(Mandatory = $true)]
    [string]$TargetVersion,

    [Parameter(Mandatory = $true)]
    [string]$OperatorRef,

    [string]$ReasonRef = "",
    [string]$ReasonFile = "",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\repair-operator-safety.ps1"

Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-LowSensitiveRepairIdentifier -Value $WorkflowID -FieldName "WorkflowID"
Assert-LowSensitiveRepairIdentifier -Value $Environment -FieldName "Environment"
Assert-LowSensitiveRepairIdentifier -Value $ConfigKind -FieldName "ConfigKind"
Assert-LowSensitiveRepairIdentifier -Value $BundleKey -FieldName "BundleKey"
Assert-LowSensitiveRepairIdentifier -Value $TargetVersion -FieldName "TargetVersion"
Assert-LowSensitiveRepairIdentifier -Value $OperatorRef -FieldName "OperatorRef"
Assert-LowSensitiveRepairIdentifier -Value $InstructionID -FieldName "InstructionID" -AllowEmpty

if ((Test-Path -LiteralPath $OutputPath -PathType Leaf) -and -not $Force) {
    throw "OutputPath already exists. Use -Force to overwrite: $OutputPath"
}

if (-not [string]::IsNullOrWhiteSpace($PayloadRefHash) -and -not [string]::IsNullOrWhiteSpace($PayloadRefFile)) {
    throw "Use either PayloadRefHash or PayloadRefFile, not both."
}
if ([string]::IsNullOrWhiteSpace($PayloadRefHash) -and [string]::IsNullOrWhiteSpace($PayloadRefFile)) {
    throw "PayloadRefHash or PayloadRefFile is required."
}

$payloadRefHashValue = $PayloadRefHash.Trim()
if (-not [string]::IsNullOrWhiteSpace($PayloadRefFile)) {
    if (-not (Test-Path -LiteralPath $PayloadRefFile -PathType Leaf)) {
        throw "Missing workflow compensation payload ref file: $PayloadRefFile"
    }
    $payloadInfo = Get-Item -LiteralPath (Resolve-Path -LiteralPath $PayloadRefFile)
    $maxPayloadRefBytes = 64 * 1024
    if ($payloadInfo.Length -gt $maxPayloadRefBytes) {
        throw "Workflow compensation payload ref file is too large: $PayloadRefFile. Keep operator payload reference files at or below 64 KiB."
    }
    $payloadBytes = [System.IO.File]::ReadAllBytes($payloadInfo.FullName)
    $payloadRefHashValue = "sha256:" + (Get-RepairSha256Hex -Bytes $payloadBytes)
}
Assert-LowSensitiveRepairIdentifier -Value $payloadRefHashValue -FieldName "PayloadRefHash"

if (-not [string]::IsNullOrWhiteSpace($ReasonRef) -and -not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    throw "Use either ReasonRef or ReasonFile, not both."
}

$reasonRefValue = $ReasonRef.Trim()
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    $reasonSummary = Read-RepairReasonFileSummary -Path $ReasonFile -MissingMessage "Missing workflow compensation reason file"
    if ($reasonSummary.Present) {
        $reasonRefValue = "reason-sha256:" + [string]$reasonSummary.Sha256
    }
}
Assert-LowSensitiveRepairIdentifier -Value $reasonRefValue -FieldName "ReasonRef"

if ([string]::IsNullOrWhiteSpace($InstructionID)) {
    $instructionIDBytes = [System.Text.Encoding]::UTF8.GetBytes($payloadRefHashValue)
    $instructionID = "wfci_" + (Get-RepairSha256Hex -Bytes $instructionIDBytes).Substring(0, 16)
}

$instruction = [ordered]@{
    instruction_id = $instructionID.Trim()
    workflow_id = $WorkflowID.Trim()
    payload_ref_hash = $payloadRefHashValue
    environment = $Environment.Trim()
    config_kind = $ConfigKind.Trim()
    bundle_key = $BundleKey.Trim()
    target_version = $TargetVersion.Trim()
    operator_ref = $OperatorRef.Trim()
    reason_ref = $reasonRefValue
}

$manifest = [ordered]@{
    instructions = @($instruction)
}

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
($manifest | ConvertTo-Json -Depth 5) | Set-Content -LiteralPath $OutputPath -Encoding UTF8

$validator = Join-Path $PSScriptRoot "validate-workflow-compensation-instruction-manifest.ps1"
& powershell -NoProfile -ExecutionPolicy Bypass -File $validator `
    -ManifestPath $OutputPath `
    -ExpectedWorkflowID $WorkflowID `
    -ExpectedPayloadRefHash $payloadRefHashValue `
    -ExpectedTargetVersion $TargetVersion | Out-Null
if ($LASTEXITCODE -ne 0) {
    Remove-Item -LiteralPath $OutputPath -Force -ErrorAction SilentlyContinue
    throw "Generated workflow compensation instruction manifest failed validation."
}

Write-Host "OK   workflow compensation instruction manifest written: $OutputPath"
