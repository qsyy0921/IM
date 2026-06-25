$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-compensation-execution-invocation.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow compensation execution invocation writer: $writerPath"
}

function Write-JsonFile {
    param(
        [string]$Path,
        [object]$Value
    )
    $Value | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-ExpectFailure {
    param(
        [scriptblock]$Script,
        [string]$Expected
    )
    try {
        & $Script
    } catch {
        if ($_.Exception.Message -notmatch [regex]::Escape($Expected)) {
            throw "Expected failure containing '$Expected', got: $($_.Exception.Message)"
        }
        return
    }
    throw "Expected failure containing '$Expected', but command succeeded."
}

function New-ReadinessFixture {
    return [ordered]@{
        schema_version = "nexusim.workflow.compensation_execution_readiness.v1"
        readiness_id = "workflow-compensation-execution-readiness-1"
        generated_at = "2026-06-25T00:00:00Z"
        reviewed_by = "operator-a"
        source_review_bundle_sha256 = "sha256:bundle"
        source_review_bundle_path_sha256 = "sha256:bundlepath"
        workflow = [ordered]@{
            workflow_id = "wf-comp-exec-1"
            workflow_type = "COMPENSATION_REQUEST"
            status = "COMPENSATION_PENDING"
            target_service = "control-plane-service"
            target_operation = "CONFIG_ROLLBACK"
            target_ref_hash = "sha256:target"
            payload_schema_version = "admin.config_rollback.v1"
            payload_ref_hash = "sha256:payload"
            approval_policy_ref = "admin.workflow.compensation.v1"
            compensation_policy_ref = "admin.compensation.control_plane.v1"
            current_step_id = "wfs-comp-exec-1"
        }
        executor_contract = [ordered]@{
            owner = "workflow-service.compensation-executor"
            service_mode = "compensation-executor"
            executor_mode = "control-plane-rollback-store"
            target_service = "control-plane-service"
            target_operation = "CONFIG_ROLLBACK"
            instruction_type = "CONTROL_PLANE_ROLLBACK"
            executes_compensation = $false
            readiness_manifest_is_execution = $false
            requires_explicit_operator_execution = $true
            final_execution_owner = "workflow-service.compensation-executor"
        }
        instruction_status = "ACTIVE"
        instruction_count = 1
        instruction_refs = @(
            [ordered]@{
                instruction_id = "wfci-comp-exec-1"
                workflow_id = "wf-comp-exec-1"
                payload_ref_hash = "sha256:payload"
                target_service = "control-plane-service"
                target_operation = "CONFIG_ROLLBACK"
                instruction_type = "CONTROL_PLANE_ROLLBACK"
                environment = "local"
                config_kind = "API_GATEWAY_TENANT_QUOTA"
                bundle_key = "tenant-a"
                target_version = "quota-v1"
                operator_ref = "operator:rollback"
                reason_ref = "reason-sha256:abc"
                status = "ACTIVE"
            }
        )
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
    }
}

function Invoke-Writer {
    param(
        [string]$ReadinessPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-alice",
        [string]$InvocationID = "workflow-compensation-execution-invocation-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -ReadinessPath $ReadinessPath `
        -GeneratedBy $GeneratedBy `
        -OutputPath $OutputPath `
        -InvocationID $InvocationID 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-execution-invocation-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $readinessPath = Join-Path $tempRoot "workflow-compensation-execution-readiness.json"
    $invocationPath = Join-Path $tempRoot "workflow-compensation-execution-invocation.json"
    $readiness = New-ReadinessFixture
    Write-JsonFile -Path $readinessPath -Value $readiness

    Invoke-Writer -ReadinessPath $readinessPath -OutputPath $invocationPath

    $raw = Get-Content -LiteralPath $invocationPath -Raw
    $invocation = $raw | ConvertFrom-Json
    if ($invocation.schema_version -ne "nexusim.workflow.compensation_execution_invocation.v1" -or
        $invocation.invocation_id -ne "workflow-compensation-execution-invocation-1" -or
        $invocation.manifest_is_execution -ne $false -or
        $invocation.executes_compensation -ne $false -or
        $invocation.requires_explicit_operator_execution -ne $true -or
        $invocation.workflow.workflow_id -ne "wf-comp-exec-1" -or
        $invocation.workflow.status -ne "COMPENSATION_PENDING" -or
        $invocation.service_runtime_contract.owner -ne "workflow-service.compensation-executor" -or
        $invocation.service_runtime_contract.mode_env -ne "NEXUSIM_WORKFLOW_SERVICE_MODE" -or
        $invocation.service_runtime_contract.mode_env_value -ne "compensation-executor" -or
        $invocation.service_runtime_contract.executor_mode_env_value -ne "control-plane-rollback-store" -or
        $invocation.service_runtime_contract.instruction_store_required -ne $true -or
        $invocation.service_runtime_contract.instruction_file_required -ne $false -or
        $invocation.instruction_count -ne 1 -or
        $invocation.instruction_refs[0].instruction_id -ne "wfci-comp-exec-1") {
        throw "workflow compensation execution invocation manifest has unexpected fields."
    }
    foreach ($expected in @(
            "readiness_manifest_verified",
            "workflow_still_compensation_pending_before_executor_start",
            "active_instruction_refs_still_bound_to_workflow",
            "postgres_store_is_authoritative_for_claimed_compensations",
            "control_plane_mutation_only_via_public_control_plane_api"
        )) {
        if (@($invocation.required_checks) -notcontains $expected) {
            throw "workflow compensation execution invocation manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "invocation_manifest_is_not_execution",
            "does_not_call_control_plane",
            "does_not_modify_workflow_or_compensation_rows",
            "workflow_service_compensation_executor_remains_final_execution_owner"
        )) {
        if (@($invocation.execution_boundary) -notcontains $expected) {
            throw "workflow compensation execution invocation manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $readinessPath,
            $invocationPath,
            "provider_body",
            "EvidencePack",
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow compensation execution invocation manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-execution-invocation.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -ReadinessPath $readinessPath -OutputPath $repoLocalOutput
    }

    Invoke-ExpectFailure -Expected "low-sensitive operator id" -Script {
        Invoke-Writer -ReadinessPath $readinessPath -OutputPath $invocationPath -GeneratedBy "operator-token-secret"
    }

    $badStatus = $readiness | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badStatus.workflow.status = "WAITING_DECISION"
    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    Write-JsonFile -Path $badStatusPath -Value $badStatus
    Invoke-ExpectFailure -Expected "COMPENSATION_PENDING" -Script {
        Invoke-Writer -ReadinessPath $badStatusPath -OutputPath $invocationPath
    }

    $badOwner = $readiness | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badOwner.executor_contract.owner = "admin-service"
    $badOwnerPath = Join-Path $tempRoot "bad-owner.json"
    Write-JsonFile -Path $badOwnerPath -Value $badOwner
    Invoke-ExpectFailure -Expected "workflow-service.compensation-executor" -Script {
        Invoke-Writer -ReadinessPath $badOwnerPath -OutputPath $invocationPath
    }

    $badNoExecution = $readiness | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badNoExecution.no_direct_execution = $false
    $badNoExecutionPath = Join-Path $tempRoot "bad-no-direct-execution.json"
    Write-JsonFile -Path $badNoExecutionPath -Value $badNoExecution
    Invoke-ExpectFailure -Expected "no_direct_execution" -Script {
        Invoke-Writer -ReadinessPath $badNoExecutionPath -OutputPath $invocationPath
    }

    $badTarget = $readiness | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badTarget.workflow.target_service = "action-executor"
    $badTarget.executor_contract.target_service = "action-executor"
    $badTarget.instruction_refs[0].target_service = "action-executor"
    $badTargetPath = Join-Path $tempRoot "bad-target.json"
    Write-JsonFile -Path $badTargetPath -Value $badTarget
    Invoke-ExpectFailure -Expected "control-plane-service" -Script {
        Invoke-Writer -ReadinessPath $badTargetPath -OutputPath $invocationPath
    }

    $badReason = $readiness | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badReason.instruction_refs[0].reason_ref = "raw:rollback-secret"
    $badReasonPath = Join-Path $tempRoot "bad-reason.json"
    Write-JsonFile -Path $badReasonPath -Value $badReason
    Invoke-ExpectFailure -Expected "credential-like" -Script {
        Invoke-Writer -ReadinessPath $badReasonPath -OutputPath $invocationPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-execution-invocation.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation execution invocation self-test"
