param(
    [Parameter(Mandatory = $true)]
    [string]$ResultManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [Parameter(Mandatory = $true)]
    [string]$TenantID,

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
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($ResultManifestPath))) "workflow-compensation-execution-audit-append.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($AuditManifestID)) {
    $AuditManifestID = "workflow-compensation-audit-append-" + [System.Guid]::NewGuid().ToString("N")
}
if ([string]::IsNullOrWhiteSpace($AuditRecordID)) {
    $AuditRecordID = "workflow-service:audit:" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $TenantID -FieldName "TenantID"
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
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-False {
    param([bool]$Condition, [string]$Message)
    if ($Condition) {
        throw $Message
    }
}

function Assert-NoRawText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|reason_text|EvidencePack|prompt|local_path|filesystem_path|input_json)") {
        throw "$FieldName contains raw, secret, prompt, local path, provider artifact, or credential-like content."
    }
}

function Assert-NoRawDocumentText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match '(?i)(bearer\s+\S+|password\s*[:=]|secret\s*[:=]|api[_-]?key\s*[:=]|access[_-]?key\s*[:=]|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.|postgres://|mysql://|mongodb://|"provider_body"\s*:|"provider_error_body"\s*:|"message_body"\s*:|"raw_payload"\s*:|"payload_body"\s*:|"input_json"\s*:|"reason_text"\s*:)') {
        throw "$FieldName contains raw, secret, prompt, provider artifact, or credential-like content."
    }
}

function Assert-LowString {
    param([string]$Value, [string]$FieldName, [switch]$AllowEmpty)
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawText -Value $Value -FieldName $FieldName
}

function Assert-ArrayContains {
    param([object[]]$Values, [string]$Expected, [string]$FieldName)
    if (@($Values) -notcontains $Expected) {
        throw "$FieldName must contain $Expected."
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
$result = Get-JsonDocument -Path $ResultManifestPath -Label "Workflow compensation execution result manifest"

Assert-True ((Get-JsonString -Object $result -Name "schema_version") -eq "nexusim.workflow.compensation_execution_result.v1") "Unsupported workflow compensation execution result schema_version."
Assert-False ([bool]$result.manifest_is_execution) "Result manifest must not be an execution."
Assert-False ([bool]$result.executes_compensation) "Result manifest must not claim it executes compensation."
Assert-False ([bool]$result.records_decision) "Result manifest must not claim it records decisions."
Assert-False ([bool]$result.calls_downstream_service) "Result manifest must not call downstream service."

$workflow = $result.workflow
$compensation = $result.compensation_result
if ($null -eq $workflow) {
    throw "result.workflow is required."
}
if ($null -eq $compensation) {
    throw "result.compensation_result is required."
}

$resultManifestID = Get-JsonString -Object $result -Name "result_manifest_id"
$workflowID = Get-JsonString -Object $workflow -Name "workflow_id"
$workflowType = Get-JsonString -Object $workflow -Name "workflow_type"
$targetService = Get-JsonString -Object $workflow -Name "target_service"
$targetOperation = Get-JsonString -Object $workflow -Name "target_operation"
$targetRefHash = Get-JsonString -Object $workflow -Name "target_ref_hash" -AllowEmpty
$payloadSchemaVersion = Get-JsonString -Object $workflow -Name "payload_schema_version" -AllowEmpty
$payloadRefHash = Get-JsonString -Object $workflow -Name "payload_ref_hash"
$approvalPolicyRef = Get-JsonString -Object $workflow -Name "approval_policy_ref" -AllowEmpty
$compensationPolicyRef = Get-JsonString -Object $workflow -Name "compensation_policy_ref" -AllowEmpty

$compensationID = Get-JsonString -Object $compensation -Name "compensation_id"
$sourceStepID = Get-JsonString -Object $compensation -Name "source_step_id" -AllowEmpty
$status = Get-JsonString -Object $compensation -Name "status"
$downstreamService = Get-JsonString -Object $compensation -Name "downstream_service" -AllowEmpty
$downstreamRequestRef = Get-JsonString -Object $compensation -Name "downstream_request_ref" -AllowEmpty
$failureClass = Get-JsonString -Object $compensation -Name "failure_class" -AllowEmpty
$publicError = Get-JsonString -Object $compensation -Name "public_error" -AllowEmpty

Assert-True ($workflowType -eq "COMPENSATION_REQUEST") "workflow.workflow_type must be COMPENSATION_REQUEST."
Assert-True (($status -eq "SUCCEEDED" -or $status -eq "FAILED")) "compensation_result.status must be terminal."

foreach ($entry in @(
        @{ name = "result_manifest_id"; value = $resultManifestID; allow = $false },
        @{ name = "workflow_id"; value = $workflowID; allow = $false },
        @{ name = "workflow_type"; value = $workflowType; allow = $false },
        @{ name = "target_service"; value = $targetService; allow = $false },
        @{ name = "target_operation"; value = $targetOperation; allow = $false },
        @{ name = "target_ref_hash"; value = $targetRefHash; allow = $true },
        @{ name = "payload_schema_version"; value = $payloadSchemaVersion; allow = $true },
        @{ name = "payload_ref_hash"; value = $payloadRefHash; allow = $false },
        @{ name = "approval_policy_ref"; value = $approvalPolicyRef; allow = $true },
        @{ name = "compensation_policy_ref"; value = $compensationPolicyRef; allow = $true },
        @{ name = "compensation_id"; value = $compensationID; allow = $false },
        @{ name = "source_step_id"; value = $sourceStepID; allow = $true },
        @{ name = "status"; value = $status; allow = $false },
        @{ name = "downstream_service"; value = $downstreamService; allow = $true },
        @{ name = "downstream_request_ref"; value = $downstreamRequestRef; allow = $true },
        @{ name = "failure_class"; value = $failureClass; allow = $true },
        @{ name = "public_error"; value = $publicError; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName ([string]$entry.name) -AllowEmpty:([bool]$entry.allow)
}

foreach ($expected in @(
        "source_invocation_manifest_verified",
        "list_compensations_summary_from_workflow_service_public_api",
        "compensation_row_matches_invocation_workflow_payload_target",
        "compensation_status_is_terminal",
        "result_manifest_contains_only_low_sensitive_refs"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $result -Name "required_checks") -Expected $expected -FieldName "result.required_checks"
}
foreach ($expected in @(
        "result_manifest_is_not_execution",
        "does_not_call_downstream_service",
        "does_not_record_workflow_decision",
        "does_not_modify_workflow_or_compensation_rows",
        "workflow_service_compensation_executor_remains_final_execution_owner"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $result -Name "execution_boundary") -Expected $expected -FieldName "result.execution_boundary"
}

$occurredAtUnixMs = 0
if ([string]::IsNullOrWhiteSpace($OccurredAt)) {
    foreach ($field in @("completed_at_unix_ms", "updated_at_unix_ms", "created_at_unix_ms")) {
        $candidate = [int64]((Get-JsonString -Object $compensation -Name $field -AllowEmpty) -as [int64])
        if ($candidate -gt 0) {
            $occurredAtUnixMs = $candidate
            break
        }
    }
    if ($occurredAtUnixMs -le 0) {
        $occurredAtUnixMs = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    }
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

$reasonCode = "WORKFLOW_COMPENSATION_EXECUTED"
if ($status -eq "FAILED") {
    $reasonCode = "WORKFLOW_COMPENSATION_FAILED"
}

$attributeValues = [ordered]@{
    downstream_request_ref = $downstreamRequestRef
    downstream_service = $downstreamService
    event_type = "WORKFLOW_COMPENSATION_EXECUTION"
    failure_class = $failureClass
    operation_id = $workflowID
    operation_type = $targetOperation
    payload_hash = $payloadRefHash
    payload_schema_version = $payloadSchemaVersion
    policy_decision_id = $approvalPolicyRef
    reason_ref = $compensationPolicyRef
    source_ref = $compensationID
    status = $status
    target_ref_hash = $targetRefHash
}
$attributesCompact = ConvertTo-CanonicalFlatJson -Values $attributeValues
$attributesSHA256 = Get-AuditStringSha256Ref -Value $attributesCompact
$attributesObject = $attributesCompact | ConvertFrom-Json
$sourceResultManifestSHA256 = Get-AuditFileSha256Ref -Path $ResultManifestPath

$manifest = [ordered]@{
    schema_version = "nexusim.audit.external_append.v1"
    manifest_id = $AuditManifestID
    source_manifest_id = $resultManifestID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    executes_append = $false
    mutates_audit_service = $false
    direct_append_allowed = $false
    requires_operator_execution = $true
    audit_stream = "security"
    source_service = "workflow-service"
    source_event_id = $compensationID
    record_type = "WORKFLOW_COMPENSATION_EXECUTION"
    actor_ref = "service:workflow-service"
    subject_ref = "workflow:$workflowID"
    resource_ref = "workflow:$workflowID`:compensation:$compensationID"
    action = "EXECUTE_COMPENSATION"
    outcome = $status
    reason_code = $reasonCode
    risk_level = "HIGH"
    occurred_at_unix_ms = $occurredAtUnixMs
    attributes_json = $attributesObject
    attributes_sha256 = $attributesSHA256
    idempotency_key = $AuditRecordID
    correlation_id = $workflowID
    causation_id = $resultManifestID
    trace_id = ""
    auth_context_contract = [ordered]@{
        tenant_id = $TenantID
        trace_id = ""
    }
    source_result_manifest_sha256 = $sourceResultManifestSHA256
    source_result_manifest_path_sha256 = Get-AuditStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $ResultManifestPath))
    execution_boundary = @(
        "audit_manifest_is_not_audit_append_execution",
        "does_not_call_audit_service",
        "does_not_execute_compensation",
        "does_not_record_workflow_decision",
        "does_not_modify_workflow_or_compensation_rows",
        "does_not_call_downstream_service"
    )
    required_checks = @(
        "source_compensation_result_manifest_verified",
        "workflow_compensation_result_low_sensitive",
        "no_raw_compensation_payload",
        "compensation_status_terminal",
        "audit_service_append_only",
        "idempotency_key_present"
    )
    forbidden_contents = @(
        "raw_payload",
        "payload_body",
        "provider_body",
        "provider_error_body",
        "reason_text",
        "EvidencePack",
        "input_json",
        "filesystem_path_material",
        "auth_material"
    )
    note = "Low-sensitive audit append manifest derived from a workflow compensation execution result manifest. It does not append audit, execute compensation, record workflow decisions, call downstream services, or embed raw payloads."
}

$encoded = $manifest | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawDocumentText -Value $encoded -FieldName "workflow compensation audit append manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($OutputPath, ($manifest | ConvertTo-Json -Depth 30), $utf8NoBom)

Write-Host "OK   workflow compensation execution audit append manifest written: $OutputPath"
