param(
    [Parameter(Mandatory = $true)]
    [string]$DeliveryPlanPath,

    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [Parameter(Mandatory = $true)]
    [string]$ReportedBy,

    [Parameter(Mandatory = $true)]
    [ValidateSet("DELIVERED", "RETRY_PENDING", "DLQ")]
    [string]$DeliveryStatus,

    [Parameter(Mandatory = $true)]
    [int]$AttemptNumber,

    [Parameter(Mandatory = $true)]
    [string]$DeliveryAttemptRef,

    [string]$DeliveryResultRef = "",
    [string]$FailureClassRef = "",
    [string]$NextRetryRef = "",
    [string]$RedrivePolicyRef = "workflow.external-callback-redrive.v1",
    [string]$StatusID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

Assert-ExternalRepairOutputPath -Value $DeliveryPlanPath -FieldName "DeliveryPlanPath"
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-LowSensitiveRepairActor -Value $ReportedBy -FieldName "ReportedBy"

if ([string]::IsNullOrWhiteSpace($StatusID)) {
    $StatusID = "workflow-external-callback-delivery-status-" + [System.Guid]::NewGuid().ToString("N")
}

foreach ($pair in @(
        @("StatusID", $StatusID),
        @("DeliveryAttemptRef", $DeliveryAttemptRef),
        @("DeliveryResultRef", $DeliveryResultRef),
        @("FailureClassRef", $FailureClassRef),
        @("NextRetryRef", $NextRetryRef),
        @("RedrivePolicyRef", $RedrivePolicyRef)
    )) {
    Assert-LowSensitiveRepairIdentifier -Value ([string]$pair[1]) -FieldName ([string]$pair[0]) -AllowEmpty:(([string]$pair[0]) -in @("DeliveryResultRef", "FailureClassRef", "NextRetryRef"))
}

function Get-CallbackStatusFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackStatusStringSha256Ref {
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

if (-not (Test-Path -LiteralPath $DeliveryPlanPath -PathType Leaf)) {
    throw "Missing workflow external callback delivery plan: $DeliveryPlanPath"
}

try {
    $planRaw = Get-Content -LiteralPath $DeliveryPlanPath -Raw
    $plan = $planRaw | ConvertFrom-Json
} catch {
    throw "DeliveryPlanPath must be valid JSON: $DeliveryPlanPath"
}

$schemaVersion = Get-JsonString -Object $plan -Name "schema_version"
Assert-True ($schemaVersion -eq "nexusim.workflow.external_callback_delivery_plan.v1") "Unsupported delivery plan schema_version: $schemaVersion"
Assert-True ([bool]$plan.no_direct_execution) "Delivery plan must set no_direct_execution=true."
Assert-True ([bool]$plan.no_decision_recorded) "Delivery plan must set no_decision_recorded=true."
Assert-True ([bool]$plan.does_not_call_provider) "Delivery plan must set does_not_call_provider=true."
Assert-True ([bool]$plan.does_not_execute_target) "Delivery plan must set does_not_execute_target=true."

$workflow = $plan.workflow_binding
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

$retry = $plan.retry_contract
if ($null -eq $retry) {
    throw "retry_contract is required."
}
$maxAttempts = [int]$retry.max_attempts
Assert-True ($maxAttempts -ge 1 -and $maxAttempts -le 10) "retry_contract.max_attempts must be between 1 and 10."
Assert-True ($AttemptNumber -ge 1 -and $AttemptNumber -le $maxAttempts) "AttemptNumber must be between 1 and retry_contract.max_attempts."

$statusValue = $DeliveryStatus.Trim().ToUpperInvariant()
switch ($statusValue) {
    "DELIVERED" {
        Assert-True (-not [string]::IsNullOrWhiteSpace($DeliveryResultRef)) "DELIVERED status requires DeliveryResultRef."
        Assert-True ([string]::IsNullOrWhiteSpace($FailureClassRef)) "DELIVERED status must not include FailureClassRef."
        Assert-True ([string]::IsNullOrWhiteSpace($NextRetryRef)) "DELIVERED status must not include NextRetryRef."
    }
    "RETRY_PENDING" {
        Assert-True (-not [string]::IsNullOrWhiteSpace($FailureClassRef)) "RETRY_PENDING status requires FailureClassRef."
        Assert-True (-not [string]::IsNullOrWhiteSpace($NextRetryRef)) "RETRY_PENDING status requires NextRetryRef."
        Assert-True ($AttemptNumber -lt $maxAttempts) "RETRY_PENDING requires AttemptNumber below max_attempts."
    }
    "DLQ" {
        Assert-True (-not [string]::IsNullOrWhiteSpace($FailureClassRef)) "DLQ status requires FailureClassRef."
        Assert-True ($AttemptNumber -ge $maxAttempts) "DLQ requires AttemptNumber to reach max_attempts."
    }
}

$resolvedPlanPath = [string](Resolve-Path -LiteralPath $DeliveryPlanPath)
$status = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_delivery_status.v1"
    status_id = $StatusID
    generated_at = [DateTime]::UtcNow.ToString("o")
    reported_by = $ReportedBy
    source_delivery_plan_sha256 = Get-CallbackStatusFileSha256Ref -Path $DeliveryPlanPath
    source_delivery_plan_path_sha256 = Get-CallbackStatusStringSha256Ref -Value $resolvedPlanPath
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
    delivery_status = $statusValue
    attempt_number = $AttemptNumber
    max_attempts = $maxAttempts
    delivery_attempt_ref = $DeliveryAttemptRef
    delivery_result_ref = $DeliveryResultRef
    failure_class_ref = $FailureClassRef
    next_retry_ref = $NextRetryRef
    redrive_policy_ref = $RedrivePolicyRef
    status_contract = [ordered]@{
        owner = "workflow-service.external-callback-delivery"
        status_writer_calls_provider = $false
        status_writer_records_decision = $false
        status_writer_executes_target = $false
        delivered_status_is_not_decision = $true
        final_decision_entrypoint = "loadtest/workflow record-decision -decision-manifest"
        final_decision_owner = "workflow-service.RecordWorkflowDecision"
    }
    retry_boundary = [ordered]@{
        retry_policy_ref = Get-JsonString -Object $retry -Name "retry_policy_ref"
        backoff_policy_ref = Get-JsonString -Object $retry -Name "backoff_policy_ref"
        callback_timeout_policy_ref = Get-JsonString -Object $retry -Name "callback_timeout_policy_ref"
        requires_explicit_redrive = [bool]$retry.requires_explicit_redrive
        retry_exhaustion_requires_operator_review = [bool]$retry.retry_exhaustion_requires_operator_review
    }
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
    note = "Low-sensitive external callback delivery status. It records delivery attempt metadata only; it does not call a provider, record a workflow decision, or execute a target action."
}

$encoded = $status | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawCallbackText -Value $encoded -FieldName "external callback delivery status"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$status | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback delivery status written: $OutputPath"
