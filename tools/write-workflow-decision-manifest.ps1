param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [Parameter(Mandatory = $true)]
    [string]$WorkflowID,

    [Parameter(Mandatory = $true)]
    [string]$StepID,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedWorkflowType,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedTargetService,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedTargetOperation,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedTargetRefHash,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedPayloadSchemaVersion,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedPayloadRefHash,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedApprovalPolicyRef,

    [string]$ExpectedStatus = "WAITING_DECISION",

    [Parameter(Mandatory = $true)]
    [ValidateSet("APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL")]
    [string]$Decision,

    [Parameter(Mandatory = $true)]
    [string]$DeciderRef,

    [string]$DecisionPolicyRef = "workflow.external-approval.v1",
    [string]$ReasonRef = "",
    [string]$ReasonFile = "",
    [string[]]$EvidenceRef = @(),
    [string]$IdempotencyKey = "",
    [string]$CorrelationID = "",
    [string]$CausationID = "",
    [string]$TraceID = "",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\repair-operator-safety.ps1"

Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-LowSensitiveRepairIdentifier -Value $WorkflowID -FieldName "WorkflowID"
Assert-LowSensitiveRepairIdentifier -Value $StepID -FieldName "StepID"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedWorkflowType -FieldName "ExpectedWorkflowType"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedStatus -FieldName "ExpectedStatus"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedTargetService -FieldName "ExpectedTargetService"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedTargetOperation -FieldName "ExpectedTargetOperation"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedTargetRefHash -FieldName "ExpectedTargetRefHash"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedPayloadSchemaVersion -FieldName "ExpectedPayloadSchemaVersion"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedPayloadRefHash -FieldName "ExpectedPayloadRefHash"
Assert-LowSensitiveRepairIdentifier -Value $ExpectedApprovalPolicyRef -FieldName "ExpectedApprovalPolicyRef"
Assert-LowSensitiveRepairActor -Value $DeciderRef -FieldName "DeciderRef"
Assert-LowSensitiveRepairIdentifier -Value $DecisionPolicyRef -FieldName "DecisionPolicyRef"
if ($ExpectedStatus.Trim().ToUpperInvariant() -ne "WAITING_DECISION") {
    throw "ExpectedStatus must be WAITING_DECISION."
}

if ((Test-Path -LiteralPath $OutputPath -PathType Leaf) -and -not $Force) {
    throw "OutputPath already exists. Use -Force to overwrite: $OutputPath"
}

if (-not [string]::IsNullOrWhiteSpace($ReasonRef) -and -not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    throw "Use either ReasonRef or ReasonFile, not both."
}

$reasonRefValue = $ReasonRef.Trim()
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    $reasonSummary = Read-RepairReasonFileSummary -Path $ReasonFile -MissingMessage "Missing workflow decision reason file"
    if ($reasonSummary.Present) {
        $reasonRefValue = "reason-sha256:" + [string]$reasonSummary.Sha256
    }
}
Assert-LowSensitiveRepairIdentifier -Value $reasonRefValue -FieldName "ReasonRef" -AllowEmpty

$normalizedEvidence = @()
$seenEvidence = @{}
foreach ($refEntry in $EvidenceRef) {
    foreach ($ref in ([string]$refEntry).Split(",")) {
        $trimmed = ([string]$ref).Trim()
        if ($trimmed.Length -eq 0) {
            continue
        }
        Assert-LowSensitiveRepairIdentifier -Value $trimmed -FieldName "EvidenceRef"
        if (-not $seenEvidence.ContainsKey($trimmed)) {
            $seenEvidence[$trimmed] = $true
            $normalizedEvidence += $trimmed
        }
    }
}

$decisionValue = $Decision.Trim().ToUpperInvariant()
if ([string]::IsNullOrWhiteSpace($IdempotencyKey)) {
    $IdempotencyKey = "external-approval:$($WorkflowID.Trim()):$($StepID.Trim()):$($decisionValue):$($DeciderRef.Trim())"
}
if ([string]::IsNullOrWhiteSpace($CorrelationID)) {
    $CorrelationID = "workflow-decision:$($WorkflowID.Trim())"
}
if ([string]::IsNullOrWhiteSpace($CausationID)) {
    $CausationID = "workflow:$($WorkflowID.Trim())"
}
if ([string]::IsNullOrWhiteSpace($TraceID)) {
    $TraceID = $CorrelationID
}

foreach ($pair in @(
    @("IdempotencyKey", $IdempotencyKey),
    @("CorrelationID", $CorrelationID),
    @("CausationID", $CausationID),
    @("TraceID", $TraceID)
)) {
    Assert-LowSensitiveRepairIdentifier -Value ([string]$pair[1]) -FieldName ([string]$pair[0])
}

$manifest = [ordered]@{
    schema_version = "nexusim.workflow.external_decision_manifest.v1"
    workflow_id = $WorkflowID.Trim()
    step_id = $StepID.Trim()
    expected_workflow_type = $ExpectedWorkflowType.Trim().ToUpperInvariant()
    expected_status = $ExpectedStatus.Trim().ToUpperInvariant()
    expected_target_service = $ExpectedTargetService.Trim()
    expected_target_operation = $ExpectedTargetOperation.Trim()
    expected_target_ref_hash = $ExpectedTargetRefHash.Trim()
    expected_payload_schema_version = $ExpectedPayloadSchemaVersion.Trim()
    expected_payload_ref_hash = $ExpectedPayloadRefHash.Trim()
    expected_approval_policy_ref = $ExpectedApprovalPolicyRef.Trim()
    decision = $decisionValue
    decider_ref = $DeciderRef.Trim()
    decision_policy_ref = $DecisionPolicyRef.Trim()
    reason_ref = $reasonRefValue
    evidence_refs = $normalizedEvidence
    idempotency_key = $IdempotencyKey.Trim()
    correlation_id = $CorrelationID.Trim()
    causation_id = $CausationID.Trim()
    trace_id = $TraceID.Trim()
}

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
($manifest | ConvertTo-Json -Depth 5) | Set-Content -LiteralPath $OutputPath -Encoding UTF8

$validator = Join-Path $PSScriptRoot "validate-workflow-decision-manifest.ps1"
& powershell -NoProfile -ExecutionPolicy Bypass -File $validator `
    -ManifestPath $OutputPath `
    -ExpectedWorkflowID $WorkflowID `
    -ExpectedStepID $StepID `
    -ExpectedDecision $decisionValue | Out-Null
if ($LASTEXITCODE -ne 0) {
    Remove-Item -LiteralPath $OutputPath -Force -ErrorAction SilentlyContinue
    throw "Generated workflow decision manifest failed validation."
}

Write-Host "OK   workflow decision manifest written: $OutputPath"
