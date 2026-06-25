param(
    [Parameter(Mandatory = $true)]
    [string]$ResultManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$AuditManifestID = "",
    [string]$AuditRecordID = "",
    [string]$OccurredAt = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $ResultManifestPath -PathType Leaf)) {
    throw "Missing ResultManifestPath: $ResultManifestPath"
}
Assert-ExternalRepairOutputPath -Value $ResultManifestPath -FieldName "ResultManifestPath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($ResultManifestPath))) "provider-replay-redrive-audit-append.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($AuditManifestID)) {
    $AuditManifestID = "action-executor-audit-append-" + [System.Guid]::NewGuid().ToString("N")
}
if ([string]::IsNullOrWhiteSpace($AuditRecordID)) {
    $AuditRecordID = "action-executor:audit:" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $AuditManifestID -FieldName "AuditManifestID"
Assert-LowSensitiveRepairIdentifier -Value $AuditRecordID -FieldName "AuditRecordID"

function Get-AuditFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-AuditStringSha256Ref {
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
    Assert-NoRawText -Value $Value -FieldName $FieldName
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

function Assert-NoRawText {
    param(
        [string]$Value,
        [string]$FieldName
    )
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|provider_body|provider_error|message_body|EvidencePack|prompt|reason_text|new_input_raw|resource_id_raw|input_json|filesystem_path)") {
        throw "$FieldName contains raw, secret, prompt, provider artifact, or credential-like content."
    }
}

function Assert-NoRawDocumentText {
    param(
        [string]$Value,
        [string]$FieldName
    )
    if ($Value -match '(?i)(bearer\s+\S+|password\s*[:=]|secret\s*[:=]|api[_-]?key\s*[:=]|access[_-]?key\s*[:=]|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.|postgres://|mysql://|mongodb://|"provider_body"\s*:|"provider_error_body"\s*:|"message_body"\s*:|"raw_provider_input"\s*:|"raw_provider_output"\s*:|"input_json"\s*:|"raw_new_input"\s*:|"raw_reason"\s*:|"resource_id_raw"\s*:|"new_input_raw"\s*:|"reason_text"\s*:)') {
        throw "$FieldName contains raw, secret, prompt, provider artifact, or credential-like content."
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

function ConvertTo-CanonicalFlatJson {
    param([hashtable]$Values)

    $parts = @()
    foreach ($key in @($Values.Keys | Sort-Object)) {
        $encodedKey = ConvertTo-Json ([string]$key) -Compress
        $encodedValue = ConvertTo-Json ([string]$Values[$key]) -Compress
        $parts += ("{0}:{1}" -f $encodedKey, $encodedValue)
    }
    return "{" + ($parts -join ",") + "}"
}

$resultRaw = Get-Content -LiteralPath $ResultManifestPath -Raw
Assert-NoRawDocumentText -Value $resultRaw -FieldName "ResultManifestPath"
$result = Get-JsonDocument -Path $ResultManifestPath -Label "Provider replay redrive result manifest"

Assert-True ((Get-JsonString -Object $result -Name "schema_version") -eq "nexusim.action_executor.provider_replay_redrive_result.v1") "Unsupported provider replay redrive result schema_version."
Assert-False ([bool]$result.manifest_is_execution) "Result manifest must not be an execution."
Assert-False ([bool]$result.executes_redrive) "Result manifest must not claim it executes redrive."
Assert-False ([bool]$result.mutates_provider_failure) "Result manifest must not mutate provider failure state."
Assert-False ([bool]$result.appends_audit) "Result manifest must not claim audit was already appended."
Assert-True ([bool]$result.source_dlq_immutable) "Result manifest must preserve source DLQ immutability."

$resultManifestID = Get-JsonString -Object $result -Name "result_manifest_id"
$sourceResultManifestSHA256 = Get-AuditFileSha256Ref -Path $ResultManifestPath

$invocation = $result.invocation
$request = $result.execution_request
$executionResult = $result.execution_result
if ($null -eq $invocation) {
    throw "result.invocation is required."
}
if ($null -eq $request) {
    throw "result.execution_request is required."
}
if ($null -eq $executionResult) {
    throw "result.execution_result is required."
}

$tenantID = Get-JsonString -Object $request -Name "tenant_id"
$sourceInvocationManifestID = Get-JsonString -Object $invocation -Name "manifest_id"
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
$traceID = Get-JsonString -Object $invocation -Name "trace_id" -AllowEmpty

$sourceExecutionID = Get-JsonString -Object $executionResult -Name "source_execution_id"
$sourceResultID = Get-JsonString -Object $executionResult -Name "source_result_id"
$redriveExecutionID = Get-JsonString -Object $executionResult -Name "redrive_execution_id"
$redriveResultID = Get-JsonString -Object $executionResult -Name "redrive_result_id"
$resultStatus = Get-JsonString -Object $executionResult -Name "result_status"
$status = Get-JsonString -Object $executionResult -Name "status" -AllowEmpty
$classification = Get-JsonString -Object $executionResult -Name "classification" -AllowEmpty
$resultRef = Get-JsonString -Object $executionResult -Name "result_ref" -AllowEmpty
$responseReasonSHA256 = Get-JsonString -Object $executionResult -Name "response_reason_sha256" -AllowEmpty

foreach ($entry in @(
        @{ name = "result_manifest_id"; value = $resultManifestID; allow = $false },
        @{ name = "tenant_id"; value = $tenantID; allow = $false },
        @{ name = "invocation.manifest_id"; value = $sourceInvocationManifestID; allow = $false },
        @{ name = "provider_failure_id"; value = $providerFailureID; allow = $false },
        @{ name = "replay_candidate_id"; value = $replayCandidateID; allow = $false },
        @{ name = "admin_operation_id"; value = $adminOperationID; allow = $false },
        @{ name = "workflow_id"; value = $workflowID; allow = $false },
        @{ name = "workflow_step_id"; value = $workflowStepID; allow = $false },
        @{ name = "proposal_id"; value = $proposalID; allow = $false },
        @{ name = "approval_id"; value = $approvalID; allow = $false },
        @{ name = "prepared_audit_id"; value = $preparedAuditID; allow = $false },
        @{ name = "skill_id"; value = $skillID; allow = $false },
        @{ name = "tool_name"; value = $toolName; allow = $false },
        @{ name = "resource_type"; value = $resourceType; allow = $false },
        @{ name = "source_execution_id"; value = $sourceExecutionID; allow = $false },
        @{ name = "source_result_id"; value = $sourceResultID; allow = $false },
        @{ name = "redrive_execution_id"; value = $redriveExecutionID; allow = $false },
        @{ name = "redrive_result_id"; value = $redriveResultID; allow = $false },
        @{ name = "result_status"; value = $resultStatus; allow = $false },
        @{ name = "status"; value = $status; allow = $true },
        @{ name = "classification"; value = $classification; allow = $true },
        @{ name = "trace_id"; value = $traceID; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName ([string]$entry.name) -AllowEmpty:([bool]$entry.allow)
}
foreach ($entry in @(
        @{ name = "resource_id_hash"; value = $resourceIDHash },
        @{ name = "new_input_sha256"; value = $newInputSHA256 },
        @{ name = "reason_sha256"; value = $reasonSHA256 },
        @{ name = "source_result_manifest_sha256"; value = $sourceResultManifestSHA256 }
    )) {
    Assert-Sha256Ref -Value ([string]$entry.value) -FieldName ([string]$entry.name)
}
if ($responseReasonSHA256.Length -gt 0) {
    Assert-Sha256Ref -Value $responseReasonSHA256 -FieldName "response_reason_sha256"
}

Assert-Same -Actual (Get-JsonString -Object $request -Name "proposal_id") -Expected $proposalID -FieldName "execution_request.proposal_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "approval_id") -Expected $approvalID -FieldName "execution_request.approval_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "prepared_audit_id") -Expected $preparedAuditID -FieldName "execution_request.prepared_audit_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "skill_id") -Expected $skillID -FieldName "execution_request.skill_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "tool_name") -Expected $toolName -FieldName "execution_request.tool_name"
Assert-Same -Actual (Get-JsonString -Object $request -Name "resource_type") -Expected $resourceType -FieldName "execution_request.resource_type"
Assert-Same -Actual (Get-JsonString -Object $request -Name "resource_id_hash") -Expected $resourceIDHash -FieldName "execution_request.resource_id_hash"
Assert-Same -Actual (Get-JsonString -Object $request -Name "new_input_sha256") -Expected $newInputSHA256 -FieldName "execution_request.new_input_sha256"
Assert-Same -Actual (Get-JsonString -Object $request -Name "reason_sha256") -Expected $reasonSHA256 -FieldName "execution_request.reason_sha256"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "provider_failure_id") -Expected $providerFailureID -FieldName "execution_result.provider_failure_id"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "proposal_id") -Expected $proposalID -FieldName "execution_result.proposal_id"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "approval_id") -Expected $approvalID -FieldName "execution_result.approval_id"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "prepared_audit_id") -Expected $preparedAuditID -FieldName "execution_result.prepared_audit_id"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "skill_id") -Expected $skillID -FieldName "execution_result.skill_id"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "tool_name") -Expected $toolName -FieldName "execution_result.tool_name"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "resource_type") -Expected $resourceType -FieldName "execution_result.resource_type"
Assert-Same -Actual (Get-JsonString -Object $executionResult -Name "resource_id_hash") -Expected $resourceIDHash -FieldName "execution_result.resource_id_hash"
Assert-True ([bool]$executionResult.executed) "execution_result.executed must be true."

foreach ($expected in @(
        "source_invocation_manifest_verified",
        "execution_summary_matches_invocation",
        "action_executor_reported_executed_redrive",
        "redrive_result_ref_present_or_status_recorded",
        "source_dlq_remains_immutable",
        "operator_must_append_external_audit_separately_if_needed"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $result -Name "required_checks") -Expected $expected -FieldName "result.required_checks"
}
foreach ($expected in @(
        "result_manifest_is_not_redrive_execution",
        "does_not_call_action_executor",
        "does_not_append_audit_record",
        "does_not_modify_provider_failure_or_dlq",
        "does_not_create_admin_or_workflow_decision"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $result -Name "execution_boundary") -Expected $expected -FieldName "result.execution_boundary"
}

$occurredAtUnixMs = 0
if ([string]::IsNullOrWhiteSpace($OccurredAt)) {
    $occurredAtUnixMs = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
} else {
    try {
        $occurredAtUnixMs = ([DateTimeOffset]::Parse($OccurredAt).ToUniversalTime()).ToUnixTimeMilliseconds()
    } catch {
        throw "OccurredAt must be an ISO-8601 timestamp."
    }
}
if ($occurredAtUnixMs -le 0) {
    throw "OccurredAt must resolve to a positive Unix milliseconds timestamp."
}

$outcome = $resultStatus
if ([string]::IsNullOrWhiteSpace($outcome)) {
    $outcome = "UNKNOWN"
}

$attributeValues = [ordered]@{
    approval_id = $approvalID
    downstream_service = "action-executor"
    execution_id = $redriveExecutionID
    operator_mode = "provider-replay-redrive"
    operation_id = $adminOperationID
    operation_type = "PROVIDER_REPLAY_REDRIVE"
    payload_hash = $sourceResultManifestSHA256
    payload_schema_version = "nexusim.action_executor.provider_replay_redrive_result.v1"
    prepared_audit_id = $preparedAuditID
    proposal_id = $proposalID
    reason_ref = $reasonSHA256
    result_id = $redriveResultID
    source_ref = $providerFailureID
    status = $outcome
    target_ref_hash = $resourceIDHash
}
$attributesCompact = ConvertTo-CanonicalFlatJson -Values $attributeValues
$attributesSHA256 = Get-AuditStringSha256Ref -Value $attributesCompact
$attributesObject = $attributesCompact | ConvertFrom-Json

$manifest = [ordered]@{
    schema_version = "nexusim.action_executor.external_audit_append.v1"
    manifest_id = $AuditManifestID
    source_manifest_id = $resultManifestID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    executes_append = $false
    mutates_audit_service = $false
    direct_append_allowed = $false
    requires_operator_execution = $true
    audit_stream = "security"
    source_service = "action-executor"
    source_event_id = $redriveExecutionID
    record_type = "ACTION_PROVIDER_REDRIVE"
    actor_ref = "service:action-executor"
    subject_ref = "workflow:$workflowID"
    resource_ref = "hash:$resourceIDHash"
    action = "REDRIVE_PROVIDER_FAILURE"
    outcome = $outcome
    reason_code = "PROVIDER_REPLAY_APPROVED"
    risk_level = "HIGH"
    occurred_at_unix_ms = $occurredAtUnixMs
    attributes_json = $attributesObject
    attributes_sha256 = $attributesSHA256
    idempotency_key = $AuditRecordID
    correlation_id = $adminOperationID
    causation_id = $sourceInvocationManifestID
    trace_id = $traceID
    auth_context_contract = [ordered]@{
        tenant_id = $tenantID
        trace_id = $traceID
    }
    source_result_manifest_sha256 = $sourceResultManifestSHA256
    source_result_manifest_path_sha256 = Get-AuditStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $ResultManifestPath))
    execution_boundary = @(
        "audit_manifest_is_not_audit_append_execution",
        "does_not_call_audit_service",
        "does_not_call_action_executor",
        "does_not_modify_provider_failure_or_dlq",
        "does_not_create_admin_or_workflow_decision"
    )
    required_checks = @(
        "source_execution_audit_low_sensitive",
        "no_raw_provider_artifacts",
        "audit_service_append_only",
        "idempotency_key_present",
        "provider_replay_result_manifest_verified",
        "source_result_manifest_bound_by_sha256"
    )
    forbidden_contents = @(
        "raw_provider_input",
        "raw_provider_output",
        "input_json",
        "raw_new_input",
        "raw_reason",
        "provider_body",
        "provider_error_body",
        "filesystem_path_material",
        "auth_material"
    )
    note = "Low-sensitive audit append manifest derived from a provider replay redrive result manifest. It does not append audit, execute redrive, mutate DLQ rows, or embed raw operator input."
}

$encoded = $manifest | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawDocumentText -Value $encoded -FieldName "provider replay redrive audit append manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($OutputPath, ($manifest | ConvertTo-Json -Depth 30), $utf8NoBom)

Write-Host "OK   provider replay redrive audit append manifest written: $OutputPath"
