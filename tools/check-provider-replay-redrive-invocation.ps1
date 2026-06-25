$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-provider-replay-redrive-invocation.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing provider replay redrive invocation writer: $writerPath"
}

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

function Get-CheckSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
}

function Get-CheckFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
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

function New-PayloadHash {
    param([hashtable]$Payload)
    $json = $Payload | ConvertTo-Json -Depth 20 -Compress
    return Get-CheckSha256Ref $json
}

function New-Fixture {
    param([string]$Root)

    $payload = [ordered]@{
        provider_failure_ref_hash = "sha256:providerfailure1234567890abcdef"
        source_execution_ref_hash = "sha256:sourceexecution1234567890abcd"
        source_result_ref_hash = "sha256:sourceresult1234567890abcdef12"
        replay_candidate_id = "provider-replay-candidate-1234"
        redrive_entrypoint = "RedriveProviderFailure"
        requires_fresh_proposal = $true
        requires_fresh_approval = $true
        requires_prepared_audit = $true
        requires_new_input = $true
        requires_reason_sha256 = $true
        source_dlq_immutable = $true
        direct_execution_allowed = $false
    }
    $payloadHash = New-PayloadHash $payload

    $handoff = [ordered]@{
        kind = "action-executor.provider-failure.replay-admin-workflow-handoff"
        handoff_contract = [ordered]@{
            admin_operation_type = "PROVIDER_REPLAY_REQUEST"
            workflow_type = "REPAIR_APPROVAL"
            target_service = "action-executor"
            target_operation = "PROVIDER_REPLAY_REQUEST"
            redrive_entrypoint = "RedriveProviderFailure"
            approval_policy_ref = "admin.workflow.provider_replay.v1"
            payload_schema_version = "admin.provider_replay_request.v1"
            direct_execution_allowed = $false
            source_dlq_immutable = $true
            requires = @(
                "admin_operation_request",
                "workflow_repair_approval",
                "fresh_agent_proposal",
                "fresh_agent_approval",
                "fresh_prepared_audit",
                "matching_skill_tool_resource",
                "new_input_json",
                "reason_sha256",
                "action_executor_redrive_entrypoint"
            )
        }
        admin_operation_requests = @([ordered]@{
            auth_tenant_id = "tenant-provider-replay"
            operator_ref = "operator:alice"
            operator_role = "OPERATOR"
            operation_type = "PROVIDER_REPLAY_REQUEST"
            target_ref_hash = $payload.provider_failure_ref_hash
            risk_level = "HIGH"
            payload_schema_version = "admin.provider_replay_request.v1"
            operation_payload = $payload
            operation_payload_hash = $payloadHash
            reason_ref = "reason:provider-replay"
            evidence_refs = @("evidence:provider-replay")
            idempotency_key = "provider-replay-admin:provider-replay-candidate-1234"
            correlation_id = "corr-provider-replay"
            causation_id = "provider-failure-1"
            trace_id = "trace-provider-replay"
            expected_workflow_policy = "admin.workflow.provider_replay.v1"
        })
        workflow_handoff_requests = @([ordered]@{
            workflow_type = "REPAIR_APPROVAL"
            requester_service = "admin-service"
            target_service = "action-executor"
            target_operation = "PROVIDER_REPLAY_REQUEST"
            risk_level = "HIGH"
            target_ref_hash = $payload.provider_failure_ref_hash
            payload_schema_version = "admin.provider_replay_request.v1"
            payload_ref_hash = $payloadHash
            approval_policy_ref = "admin.workflow.provider_replay.v1"
            reason_ref = "reason:provider-replay"
            evidence_refs = @("evidence:provider-replay")
            idempotency_key = "admin-workflow:admop-provider-replay-1"
            correlation_id = "corr-provider-replay"
            causation_id = "admop-provider-replay-1"
            trace_id = "trace-provider-replay"
        })
        rows = @([ordered]@{
            replay_candidate_id = "provider-replay-candidate-1234"
            replay_state = "AWAITING_ADMIN_WORKFLOW"
            tenant_id = "tenant-provider-replay"
            provider_failure_id = "provider-failure-1"
            execution_id = "execution-source-1"
            result_id = "result-source-1"
            proposal_id = "proposal-source-1"
            approval_id = "approval-source-1"
            prepared_audit_id = "audit-source-1"
            user_id_hash = "userhash"
            skill_id = "skill.demo"
            tool_name = "nexusim.local.echo"
            resource_type = "conversation"
            resource_id_hash = "resourcehash"
            classification = "PROVIDER_UNAVAILABLE"
            status = "DLQ"
            retryable = $false
            retry_count = 3
            failure_ref_hash = "failurehash"
            created_at = "2026-06-25T00:00:00Z"
        })
    }

    $admin = [ordered]@{
        mode = "provider-replay-approve"
        operation = [ordered]@{
            operation_id = "admop-provider-replay-1"
            operation_type = "PROVIDER_REPLAY_REQUEST"
            target_ref_hash = $payload.provider_failure_ref_hash
            risk_level = "HIGH"
            payload_schema_version = "admin.provider_replay_request.v1"
            payload_hash = $payloadHash
            reason_ref = "reason:provider-replay"
            evidence_refs = @("evidence:provider-replay")
            status = "APPROVED"
            requested_by = "operator:alice"
            approved_by = "operator:bob"
            correlation_id = "corr-provider-replay"
            causation_id = "provider-failure-1"
            trace_id = "trace-provider-replay"
        }
        approval = [ordered]@{
            approval_id = "admin-approval-1"
            operation_id = "admop-provider-replay-1"
            approver_ref = "operator:bob"
            decision = "APPROVE"
            approval_policy_ref = "admin.workflow.provider_replay.v1"
            reason_ref = "reason:approved"
            evidence_refs = @("evidence:approved")
        }
    }

    $decision = [ordered]@{
        schema_version = "nexusim.workflow.external_decision_manifest.v1"
        workflow_id = "workflow-provider-replay-1"
        step_id = "workflow-step-1"
        expected_workflow_type = "REPAIR_APPROVAL"
        expected_status = "WAITING_DECISION"
        expected_target_service = "action-executor"
        expected_target_operation = "PROVIDER_REPLAY_REQUEST"
        expected_target_ref_hash = $payload.provider_failure_ref_hash
        expected_payload_schema_version = "admin.provider_replay_request.v1"
        expected_payload_ref_hash = $payloadHash
        expected_approval_policy_ref = "admin.workflow.provider_replay.v1"
        decision = "APPROVE"
        decider_ref = "operator:bob"
        decision_policy_ref = "workflow.external-approval.v1"
        reason_ref = "reason:workflow-approved"
        evidence_refs = @("evidence:workflow-approved")
        idempotency_key = "external-approval:workflow-provider-replay-1:workflow-step-1:APPROVE:operator:bob"
        correlation_id = "workflow-decision:workflow-provider-replay-1"
        causation_id = "workflow:workflow-provider-replay-1"
        trace_id = "workflow-decision:workflow-provider-replay-1"
    }

    $handoffPath = Join-Path $Root "provider-replay-handoff.json"
    $adminPath = Join-Path $Root "provider-replay-admin.json"
    $decisionPath = Join-Path $Root "provider-replay-decision.json"
    $proofPath = Join-Path $Root "provider-replay-fresh-proof.json"
    Write-JsonFile -Path $handoffPath -Value $handoff
    Write-JsonFile -Path $adminPath -Value $admin
    Write-JsonFile -Path $decisionPath -Value $decision

    $proof = [ordered]@{
        schema_version = "nexusim.action_executor.provider_replay_fresh_proof.v1"
        replay_candidate_id = "provider-replay-candidate-1234"
        provider_failure_ref_hash = $payload.provider_failure_ref_hash
        admin_operation_id = "admop-provider-replay-1"
        admin_operation_payload_hash = $payloadHash
        workflow_id = "workflow-provider-replay-1"
        workflow_step_id = "workflow-step-1"
        workflow_decision_manifest_sha256 = Get-CheckFileSha256Ref $decisionPath
        proposal_id = "proposal-fresh-1"
        approval_id = "approval-fresh-1"
        prepared_audit_id = "audit-fresh-1"
        skill_id = "skill.demo"
        tool_name = "nexusim.local.echo"
        resource_type = "conversation"
        resource_id_hash = "resourcehash"
        new_input_sha256 = Get-CheckSha256Ref "fresh input only stored outside this manifest"
        reason_sha256 = Get-CheckSha256Ref "operator reason only stored outside this manifest"
        policy_check_ref = "policy-check:provider-replay"
        prepared_audit_ref = "prepared-audit:provider-replay"
        generated_by = "operator:alice"
        correlation_id = "corr-provider-replay"
        trace_id = "trace-provider-replay"
        direct_execution_allowed = $false
        source_dlq_immutable = $true
        raw_input_present = $false
        raw_reason_present = $false
        raw_provider_artifact_present = $false
    }
    Write-JsonFile -Path $proofPath -Value $proof

    return [ordered]@{
        HandoffPath = $handoffPath
        AdminPath = $adminPath
        DecisionPath = $decisionPath
        ProofPath = $proofPath
        ManifestPath = (Join-Path $Root "provider-replay-redrive-invocation.json")
        Admin = $admin
        Decision = $decision
        Proof = $proof
    }
}

function Invoke-Writer {
    param(
        [hashtable]$Fixture,
        [string]$ManifestPath
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -HandoffPath $Fixture.HandoffPath `
        -AdminOperationPath $Fixture.AdminPath `
        -WorkflowDecisionManifestPath $Fixture.DecisionPath `
        -FreshProofPath $Fixture.ProofPath `
        -GeneratedBy "operator-alice" `
        -OutputPath $ManifestPath `
        -ManifestID "provider-replay-redrive-invocation-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-provider-replay-redrive-invocation-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $fixture = New-Fixture -Root $tempRoot
    Invoke-Writer -Fixture $fixture -ManifestPath $fixture.ManifestPath
    $manifestRaw = Get-Content -LiteralPath $fixture.ManifestPath -Raw
    $manifest = $manifestRaw | ConvertFrom-Json

    if ($manifest.schema_version -ne "nexusim.action_executor.provider_replay_redrive_invocation.v1" -or
        $manifest.manifest_id -ne "provider-replay-redrive-invocation-1" -or
        $manifest.entrypoint -ne "RedriveProviderFailure" -or
        $manifest.rpc_full_method -ne "/nexusim.actionexecutor.v1.ActionExecutorService/RedriveProviderFailure" -or
        $manifest.executes_redrive -ne $false -or
        $manifest.mutates_provider_failure -ne $false -or
        $manifest.direct_execution_allowed -ne $false -or
        $manifest.requires_operator_execution -ne $true -or
        $manifest.operator_must_supply_raw_resource_id_and_new_input_outside_manifest -ne $true) {
        throw "provider replay redrive invocation manifest has unexpected top-level fields."
    }
    if ($manifest.provider_failure_id -ne "provider-failure-1" -or
        $manifest.replay_candidate_id -ne "provider-replay-candidate-1234" -or
        $manifest.admin_operation_id -ne "admop-provider-replay-1" -or
        $manifest.proposal_id -ne "proposal-fresh-1" -or
        $manifest.approval_id -ne "approval-fresh-1" -or
        $manifest.prepared_audit_id -ne "audit-fresh-1" -or
        $manifest.resource_id_hash -ne "resourcehash") {
        throw "provider replay redrive invocation manifest did not bind the expected low-sensitive refs."
    }
    if ($manifest.redrive_request_contract.provider_failure_id -ne "provider-failure-1" -or
        $manifest.redrive_request_contract.input_json_source -notlike "*operator-supplied-outside-manifest*" -or
        $manifest.redrive_request_contract.resource_id_source -notlike "*operator-supplied-outside-manifest*" -or
        $manifest.redrive_request_contract.new_input_sha256 -ne $manifest.new_input_sha256) {
        throw "provider replay redrive invocation request contract is incomplete."
    }
    foreach ($expected in @(
        "admin_operation_approved",
        "workflow_approval_recorded",
        "fresh_agent_proposal",
        "new_input_sha256_matches_external_file",
        "action_executor_redrive_provider_failure_only"
    )) {
        if (@($manifest.required_checks) -notcontains $expected) {
            throw "provider replay redrive invocation manifest missing check: $expected"
        }
    }
    foreach ($forbidden in @(
        $tempRoot,
        $fixture.HandoffPath,
        $fixture.AdminPath,
        $fixture.DecisionPath,
        $fixture.ProofPath,
        "fresh input only stored outside this manifest",
        "operator reason only stored outside this manifest",
        "provider_body",
        "raw:",
        "secret",
        "password"
    )) {
        if ($manifestRaw -like "*$forbidden*") {
            throw "provider replay redrive invocation manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-provider-replay-redrive-invocation.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -Fixture $fixture -ManifestPath $repoLocalOutput
    }

    $adminRejected = $fixture.Admin
    $adminRejected.operation.status = "SUBMITTED"
    Write-JsonFile -Path $fixture.AdminPath -Value $adminRejected
    Invoke-ExpectFailure -Expected "must be APPROVED" -Script {
        Invoke-Writer -Fixture $fixture -ManifestPath $fixture.ManifestPath
    }
    $adminRejected.operation.status = "APPROVED"
    Write-JsonFile -Path $fixture.AdminPath -Value $adminRejected

    $proof = $fixture.Proof
    $proof.approval_id = "approval-source-1"
    Write-JsonFile -Path $fixture.ProofPath -Value $proof
    Invoke-ExpectFailure -Expected "must not reuse source approval" -Script {
        Invoke-Writer -Fixture $fixture -ManifestPath $fixture.ManifestPath
    }
    $proof.approval_id = "approval-fresh-1"
    Write-JsonFile -Path $fixture.ProofPath -Value $proof

    $proof.direct_execution_allowed = $true
    Write-JsonFile -Path $fixture.ProofPath -Value $proof
    Invoke-ExpectFailure -Expected "direct_execution_allowed must be false" -Script {
        Invoke-Writer -Fixture $fixture -ManifestPath $fixture.ManifestPath
    }
    $proof.direct_execution_allowed = $false
    Write-JsonFile -Path $fixture.ProofPath -Value $proof

    $decision = $fixture.Decision
    $decision.decision = "REJECT"
    Write-JsonFile -Path $fixture.DecisionPath -Value $decision
    $proof.workflow_decision_manifest_sha256 = Get-CheckFileSha256Ref $fixture.DecisionPath
    Write-JsonFile -Path $fixture.ProofPath -Value $proof
    Invoke-ExpectFailure -Expected "must APPROVE" -Script {
        Invoke-Writer -Fixture $fixture -ManifestPath $fixture.ManifestPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-provider-replay-redrive-invocation.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   provider replay redrive invocation manifest self-test"
