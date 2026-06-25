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
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($ResultManifestPath))) "workflow-external-callback-batch-redrive-audit-append.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($AuditManifestID)) {
    $AuditManifestID = "workflow-callback-redrive-audit-append-" + [System.Guid]::NewGuid().ToString("N")
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
    param([string]$Path, [string]$Label)
    try {
        return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
    } catch {
        throw "$Label must be valid JSON: $Path"
    }
}

function Get-JsonString {
    param([object]$Object, [string]$Name, [switch]$AllowEmpty)
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
    param([object]$Object, [string]$Name)
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
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|reason_text|EvidencePack|prompt|local_path|filesystem_path|input_json)") {
        throw "$FieldName contains raw, secret, prompt, URL, local path, provider artifact, or credential-like content."
    }
}

function Assert-NoRawDocumentText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match '(?i)(bearer\s+\S+|password\s*[:=]|secret\s*[:=]|api[_-]?key\s*[:=]|access[_-]?key\s*[:=]|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.|https?://|postgres://|mysql://|mongodb://|"provider_body"\s*:|"provider_error_body"\s*:|"message_body"\s*:|"raw_callback_url"\s*:|"raw_payload"\s*:|"payload_body"\s*:|"input_json"\s*:|"reason_text"\s*:)') {
        throw "$FieldName contains raw, secret, prompt, provider artifact, URL, or credential-like content."
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
$result = Get-JsonDocument -Path $ResultManifestPath -Label "Workflow external callback batch redrive result manifest"

Assert-True ((Get-JsonString -Object $result -Name "schema_version") -eq "nexusim.workflow.external_callback_batch_redrive_result.v1") "Unsupported workflow external callback batch redrive result schema_version."
Assert-False ([bool]$result.manifest_is_execution) "Result manifest must not be an execution."
Assert-False ([bool]$result.records_decision) "Result manifest must not record decisions."
Assert-False ([bool]$result.calls_provider) "Result manifest must not call provider."
Assert-False ([bool]$result.executes_target) "Result manifest must not execute target."
Assert-False ([bool]$result.mutates_delivery_fact) "Result manifest must not mutate delivery fact."

$runtime = $result.runtime_contract
if ($null -eq $runtime) {
    throw "result.runtime_contract is required."
}
Assert-True ((Get-JsonString -Object $runtime -Name "service") -eq "workflow-service") "runtime_contract.service must be workflow-service."
Assert-True ((Get-JsonString -Object $runtime -Name "mode") -eq "external-callback-delivery-redrive") "runtime_contract.mode must be external-callback-delivery-redrive."
Assert-False ([bool]$runtime.result_manifest_calls_service) "Result manifest runtime contract must not call service."
Assert-False ([bool]$runtime.result_manifest_records_decision) "Result manifest runtime contract must not record decision."
Assert-False ([bool]$runtime.result_manifest_calls_provider) "Result manifest runtime contract must not call provider."
Assert-False ([bool]$runtime.result_manifest_executes_target) "Result manifest runtime contract must not execute target."

$resultManifestID = Get-JsonString -Object $result -Name "result_manifest_id"
$batchInvocationID = Get-JsonString -Object $result -Name "batch_invocation_id"
$results = @($result.results)
Assert-True ($results.Count -gt 0) "result.results is required."
Assert-True ([int]$result.expected_redrive_count -eq $results.Count) "expected_redrive_count must match result count."
Assert-True ([int]$result.execution_summary_count -eq $results.Count) "execution_summary_count must match result count."
Assert-True ([int]$result.result_count -eq $results.Count) "result_count must match result count."

foreach ($expected in @(
        "source_batch_invocation_manifest_verified",
        "one_execution_summary_per_redrive_plan",
        "execution_summary_matches_invocation_binding",
        "workflow_service_runtime_reported_executed_redrive",
        "delivery_fact_returned_to_pending",
        "redriven_outbox_event_declared",
        "result_manifest_contains_only_low_sensitive_refs"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $result -Name "required_checks") -Expected $expected -FieldName "result.required_checks"
}
foreach ($expected in @(
        "result_manifest_is_not_redrive_execution",
        "does_not_call_workflow_service",
        "does_not_record_workflow_decision",
        "does_not_call_provider",
        "does_not_execute_target_action",
        "does_not_modify_delivery_rows"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $result -Name "execution_boundary") -Expected $expected -FieldName "result.execution_boundary"
}

$redriveRefs = @()
$workflowRefs = @()
$targetServices = @{}
$targetOperations = @{}
foreach ($item in $results) {
    $tenant = Get-JsonString -Object $item -Name "tenant_id"
    Assert-True ($tenant -eq $TenantID) "result item tenant_id must match TenantID."
    Assert-True ((Get-JsonString -Object $item -Name "delivery_status") -eq "PENDING") "result delivery_status must be PENDING."
    Assert-True ((Get-JsonString -Object $item -Name "outbox_event_type") -eq "workflow.external_callback.redriven.v1") "result outbox_event_type mismatch."
    Assert-True ([int]$item.redrive_count -gt 0) "result redrive_count must be positive."
    foreach ($field in @(
            "redrive_plan_id",
            "workflow_id",
            "step_id",
            "delivery_id",
            "tenant_id",
            "delivery_status",
            "redrive_plan_sha256",
            "source_delivery_status_sha256",
            "source_delivery_plan_sha256",
            "execution_summary_sha256",
            "execution_summary_path_sha256",
            "target_service",
            "target_operation",
            "payload_ref_hash",
            "approval_policy_ref",
            "last_redrive_reason_ref",
            "outbox_event_type"
        )) {
        Assert-LowString -Value (Get-JsonString -Object $item -Name $field) -FieldName "result.$field"
    }
    $redriveRefs += ("{0}|{1}|{2}|{3}|{4}" -f `
            (Get-JsonString -Object $item -Name "redrive_plan_id"), `
            (Get-JsonString -Object $item -Name "workflow_id"), `
            (Get-JsonString -Object $item -Name "step_id"), `
            (Get-JsonString -Object $item -Name "delivery_id"), `
            (Get-JsonString -Object $item -Name "execution_summary_sha256"))
    $workflowRefs += (Get-JsonString -Object $item -Name "workflow_id")
    $targetServices[(Get-JsonString -Object $item -Name "target_service")] = $true
    $targetOperations[(Get-JsonString -Object $item -Name "target_operation")] = $true
}

foreach ($entry in @(
        @{ name = "result_manifest_id"; value = $resultManifestID },
        @{ name = "batch_invocation_id"; value = $batchInvocationID }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName ([string]$entry.name)
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

$redriveRefsSHA256 = Get-AuditStringSha256Ref -Value (($redriveRefs | Sort-Object) -join "`n")
$workflowRefsSHA256 = Get-AuditStringSha256Ref -Value ((@($workflowRefs | Sort-Object -Unique)) -join "`n")
$sourceResultManifestSHA256 = Get-AuditFileSha256Ref -Path $ResultManifestPath

$attributeValues = [ordered]@{
    batch_invocation_id = $batchInvocationID
    event_type = "WORKFLOW_EXTERNAL_CALLBACK_BATCH_REDRIVE"
    result_count = [string]$results.Count
    result_manifest_id = $resultManifestID
    redrive_refs_sha256 = $redriveRefsSHA256
    source_result_manifest_sha256 = $sourceResultManifestSHA256
    target_operation_count = [string]$targetOperations.Count
    target_service_count = [string]$targetServices.Count
    workflow_refs_sha256 = $workflowRefsSHA256
}
$attributesCompact = ConvertTo-CanonicalFlatJson -Values $attributeValues
$attributesSHA256 = Get-AuditStringSha256Ref -Value $attributesCompact
$attributesObject = $attributesCompact | ConvertFrom-Json

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
    source_event_id = $batchInvocationID
    record_type = "WORKFLOW_EXTERNAL_CALLBACK_BATCH_REDRIVE"
    actor_ref = "service:workflow-service"
    subject_ref = "tenant:$TenantID"
    resource_ref = "workflow-external-callback-batch:$batchInvocationID"
    action = "REDRIVE_EXTERNAL_CALLBACK_DELIVERIES"
    outcome = "REDRIVEN"
    reason_code = "WORKFLOW_EXTERNAL_CALLBACK_BATCH_REDRIVEN"
    risk_level = "HIGH"
    occurred_at_unix_ms = $occurredAtUnixMs
    attributes_json = $attributesObject
    attributes_sha256 = $attributesSHA256
    idempotency_key = $AuditRecordID
    correlation_id = $batchInvocationID
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
        "does_not_redrive_external_callback",
        "does_not_record_workflow_decision",
        "does_not_call_provider",
        "does_not_execute_target_action",
        "does_not_modify_delivery_rows"
    )
    required_checks = @(
        "source_external_callback_batch_redrive_result_manifest_verified",
        "workflow_service_redrive_runtime_reported",
        "delivery_fact_returned_to_pending",
        "redriven_outbox_event_declared",
        "audit_service_append_only",
        "idempotency_key_present"
    )
    forbidden_contents = @(
        "raw_callback_url",
        "callback_request_material",
        "provider_response_material",
        "decision_material",
        "payload_material",
        "evidence_text_material",
        "attributes_json",
        "filesystem_path_material",
        "auth_material"
    )
    note = "Low-sensitive audit append manifest derived from a workflow external callback batch redrive result manifest. It does not append audit, redrive callback deliveries, record decisions, call providers, execute targets, modify delivery rows, or embed raw callback/provider/payload material."
}

$encoded = $manifest | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawDocumentText -Value $encoded -FieldName "workflow external callback batch redrive audit append manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($OutputPath, ($manifest | ConvertTo-Json -Depth 40), $utf8NoBom)

Write-Host "OK   workflow external callback batch redrive audit append manifest written: $OutputPath"
