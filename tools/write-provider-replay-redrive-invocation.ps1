param(
    [Parameter(Mandatory = $true)]
    [string]$HandoffPath,

    [Parameter(Mandatory = $true)]
    [string]$AdminOperationPath,

    [Parameter(Mandatory = $true)]
    [string]$WorkflowDecisionManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$FreshProofPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$ManifestID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($pathPair in @(
    @("HandoffPath", $HandoffPath),
    @("AdminOperationPath", $AdminOperationPath),
    @("WorkflowDecisionManifestPath", $WorkflowDecisionManifestPath),
    @("FreshProofPath", $FreshProofPath)
)) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing $name`: $path"
    }
    Assert-ExternalRepairOutputPath -Value $path -FieldName $name
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($FreshProofPath))) "provider-replay-redrive-invocation.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($ManifestID)) {
    $ManifestID = "provider-replay-redrive-invocation-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $ManifestID -FieldName "ManifestID"

function Get-FileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-JsonDocument {
    param(
        [string]$Path,
        [string]$Label
    )
    try {
        return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
    } catch {
        throw "$Label must be valid JSON: $Path"
    }
}

function Get-RequiredString {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        throw "$Name is required."
    }
    $value = ([string]$Object.$Name).Trim()
    if ($value.Length -eq 0) {
        throw "$Name is required."
    }
    return $value
}

function Get-OptionalString {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return ""
    }
    return ([string]$Object.$Name).Trim()
}

function Get-Array {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name] -or $null -eq $Object.$Name) {
        return @()
    }
    return @($Object.$Name)
}

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-False {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if ($Condition) {
        throw $Message
    }
}

function Assert-LowRef {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
}

function Assert-LowDocument {
    param(
        [object]$Value,
        [string]$FieldName
    )
    $encoded = $Value | ConvertTo-Json -Depth 30 -Compress
    if ($encoded -match "(?i)(password|passwd|secret|bearer|credential|api[_-]?key|access[_-]?key|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|private://|raw:|provider_body|message_body|evidencepack|prompt|reason_text|new_input_raw|provider_error_raw)") {
        throw "$FieldName contains raw, secret, prompt, provider artifact, or credential-like content."
    }
}

function Get-AdminOperation {
    param([object]$Document)

    if ($null -ne $Document.PSObject.Properties["operation"] -and $null -ne $Document.operation) {
        return $Document.operation
    }
    if ($null -ne $Document.PSObject.Properties["operations"]) {
        $items = @($Document.operations)
        if ($items.Count -eq 1) {
            return $items[0]
        }
        throw "Admin operation document must contain exactly one operation when operations[] is used."
    }
    if ($null -ne $Document.PSObject.Properties["operation_id"]) {
        return $Document
    }
    throw "Admin operation document must contain operation or operations[]."
}

function Get-MatchingAdminRequest {
    param(
        [object]$Handoff,
        [object]$Operation
    )
    $payloadHash = Get-RequiredString -Object $Operation -Name "payload_hash"
    $targetRefHash = Get-RequiredString -Object $Operation -Name "target_ref_hash"
    foreach ($request in (Get-Array -Object $Handoff -Name "admin_operation_requests")) {
        if ((Get-RequiredString -Object $request -Name "operation_payload_hash") -eq $payloadHash -and
            (Get-RequiredString -Object $request -Name "target_ref_hash") -eq $targetRefHash) {
            return $request
        }
    }
    throw "No matching handoff admin operation request for approved admin operation."
}

function Get-ReplayCandidateIDFromPayload {
    param([object]$Payload)
    if ($null -eq $Payload -or $null -eq $Payload.PSObject.Properties["replay_candidate_id"]) {
        throw "operation_payload.replay_candidate_id is required."
    }
    return ([string]$Payload.replay_candidate_id).Trim()
}

function Get-MatchingRow {
    param(
        [object]$Handoff,
        [string]$ReplayCandidateID
    )
    foreach ($row in (Get-Array -Object $Handoff -Name "rows")) {
        if ((Get-RequiredString -Object $row -Name "replay_candidate_id") -eq $ReplayCandidateID) {
            return $row
        }
    }
    throw "No matching handoff row for replay candidate: $ReplayCandidateID"
}

$handoff = Get-JsonDocument -Path $HandoffPath -Label "Provider replay handoff"
$adminDocument = Get-JsonDocument -Path $AdminOperationPath -Label "Admin operation"
$decision = Get-JsonDocument -Path $WorkflowDecisionManifestPath -Label "Workflow decision manifest"
$proof = Get-JsonDocument -Path $FreshProofPath -Label "Provider replay fresh proof"

Assert-LowDocument -Value $handoff -FieldName "HandoffPath"
Assert-LowDocument -Value $adminDocument -FieldName "AdminOperationPath"
Assert-LowDocument -Value $decision -FieldName "WorkflowDecisionManifestPath"
Assert-LowDocument -Value $proof -FieldName "FreshProofPath"

Assert-True ((Get-RequiredString -Object $handoff -Name "kind") -eq "action-executor.provider-failure.replay-admin-workflow-handoff") "Unsupported handoff kind."
$contract = $handoff.handoff_contract
if ($null -eq $contract) {
    throw "handoff_contract is required."
}
Assert-True ((Get-RequiredString -Object $contract -Name "admin_operation_type") -eq "PROVIDER_REPLAY_REQUEST") "handoff_contract.admin_operation_type must be PROVIDER_REPLAY_REQUEST."
Assert-True ((Get-RequiredString -Object $contract -Name "workflow_type") -eq "REPAIR_APPROVAL") "handoff_contract.workflow_type must be REPAIR_APPROVAL."
Assert-True ((Get-RequiredString -Object $contract -Name "target_service") -eq "action-executor") "handoff_contract.target_service must be action-executor."
Assert-True ((Get-RequiredString -Object $contract -Name "target_operation") -eq "PROVIDER_REPLAY_REQUEST") "handoff_contract.target_operation must be PROVIDER_REPLAY_REQUEST."
Assert-True ((Get-RequiredString -Object $contract -Name "redrive_entrypoint") -eq "RedriveProviderFailure") "handoff_contract.redrive_entrypoint must be RedriveProviderFailure."
Assert-True ((Get-RequiredString -Object $contract -Name "approval_policy_ref") -eq "admin.workflow.provider_replay.v1") "handoff_contract.approval_policy_ref mismatch."
Assert-False ([bool]$contract.direct_execution_allowed) "handoff_contract.direct_execution_allowed must be false."
Assert-True ([bool]$contract.source_dlq_immutable) "handoff_contract.source_dlq_immutable must be true."

$operation = Get-AdminOperation -Document $adminDocument
$operationID = Get-RequiredString -Object $operation -Name "operation_id"
$operationType = Get-RequiredString -Object $operation -Name "operation_type"
$operationStatus = (Get-RequiredString -Object $operation -Name "status").ToUpperInvariant()
$targetRefHash = Get-RequiredString -Object $operation -Name "target_ref_hash"
$payloadSchema = Get-RequiredString -Object $operation -Name "payload_schema_version"
$payloadHash = Get-RequiredString -Object $operation -Name "payload_hash"

Assert-True ($operationType -eq "PROVIDER_REPLAY_REQUEST") "Admin operation must be PROVIDER_REPLAY_REQUEST."
Assert-True ($operationStatus -eq "APPROVED") "Admin operation must be APPROVED before redrive invocation manifest."
Assert-True ($payloadSchema -eq "admin.provider_replay_request.v1") "Admin operation payload schema mismatch."
Assert-LowRef -Value $operationID -FieldName "operation.operation_id"
Assert-LowRef -Value $targetRefHash -FieldName "operation.target_ref_hash"
Assert-LowRef -Value $payloadHash -FieldName "operation.payload_hash"

$adminRequest = Get-MatchingAdminRequest -Handoff $handoff -Operation $operation
$operationPayload = $adminRequest.operation_payload
if ($null -eq $operationPayload) {
    throw "handoff admin request operation_payload is required."
}
$replayCandidateID = Get-ReplayCandidateIDFromPayload -Payload $operationPayload
$row = Get-MatchingRow -Handoff $handoff -ReplayCandidateID $replayCandidateID
Assert-True ((Get-RequiredString -Object $row -Name "status") -eq "DLQ") "Handoff row status must be DLQ."
Assert-True ((Get-RequiredString -Object $row -Name "replay_state") -eq "AWAITING_ADMIN_WORKFLOW") "Handoff row replay_state must be AWAITING_ADMIN_WORKFLOW."

$decisionSha = Get-FileSha256Ref -Path $WorkflowDecisionManifestPath
Assert-True ((Get-RequiredString -Object $decision -Name "schema_version") -eq "nexusim.workflow.external_decision_manifest.v1") "Unsupported workflow decision manifest."
Assert-True (((Get-RequiredString -Object $decision -Name "decision").ToUpperInvariant()) -eq "APPROVE") "Workflow decision manifest must APPROVE provider replay."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_workflow_type") -eq "REPAIR_APPROVAL") "Workflow expected type must be REPAIR_APPROVAL."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_target_service") -eq "action-executor") "Workflow target service must be action-executor."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_target_operation") -eq "PROVIDER_REPLAY_REQUEST") "Workflow target operation must be PROVIDER_REPLAY_REQUEST."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_target_ref_hash") -eq $targetRefHash) "Workflow target ref hash must match approved admin operation."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_payload_schema_version") -eq $payloadSchema) "Workflow payload schema must match approved admin operation."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_payload_ref_hash") -eq $payloadHash) "Workflow payload ref hash must match approved admin operation."
Assert-True ((Get-RequiredString -Object $decision -Name "expected_approval_policy_ref") -eq "admin.workflow.provider_replay.v1") "Workflow approval policy mismatch."

Assert-True ((Get-RequiredString -Object $proof -Name "schema_version") -eq "nexusim.action_executor.provider_replay_fresh_proof.v1") "Unsupported fresh proof schema_version."
Assert-True ((Get-RequiredString -Object $proof -Name "replay_candidate_id") -eq $replayCandidateID) "Fresh proof replay_candidate_id mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "provider_failure_ref_hash") -eq $targetRefHash) "Fresh proof provider_failure_ref_hash mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "admin_operation_id") -eq $operationID) "Fresh proof admin_operation_id mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "admin_operation_payload_hash") -eq $payloadHash) "Fresh proof admin_operation_payload_hash mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "workflow_decision_manifest_sha256") -eq $decisionSha) "Fresh proof workflow_decision_manifest_sha256 mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "skill_id") -eq (Get-RequiredString -Object $row -Name "skill_id")) "Fresh proof skill_id mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "tool_name") -eq (Get-RequiredString -Object $row -Name "tool_name")) "Fresh proof tool_name mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "resource_type") -eq (Get-RequiredString -Object $row -Name "resource_type")) "Fresh proof resource_type mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "resource_id_hash") -eq (Get-RequiredString -Object $row -Name "resource_id_hash")) "Fresh proof resource_id_hash mismatch."
Assert-True ((Get-RequiredString -Object $proof -Name "new_input_sha256") -match "^sha256:[a-f0-9]{64}$") "Fresh proof new_input_sha256 must be sha256:<hex>."
Assert-True ((Get-RequiredString -Object $proof -Name "reason_sha256") -match "^sha256:[a-f0-9]{64}$") "Fresh proof reason_sha256 must be sha256:<hex>."
Assert-False ([bool]$proof.direct_execution_allowed) "Fresh proof direct_execution_allowed must be false."
Assert-True ([bool]$proof.source_dlq_immutable) "Fresh proof source_dlq_immutable must be true."
Assert-False ([bool]$proof.raw_input_present) "Fresh proof raw_input_present must be false."
Assert-False ([bool]$proof.raw_reason_present) "Fresh proof raw_reason_present must be false."
Assert-False ([bool]$proof.raw_provider_artifact_present) "Fresh proof raw_provider_artifact_present must be false."

$proposalID = Get-RequiredString -Object $proof -Name "proposal_id"
$approvalID = Get-RequiredString -Object $proof -Name "approval_id"
$preparedAuditID = Get-RequiredString -Object $proof -Name "prepared_audit_id"
Assert-LowRef -Value $proposalID -FieldName "proof.proposal_id"
Assert-LowRef -Value $approvalID -FieldName "proof.approval_id"
Assert-LowRef -Value $preparedAuditID -FieldName "proof.prepared_audit_id"
Assert-True ($proposalID -ne (Get-RequiredString -Object $row -Name "proposal_id")) "Fresh proof proposal_id must not reuse source proposal."
Assert-True ($approvalID -ne (Get-RequiredString -Object $row -Name "approval_id")) "Fresh proof approval_id must not reuse source approval."
Assert-True ($preparedAuditID -ne (Get-RequiredString -Object $row -Name "prepared_audit_id")) "Fresh proof prepared_audit_id must not reuse source prepared audit."

foreach ($field in @(
    "workflow_id",
    "workflow_step_id",
    "policy_check_ref",
    "prepared_audit_ref",
    "generated_by",
    "correlation_id",
    "trace_id"
)) {
    Assert-LowRef -Value (Get-OptionalString -Object $proof -Name $field) -FieldName "proof.$field" -AllowEmpty
}

$manifest = [ordered]@{
    schema_version = "nexusim.action_executor.provider_replay_redrive_invocation.v1"
    manifest_id = $ManifestID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    entrypoint = "RedriveProviderFailure"
    rpc_full_method = "/nexusim.actionexecutor.v1.ActionExecutorService/RedriveProviderFailure"
    executes_redrive = $false
    mutates_provider_failure = $false
    source_dlq_immutable = $true
    direct_execution_allowed = $false
    requires_operator_execution = $true
    operator_must_supply_raw_resource_id_and_new_input_outside_manifest = $true
    provider_failure_id = Get-RequiredString -Object $row -Name "provider_failure_id"
    provider_failure_ref_hash = $targetRefHash
    replay_candidate_id = $replayCandidateID
    admin_operation_id = $operationID
    admin_operation_payload_hash = $payloadHash
    workflow_id = Get-RequiredString -Object $decision -Name "workflow_id"
    workflow_step_id = Get-RequiredString -Object $decision -Name "step_id"
    workflow_decision_manifest_sha256 = $decisionSha
    proposal_id = $proposalID
    approval_id = $approvalID
    prepared_audit_id = $preparedAuditID
    skill_id = Get-RequiredString -Object $proof -Name "skill_id"
    tool_name = Get-RequiredString -Object $proof -Name "tool_name"
    action = "EXECUTE"
    resource_type = Get-RequiredString -Object $proof -Name "resource_type"
    resource_id_hash = Get-RequiredString -Object $proof -Name "resource_id_hash"
    new_input_sha256 = Get-RequiredString -Object $proof -Name "new_input_sha256"
    reason_sha256 = Get-RequiredString -Object $proof -Name "reason_sha256"
    auth_context_contract = [ordered]@{
        tenant_id = Get-RequiredString -Object $row -Name "tenant_id"
        user_id = "operator-supplied-outside-manifest"
        device_id = "operator-supplied-outside-manifest"
        session_id = "operator-supplied-outside-manifest"
        trace_id = Get-OptionalString -Object $proof -Name "trace_id"
        request_id = "operator-generated-at-execution-time"
    }
    redrive_request_contract = [ordered]@{
        provider_failure_id = Get-RequiredString -Object $row -Name "provider_failure_id"
        reason_sha256 = Get-RequiredString -Object $proof -Name "reason_sha256"
        proposal_id = $proposalID
        approval_id = $approvalID
        prepared_audit_id = $preparedAuditID
        skill_id = Get-RequiredString -Object $proof -Name "skill_id"
        tool_name = Get-RequiredString -Object $proof -Name "tool_name"
        action = "EXECUTE"
        resource_type = Get-RequiredString -Object $proof -Name "resource_type"
        resource_id_source = "operator-supplied-outside-manifest; must hash to resource_id_hash"
        resource_id_hash = Get-RequiredString -Object $proof -Name "resource_id_hash"
        risk_level = Get-OptionalString -Object $operation -Name "risk_level"
        intent = "provider failure redrive after approved repair workflow"
        input_json_source = "operator-supplied-outside-manifest; must hash to new_input_sha256"
        new_input_sha256 = Get-RequiredString -Object $proof -Name "new_input_sha256"
        idempotency_key_hint = "provider-replay-redrive:$replayCandidateID`:$approvalID"
    }
    required_checks = @(
        "admin_operation_approved",
        "workflow_approval_recorded",
        "fresh_agent_proposal",
        "fresh_agent_approval",
        "fresh_prepared_audit",
        "matching_skill_tool_resource",
        "new_input_sha256_matches_external_file",
        "reason_sha256_matches_external_file",
        "resource_id_hash_matches_operator_supplied_resource",
        "action_executor_redrive_provider_failure_only"
    )
    forbidden_contents = @(
        "raw_provider_input",
        "raw_provider_output",
        "raw_provider_error",
        "raw_new_input",
        "raw_reason",
        "EvidencePack",
        "local_file_path",
        "credentials"
    )
    note = "Low-sensitive final operator invocation manifest. It does not call RedriveProviderFailure; operator execution must happen separately against action-executor after verifying external raw inputs against the recorded hashes."
}

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$manifest | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   provider replay redrive invocation manifest written: $OutputPath"
