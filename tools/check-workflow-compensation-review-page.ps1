$ErrorActionPreference = "Stop"

$pageWriterPath = Join-Path $PSScriptRoot "write-workflow-compensation-review-page.ps1"
if (-not (Test-Path -LiteralPath $pageWriterPath -PathType Leaf)) {
    throw "Missing workflow compensation review page writer: $pageWriterPath"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-review-page-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

function Invoke-WriterExpectFailure {
    param(
        [string]$BundlePath,
        [string]$OutputPath,
        [string]$FailureName
    )

    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
            -BundlePath $BundlePath `
            -GeneratedBy "operator-a" `
            -PageID "workflow-compensation-review-page-1" `
            -OutputPath $OutputPath 2>$null | Out-Null
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($exitCode -eq 0) {
        throw "$FailureName should have failed."
    }
}

try {
    $leakMarker = "do-not-leak-workflow-compensation-review-secret-body"
    $bundlePath = Join-Path $tempRoot "workflow-compensation-review-bundle.json"
    $pagePath = Join-Path $tempRoot "workflow-compensation-review.html"
    $badNoExecutionPath = Join-Path $tempRoot "workflow-compensation-review-bad-execution.json"
    $badMismatchPath = Join-Path $tempRoot "workflow-compensation-review-bad-mismatch.json"

    $bundle = [ordered]@{
        mode = "compensation-review-bundle"
        tenant_id = "tenant-a"
        workflow_id = "wf_comp_1"
        debug_note = $leakMarker
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
    ($bundle | ConvertTo-Json -Depth 12) | Set-Content -LiteralPath $bundlePath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
        -BundlePath $bundlePath `
        -GeneratedBy "operator-a" `
        -PageID "workflow-compensation-review-page-1" `
        -OutputPath $pagePath
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-compensation-review-page.ps1 failed"
    }

    $html = Get-Content -LiteralPath $pagePath -Raw
    foreach ($expected in @(
            "NexusIM Workflow Compensation Review",
            "wf_comp_1",
            "COMPENSATION_REQUEST",
            "COMPENSATION_PENDING",
            "wfci_1",
            "CONTROL_PLANE_ROLLBACK",
            "no_direct_execution",
            "no_decision_recorded",
            "bundle_sha256",
            "bundle_path_sha256",
            "does_not_execute_compensation"
        )) {
        if (-not $html.Contains($expected)) {
            throw "workflow compensation review page missing expected low-sensitive content: $expected"
        }
    }

    foreach ($forbidden in @(
            $bundlePath,
            $pagePath,
            $tempRoot,
            $leakMarker,
            "secret-body",
            "debug_note"
        )) {
        if ($html.Contains($forbidden)) {
            throw "workflow compensation review page leaked sensitive or local artifact content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-review.html"
    Invoke-WriterExpectFailure -BundlePath $bundlePath -OutputPath $repoLocalOutput -FailureName "repository-local OutputPath"

    $badNoExecution = $bundle | ConvertTo-Json -Depth 12 | ConvertFrom-Json
    $badNoExecution.compensation_review.no_direct_execution = $false
    ($badNoExecution | ConvertTo-Json -Depth 12) | Set-Content -LiteralPath $badNoExecutionPath -Encoding UTF8
    Invoke-WriterExpectFailure -BundlePath $badNoExecutionPath -OutputPath (Join-Path $tempRoot "bad-no-execution.html") -FailureName "unsafe no_direct_execution"

    $badMismatch = $bundle | ConvertTo-Json -Depth 12 | ConvertFrom-Json
    $badMismatch.compensation_review.instructions[0].payload_ref_hash = "sha256:other"
    ($badMismatch | ConvertTo-Json -Depth 12) | Set-Content -LiteralPath $badMismatchPath -Encoding UTF8
    Invoke-WriterExpectFailure -BundlePath $badMismatchPath -OutputPath (Join-Path $tempRoot "bad-mismatch.html") -FailureName "instruction payload mismatch"
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation review page self-test"
