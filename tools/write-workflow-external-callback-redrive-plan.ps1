param(
    [Parameter(Mandatory = $true)]
    [string]$DeliveryStatusPath,

    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [Parameter(Mandatory = $true)]
    [string]$PreparedBy,

    [Parameter(Mandatory = $true)]
    [string]$RedriveQueueRef,

    [Parameter(Mandatory = $true)]
    [string]$RedriveReasonRef,

    [string]$RedrivePlanID = "",
    [string]$OperatorReviewRef = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

Assert-ExternalRepairOutputPath -Value $DeliveryStatusPath -FieldName "DeliveryStatusPath"
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-LowSensitiveRepairActor -Value $PreparedBy -FieldName "PreparedBy"

if ([string]::IsNullOrWhiteSpace($RedrivePlanID)) {
    $RedrivePlanID = "workflow-external-callback-redrive-plan-" + [System.Guid]::NewGuid().ToString("N")
}

foreach ($pair in @(
        @("RedrivePlanID", $RedrivePlanID),
        @("RedriveQueueRef", $RedriveQueueRef),
        @("RedriveReasonRef", $RedriveReasonRef),
        @("OperatorReviewRef", $OperatorReviewRef)
    )) {
    Assert-LowSensitiveRepairIdentifier -Value ([string]$pair[1]) -FieldName ([string]$pair[0]) -AllowEmpty:(([string]$pair[0]) -eq "OperatorReviewRef")
}

function Get-CallbackRedriveFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackRedriveStringSha256Ref {
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

    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|EvidencePack|prompt)") {
        throw "$FieldName contains raw, secret, provider artifact, prompt, URL, or credential-like content."
    }
}

if (-not (Test-Path -LiteralPath $DeliveryStatusPath -PathType Leaf)) {
    throw "Missing workflow external callback delivery status: $DeliveryStatusPath"
}

try {
    $statusRaw = Get-Content -LiteralPath $DeliveryStatusPath -Raw
    $status = $statusRaw | ConvertFrom-Json
} catch {
    throw "DeliveryStatusPath must be valid JSON: $DeliveryStatusPath"
}

$schemaVersion = Get-JsonString -Object $status -Name "schema_version"
Assert-True ($schemaVersion -eq "nexusim.workflow.external_callback_delivery_status.v1") "Unsupported delivery status schema_version: $schemaVersion"
Assert-True ([bool]$status.no_direct_execution) "Delivery status must set no_direct_execution=true."
Assert-True ([bool]$status.no_decision_recorded) "Delivery status must set no_decision_recorded=true."
Assert-True ([bool]$status.does_not_call_provider) "Delivery status must set does_not_call_provider=true."
Assert-True ([bool]$status.does_not_execute_target) "Delivery status must set does_not_execute_target=true."

$deliveryStatus = (Get-JsonString -Object $status -Name "delivery_status").ToUpperInvariant()
if (@("RETRY_PENDING", "DLQ") -notcontains $deliveryStatus) {
    throw "Delivery status must be RETRY_PENDING or DLQ for redrive planning."
}

$workflow = $status.workflow_binding
if ($null -eq $workflow) {
    throw "workflow_binding is required."
}

$workflowID = Get-JsonString -Object $workflow -Name "workflow_id"
$stepID = Get-JsonString -Object $workflow -Name "step_id"
$expectedWorkflowType = Get-JsonString -Object $workflow -Name "expected_workflow_type"
$expectedStatus = Get-JsonString -Object $workflow -Name "expected_status"
$expectedTargetService = Get-JsonString -Object $workflow -Name "expected_target_service"
$expectedTargetOperation = Get-JsonString -Object $workflow -Name "expected_target_operation"
$expectedTargetRefHash = Get-JsonString -Object $workflow -Name "expected_target_ref_hash"
$expectedPayloadSchemaVersion = Get-JsonString -Object $workflow -Name "expected_payload_schema_version"
$expectedPayloadRefHash = Get-JsonString -Object $workflow -Name "expected_payload_ref_hash"
$expectedApprovalPolicyRef = Get-JsonString -Object $workflow -Name "expected_approval_policy_ref"
$decisionPolicyRef = Get-JsonString -Object $workflow -Name "decision_policy_ref"

foreach ($entry in @(
        @{ name = "workflow_id"; value = $workflowID },
        @{ name = "step_id"; value = $stepID },
        @{ name = "expected_workflow_type"; value = $expectedWorkflowType },
        @{ name = "expected_status"; value = $expectedStatus },
        @{ name = "expected_target_service"; value = $expectedTargetService },
        @{ name = "expected_target_operation"; value = $expectedTargetOperation },
        @{ name = "expected_target_ref_hash"; value = $expectedTargetRefHash },
        @{ name = "expected_payload_schema_version"; value = $expectedPayloadSchemaVersion },
        @{ name = "expected_payload_ref_hash"; value = $expectedPayloadRefHash },
        @{ name = "expected_approval_policy_ref"; value = $expectedApprovalPolicyRef },
        @{ name = "decision_policy_ref"; value = $decisionPolicyRef }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName "workflow_binding.$($entry.name)"
    Assert-NoRawCallbackText -Value ([string]$entry.value) -FieldName "workflow_binding.$($entry.name)"
}
Assert-True ($expectedStatus -eq "WAITING_DECISION") "workflow_binding.expected_status must be WAITING_DECISION."

$attemptNumber = [int]$status.attempt_number
$maxAttempts = [int]$status.max_attempts
$deliveryAttemptRef = Get-JsonString -Object $status -Name "delivery_attempt_ref"
$failureClassRef = Get-JsonString -Object $status -Name "failure_class_ref"
$redrivePolicyRef = Get-JsonString -Object $status -Name "redrive_policy_ref"

foreach ($entry in @(
        @{ name = "delivery_attempt_ref"; value = $deliveryAttemptRef },
        @{ name = "failure_class_ref"; value = $failureClassRef },
        @{ name = "redrive_policy_ref"; value = $redrivePolicyRef }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName $entry.name
    Assert-NoRawCallbackText -Value ([string]$entry.value) -FieldName $entry.name
}
Assert-True ($attemptNumber -ge 1 -and $attemptNumber -le $maxAttempts) "delivery status attempt_number must be within max_attempts."

$statusContract = $status.status_contract
Assert-True ($null -ne $statusContract) "status_contract is required."
Assert-True ([bool]$statusContract.delivered_status_is_not_decision) "status_contract.delivered_status_is_not_decision must be true."

$resolvedStatusPath = [string](Resolve-Path -LiteralPath $DeliveryStatusPath)
$plan = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_redrive_plan.v1"
    redrive_plan_id = $RedrivePlanID
    generated_at = [DateTime]::UtcNow.ToString("o")
    prepared_by = $PreparedBy
    source_delivery_status_sha256 = Get-CallbackRedriveFileSha256Ref -Path $DeliveryStatusPath
    source_delivery_status_path_sha256 = Get-CallbackRedriveStringSha256Ref -Value $resolvedStatusPath
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
    redrive_source = [ordered]@{
        delivery_status = $deliveryStatus
        attempt_number = $attemptNumber
        max_attempts = $maxAttempts
        delivery_attempt_ref = $deliveryAttemptRef
        failure_class_ref = $failureClassRef
        redrive_policy_ref = $redrivePolicyRef
    }
    redrive_contract = [ordered]@{
        owner = "workflow-service.external-callback-delivery"
        redrive_queue_ref = $RedriveQueueRef
        redrive_reason_ref = $RedriveReasonRef
        operator_review_ref = $OperatorReviewRef
        redrive_plan_calls_provider = $false
        redrive_plan_records_decision = $false
        redrive_plan_executes_target = $false
        requires_new_delivery_attempt_ref = $true
        requires_existing_waiting_workflow = $true
        final_decision_entrypoint = "loadtest/workflow record-decision -decision-manifest"
        final_decision_owner = "workflow-service.RecordWorkflowDecision"
    }
    preflight_checks = @(
        "delivery_status_verified",
        "workflow_waiting_decision_verified",
        "failure_class_present",
        "redrive_policy_ref_verified",
        "redrive_queue_ref_verified",
        "no_raw_provider_or_callback_material"
    )
    approval_boundary = @(
        "redrive_plan_does_not_record_decision",
        "redrive_plan_does_not_create_or_reuse_approval",
        "external_system_must_return_explicit_decision_manifest",
        "record_decision_must_revalidate_workflow_binding"
    )
    execution_boundary = @(
        "redrive_plan_is_not_provider_call",
        "does_not_call_target_service",
        "does_not_call_action_executor",
        "does_not_call_compensation_executor",
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
    note = "Low-sensitive external callback redrive plan. It can requeue delivery work for a still-waiting workflow but does not call a provider, record a decision, or execute a target action."
}

$encoded = $plan | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawCallbackText -Value $encoded -FieldName "external callback redrive plan"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$plan | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback redrive plan written: $OutputPath"
