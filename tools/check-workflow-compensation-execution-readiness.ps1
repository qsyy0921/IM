$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-compensation-execution-readiness.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow compensation execution readiness writer: $writerPath"
}

function Write-JsonFile {
    param(
        [string]$Path,
        [object]$Value
    )
    $Value | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Writer {
    param(
        [string]$BundlePath,
        [string]$OutputPath,
        [string]$ReviewedBy = "operator-a",
        [string]$ExecutorMode = "control-plane-rollback-store"
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -BundlePath $BundlePath `
        -ReviewedBy $ReviewedBy `
        -ReadinessID "workflow-compensation-execution-readiness-1" `
        -ExecutorMode $ExecutorMode `
        -OutputPath $OutputPath 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
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

function New-FixtureBundle {
    param(
        [string]$LeakMarker = "do-not-leak-workflow-compensation-execution-secret-body"
    )

    return [ordered]@{
        mode = "compensation-review-bundle"
        tenant_id = "tenant-a"
        workflow_id = "wf_comp_1"
        debug_note = $LeakMarker
        compensation_review = [ordered]@{
            schema_version = "nexusim.workflow.compensation_review_bundle.v1"
            workflow = [ordered]@{
                workflow_id = "wf_comp_1"
                workflow_type = "COMPENSATION_REQUEST"
                risk_level = "HIGH"
                requester_ref = "operator:admin"
                requester_service = "admin-service"
                target_service = "control-plane-service"
                target_operation = "CONFIG_ROLLBACK"
                target_ref_hash = "sha256:target"
                payload_schema_version = "admin.config_rollback.v1"
                payload_ref_hash = "sha256:payload"
                approval_policy_ref = "admin.workflow.compensation.v1"
                timeout_policy_ref = "workflow.approval_timeout.v1"
                compensation_policy_ref = "admin.compensation.control_plane.v1"
                reason_ref = "reason-sha256:abc"
                evidence_refs = @("evidence:ticket-123")
                status = "COMPENSATION_PENDING"
                current_step_id = "wfs_comp_1"
                correlation_id = "corr-1"
                causation_id = "cause-1"
                trace_id = "trace-1"
            }
            instruction_status = "ACTIVE"
            instruction_count = 1
            instructions = @(
                [ordered]@{
                    instruction_id = "wfci_1"
                    workflow_id = "wf_comp_1"
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
            review_checks = @(
                "workflow_type_status_payload_binding_verified",
                "active_instruction_refs_bound_to_same_workflow",
                "instruction_payload_hash_matches_workflow_payload_hash",
                "instruction_target_matches_workflow_target",
                "operator_must_use_explicit_approval_or_repair_invocation"
            )
            approval_boundary = @(
                "review_bundle_is_read_only",
                "does_not_record_workflow_decision",
                "does_not_create_or_reuse_approval",
                "does_not_modify_compensation_instruction_status"
            )
            execution_boundary = @(
                "does_not_execute_compensation",
                "does_not_call_control_plane_or_action_executor",
                "workflow_compensation_executor_remains_final_compensation_execution_owner",
                "downstream_mutation_requires_public_service_api_and_audit"
            )
            no_direct_execution = $true
            no_decision_recorded = $true
        }
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-execution-readiness-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-compensation-execution-secret-body"
    $bundlePath = Join-Path $tempRoot "workflow-compensation-review-bundle.json"
    $readinessPath = Join-Path $tempRoot "workflow-compensation-execution-readiness.json"
    $bundle = New-FixtureBundle -LeakMarker $leakMarker
    Write-JsonFile -Path $bundlePath -Value $bundle

    Invoke-Writer -BundlePath $bundlePath -OutputPath $readinessPath

    $raw = Get-Content -LiteralPath $readinessPath -Raw
    $readiness = $raw | ConvertFrom-Json
    if ($readiness.schema_version -ne "nexusim.workflow.compensation_execution_readiness.v1" -or
        $readiness.readiness_id -ne "workflow-compensation-execution-readiness-1" -or
        $readiness.workflow.workflow_id -ne "wf_comp_1" -or
        $readiness.workflow.status -ne "COMPENSATION_PENDING" -or
        $readiness.workflow.target_service -ne "control-plane-service" -or
        $readiness.workflow.target_operation -ne "CONFIG_ROLLBACK" -or
        $readiness.executor_contract.owner -ne "workflow-service.compensation-executor" -or
        $readiness.executor_contract.executor_mode -ne "control-plane-rollback-store" -or
        $readiness.executor_contract.executes_compensation -ne $false -or
        $readiness.no_direct_execution -ne $true -or
        $readiness.no_decision_recorded -ne $true -or
        $readiness.does_not_execute_compensation -ne $true) {
        throw "workflow compensation execution readiness manifest has unexpected fields."
    }
    if ($readiness.instruction_count -ne 1 -or
        $readiness.instruction_refs[0].instruction_id -ne "wfci_1" -or
        $readiness.instruction_refs[0].instruction_type -ne "CONTROL_PLANE_ROLLBACK") {
        throw "workflow compensation execution readiness manifest did not bind expected instruction refs."
    }
    foreach ($expected in @(
            "compensation_review_bundle_verified",
            "workflow_compensation_pending",
            "active_instruction_bound_to_workflow",
            "control_plane_rollback_instruction_verified",
            "executor_mode_explicit",
            "no_raw_payload_or_reason"
        )) {
        if (@($readiness.preflight_checks) -notcontains $expected) {
            throw "workflow compensation execution readiness manifest missing preflight check: $expected"
        }
    }
    foreach ($expected in @(
            "readiness_manifest_is_not_execution",
            "does_not_execute_compensation",
            "operator_must_start_workflow_service_compensation_executor_explicitly",
            "workflow_service_compensation_executor_remains_final_execution_owner"
        )) {
        if (@($readiness.execution_boundary) -notcontains $expected) {
            throw "workflow compensation execution readiness manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $bundlePath,
            $readinessPath,
            $leakMarker,
            "secret-body",
            "debug_note",
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow compensation execution readiness manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-execution-readiness.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -BundlePath $bundlePath -OutputPath $repoLocalOutput
    }

    Invoke-ExpectFailure -Expected "low-sensitive operator id" -Script {
        Invoke-Writer -BundlePath $bundlePath -OutputPath $readinessPath -ReviewedBy "operator-token-secret"
    }

    $badNoExecution = $bundle | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badNoExecution.compensation_review.no_direct_execution = $false
    $badNoExecutionPath = Join-Path $tempRoot "bad-no-direct-execution.json"
    Write-JsonFile -Path $badNoExecutionPath -Value $badNoExecution
    Invoke-ExpectFailure -Expected "no_direct_execution=true" -Script {
        Invoke-Writer -BundlePath $badNoExecutionPath -OutputPath $readinessPath
    }

    $badStatus = $bundle | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badStatus.compensation_review.workflow.status = "WAITING_DECISION"
    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    Write-JsonFile -Path $badStatusPath -Value $badStatus
    Invoke-ExpectFailure -Expected "COMPENSATION_PENDING" -Script {
        Invoke-Writer -BundlePath $badStatusPath -OutputPath $readinessPath
    }

    $badMismatch = $bundle | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badMismatch.compensation_review.instructions[0].payload_ref_hash = "sha256:other"
    $badMismatchPath = Join-Path $tempRoot "bad-mismatch.json"
    Write-JsonFile -Path $badMismatchPath -Value $badMismatch
    Invoke-ExpectFailure -Expected "payload_ref_hash does not match" -Script {
        Invoke-Writer -BundlePath $badMismatchPath -OutputPath $readinessPath
    }

    $badTarget = $bundle | ConvertTo-Json -Depth 30 | ConvertFrom-Json
    $badTarget.compensation_review.workflow.target_service = "action-executor"
    $badTarget.compensation_review.instructions[0].target_service = "action-executor"
    $badTargetPath = Join-Path $tempRoot "bad-target.json"
    Write-JsonFile -Path $badTargetPath -Value $badTarget
    Invoke-ExpectFailure -Expected "control-plane-service" -Script {
        Invoke-Writer -BundlePath $badTargetPath -OutputPath $readinessPath
    }

    Invoke-ExpectFailure -Expected "ExecutorMode must be" -Script {
        Invoke-Writer -BundlePath $bundlePath -OutputPath $readinessPath -ExecutorMode "silent-default-mode"
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-execution-readiness.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation execution readiness self-test"
