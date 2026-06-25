param(
    [Parameter(Mandatory = $true)]
    [string]$RedrivePlanRootPath,

    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [Parameter(Mandatory = $true)]
    [string]$PreparedBy,

    [string]$DashboardPath = "",
    [string]$InvocationID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $RedrivePlanRootPath -PathType Container)) {
    throw "Missing workflow external callback redrive plan root: $RedrivePlanRootPath"
}
Assert-ExternalRepairOutputPath -Value $RedrivePlanRootPath -FieldName "RedrivePlanRootPath"
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-LowSensitiveRepairActor -Value $PreparedBy -FieldName "PreparedBy"

if (-not [string]::IsNullOrWhiteSpace($DashboardPath)) {
    if (-not (Test-Path -LiteralPath $DashboardPath -PathType Leaf)) {
        throw "Missing workflow external callback delivery dashboard: $DashboardPath"
    }
    Assert-ExternalRepairOutputPath -Value $DashboardPath -FieldName "DashboardPath"
}

if ([string]::IsNullOrWhiteSpace($InvocationID)) {
    $InvocationID = "workflow-external-callback-batch-redrive-invocation-" + [System.Guid]::NewGuid().ToString("N")
}
Assert-LowSensitiveRepairIdentifier -Value $InvocationID -FieldName "InvocationID"

function Get-CallbackBatchFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackBatchStringSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
}

function Read-JsonDocument {
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

function Get-ObjectString {
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

function Assert-NoRawCallbackBatchText {
    param(
        [string]$Value,
        [string]$FieldName
    )

    $match = [regex]::Match($Value, "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|EvidencePack|prompt)")
    if ($match.Success) {
        throw "$FieldName contains raw, secret, provider artifact, model input, URL, or credential-like content."
    }
}

function Assert-LowValue {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawCallbackBatchText -Value $Value -FieldName $FieldName
}

function Read-WorkflowBinding {
    param([object]$Binding)

    if ($null -eq $Binding) {
        throw "workflow_binding is required."
    }

    $result = [ordered]@{}
    foreach ($field in @(
            "workflow_id",
            "step_id",
            "expected_workflow_type",
            "expected_status",
            "expected_target_service",
            "expected_target_operation",
            "expected_target_ref_hash",
            "expected_payload_schema_version",
            "expected_payload_ref_hash",
            "expected_approval_policy_ref",
            "decision_policy_ref"
        )) {
        $value = Get-ObjectString -Object $Binding -Name $field
        Assert-LowValue -Value $value -FieldName "workflow_binding.$field"
        $result[$field] = $value
    }
    Assert-True ($result.expected_status -eq "WAITING_DECISION") "workflow_binding.expected_status must be WAITING_DECISION."
    return [pscustomobject]$result
}

function Read-RedrivePlan {
    param([string]$Path)

    $redrive = Read-JsonDocument -Path $Path -Label "Workflow external callback redrive plan"
    Assert-True ((Get-ObjectString -Object $redrive -Name "schema_version") -eq "nexusim.workflow.external_callback_redrive_plan.v1") "Unsupported redrive plan schema_version."
    Assert-True ([bool]$redrive.no_direct_execution) "Redrive plan must set no_direct_execution=true."
    Assert-True ([bool]$redrive.no_decision_recorded) "Redrive plan must set no_decision_recorded=true."
    Assert-True ([bool]$redrive.does_not_call_provider) "Redrive plan must set does_not_call_provider=true."
    Assert-True ([bool]$redrive.does_not_execute_target) "Redrive plan must set does_not_execute_target=true."

    $redrivePlanID = Get-ObjectString -Object $redrive -Name "redrive_plan_id"
    $sourceStatusHash = Get-ObjectString -Object $redrive -Name "source_delivery_status_sha256"
    $sourceDeliveryPlanHash = Get-ObjectString -Object $redrive -Name "source_delivery_plan_sha256"
    Assert-LowValue -Value $redrivePlanID -FieldName "redrive_plan_id"
    Assert-LowValue -Value $sourceStatusHash -FieldName "source_delivery_status_sha256"
    Assert-LowValue -Value $sourceDeliveryPlanHash -FieldName "source_delivery_plan_sha256"

    $binding = Read-WorkflowBinding -Binding $redrive.workflow_binding

    $source = $redrive.redrive_source
    Assert-True ($null -ne $source) "redrive_source is required."
    $deliveryStatus = (Get-ObjectString -Object $source -Name "delivery_status").ToUpperInvariant()
    Assert-True (@("RETRY_PENDING", "DLQ") -contains $deliveryStatus) "redrive_source.delivery_status must be RETRY_PENDING or DLQ."
    $attemptNumber = [int]$source.attempt_number
    $maxAttempts = [int]$source.max_attempts
    Assert-True ($attemptNumber -ge 1 -and $attemptNumber -le $maxAttempts) "redrive_source attempt_number must be within max_attempts."

    foreach ($entry in @(
            @{ name = "source_delivery_plan_sha256"; value = (Get-ObjectString -Object $source -Name "source_delivery_plan_sha256") },
            @{ name = "delivery_attempt_ref"; value = (Get-ObjectString -Object $source -Name "delivery_attempt_ref") },
            @{ name = "failure_class_ref"; value = (Get-ObjectString -Object $source -Name "failure_class_ref") },
            @{ name = "redrive_policy_ref"; value = (Get-ObjectString -Object $source -Name "redrive_policy_ref") }
        )) {
        Assert-LowValue -Value ([string]$entry.value) -FieldName "redrive_source.$($entry.name)"
    }
    Assert-True ((Get-ObjectString -Object $source -Name "source_delivery_plan_sha256") -eq $sourceDeliveryPlanHash) "redrive_source.source_delivery_plan_sha256 must match source_delivery_plan_sha256."

    $contract = $redrive.redrive_contract
    Assert-True ($null -ne $contract) "redrive_contract is required."
    foreach ($entry in @(
            @{ name = "redrive_queue_ref"; value = (Get-ObjectString -Object $contract -Name "redrive_queue_ref") },
            @{ name = "redrive_reason_ref"; value = (Get-ObjectString -Object $contract -Name "redrive_reason_ref") },
            @{ name = "operator_review_ref"; value = (Get-ObjectString -Object $contract -Name "operator_review_ref" -AllowEmpty); allow_empty = $true },
            @{ name = "final_decision_owner"; value = (Get-ObjectString -Object $contract -Name "final_decision_owner") }
        )) {
        Assert-LowValue -Value ([string]$entry.value) -FieldName "redrive_contract.$($entry.name)" -AllowEmpty:([bool]$entry.allow_empty)
    }
    Assert-True (-not [bool]$contract.redrive_plan_calls_provider) "redrive_contract.redrive_plan_calls_provider must be false."
    Assert-True (-not [bool]$contract.redrive_plan_records_decision) "redrive_contract.redrive_plan_records_decision must be false."
    Assert-True (-not [bool]$contract.redrive_plan_executes_target) "redrive_contract.redrive_plan_executes_target must be false."
    Assert-True ([bool]$contract.requires_new_delivery_attempt_ref) "redrive_contract.requires_new_delivery_attempt_ref must be true."
    Assert-True ([bool]$contract.requires_existing_waiting_workflow) "redrive_contract.requires_existing_waiting_workflow must be true."

    $fileHash = Get-CallbackBatchFileSha256Ref -Path $Path
    $pathHash = Get-CallbackBatchStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $Path))

    return [pscustomobject]@{
        redrive_plan_id = $redrivePlanID
        workflow_id = $binding.workflow_id
        step_id = $binding.step_id
        delivery_status = $deliveryStatus
        attempt_number = $attemptNumber
        max_attempts = $maxAttempts
        source_delivery_status_sha256 = $sourceStatusHash
        source_delivery_plan_sha256 = $sourceDeliveryPlanHash
        redrive_plan_sha256 = $fileHash
        redrive_plan_path_sha256 = $pathHash
        redrive_queue_ref = Get-ObjectString -Object $contract -Name "redrive_queue_ref"
        redrive_reason_ref = Get-ObjectString -Object $contract -Name "redrive_reason_ref"
        operator_review_ref = Get-ObjectString -Object $contract -Name "operator_review_ref" -AllowEmpty
        target_service = $binding.expected_target_service
        target_operation = $binding.expected_target_operation
        target_ref_hash = $binding.expected_target_ref_hash
        payload_schema_version = $binding.expected_payload_schema_version
        payload_ref_hash = $binding.expected_payload_ref_hash
        approval_policy_ref = $binding.expected_approval_policy_ref
        decision_policy_ref = $binding.decision_policy_ref
        final_decision_owner = Get-ObjectString -Object $contract -Name "final_decision_owner"
    }
}

$redriveFiles = @(Get-ChildItem -LiteralPath $RedrivePlanRootPath -Filter "*.json" -File | Sort-Object Name)
if ($redriveFiles.Count -eq 0) {
    throw "RedrivePlanRootPath must contain at least one redrive plan JSON file."
}

$seenPlanIDs = @{}
$seenStatusHashes = @{}
$redrives = @()
foreach ($file in $redriveFiles) {
    $redrive = Read-RedrivePlan -Path $file.FullName
    if ($seenPlanIDs.ContainsKey($redrive.redrive_plan_id)) {
        throw "Duplicate redrive_plan_id in batch invocation: $($redrive.redrive_plan_id)"
    }
    if ($seenStatusHashes.ContainsKey($redrive.source_delivery_status_sha256)) {
        throw "Duplicate source_delivery_status_sha256 in batch invocation: $($redrive.source_delivery_status_sha256)"
    }
    $seenPlanIDs[$redrive.redrive_plan_id] = $true
    $seenStatusHashes[$redrive.source_delivery_status_sha256] = $true
    $redrives += $redrive
}

$manifestRedrives = @()
foreach ($redrive in $redrives) {
    $manifestRedrives += [ordered]@{
        redrive_plan_id = $redrive.redrive_plan_id
        workflow_id = $redrive.workflow_id
        step_id = $redrive.step_id
        delivery_status = $redrive.delivery_status
        attempt_number = $redrive.attempt_number
        max_attempts = $redrive.max_attempts
        source_delivery_status_sha256 = $redrive.source_delivery_status_sha256
        source_delivery_plan_sha256 = $redrive.source_delivery_plan_sha256
        redrive_plan_sha256 = $redrive.redrive_plan_sha256
        redrive_plan_path_sha256 = $redrive.redrive_plan_path_sha256
        redrive_queue_ref = $redrive.redrive_queue_ref
        redrive_reason_ref = $redrive.redrive_reason_ref
        operator_review_ref = $redrive.operator_review_ref
        target_service = $redrive.target_service
        target_operation = $redrive.target_operation
        target_ref_hash = $redrive.target_ref_hash
        payload_schema_version = $redrive.payload_schema_version
        payload_ref_hash = $redrive.payload_ref_hash
        approval_policy_ref = $redrive.approval_policy_ref
        decision_policy_ref = $redrive.decision_policy_ref
        final_decision_owner = $redrive.final_decision_owner
    }
}

$invocation = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_batch_redrive_invocation.v1"
    invocation_id = $InvocationID
    generated_at = [DateTime]::UtcNow.ToString("o")
    prepared_by = $PreparedBy
    source_redrive_plan_root_sha256 = Get-CallbackBatchStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $RedrivePlanRootPath))
    source_dashboard_sha256 = if ([string]::IsNullOrWhiteSpace($DashboardPath)) { "" } else { Get-CallbackBatchFileSha256Ref -Path $DashboardPath }
    source_dashboard_path_sha256 = if ([string]::IsNullOrWhiteSpace($DashboardPath)) { "" } else { Get-CallbackBatchStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $DashboardPath)) }
    redrive_count = $manifestRedrives.Count
    runtime_contract = [ordered]@{
        service = "workflow-service"
        mode = "external-callback-delivery-redrive"
        plan_env = "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE"
        batch_invocation_calls_service = $false
        batch_invocation_records_decision = $false
        batch_invocation_calls_provider = $false
        batch_invocation_executes_target = $false
        requires_one_runtime_call_per_redrive_plan = $true
        runtime_must_revalidate_workflow_and_delivery = $true
    }
    redrives = $manifestRedrives
    batch_boundary = [ordered]@{
        owner = "workflow-service.external-callback-delivery"
        invocation_is_manifest_only = $true
        invocation_does_not_requeue_delivery = $true
        invocation_does_not_call_provider = $true
        invocation_does_not_record_decision = $true
        invocation_does_not_execute_target = $true
        redrive_runtime_owner = "workflow-service.external-callback-delivery-redrive"
        final_decision_owner = "workflow-service.RecordWorkflowDecision"
    }
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
    operator_note = "Low-sensitive batch redrive invocation manifest. It enumerates reviewed redrive plans and runtime contract only; it does not execute redrive, call a provider, record a workflow decision, or execute a target action."
}

$encoded = $invocation | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawCallbackBatchText -Value $encoded -FieldName "external callback batch redrive invocation manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$invocation | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback batch redrive invocation manifest written: $OutputPath"
