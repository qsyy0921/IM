param(
    [Parameter(Mandatory = $true)]
    [string]$BundlePath,

    [Parameter(Mandatory = $true)]
    [string]$ReviewedBy,

    [string]$OutputPath = "",
    [string]$ReadinessID = "",
    [string]$ExecutorMode = "control-plane-rollback-store"
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $BundlePath -PathType Leaf)) {
    throw "Missing workflow compensation review bundle: $BundlePath"
}
Assert-ExternalRepairOutputPath -Value $BundlePath -FieldName "BundlePath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($BundlePath))) "workflow-compensation-execution-readiness.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($ReadinessID)) {
    $ReadinessID = "workflow-compensation-execution-readiness-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $ReviewedBy -FieldName "ReviewedBy"
Assert-LowSensitiveRepairIdentifier -Value $ReadinessID -FieldName "ReadinessID"
Assert-LowSensitiveRepairIdentifier -Value $ExecutorMode -FieldName "ExecutorMode"

function Get-CompReadyFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CompReadyStringSha256Ref {
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

function Assert-LowString {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
}

function Assert-NoRawText {
    param(
        [string]$Value,
        [string]$FieldName
    )

    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|reason_text|EvidencePack|prompt)") {
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

$document = Get-JsonDocument -Path $BundlePath -Label "Workflow compensation review bundle"
$bundle = $document
if ($null -ne $document.PSObject.Properties["compensation_review"]) {
    $bundle = $document.compensation_review
}

$schemaVersion = Get-JsonString -Object $bundle -Name "schema_version"
Assert-True ($schemaVersion -eq "nexusim.workflow.compensation_review_bundle.v1") "Unsupported workflow compensation review bundle schema_version: $schemaVersion"
Assert-True ([bool]$bundle.no_direct_execution) "Workflow compensation review bundle must set no_direct_execution=true."
Assert-True ([bool]$bundle.no_decision_recorded) "Workflow compensation review bundle must set no_decision_recorded=true."

$workflow = $bundle.workflow
if ($null -eq $workflow) {
    throw "workflow is required."
}

$workflowID = Get-JsonString -Object $workflow -Name "workflow_id"
$workflowType = Get-JsonString -Object $workflow -Name "workflow_type"
$workflowStatus = Get-JsonString -Object $workflow -Name "status"
$payloadRefHash = Get-JsonString -Object $workflow -Name "payload_ref_hash"
$targetService = Get-JsonString -Object $workflow -Name "target_service"
$targetOperation = Get-JsonString -Object $workflow -Name "target_operation"
$targetRefHash = Get-JsonString -Object $workflow -Name "target_ref_hash" -AllowEmpty
$payloadSchemaVersion = Get-JsonString -Object $workflow -Name "payload_schema_version" -AllowEmpty
$approvalPolicyRef = Get-JsonString -Object $workflow -Name "approval_policy_ref" -AllowEmpty
$compensationPolicyRef = Get-JsonString -Object $workflow -Name "compensation_policy_ref" -AllowEmpty
$currentStepID = Get-JsonString -Object $workflow -Name "current_step_id" -AllowEmpty

foreach ($entry in @(
        @{ name = "workflow_id"; value = $workflowID; allow = $false },
        @{ name = "workflow_type"; value = $workflowType; allow = $false },
        @{ name = "status"; value = $workflowStatus; allow = $false },
        @{ name = "payload_ref_hash"; value = $payloadRefHash; allow = $false },
        @{ name = "target_service"; value = $targetService; allow = $false },
        @{ name = "target_operation"; value = $targetOperation; allow = $false },
        @{ name = "target_ref_hash"; value = $targetRefHash; allow = $true },
        @{ name = "payload_schema_version"; value = $payloadSchemaVersion; allow = $true },
        @{ name = "approval_policy_ref"; value = $approvalPolicyRef; allow = $true },
        @{ name = "compensation_policy_ref"; value = $compensationPolicyRef; allow = $true },
        @{ name = "current_step_id"; value = $currentStepID; allow = $true },
        @{ name = "risk_level"; value = Get-JsonString -Object $workflow -Name "risk_level" -AllowEmpty; allow = $true },
        @{ name = "requester_ref"; value = Get-JsonString -Object $workflow -Name "requester_ref" -AllowEmpty; allow = $true },
        @{ name = "requester_service"; value = Get-JsonString -Object $workflow -Name "requester_service" -AllowEmpty; allow = $true },
        @{ name = "reason_ref"; value = Get-JsonString -Object $workflow -Name "reason_ref" -AllowEmpty; allow = $true },
        @{ name = "correlation_id"; value = Get-JsonString -Object $workflow -Name "correlation_id" -AllowEmpty; allow = $true },
        @{ name = "causation_id"; value = Get-JsonString -Object $workflow -Name "causation_id" -AllowEmpty; allow = $true },
        @{ name = "trace_id"; value = Get-JsonString -Object $workflow -Name "trace_id" -AllowEmpty; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName "workflow.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
    Assert-NoRawText -Value ([string]$entry.value) -FieldName "workflow.$($entry.name)"
}

Assert-True ($workflowType -eq "COMPENSATION_REQUEST") "workflow.workflow_type must be COMPENSATION_REQUEST."
Assert-True ($workflowStatus -eq "COMPENSATION_PENDING") "workflow.status must be COMPENSATION_PENDING."
Assert-True ($targetService -eq "control-plane-service") "workflow.target_service must be control-plane-service for this first readiness path."
Assert-True ($targetOperation -eq "CONFIG_ROLLBACK") "workflow.target_operation must be CONFIG_ROLLBACK for this first readiness path."
Assert-True ($ExecutorMode -in @("control-plane-rollback-store", "control-plane-rollback-file")) "ExecutorMode must be control-plane-rollback-store or control-plane-rollback-file."

$instructionStatus = Get-JsonString -Object $bundle -Name "instruction_status"
Assert-True ($instructionStatus -eq "ACTIVE") "instruction_status must be ACTIVE."

$instructionCount = [int]$bundle.instruction_count
$instructions = @(Get-JsonArray -Object $bundle -Name "instructions")
Assert-True ($instructionCount -eq $instructions.Count) "instruction_count does not match instructions count."
Assert-True ($instructions.Count -gt 0) "At least one compensation instruction is required."

$instructionRefs = [System.Collections.Generic.List[object]]::new()
foreach ($instruction in $instructions) {
    $instructionID = Get-JsonString -Object $instruction -Name "instruction_id"
    $instructionWorkflowID = Get-JsonString -Object $instruction -Name "workflow_id"
    $instructionPayloadRefHash = Get-JsonString -Object $instruction -Name "payload_ref_hash"
    $instructionTargetService = Get-JsonString -Object $instruction -Name "target_service"
    $instructionTargetOperation = Get-JsonString -Object $instruction -Name "target_operation"
    $instructionType = Get-JsonString -Object $instruction -Name "instruction_type"
    $status = Get-JsonString -Object $instruction -Name "status"
    $targetVersion = Get-JsonString -Object $instruction -Name "target_version" -AllowEmpty

    foreach ($entry in @(
            @{ name = "instruction_id"; value = $instructionID; allow = $false },
            @{ name = "workflow_id"; value = $instructionWorkflowID; allow = $false },
            @{ name = "payload_ref_hash"; value = $instructionPayloadRefHash; allow = $false },
            @{ name = "target_service"; value = $instructionTargetService; allow = $false },
            @{ name = "target_operation"; value = $instructionTargetOperation; allow = $false },
            @{ name = "instruction_type"; value = $instructionType; allow = $false },
            @{ name = "environment"; value = Get-JsonString -Object $instruction -Name "environment" -AllowEmpty; allow = $true },
            @{ name = "config_kind"; value = Get-JsonString -Object $instruction -Name "config_kind" -AllowEmpty; allow = $true },
            @{ name = "bundle_key"; value = Get-JsonString -Object $instruction -Name "bundle_key" -AllowEmpty; allow = $true },
            @{ name = "target_version"; value = $targetVersion; allow = $true },
            @{ name = "operator_ref"; value = Get-JsonString -Object $instruction -Name "operator_ref" -AllowEmpty; allow = $true },
            @{ name = "reason_ref"; value = Get-JsonString -Object $instruction -Name "reason_ref" -AllowEmpty; allow = $true },
            @{ name = "status"; value = $status; allow = $false }
        )) {
        Assert-LowString -Value ([string]$entry.value) -FieldName "instruction.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
        Assert-NoRawText -Value ([string]$entry.value) -FieldName "instruction.$($entry.name)"
    }

    Assert-True ($instructionWorkflowID -eq $workflowID) "Instruction $instructionID workflow_id does not match workflow.workflow_id."
    Assert-True ($instructionPayloadRefHash -eq $payloadRefHash) "Instruction $instructionID payload_ref_hash does not match workflow.payload_ref_hash."
    Assert-True ($instructionTargetService -eq $targetService) "Instruction $instructionID target_service does not match workflow.target_service."
    Assert-True ($instructionTargetOperation -eq $targetOperation) "Instruction $instructionID target_operation does not match workflow.target_operation."
    Assert-True ($instructionType -eq "CONTROL_PLANE_ROLLBACK") "Instruction $instructionID instruction_type must be CONTROL_PLANE_ROLLBACK for this first readiness path."
    Assert-True ($status -eq "ACTIVE") "Instruction $instructionID status must be ACTIVE."

    $instructionRefs.Add([ordered]@{
        instruction_id = $instructionID
        workflow_id = $instructionWorkflowID
        payload_ref_hash = $instructionPayloadRefHash
        target_service = $instructionTargetService
        target_operation = $instructionTargetOperation
        instruction_type = $instructionType
        environment = Get-JsonString -Object $instruction -Name "environment" -AllowEmpty
        config_kind = Get-JsonString -Object $instruction -Name "config_kind" -AllowEmpty
        bundle_key = Get-JsonString -Object $instruction -Name "bundle_key" -AllowEmpty
        target_version = $targetVersion
        operator_ref = Get-JsonString -Object $instruction -Name "operator_ref" -AllowEmpty
        reason_ref = Get-JsonString -Object $instruction -Name "reason_ref" -AllowEmpty
        status = $status
    })
}

$reviewChecks = @(Get-JsonArray -Object $bundle -Name "review_checks")
$approvalBoundary = @(Get-JsonArray -Object $bundle -Name "approval_boundary")
$executionBoundary = @(Get-JsonArray -Object $bundle -Name "execution_boundary")
foreach ($value in @($reviewChecks + $approvalBoundary + $executionBoundary)) {
    Assert-LowString -Value ([string]$value) -FieldName "bundle.boundary"
    Assert-NoRawText -Value ([string]$value) -FieldName "bundle.boundary"
}
Assert-ArrayContains -Values $executionBoundary -Expected "does_not_execute_compensation" -FieldName "execution_boundary"
Assert-ArrayContains -Values $executionBoundary -Expected "workflow_compensation_executor_remains_final_compensation_execution_owner" -FieldName "execution_boundary"

$bundleResolvedPath = [string](Resolve-Path -LiteralPath $BundlePath)
$readiness = [ordered]@{
    schema_version = "nexusim.workflow.compensation_execution_readiness.v1"
    readiness_id = $ReadinessID
    generated_at = [DateTime]::UtcNow.ToString("o")
    reviewed_by = $ReviewedBy
    source_review_bundle_sha256 = Get-CompReadyFileSha256Ref -Path $BundlePath
    source_review_bundle_path_sha256 = Get-CompReadyStringSha256Ref -Value $bundleResolvedPath
    workflow = [ordered]@{
        workflow_id = $workflowID
        workflow_type = $workflowType
        status = $workflowStatus
        target_service = $targetService
        target_operation = $targetOperation
        target_ref_hash = $targetRefHash
        payload_schema_version = $payloadSchemaVersion
        payload_ref_hash = $payloadRefHash
        approval_policy_ref = $approvalPolicyRef
        compensation_policy_ref = $compensationPolicyRef
        current_step_id = $currentStepID
    }
    executor_contract = [ordered]@{
        owner = "workflow-service.compensation-executor"
        service_mode = "compensation-executor"
        executor_mode = $ExecutorMode
        target_service = $targetService
        target_operation = $targetOperation
        instruction_type = "CONTROL_PLANE_ROLLBACK"
        executes_compensation = $false
        readiness_manifest_is_execution = $false
        requires_explicit_operator_execution = $true
        final_execution_owner = "workflow-service.compensation-executor"
    }
    instruction_status = $instructionStatus
    instruction_count = $instructions.Count
    instruction_refs = @($instructionRefs)
    preflight_checks = @(
        "compensation_review_bundle_verified",
        "workflow_compensation_pending",
        "active_instruction_bound_to_workflow",
        "instruction_payload_hash_matches_workflow",
        "instruction_target_matches_workflow",
        "control_plane_rollback_instruction_verified",
        "executor_mode_explicit",
        "no_raw_payload_or_reason"
    )
    approval_boundary = @(
        "readiness_manifest_does_not_record_decision",
        "readiness_manifest_does_not_create_or_reuse_approval",
        "operator_must_use_existing_review_bundle_and_instruction_refs",
        "approval_or_compensation_policy_changes_require_new_workflow_fact"
    )
    execution_boundary = @(
        "readiness_manifest_is_not_execution",
        "does_not_execute_compensation",
        "does_not_call_control_plane_or_action_executor",
        "operator_must_start_workflow_service_compensation_executor_explicitly",
        "executor_claims_requested_compensations_from_workflow_service_store",
        "control_plane_mutation_only_via_public_control_plane_api",
        "workflow_service_compensation_executor_remains_final_execution_owner"
    )
    no_direct_execution = $true
    no_decision_recorded = $true
    does_not_execute_compensation = $true
    forbidden_contents = @(
        "payload_material",
        "operator_reason_material",
        "provider_artifact_material",
        "evidence_text_material",
        "local_path_material",
        "auth_material"
    )
    note = "Low-sensitive compensation execution readiness manifest. It binds a reviewed compensation bundle to the explicit workflow-service compensation-executor contract only; it does not execute compensation or call downstream services."
}

$encoded = $readiness | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawText -Value $encoded -FieldName "readiness manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$readiness | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow compensation execution readiness manifest written: $OutputPath"
