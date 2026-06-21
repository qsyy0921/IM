param(
    [Parameter(Mandatory = $true)]
    [string]$ManifestPath,

    [string]$ExpectedWorkflowID = "",
    [string]$ExpectedPayloadRefHash = "",
    [string]$ExpectedTargetVersion = ""
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\repair-operator-safety.ps1"

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-OnlyKnownFields {
    param(
        [object]$Object,
        [string[]]$Allowed,
        [string]$Prefix
    )
    $allowedSet = @{}
    foreach ($name in $Allowed) {
        $allowedSet[$name] = $true
    }
    foreach ($property in $Object.PSObject.Properties) {
        if (-not $allowedSet.ContainsKey($property.Name)) {
            throw "$Prefix contains unknown field: $($property.Name)"
        }
    }
}

function Get-RequiredString {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object.PSObject.Properties[$Name]) {
        throw "Workflow compensation instruction manifest missing required field: $Name"
    }
    $value = ([string]$Object.$Name).Trim()
    if ($value.Length -eq 0) {
        throw "Workflow compensation instruction manifest field is required: $Name"
    }
    return $value
}

function Assert-LowSensitiveWorkflowRef {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    $text = ([string]$Value).Trim()
    if ($text.Length -gt 0 -and
        ($text -match "(?i)(private://|raw:|dsn=|postgres://|mysql://|mongodb://)" -or
         $text -match "\s")) {
        throw "$FieldName must be a low-sensitive ref or hash, not raw text or connection material."
    }
}

if (-not (Test-Path -LiteralPath $ManifestPath -PathType Leaf)) {
    throw "Missing workflow compensation instruction manifest: $ManifestPath"
}

try {
    $raw = Get-Content -LiteralPath $ManifestPath -Raw
    $manifest = $raw | ConvertFrom-Json
} catch {
    throw "Invalid workflow compensation instruction manifest JSON: $($_.Exception.Message)"
}

Assert-OnlyKnownFields -Object $manifest -Prefix "Workflow compensation instruction manifest" -Allowed @("instructions")
Assert-Condition ($null -ne $manifest.PSObject.Properties["instructions"]) "Workflow compensation instruction manifest requires instructions."

$instructions = @($manifest.instructions)
Assert-Condition ($instructions.Count -gt 0) "Workflow compensation instruction manifest requires at least one instruction."
Assert-Condition ($instructions.Count -le 100) "Workflow compensation instruction manifest supports at most 100 instructions."

$seenPayloadRefs = @{}
$summaries = @()
foreach ($instruction in $instructions) {
    Assert-OnlyKnownFields -Object $instruction -Prefix "Workflow compensation instruction" -Allowed @(
        "instruction_id",
        "workflow_id",
        "payload_ref_hash",
        "environment",
        "config_kind",
        "bundle_key",
        "target_version",
        "operator_ref",
        "reason_ref"
    )

    $workflowID = Get-RequiredString $instruction "workflow_id"
    $payloadRefHash = Get-RequiredString $instruction "payload_ref_hash"
    $environment = Get-RequiredString $instruction "environment"
    $configKind = Get-RequiredString $instruction "config_kind"
    $bundleKey = Get-RequiredString $instruction "bundle_key"
    $targetVersion = Get-RequiredString $instruction "target_version"
    $operatorRef = Get-RequiredString $instruction "operator_ref"
    $reasonRef = Get-RequiredString $instruction "reason_ref"
    $instructionID = ""
    if ($null -ne $instruction.PSObject.Properties["instruction_id"]) {
        $instructionID = ([string]$instruction.instruction_id).Trim()
    }

    Assert-LowSensitiveWorkflowRef -Value $instructionID -FieldName "instruction_id" -AllowEmpty
    Assert-LowSensitiveWorkflowRef -Value $workflowID -FieldName "workflow_id"
    Assert-LowSensitiveWorkflowRef -Value $payloadRefHash -FieldName "payload_ref_hash"
    Assert-LowSensitiveWorkflowRef -Value $environment -FieldName "environment"
    Assert-LowSensitiveWorkflowRef -Value $configKind -FieldName "config_kind"
    Assert-LowSensitiveWorkflowRef -Value $bundleKey -FieldName "bundle_key"
    Assert-LowSensitiveWorkflowRef -Value $targetVersion -FieldName "target_version"
    Assert-LowSensitiveWorkflowRef -Value $operatorRef -FieldName "operator_ref"
    Assert-LowSensitiveWorkflowRef -Value $reasonRef -FieldName "reason_ref"

    if ($seenPayloadRefs.ContainsKey($payloadRefHash)) {
        throw "Workflow compensation instruction manifest contains duplicate payload_ref_hash: $payloadRefHash"
    }
    $seenPayloadRefs[$payloadRefHash] = $true

    if ($ExpectedWorkflowID.Trim().Length -gt 0) {
        Assert-Condition ($workflowID -eq $ExpectedWorkflowID.Trim()) "Workflow compensation instruction workflow_id does not match expected value."
    }
    if ($ExpectedPayloadRefHash.Trim().Length -gt 0) {
        Assert-Condition ($payloadRefHash -eq $ExpectedPayloadRefHash.Trim()) "Workflow compensation instruction payload_ref_hash does not match expected value."
    }
    if ($ExpectedTargetVersion.Trim().Length -gt 0) {
        Assert-Condition ($targetVersion -eq $ExpectedTargetVersion.Trim()) "Workflow compensation instruction target_version does not match expected value."
    }

    $summaries += [ordered]@{
        instruction_id = $instructionID
        workflow_id = $workflowID
        payload_ref_hash = $payloadRefHash
        environment = $environment
        config_kind = $configKind
        bundle_key = $bundleKey
        target_version = $targetVersion
    }
}

$summary = [ordered]@{
    manifest_path = $ManifestPath
    schema_version = "nexusim.workflow.compensation_instruction_manifest.v1-runtime-json"
    instruction_count = $instructions.Count
    manifest_sha256 = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($raw))
    instructions = $summaries
    note = "Workflow compensation instruction manifest validation only. It does not call workflow-service, control-plane-service, or copy reason text, payload, EvidencePack, or provider body."
}

$summary | ConvertTo-Json -Depth 6
