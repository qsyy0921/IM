param(
    [Parameter(Mandatory = $true)]
    [string]$InvocationPath,

    [Parameter(Mandatory = $true)]
    [string]$ExecutionResultPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$ResultManifestID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($pathPair in @(
    @("InvocationPath", $InvocationPath),
    @("ExecutionResultPath", $ExecutionResultPath)
)) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing $name`: $path"
    }
    Assert-ExternalRepairOutputPath -Value $path -FieldName $name
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($ExecutionResultPath))) "provider-replay-redrive-result-manifest.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($ResultManifestID)) {
    $ResultManifestID = "provider-replay-redrive-result-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $ResultManifestID -FieldName "ResultManifestID"

function Get-ResultFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-ResultStringSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
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

function Get-JsonString {
    param(
        [object]$Object,
        [string]$Name,
        [switch]$AllowEmpty
    )
    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        if ($AllowEmpty) {
            return ""
        }
        throw "$Name is required."
    }
    $value = ([string]$Object.$Name).Trim()
    if ($value.Length -eq 0 -and -not $AllowEmpty) {
        throw "$Name is required."
    }
    return $value
}

function Get-JsonArray {
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

function Assert-LowString {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
}

function Assert-LowResultRef {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )
    $text = ([string]$Value).Trim()
    if ($text.Length -eq 0) {
        if ($AllowEmpty) {
            return
        }
        throw "$FieldName is required."
    }
    if ($text.Length -gt 256 -or $text -notmatch "^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,255}$") {
        throw "$FieldName must be a low-sensitive result ref using letters, digits, dot, underscore, dash, colon, or slash."
    }
    Assert-NoRawText -Value $text -FieldName $FieldName
}

function Assert-Sha256Ref {
    param(
        [string]$Value,
        [string]$FieldName
    )
    if ($Value -notmatch "^sha256:[a-f0-9]{64}$") {
        throw "$FieldName must be sha256:<hex>."
    }
}

function Assert-NoRawText {
    param(
        [string]$Value,
        [string]$FieldName
    )
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|provider_body|provider_error|message_body|EvidencePack|prompt|reason_text|new_input_raw|resource_id_raw|input_json)") {
        throw "$FieldName contains raw, secret, prompt, provider artifact, or credential-like content."
    }
}

function Assert-ArrayContains {
    param(
        [object[]]$Values,
        [string]$Expected,
        [string]$FieldName
    )
    if (@($Values) -notcontains $Expected) {
        throw "$FieldName must contain $Expected."
    }
}

function Assert-Same {
    param(
        [string]$Actual,
        [string]$Expected,
        [string]$FieldName
    )
    if ($Actual -ne $Expected) {
        throw "$FieldName mismatch."
    }
}

$executionRaw = Get-Content -LiteralPath $ExecutionResultPath -Raw
Assert-NoRawText -Value $executionRaw -FieldName "ExecutionResultPath"

$invocation = Get-JsonDocument -Path $InvocationPath -Label "Provider replay redrive invocation manifest"
$execution = Get-JsonDocument -Path $ExecutionResultPath -Label "Provider replay redrive execution summary"

$invocationSchema = Get-JsonString -Object $invocation -Name "schema_version"
Assert-True ($invocationSchema -eq "nexusim.action_executor.provider_replay_redrive_invocation.v1") "Unsupported provider replay redrive invocation schema_version: $invocationSchema"
Assert-True ((Get-JsonString -Object $invocation -Name "entrypoint") -eq "RedriveProviderFailure") "Invocation entrypoint must be RedriveProviderFailure."
Assert-True ((Get-JsonString -Object $invocation -Name "rpc_full_method") -eq "/nexusim.actionexecutor.v1.ActionExecutorService/RedriveProviderFailure") "Invocation RPC method mismatch."
Assert-False ([bool]$invocation.executes_redrive) "Invocation manifest must not claim it executed redrive."
Assert-False ([bool]$invocation.mutates_provider_failure) "Invocation manifest must not mutate provider failure state."
Assert-False ([bool]$invocation.direct_execution_allowed) "Invocation direct_execution_allowed must be false."
Assert-True ([bool]$invocation.source_dlq_immutable) "Invocation source_dlq_immutable must be true."
Assert-True ([bool]$invocation.requires_operator_execution) "Invocation requires_operator_execution must be true."

$manifestID = Get-JsonString -Object $invocation -Name "manifest_id"
$providerFailureID = Get-JsonString -Object $invocation -Name "provider_failure_id"
$replayCandidateID = Get-JsonString -Object $invocation -Name "replay_candidate_id"
$adminOperationID = Get-JsonString -Object $invocation -Name "admin_operation_id"
$workflowID = Get-JsonString -Object $invocation -Name "workflow_id"
$workflowStepID = Get-JsonString -Object $invocation -Name "workflow_step_id"
$proposalID = Get-JsonString -Object $invocation -Name "proposal_id"
$approvalID = Get-JsonString -Object $invocation -Name "approval_id"
$preparedAuditID = Get-JsonString -Object $invocation -Name "prepared_audit_id"
$skillID = Get-JsonString -Object $invocation -Name "skill_id"
$toolName = Get-JsonString -Object $invocation -Name "tool_name"
$resourceType = Get-JsonString -Object $invocation -Name "resource_type"
$resourceIDHash = Get-JsonString -Object $invocation -Name "resource_id_hash"
$newInputSHA256 = Get-JsonString -Object $invocation -Name "new_input_sha256"
$reasonSHA256 = Get-JsonString -Object $invocation -Name "reason_sha256"

foreach ($entry in @(
        @{ name = "manifest_id"; value = $manifestID },
        @{ name = "provider_failure_id"; value = $providerFailureID },
        @{ name = "replay_candidate_id"; value = $replayCandidateID },
        @{ name = "admin_operation_id"; value = $adminOperationID },
        @{ name = "workflow_id"; value = $workflowID },
        @{ name = "workflow_step_id"; value = $workflowStepID },
        @{ name = "proposal_id"; value = $proposalID },
        @{ name = "approval_id"; value = $approvalID },
        @{ name = "prepared_audit_id"; value = $preparedAuditID },
        @{ name = "skill_id"; value = $skillID },
        @{ name = "tool_name"; value = $toolName },
        @{ name = "resource_type"; value = $resourceType },
        @{ name = "resource_id_hash"; value = $resourceIDHash },
        @{ name = "new_input_sha256"; value = $newInputSHA256 },
        @{ name = "reason_sha256"; value = $reasonSHA256 }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName "invocation.$($entry.name)"
    Assert-NoRawText -Value ([string]$entry.value) -FieldName "invocation.$($entry.name)"
}
Assert-Sha256Ref -Value $newInputSHA256 -FieldName "invocation.new_input_sha256"
Assert-Sha256Ref -Value $reasonSHA256 -FieldName "invocation.reason_sha256"

$authContract = $invocation.auth_context_contract
if ($null -eq $authContract) {
    throw "invocation.auth_context_contract is required."
}
$tenantID = Get-JsonString -Object $authContract -Name "tenant_id"
$traceID = Get-JsonString -Object $authContract -Name "trace_id" -AllowEmpty
Assert-LowString -Value $tenantID -FieldName "auth_context_contract.tenant_id"
Assert-LowString -Value $traceID -FieldName "auth_context_contract.trace_id" -AllowEmpty

$redriveContract = $invocation.redrive_request_contract
if ($null -eq $redriveContract) {
    throw "invocation.redrive_request_contract is required."
}
$idempotencyKey = Get-JsonString -Object $redriveContract -Name "idempotency_key_hint"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "provider_failure_id") -Expected $providerFailureID -FieldName "redrive_request_contract.provider_failure_id"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "proposal_id") -Expected $proposalID -FieldName "redrive_request_contract.proposal_id"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "approval_id") -Expected $approvalID -FieldName "redrive_request_contract.approval_id"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "prepared_audit_id") -Expected $preparedAuditID -FieldName "redrive_request_contract.prepared_audit_id"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "skill_id") -Expected $skillID -FieldName "redrive_request_contract.skill_id"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "tool_name") -Expected $toolName -FieldName "redrive_request_contract.tool_name"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "resource_type") -Expected $resourceType -FieldName "redrive_request_contract.resource_type"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "resource_id_hash") -Expected $resourceIDHash -FieldName "redrive_request_contract.resource_id_hash"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "new_input_sha256") -Expected $newInputSHA256 -FieldName "redrive_request_contract.new_input_sha256"
Assert-Same -Actual (Get-JsonString -Object $redriveContract -Name "reason_sha256") -Expected $reasonSHA256 -FieldName "redrive_request_contract.reason_sha256"
Assert-LowString -Value $idempotencyKey -FieldName "redrive_request_contract.idempotency_key_hint"

$invocationChecks = @(Get-JsonArray -Object $invocation -Name "required_checks")
foreach ($expected in @(
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
    )) {
    Assert-ArrayContains -Values $invocationChecks -Expected $expected -FieldName "invocation.required_checks"
}

Assert-True ((Get-JsonString -Object $execution -Name "mode") -eq "provider-replay-redrive") "Execution mode must be provider-replay-redrive."
Assert-Same -Actual (Get-JsonString -Object $execution -Name "manifest_id") -Expected $manifestID -FieldName "execution.manifest_id"
Assert-Same -Actual (Get-JsonString -Object $execution -Name "replay_candidate_id") -Expected $replayCandidateID -FieldName "execution.replay_candidate_id"
Assert-Same -Actual (Get-JsonString -Object $execution -Name "provider_failure_id") -Expected $providerFailureID -FieldName "execution.provider_failure_id"
Assert-Same -Actual (Get-JsonString -Object $execution -Name "admin_operation_id") -Expected $adminOperationID -FieldName "execution.admin_operation_id"
Assert-Same -Actual (Get-JsonString -Object $execution -Name "workflow_id") -Expected $workflowID -FieldName "execution.workflow_id"
Assert-True ([bool]$execution.executed_redrive) "Execution summary must have executed_redrive=true."

$request = $execution.request
if ($null -eq $request) {
    throw "execution.request is required."
}
$operatorUserRef = Get-JsonString -Object $request -Name "user_id"
$operatorDeviceRef = Get-JsonString -Object $request -Name "device_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "tenant_id") -Expected $tenantID -FieldName "request.tenant_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "proposal_id") -Expected $proposalID -FieldName "request.proposal_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "approval_id") -Expected $approvalID -FieldName "request.approval_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "prepared_audit_id") -Expected $preparedAuditID -FieldName "request.prepared_audit_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "skill_id") -Expected $skillID -FieldName "request.skill_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "tool_name") -Expected $toolName -FieldName "request.tool_name"
Assert-Same -Actual (Get-JsonString -Object $request -Name "resource_type") -Expected $resourceType -FieldName "request.resource_type"
Assert-Same -Actual (Get-JsonString -Object $request -Name "resource_id_hash") -Expected $resourceIDHash -FieldName "request.resource_id_hash"
Assert-Same -Actual (Get-JsonString -Object $request -Name "input_sha256") -Expected $newInputSHA256 -FieldName "request.input_sha256"
Assert-Same -Actual (Get-JsonString -Object $request -Name "reason_sha256") -Expected $reasonSHA256 -FieldName "request.reason_sha256"
Assert-Same -Actual (Get-JsonString -Object $request -Name "idempotency_key") -Expected $idempotencyKey -FieldName "request.idempotency_key"
Assert-LowString -Value $operatorUserRef -FieldName "request.user_id"
Assert-LowString -Value $operatorDeviceRef -FieldName "request.device_id"

$response = $execution.response
if ($null -eq $response) {
    throw "execution.response is required."
}
$sourceExecutionID = Get-JsonString -Object $response -Name "source_execution_id"
$sourceResultID = Get-JsonString -Object $response -Name "source_result_id"
$redriveExecutionID = Get-JsonString -Object $response -Name "redrive_execution_id"
$redriveResultID = Get-JsonString -Object $response -Name "redrive_result_id"
$resultStatus = Get-JsonString -Object $response -Name "result_status"
$status = Get-JsonString -Object $response -Name "status" -AllowEmpty
$classification = Get-JsonString -Object $response -Name "classification" -AllowEmpty
$resultRef = Get-JsonString -Object $response -Name "result_ref" -AllowEmpty
$responseReason = Get-JsonString -Object $response -Name "reason" -AllowEmpty

Assert-Same -Actual (Get-JsonString -Object $response -Name "provider_failure_id") -Expected $providerFailureID -FieldName "response.provider_failure_id"
Assert-Same -Actual (Get-JsonString -Object $response -Name "proposal_id") -Expected $proposalID -FieldName "response.proposal_id"
Assert-Same -Actual (Get-JsonString -Object $response -Name "approval_id") -Expected $approvalID -FieldName "response.approval_id"
Assert-Same -Actual (Get-JsonString -Object $response -Name "prepared_audit_id") -Expected $preparedAuditID -FieldName "response.prepared_audit_id"
Assert-Same -Actual (Get-JsonString -Object $response -Name "skill_id") -Expected $skillID -FieldName "response.skill_id"
Assert-Same -Actual (Get-JsonString -Object $response -Name "tool_name") -Expected $toolName -FieldName "response.tool_name"
Assert-Same -Actual (Get-JsonString -Object $response -Name "resource_type") -Expected $resourceType -FieldName "response.resource_type"
Assert-Same -Actual (Get-JsonString -Object $response -Name "resource_id_hash") -Expected $resourceIDHash -FieldName "response.resource_id_hash"
Assert-True ([bool]$response.executed) "response.executed must be true."

foreach ($entry in @(
        @{ name = "source_execution_id"; value = $sourceExecutionID; allow = $false },
        @{ name = "source_result_id"; value = $sourceResultID; allow = $false },
        @{ name = "redrive_execution_id"; value = $redriveExecutionID; allow = $false },
        @{ name = "redrive_result_id"; value = $redriveResultID; allow = $false },
        @{ name = "result_status"; value = $resultStatus; allow = $false },
        @{ name = "status"; value = $status; allow = $true },
        @{ name = "classification"; value = $classification; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName "response.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
    Assert-NoRawText -Value ([string]$entry.value) -FieldName "response.$($entry.name)"
}
Assert-LowResultRef -Value $resultRef -FieldName "response.result_ref" -AllowEmpty

$sourceInvocationResolvedPath = [string](Resolve-Path -LiteralPath $InvocationPath)
$sourceExecutionResolvedPath = [string](Resolve-Path -LiteralPath $ExecutionResultPath)
$responseReasonSHA256 = ""
if ($responseReason.Length -gt 0) {
    Assert-NoRawText -Value $responseReason -FieldName "response.reason"
    $responseReasonSHA256 = Get-ResultStringSha256Ref -Value $responseReason
}

$manifest = [ordered]@{
    schema_version = "nexusim.action_executor.provider_replay_redrive_result.v1"
    result_manifest_id = $ResultManifestID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    source_invocation_sha256 = Get-ResultFileSha256Ref -Path $InvocationPath
    source_execution_summary_sha256 = Get-ResultFileSha256Ref -Path $ExecutionResultPath
    source_invocation_path_sha256 = Get-ResultStringSha256Ref -Value $sourceInvocationResolvedPath
    source_execution_summary_path_sha256 = Get-ResultStringSha256Ref -Value $sourceExecutionResolvedPath
    manifest_is_execution = $false
    executes_redrive = $false
    mutates_provider_failure = $false
    appends_audit = $false
    source_dlq_immutable = $true
    invocation = [ordered]@{
        manifest_id = $manifestID
        provider_failure_id = $providerFailureID
        replay_candidate_id = $replayCandidateID
        admin_operation_id = $adminOperationID
        workflow_id = $workflowID
        workflow_step_id = $workflowStepID
        proposal_id = $proposalID
        approval_id = $approvalID
        prepared_audit_id = $preparedAuditID
        skill_id = $skillID
        tool_name = $toolName
        resource_type = $resourceType
        resource_id_hash = $resourceIDHash
        new_input_sha256 = $newInputSHA256
        reason_sha256 = $reasonSHA256
        trace_id = $traceID
    }
    execution_request = [ordered]@{
        tenant_id = $tenantID
        operator_user_ref = $operatorUserRef
        operator_device_ref = $operatorDeviceRef
        proposal_id = $proposalID
        approval_id = $approvalID
        prepared_audit_id = $preparedAuditID
        skill_id = $skillID
        tool_name = $toolName
        resource_type = $resourceType
        resource_id_hash = $resourceIDHash
        new_input_sha256 = $newInputSHA256
        reason_sha256 = $reasonSHA256
        idempotency_key = $idempotencyKey
    }
    execution_result = [ordered]@{
        provider_failure_id = $providerFailureID
        source_execution_id = $sourceExecutionID
        source_result_id = $sourceResultID
        redrive_execution_id = $redriveExecutionID
        redrive_result_id = $redriveResultID
        proposal_id = $proposalID
        approval_id = $approvalID
        prepared_audit_id = $preparedAuditID
        skill_id = $skillID
        tool_name = $toolName
        resource_type = $resourceType
        resource_id_hash = $resourceIDHash
        status = $status
        result_status = $resultStatus
        executed = $true
        classification = $classification
        result_ref = $resultRef
        response_reason_sha256 = $responseReasonSHA256
    }
    required_checks = @(
        "source_invocation_manifest_verified",
        "execution_summary_matches_invocation",
        "action_executor_reported_executed_redrive",
        "redrive_result_ref_present_or_status_recorded",
        "source_dlq_remains_immutable",
        "operator_must_append_external_audit_separately_if_needed"
    )
    forbidden_contents = @(
        "payload_material",
        "provider_artifact_material",
        "operator_reason_material",
        "evidence_text_material",
        "filesystem_path_material",
        "auth_material"
    )
    execution_boundary = @(
        "result_manifest_is_not_redrive_execution",
        "does_not_call_action_executor",
        "does_not_append_audit_record",
        "does_not_modify_provider_failure_or_dlq",
        "does_not_create_admin_or_workflow_decision"
    )
    note = "Low-sensitive provider replay redrive result manifest. It binds an executed action-executor redrive summary to its approved invocation, but does not execute redrive, append audit, mutate DLQ rows, or embed raw operator input."
}

$encoded = $manifest | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawText -Value $encoded -FieldName "provider replay redrive result manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$manifest | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   provider replay redrive result manifest written: $OutputPath"
