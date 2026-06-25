$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-provider-replay-redrive-result-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing provider replay redrive result manifest writer: $writerPath"
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

function New-InvocationFixture {
    return [ordered]@{
        schema_version = "nexusim.action_executor.provider_replay_redrive_invocation.v1"
        manifest_id = "provider-replay-redrive-invocation-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        entrypoint = "RedriveProviderFailure"
        rpc_full_method = "/nexusim.actionexecutor.v1.ActionExecutorService/RedriveProviderFailure"
        executes_redrive = $false
        mutates_provider_failure = $false
        source_dlq_immutable = $true
        direct_execution_allowed = $false
        requires_operator_execution = $true
        provider_failure_id = "provider-failure-1"
        provider_failure_ref_hash = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
        replay_candidate_id = "provider-replay-candidate-1"
        admin_operation_id = "admop-provider-replay-1"
        admin_operation_payload_hash = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
        workflow_id = "workflow-provider-replay-1"
        workflow_step_id = "workflow-step-provider-replay-1"
        workflow_decision_manifest_sha256 = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
        proposal_id = "proposal-fresh-1"
        approval_id = "approval-fresh-1"
        prepared_audit_id = "audit-fresh-1"
        skill_id = "skill.demo"
        tool_name = "nexusim.local.echo"
        action = "EXECUTE"
        resource_type = "conversation"
        resource_id_hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        new_input_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        reason_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
        auth_context_contract = [ordered]@{
            tenant_id = "tenant-demo"
            user_id = "operator-supplied-outside-manifest"
            device_id = "operator-supplied-outside-manifest"
            trace_id = "trace-redrive-1"
            request_id = "operator-generated-at-execution-time"
        }
        redrive_request_contract = [ordered]@{
            provider_failure_id = "provider-failure-1"
            reason_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
            proposal_id = "proposal-fresh-1"
            approval_id = "approval-fresh-1"
            prepared_audit_id = "audit-fresh-1"
            skill_id = "skill.demo"
            tool_name = "nexusim.local.echo"
            action = "EXECUTE"
            resource_type = "conversation"
            resource_id_source = "operator-supplied-outside-manifest"
            resource_id_hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            risk_level = "LOW"
            intent = "provider failure redrive after approved repair workflow"
            input_json_source = "operator-supplied-outside-manifest"
            new_input_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
            idempotency_key_hint = "provider-replay-redrive:provider-replay-candidate-1:approval-fresh-1"
        }
        required_checks = @(
            "admin_operation_approved",
            "workflow_approval_recorded",
            "fresh_agent_proposal",
            "fresh_agent_approval",
            "fresh_prepared_audit",
            "matching_skill_tool_resource",
            "new_input_sha256_matches_external_file",
            "reason_sha256_matches_external_file",
            "resource_id_hash_matches_operator_supplied_resource",
            "action_executor_redrive_provider_failure_only"
        )
        forbidden_contents = @(
            "raw_provider_input",
            "raw_provider_output",
            "raw_new_input",
            "raw_reason"
        )
    }
}

function New-ExecutionFixture {
    return [ordered]@{
        mode = "provider-replay-redrive"
        target = "127.0.0.1:10660"
        manifest_id = "provider-replay-redrive-invocation-1"
        replay_candidate_id = "provider-replay-candidate-1"
        provider_failure_id = "provider-failure-1"
        admin_operation_id = "admop-provider-replay-1"
        workflow_id = "workflow-provider-replay-1"
        request = [ordered]@{
            tenant_id = "tenant-demo"
            user_id = "operator-a"
            device_id = "operator-device-a"
            proposal_id = "proposal-fresh-1"
            approval_id = "approval-fresh-1"
            prepared_audit_id = "audit-fresh-1"
            skill_id = "skill.demo"
            tool_name = "nexusim.local.echo"
            resource_type = "conversation"
            resource_id_hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            input_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
            reason_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
            idempotency_key = "provider-replay-redrive:provider-replay-candidate-1:approval-fresh-1"
        }
        response = [ordered]@{
            provider_failure_id = "provider-failure-1"
            source_execution_id = "execution-source-1"
            source_result_id = "result-source-1"
            redrive_execution_id = "execution-redrive-1"
            redrive_result_id = "result-redrive-1"
            proposal_id = "proposal-fresh-1"
            approval_id = "approval-fresh-1"
            prepared_audit_id = "audit-fresh-1"
            skill_id = "skill.demo"
            tool_name = "nexusim.local.echo"
            resource_type = "conversation"
            resource_id_hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            status = "ACTION_EXECUTION_STATUS_SUCCEEDED"
            result_status = "SUCCEEDED"
            executed = $true
            classification = "TOOL_ALLOWED"
            result_ref = "action-executor://executions/execution-redrive-1/results/result-redrive-1"
        }
        executed_redrive = $true
        verified = @(
            "manifest_contract_valid",
            "operator_raw_resource_id_hash_matches",
            "operator_new_input_sha256_matches",
            "operator_reason_sha256_matches",
            "request_targets_action_executor_redrive_provider_failure"
        )
        checked_at = "2026-06-25T00:00:00Z"
    }
}

function Invoke-Writer {
    param(
        [string]$InvocationPath,
        [string]$ExecutionResultPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$ResultManifestID = "provider-replay-redrive-result-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -InvocationPath $InvocationPath `
        -ExecutionResultPath $ExecutionResultPath `
        -GeneratedBy $GeneratedBy `
        -OutputPath $OutputPath `
        -ResultManifestID $ResultManifestID 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-provider-replay-redrive-result-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $invocationPath = Join-Path $tempRoot "provider-replay-redrive-invocation.json"
    $executionPath = Join-Path $tempRoot "provider-replay-redrive-execution.json"
    $resultPath = Join-Path $tempRoot "provider-replay-redrive-result-manifest.json"
    Write-JsonFile -Path $invocationPath -Value (New-InvocationFixture)
    Write-JsonFile -Path $executionPath -Value (New-ExecutionFixture)

    Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $executionPath -OutputPath $resultPath

    $raw = Get-Content -LiteralPath $resultPath -Raw
    $result = $raw | ConvertFrom-Json
    if ($result.schema_version -ne "nexusim.action_executor.provider_replay_redrive_result.v1" -or
        $result.result_manifest_id -ne "provider-replay-redrive-result-1" -or
        $result.manifest_is_execution -ne $false -or
        $result.executes_redrive -ne $false -or
        $result.mutates_provider_failure -ne $false -or
        $result.appends_audit -ne $false -or
        $result.source_dlq_immutable -ne $true -or
        $result.invocation.manifest_id -ne "provider-replay-redrive-invocation-1" -or
        $result.execution_request.new_input_sha256 -ne "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" -or
        $result.execution_result.redrive_execution_id -ne "execution-redrive-1" -or
        $result.execution_result.redrive_result_id -ne "result-redrive-1" -or
        $result.execution_result.executed -ne $true) {
        throw "provider replay redrive result manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_invocation_manifest_verified",
            "execution_summary_matches_invocation",
            "action_executor_reported_executed_redrive",
            "redrive_result_ref_present_or_status_recorded",
            "source_dlq_remains_immutable",
            "operator_must_append_external_audit_separately_if_needed"
        )) {
        if (@($result.required_checks) -notcontains $expected) {
            throw "provider replay redrive result manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "result_manifest_is_not_redrive_execution",
            "does_not_call_action_executor",
            "does_not_append_audit_record",
            "does_not_modify_provider_failure_or_dlq",
            "does_not_create_admin_or_workflow_decision"
        )) {
        if (@($result.execution_boundary) -notcontains $expected) {
            throw "provider replay redrive result manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $invocationPath,
            $executionPath,
            $resultPath,
            "provider_body",
            "EvidencePack",
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "provider replay redrive result manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-provider-replay-redrive-result.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $executionPath -OutputPath $repoLocalOutput
    }

    $badNoExecute = New-ExecutionFixture
    $badNoExecute.executed_redrive = $false
    $badNoExecutePath = Join-Path $tempRoot "bad-no-execute.json"
    Write-JsonFile -Path $badNoExecutePath -Value $badNoExecute
    Invoke-ExpectFailure -Expected "executed_redrive=true" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $badNoExecutePath -OutputPath $resultPath
    }

    $badManifestID = New-ExecutionFixture
    $badManifestID.manifest_id = "provider-replay-redrive-invocation-other"
    $badManifestIDPath = Join-Path $tempRoot "bad-manifest-id.json"
    Write-JsonFile -Path $badManifestIDPath -Value $badManifestID
    Invoke-ExpectFailure -Expected "execution.manifest_id mismatch" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $badManifestIDPath -OutputPath $resultPath
    }

    $badInput = New-ExecutionFixture
    $badInput.request.input_sha256 = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
    $badInputPath = Join-Path $tempRoot "bad-input.json"
    Write-JsonFile -Path $badInputPath -Value $badInput
    Invoke-ExpectFailure -Expected "request.input_sha256 mismatch" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $badInputPath -OutputPath $resultPath
    }

    $missingResponse = New-ExecutionFixture
    $missingResponse.Remove("response")
    $missingResponsePath = Join-Path $tempRoot "missing-response.json"
    Write-JsonFile -Path $missingResponsePath -Value $missingResponse
    Invoke-ExpectFailure -Expected "execution.response is required" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $missingResponsePath -OutputPath $resultPath
    }

    $badResource = New-ExecutionFixture
    $badResource.response.resource_id_hash = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
    $badResourcePath = Join-Path $tempRoot "bad-resource.json"
    Write-JsonFile -Path $badResourcePath -Value $badResource
    Invoke-ExpectFailure -Expected "response.resource_id_hash mismatch" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $badResourcePath -OutputPath $resultPath
    }

    $badRaw = New-ExecutionFixture
    $badRaw.response.provider_body = "provider raw body"
    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-Writer -InvocationPath $invocationPath -ExecutionResultPath $badRawPath -OutputPath $resultPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-provider-replay-redrive-result.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   provider replay redrive result manifest self-test"
