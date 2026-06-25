param(
    [Parameter(Mandatory = $true)]
    [string]$DecisionManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [Parameter(Mandatory = $true)]
    [string]$PreparedBy,

    [Parameter(Mandatory = $true)]
    [string]$CallbackProviderRef,

    [Parameter(Mandatory = $true)]
    [string]$CallbackEndpointRef,

    [Parameter(Mandatory = $true)]
    [string]$DeliveryQueueRef,

    [Parameter(Mandatory = $true)]
    [string]$RetryPolicyRef,

    [Parameter(Mandatory = $true)]
    [string]$BackoffPolicyRef,

    [Parameter(Mandatory = $true)]
    [string]$CallbackTimeoutPolicyRef,

    [int]$MaxAttempts = 3,
    [string]$DeliveryPlanID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

Assert-ExternalRepairOutputPath -Value $DecisionManifestPath -FieldName "DecisionManifestPath"
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-LowSensitiveRepairActor -Value $PreparedBy -FieldName "PreparedBy"

if ([string]::IsNullOrWhiteSpace($DeliveryPlanID)) {
    $DeliveryPlanID = "workflow-external-callback-delivery-plan-" + [System.Guid]::NewGuid().ToString("N")
}

foreach ($pair in @(
        @("DeliveryPlanID", $DeliveryPlanID),
        @("CallbackProviderRef", $CallbackProviderRef),
        @("CallbackEndpointRef", $CallbackEndpointRef),
        @("DeliveryQueueRef", $DeliveryQueueRef),
        @("RetryPolicyRef", $RetryPolicyRef),
        @("BackoffPolicyRef", $BackoffPolicyRef),
        @("CallbackTimeoutPolicyRef", $CallbackTimeoutPolicyRef)
    )) {
    Assert-LowSensitiveRepairIdentifier -Value ([string]$pair[1]) -FieldName ([string]$pair[0])
}

if ($MaxAttempts -lt 1 -or $MaxAttempts -gt 10) {
    throw "MaxAttempts must be between 1 and 10."
}

function Get-CallbackFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackStringSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
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

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
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

function Assert-NoRawCallbackText {
    param(
        [string]$Value,
        [string]$FieldName
    )

    $match = [regex]::Match($Value, "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|EvidencePack|prompt)")
    if ($match.Success) {
        throw "$FieldName contains raw, secret, provider artifact, prompt, URL, or credential-like content."
    }
}

if (-not (Test-Path -LiteralPath $DecisionManifestPath -PathType Leaf)) {
    throw "Missing workflow external decision manifest template: $DecisionManifestPath"
}

try {
    $manifestRaw = Get-Content -LiteralPath $DecisionManifestPath -Raw
    $manifest = $manifestRaw | ConvertFrom-Json
} catch {
    throw "DecisionManifestPath must be valid JSON: $DecisionManifestPath"
}

$schemaVersion = Get-JsonString -Object $manifest -Name "schema_version"
Assert-True ($schemaVersion -eq "nexusim.workflow.external_decision_manifest.v1") "Unsupported decision manifest schema_version: $schemaVersion"

$workflowID = Get-JsonString -Object $manifest -Name "workflow_id"
$stepID = Get-JsonString -Object $manifest -Name "step_id"
$expectedWorkflowType = (Get-JsonString -Object $manifest -Name "expected_workflow_type").ToUpperInvariant()
$expectedStatus = (Get-JsonString -Object $manifest -Name "expected_status").ToUpperInvariant()
$expectedTargetService = Get-JsonString -Object $manifest -Name "expected_target_service"
$expectedTargetOperation = Get-JsonString -Object $manifest -Name "expected_target_operation"
$expectedTargetRefHash = Get-JsonString -Object $manifest -Name "expected_target_ref_hash"
$expectedPayloadSchemaVersion = Get-JsonString -Object $manifest -Name "expected_payload_schema_version"
$expectedPayloadRefHash = Get-JsonString -Object $manifest -Name "expected_payload_ref_hash"
$expectedApprovalPolicyRef = Get-JsonString -Object $manifest -Name "expected_approval_policy_ref"
$decision = (Get-JsonString -Object $manifest -Name "decision" -AllowEmpty).ToUpperInvariant()
$deciderRef = Get-JsonString -Object $manifest -Name "decider_ref" -AllowEmpty
$decisionPolicyRef = Get-JsonString -Object $manifest -Name "decision_policy_ref"
$idempotencyKey = Get-JsonString -Object $manifest -Name "idempotency_key" -AllowEmpty
$correlationID = Get-JsonString -Object $manifest -Name "correlation_id" -AllowEmpty
$causationID = Get-JsonString -Object $manifest -Name "causation_id" -AllowEmpty
$traceID = Get-JsonString -Object $manifest -Name "trace_id" -AllowEmpty

Assert-True ($expectedStatus -eq "WAITING_DECISION") "Decision manifest expected_status must be WAITING_DECISION."
Assert-True ($decision.Length -eq 0) "Decision manifest template must not already contain a decision."
Assert-True ($deciderRef.Length -eq 0) "Decision manifest template must not already contain a decider_ref."

foreach ($entry in @(
        @{ name = "workflow_id"; value = $workflowID; allow = $false },
        @{ name = "step_id"; value = $stepID; allow = $false },
        @{ name = "expected_workflow_type"; value = $expectedWorkflowType; allow = $false },
        @{ name = "expected_status"; value = $expectedStatus; allow = $false },
        @{ name = "expected_target_service"; value = $expectedTargetService; allow = $false },
        @{ name = "expected_target_operation"; value = $expectedTargetOperation; allow = $false },
        @{ name = "expected_target_ref_hash"; value = $expectedTargetRefHash; allow = $false },
        @{ name = "expected_payload_schema_version"; value = $expectedPayloadSchemaVersion; allow = $false },
        @{ name = "expected_payload_ref_hash"; value = $expectedPayloadRefHash; allow = $false },
        @{ name = "expected_approval_policy_ref"; value = $expectedApprovalPolicyRef; allow = $false },
        @{ name = "decision_policy_ref"; value = $decisionPolicyRef; allow = $false },
        @{ name = "idempotency_key"; value = $idempotencyKey; allow = $true },
        @{ name = "correlation_id"; value = $correlationID; allow = $true },
        @{ name = "causation_id"; value = $causationID; allow = $true },
        @{ name = "trace_id"; value = $traceID; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName "decision_manifest.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
    Assert-NoRawCallbackText -Value ([string]$entry.value) -FieldName "decision_manifest.$($entry.name)"
}

foreach ($field in @("reason_ref", "evidence_refs")) {
    if ($null -ne $manifest.PSObject.Properties[$field]) {
        foreach ($value in @($manifest.$field)) {
            Assert-LowString -Value ([string]$value) -FieldName "decision_manifest.$field" -AllowEmpty
            Assert-NoRawCallbackText -Value ([string]$value) -FieldName "decision_manifest.$field"
        }
    }
}

$resolvedManifestPath = [string](Resolve-Path -LiteralPath $DecisionManifestPath)
$plan = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_delivery_plan.v1"
    delivery_plan_id = $DeliveryPlanID
    generated_at = [DateTime]::UtcNow.ToString("o")
    prepared_by = $PreparedBy
    source_decision_manifest_sha256 = Get-CallbackFileSha256Ref -Path $DecisionManifestPath
    source_decision_manifest_path_sha256 = Get-CallbackStringSha256Ref -Value $resolvedManifestPath
    workflow_binding = [ordered]@{
        workflow_id = $workflowID
        step_id = $stepID
        expected_workflow_type = $expectedWorkflowType
        expected_status = $expectedStatus
        expected_target_service = $expectedTargetService
        expected_target_operation = $expectedTargetOperation
        expected_target_ref_hash = $expectedTargetRefHash
        expected_payload_schema_version = $expectedPayloadSchemaVersion
        expected_payload_ref_hash = $expectedPayloadRefHash
        expected_approval_policy_ref = $expectedApprovalPolicyRef
        decision_policy_ref = $decisionPolicyRef
    }
    callback_delivery_contract = [ordered]@{
        owner = "workflow-service.external-callback-delivery"
        callback_provider_ref = $CallbackProviderRef
        callback_endpoint_ref = $CallbackEndpointRef
        delivery_queue_ref = $DeliveryQueueRef
        endpoint_ref_only = $true
        raw_callback_url_allowed = $false
        delivery_plan_calls_provider = $false
        delivery_plan_records_decision = $false
        delivery_plan_executes_target = $false
        callback_payload_schema_version = "nexusim.workflow.external_decision_manifest.v1"
        callback_payload_ref_hash = Get-CallbackStringSha256Ref -Value $manifestRaw
    }
    retry_contract = [ordered]@{
        retry_policy_ref = $RetryPolicyRef
        max_attempts = $MaxAttempts
        backoff_policy_ref = $BackoffPolicyRef
        callback_timeout_policy_ref = $CallbackTimeoutPolicyRef
        requires_explicit_redrive = $true
        no_silent_drop = $true
        retry_exhaustion_requires_operator_review = $true
    }
    final_decision_contract = [ordered]@{
        required_entrypoint = "loadtest/workflow record-decision -decision-manifest"
        final_decision_owner = "workflow-service.RecordWorkflowDecision"
        requires_binding_check = $true
        requires_explicit_decision = $true
        delivery_plan_is_not_decision = $true
        delivery_plan_is_not_final_execution = $true
    }
    preflight_checks = @(
        "external_decision_manifest_template_verified",
        "workflow_waiting_decision_verified",
        "decision_and_decider_are_empty",
        "target_payload_policy_binding_verified",
        "endpoint_is_ref_not_raw_url",
        "retry_policy_is_explicit",
        "no_raw_payload_or_provider_artifact"
    )
    approval_boundary = @(
        "delivery_plan_does_not_record_decision",
        "delivery_plan_does_not_create_or_reuse_approval",
        "external_system_must_return_explicit_decision_manifest",
        "record_decision_must_revalidate_workflow_binding"
    )
    execution_boundary = @(
        "delivery_plan_is_not_execution",
        "does_not_call_target_service",
        "does_not_call_action_executor",
        "does_not_call_compensation_executor",
        "workflow_service_records_decision_only_after_binding_check",
        "target_execution_remains_with_approved_downstream_owner"
    )
    no_direct_execution = $true
    no_decision_recorded = $true
    does_not_call_provider = $true
    does_not_execute_target = $true
    forbidden_contents = @(
        "raw_callback_url",
        "callback_request_material",
        "provider_response_material",
        "decision_material",
        "payload_material",
        "evidence_text_material",
        "local_path_material",
        "auth_material"
    )
    note = "Low-sensitive external callback delivery plan. It binds a WAITING_DECISION workflow decision template to endpoint and retry refs only; it does not call the provider, record a decision, or execute the target action."
}

$encoded = $plan | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawCallbackText -Value $encoded -FieldName "external callback delivery plan"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$plan | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback delivery plan written: $OutputPath"
