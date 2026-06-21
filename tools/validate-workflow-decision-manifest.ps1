param(
    [Parameter(Mandatory = $true)]
    [string]$ManifestPath,

    [string]$ExpectedWorkflowID = "",
    [string]$ExpectedStepID = "",
    [string]$ExpectedDecision = ""
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

function Get-RequiredString {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object.PSObject.Properties[$Name]) {
        throw "Workflow decision manifest missing required field: $Name"
    }
    $value = ([string]$Object.$Name).Trim()
    if ($value.Length -eq 0) {
        throw "Workflow decision manifest field is required: $Name"
    }
    return $value
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
    throw "Missing workflow decision manifest: $ManifestPath"
}

try {
    $raw = Get-Content -LiteralPath $ManifestPath -Raw
    $manifest = $raw | ConvertFrom-Json
} catch {
    throw "Invalid workflow decision manifest JSON: $($_.Exception.Message)"
}

Assert-OnlyKnownFields -Object $manifest -Prefix "Workflow decision manifest" -Allowed @(
    "schema_version",
    "workflow_id",
    "step_id",
    "decision",
    "decider_ref",
    "decision_policy_ref",
    "reason_ref",
    "evidence_refs",
    "idempotency_key",
    "correlation_id",
    "causation_id",
    "trace_id"
)

Assert-Condition ((Get-RequiredString $manifest "schema_version") -eq "nexusim.workflow.decision_manifest.v1") "Unsupported workflow decision manifest schema_version."

$workflowID = Get-RequiredString $manifest "workflow_id"
$stepID = Get-RequiredString $manifest "step_id"
$decision = (Get-RequiredString $manifest "decision").ToUpperInvariant()
$deciderRef = Get-RequiredString $manifest "decider_ref"
$decisionPolicyRef = Get-RequiredString $manifest "decision_policy_ref"
$idempotencyKey = Get-RequiredString $manifest "idempotency_key"

Assert-LowSensitiveWorkflowRef -Value $workflowID -FieldName "workflow_id"
Assert-LowSensitiveWorkflowRef -Value $stepID -FieldName "step_id"
Assert-LowSensitiveRepairActor -Value $deciderRef -FieldName "decider_ref"
Assert-LowSensitiveWorkflowRef -Value $decisionPolicyRef -FieldName "decision_policy_ref"
Assert-LowSensitiveWorkflowRef -Value $idempotencyKey -FieldName "idempotency_key"

if (@("APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL") -notcontains $decision) {
    throw "Workflow decision manifest decision must be APPROVE, REJECT, REQUEST_CHANGES, or CANCEL."
}

if ($null -ne $manifest.PSObject.Properties["reason_ref"]) {
    Assert-LowSensitiveWorkflowRef -Value ([string]$manifest.reason_ref) -FieldName "reason_ref" -AllowEmpty
}

if ($null -ne $manifest.PSObject.Properties["evidence_refs"]) {
    foreach ($ref in @($manifest.evidence_refs)) {
        Assert-LowSensitiveWorkflowRef -Value ([string]$ref) -FieldName "evidence_refs"
    }
}

foreach ($field in @("correlation_id", "causation_id", "trace_id")) {
    if ($null -ne $manifest.PSObject.Properties[$field]) {
        Assert-LowSensitiveWorkflowRef -Value ([string]$manifest.$field) -FieldName $field -AllowEmpty
    }
}

if ($ExpectedWorkflowID.Trim().Length -gt 0) {
    Assert-Condition ($workflowID -eq $ExpectedWorkflowID.Trim()) "Workflow decision manifest workflow_id does not match expected value."
}
if ($ExpectedStepID.Trim().Length -gt 0) {
    Assert-Condition ($stepID -eq $ExpectedStepID.Trim()) "Workflow decision manifest step_id does not match expected value."
}
if ($ExpectedDecision.Trim().Length -gt 0) {
    Assert-Condition ($decision -eq $ExpectedDecision.Trim().ToUpperInvariant()) "Workflow decision manifest decision does not match expected value."
}

$summary = [ordered]@{
    manifest_path = $ManifestPath
    schema_version = "nexusim.workflow.decision_manifest.v1"
    workflow_id = $workflowID
    step_id = $stepID
    decision = $decision
    decider_ref = $deciderRef
    manifest_sha256 = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($raw))
    note = "Workflow decision manifest validation only. It does not call workflow-service or copy reason text, payload, EvidencePack, or provider body."
}

$summary | ConvertTo-Json -Depth 4
