param(
    [Parameter(Mandatory = $true)]
    [string]$ReadinessPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$InvocationID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $ReadinessPath -PathType Leaf)) {
    throw "Missing workflow compensation execution readiness manifest: $ReadinessPath"
}
Assert-ExternalRepairOutputPath -Value $ReadinessPath -FieldName "ReadinessPath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($ReadinessPath))) "workflow-compensation-execution-invocation.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($InvocationID)) {
    $InvocationID = "workflow-compensation-execution-invocation-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $InvocationID -FieldName "InvocationID"

function Get-CompInvokeFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CompInvokeStringSha256Ref {
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

function Assert-NoRawText {
    param(
        [string]$Value,
        [string]$FieldName
    )
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|reason_text|EvidencePack|prompt|local_path)") {
        throw "$FieldName contains raw, secret, prompt, local path, provider artifact, or credential-like content."
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

$readiness = Get-JsonDocument -Path $ReadinessPath -Label "Workflow compensation execution readiness manifest"

$schemaVersion = Get-JsonString -Object $readiness -Name "schema_version"
Assert-True ($schemaVersion -eq "nexusim.workflow.compensation_execution_readiness.v1") "Unsupported workflow compensation execution readiness schema_version: $schemaVersion"
Assert-True ([bool]$readiness.no_direct_execution) "readiness.no_direct_execution must be true."
Assert-True ([bool]$readiness.no_decision_recorded) "readiness.no_decision_recorded must be true."
Assert-True ([bool]$readiness.does_not_execute_compensation) "readiness.does_not_execute_compensation must be true."

$workflow = $readiness.workflow
if ($null -eq $workflow) {
    throw "readiness.workflow is required."
}
$workflowID = Get-JsonString -Object $workflow -Name "workflow_id"
$workflowType = Get-JsonString -Object $workflow -Name "workflow_type"
$workflowStatus = Get-JsonString -Object $workflow -Name "status"
$targetService = Get-JsonString -Object $workflow -Name "target_service"
$targetOperation = Get-JsonString -Object $workflow -Name "target_operation"
$targetRefHash = Get-JsonString -Object $workflow -Name "target_ref_hash" -AllowEmpty
$payloadSchemaVersion = Get-JsonString -Object $workflow -Name "payload_schema_version" -AllowEmpty
$payloadRefHash = Get-JsonString -Object $workflow -Name "payload_ref_hash"
$approvalPolicyRef = Get-JsonString -Object $workflow -Name "approval_policy_ref" -AllowEmpty
$compensationPolicyRef = Get-JsonString -Object $workflow -Name "compensation_policy_ref" -AllowEmpty
$currentStepID = Get-JsonString -Object $workflow -Name "current_step_id" -AllowEmpty

foreach ($entry in @(
        @{ name = "workflow_id"; value = $workflowID; allow = $false },
        @{ name = "workflow_type"; value = $workflowType; allow = $false },
        @{ name = "status"; value = $workflowStatus; allow = $false },
        @{ name = "target_service"; value = $targetService; allow = $false },
        @{ name = "target_operation"; value = $targetOperation; allow = $false },
        @{ name = "target_ref_hash"; value = $targetRefHash; allow = $true },
        @{ name = "payload_schema_version"; value = $payloadSchemaVersion; allow = $true },
        @{ name = "payload_ref_hash"; value = $payloadRefHash; allow = $false },
        @{ name = "approval_policy_ref"; value = $approvalPolicyRef; allow = $true },
        @{ name = "compensation_policy_ref"; value = $compensationPolicyRef; allow = $true },
        @{ name = "current_step_id"; value = $currentStepID; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName "workflow.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
    Assert-NoRawText -Value ([string]$entry.value) -FieldName "workflow.$($entry.name)"
}

Assert-True ($workflowType -eq "COMPENSATION_REQUEST") "workflow.workflow_type must be COMPENSATION_REQUEST."
Assert-True ($workflowStatus -eq "COMPENSATION_PENDING") "workflow.status must be COMPENSATION_PENDING."
Assert-True ($targetService -eq "control-plane-service") "workflow.target_service must be control-plane-service for this invocation path."
Assert-True ($targetOperation -eq "CONFIG_ROLLBACK") "workflow.target_operation must be CONFIG_ROLLBACK for this invocation path."

$contract = $readiness.executor_contract
if ($null -eq $contract) {
    throw "readiness.executor_contract is required."
}
$owner = Get-JsonString -Object $contract -Name "owner"
$serviceMode = Get-JsonString -Object $contract -Name "service_mode"
$executorMode = Get-JsonString -Object $contract -Name "executor_mode"
$contractTargetService = Get-JsonString -Object $contract -Name "target_service"
$contractTargetOperation = Get-JsonString -Object $contract -Name "target_operation"
$instructionType = Get-JsonString -Object $contract -Name "instruction_type"

Assert-True ($owner -eq "workflow-service.compensation-executor") "executor_contract.owner must be workflow-service.compensation-executor."
Assert-True ($serviceMode -eq "compensation-executor") "executor_contract.service_mode must be compensation-executor."
Assert-True ($executorMode -in @("control-plane-rollback-store", "control-plane-rollback-file")) "executor_contract.executor_mode is unsupported."
Assert-True ($contractTargetService -eq $targetService) "executor_contract.target_service must match workflow.target_service."
Assert-True ($contractTargetOperation -eq $targetOperation) "executor_contract.target_operation must match workflow.target_operation."
Assert-True ($instructionType -eq "CONTROL_PLANE_ROLLBACK") "executor_contract.instruction_type must be CONTROL_PLANE_ROLLBACK."
Assert-False ([bool]$contract.executes_compensation) "readiness executor_contract.executes_compensation must be false."
Assert-True ([bool]$contract.requires_explicit_operator_execution) "executor_contract.requires_explicit_operator_execution must be true."
Assert-True ((Get-JsonString -Object $contract -Name "final_execution_owner") -eq "workflow-service.compensation-executor") "executor_contract.final_execution_owner mismatch."

foreach ($value in @(
        $owner,
        $serviceMode,
        $executorMode,
        $contractTargetService,
        $contractTargetOperation,
        $instructionType
    )) {
    Assert-LowString -Value $value -FieldName "executor_contract"
    Assert-NoRawText -Value $value -FieldName "executor_contract"
}

$instructionStatus = Get-JsonString -Object $readiness -Name "instruction_status"
Assert-True ($instructionStatus -eq "ACTIVE") "instruction_status must be ACTIVE."
$instructionRefs = @(Get-JsonArray -Object $readiness -Name "instruction_refs")
Assert-True ($instructionRefs.Count -gt 0) "readiness.instruction_refs must contain at least one instruction."

$boundInstructions = [System.Collections.Generic.List[object]]::new()
foreach ($instruction in $instructionRefs) {
    $instructionID = Get-JsonString -Object $instruction -Name "instruction_id"
    $instructionWorkflowID = Get-JsonString -Object $instruction -Name "workflow_id"
    $instructionPayloadRefHash = Get-JsonString -Object $instruction -Name "payload_ref_hash"
    $instructionTargetService = Get-JsonString -Object $instruction -Name "target_service"
    $instructionTargetOperation = Get-JsonString -Object $instruction -Name "target_operation"
    $instructionTypeValue = Get-JsonString -Object $instruction -Name "instruction_type"
    $status = Get-JsonString -Object $instruction -Name "status"
    $environment = Get-JsonString -Object $instruction -Name "environment" -AllowEmpty
    $configKind = Get-JsonString -Object $instruction -Name "config_kind" -AllowEmpty
    $bundleKey = Get-JsonString -Object $instruction -Name "bundle_key" -AllowEmpty
    $targetVersion = Get-JsonString -Object $instruction -Name "target_version" -AllowEmpty
    $operatorRef = Get-JsonString -Object $instruction -Name "operator_ref" -AllowEmpty
    $reasonRef = Get-JsonString -Object $instruction -Name "reason_ref" -AllowEmpty

    foreach ($entry in @(
            @{ name = "instruction_id"; value = $instructionID; allow = $false },
            @{ name = "workflow_id"; value = $instructionWorkflowID; allow = $false },
            @{ name = "payload_ref_hash"; value = $instructionPayloadRefHash; allow = $false },
            @{ name = "target_service"; value = $instructionTargetService; allow = $false },
            @{ name = "target_operation"; value = $instructionTargetOperation; allow = $false },
            @{ name = "instruction_type"; value = $instructionTypeValue; allow = $false },
            @{ name = "environment"; value = $environment; allow = $true },
            @{ name = "config_kind"; value = $configKind; allow = $true },
            @{ name = "bundle_key"; value = $bundleKey; allow = $true },
            @{ name = "target_version"; value = $targetVersion; allow = $true },
            @{ name = "operator_ref"; value = $operatorRef; allow = $true },
            @{ name = "reason_ref"; value = $reasonRef; allow = $true },
            @{ name = "status"; value = $status; allow = $false }
        )) {
        Assert-LowString -Value ([string]$entry.value) -FieldName "instruction.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
        Assert-NoRawText -Value ([string]$entry.value) -FieldName "instruction.$($entry.name)"
    }

    Assert-True ($instructionWorkflowID -eq $workflowID) "Instruction $instructionID workflow_id does not match workflow.workflow_id."
    Assert-True ($instructionPayloadRefHash -eq $payloadRefHash) "Instruction $instructionID payload_ref_hash does not match workflow.payload_ref_hash."
    Assert-True ($instructionTargetService -eq $targetService) "Instruction $instructionID target_service does not match workflow.target_service."
    Assert-True ($instructionTargetOperation -eq $targetOperation) "Instruction $instructionID target_operation does not match workflow.target_operation."
    Assert-True ($instructionTypeValue -eq "CONTROL_PLANE_ROLLBACK") "Instruction $instructionID instruction_type must be CONTROL_PLANE_ROLLBACK."
    Assert-True ($status -eq "ACTIVE") "Instruction $instructionID status must be ACTIVE."

    $boundInstructions.Add([ordered]@{
        instruction_id = $instructionID
        workflow_id = $instructionWorkflowID
        payload_ref_hash = $instructionPayloadRefHash
        target_service = $instructionTargetService
        target_operation = $instructionTargetOperation
        instruction_type = $instructionTypeValue
        environment = $environment
        config_kind = $configKind
        bundle_key = $bundleKey
        target_version = $targetVersion
        operator_ref = $operatorRef
        reason_ref = $reasonRef
        status = $status
    })
}

$preflightChecks = @(Get-JsonArray -Object $readiness -Name "preflight_checks")
$executionBoundary = @(Get-JsonArray -Object $readiness -Name "execution_boundary")
Assert-ArrayContains -Values $preflightChecks -Expected "compensation_review_bundle_verified" -FieldName "preflight_checks"
Assert-ArrayContains -Values $preflightChecks -Expected "workflow_compensation_pending" -FieldName "preflight_checks"
Assert-ArrayContains -Values $executionBoundary -Expected "operator_must_start_workflow_service_compensation_executor_explicitly" -FieldName "execution_boundary"
Assert-ArrayContains -Values $executionBoundary -Expected "workflow_service_compensation_executor_remains_final_execution_owner" -FieldName "execution_boundary"

foreach ($value in @($preflightChecks + $executionBoundary)) {
    Assert-LowString -Value ([string]$value) -FieldName "readiness.boundary"
    Assert-NoRawText -Value ([string]$value) -FieldName "readiness.boundary"
}

$readinessResolvedPath = [string](Resolve-Path -LiteralPath $ReadinessPath)
$manifest = [ordered]@{
    schema_version = "nexusim.workflow.compensation_execution_invocation.v1"
    invocation_id = $InvocationID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    source_readiness_sha256 = Get-CompInvokeFileSha256Ref -Path $ReadinessPath
    source_readiness_path_sha256 = Get-CompInvokeStringSha256Ref -Value $readinessResolvedPath
    manifest_is_execution = $false
    executes_compensation = $false
    requires_explicit_operator_execution = $true
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
    service_runtime_contract = [ordered]@{
        owner = "workflow-service.compensation-executor"
        command = "services/workflow-service/cmd/workflow-service"
        mode_env = "NEXUSIM_WORKFLOW_SERVICE_MODE"
        mode_env_value = "compensation-executor"
        executor_mode_env = "NEXUSIM_WORKFLOW_COMPENSATION_EXECUTOR_MODE"
        executor_mode_env_value = $executorMode
        postgres_dsn_env = "NEXUSIM_PG_DSN"
        control_plane_addr_env = "NEXUSIM_CONTROL_PLANE_GRPC_ADDR"
        instruction_file_env = "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE"
        instruction_file_required = ($executorMode -eq "control-plane-rollback-file")
        instruction_store_required = ($executorMode -eq "control-plane-rollback-store")
        execution_batch_size_env = "NEXUSIM_WORKFLOW_COMPENSATION_EXECUTION_BATCH_SIZE"
        final_execution_owner = "workflow-service.compensation-executor"
    }
    instruction_status = $instructionStatus
    instruction_count = $boundInstructions.Count
    instruction_refs = @($boundInstructions)
    required_checks = @(
        "readiness_manifest_verified",
        "workflow_still_compensation_pending_before_executor_start",
        "active_instruction_refs_still_bound_to_workflow",
        "operator_starts_workflow_service_compensation_executor_explicitly",
        "postgres_store_is_authoritative_for_claimed_compensations",
        "control_plane_mutation_only_via_public_control_plane_api",
        "executor_result_written_by_workflow_service_store"
    )
    forbidden_contents = @(
        "payload_material",
        "operator_reason_material",
        "provider_artifact_material",
        "evidence_text_material",
        "filesystem_path_material",
        "auth_material"
    )
    execution_boundary = @(
        "invocation_manifest_is_not_execution",
        "does_not_record_workflow_decision",
        "does_not_call_control_plane",
        "does_not_modify_workflow_or_compensation_rows",
        "operator_must_start_workflow_service_compensation_executor_explicitly",
        "workflow_service_compensation_executor_remains_final_execution_owner"
    )
    note = "Low-sensitive compensation execution invocation manifest. It prepares explicit workflow-service compensation-executor runtime settings only; it does not execute compensation, call control-plane-service, record decisions, or embed raw payloads."
}

$encoded = $manifest | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawText -Value $encoded -FieldName "compensation execution invocation manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$manifest | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow compensation execution invocation manifest written: $OutputPath"
