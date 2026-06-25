$ErrorActionPreference = "Stop"

$pageWriterPath = Join-Path $PSScriptRoot "write-provider-replay-handoff-review-page.ps1"
if (-not (Test-Path -LiteralPath $pageWriterPath -PathType Leaf)) {
    throw "Missing provider replay handoff review page writer: $pageWriterPath"
}

function Get-TestSha256 {
    param([string]$Text)

    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes($Text)
        $hash = $sha.ComputeHash($bytes)
        return -join ($hash | ForEach-Object { $_.ToString("x2") })
    } finally {
        $sha.Dispose()
    }
}

function ConvertTo-TestCanonicalJson {
    param([object]$Object)

    if ($null -eq $Object) {
        return "null"
    }
    if ($Object -is [bool]) {
        if ($Object) { return "true" }
        return "false"
    }
    if ($Object -is [int] -or $Object -is [long] -or $Object -is [double] -or $Object -is [decimal]) {
        return [string]$Object
    }
    if ($Object -is [string]) {
        return ($Object | ConvertTo-Json -Compress)
    }
    if ($Object -is [System.Collections.IEnumerable] -and -not ($Object -is [string]) -and $null -eq $Object.PSObject.Properties["Keys"]) {
        $items = @()
        foreach ($item in @($Object)) {
            $items += ConvertTo-TestCanonicalJson -Object $item
        }
        return "[" + ($items -join ",") + "]"
    }

    $properties = @($Object.PSObject.Properties | Sort-Object Name)
    $pairs = @()
    foreach ($property in $properties) {
        $encodedName = ([string]$property.Name | ConvertTo-Json -Compress)
        $encodedValue = ConvertTo-TestCanonicalJson -Object $property.Value
        $pairs += "$encodedName`:$encodedValue"
    }
    return "{" + ($pairs -join ",") + "}"
}

function Invoke-WriterExpectFailure {
    param(
        [string]$HandoffPath,
        [string]$OutputPath,
        [string]$FailureName
    )

    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
            -HandoffPath $HandoffPath `
            -GeneratedBy "operator-a" `
            -PageID "provider-replay-handoff-review-page-1" `
            -OutputPath $OutputPath 2>$null | Out-Null
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($exitCode -eq 0) {
        throw "$FailureName should have failed."
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-provider-replay-handoff-review-page-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $leakMarker = "do-not-leak-provider-replay-raw-body"
    $payload = [ordered]@{
        provider_failure_ref_hash = "sha256:provider-failure"
        source_execution_ref_hash = "sha256:execution"
        source_result_ref_hash = "sha256:result"
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
    $payloadHash = "sha256:" + (Get-TestSha256 -Text (ConvertTo-TestCanonicalJson -Object ([pscustomobject]$payload)))

    $handoff = [ordered]@{
        kind = "action-executor.provider-failure.replay-admin-workflow-handoff"
        debug_note = $leakMarker
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
                "new_input_json",
                "reason_sha256",
                "action_executor_redrive_entrypoint"
            )
        }
        admin_operation_requests = @(
            [ordered]@{
                auth_tenant_id = "tenant-provider-replay"
                operator_ref = "operator:alice"
                operator_role = "OPERATOR"
                operation_type = "PROVIDER_REPLAY_REQUEST"
                target_ref_hash = "sha256:provider-failure"
                risk_level = "HIGH"
                payload_schema_version = "admin.provider_replay_request.v1"
                operation_payload = $payload
                operation_payload_hash = $payloadHash
                reason_ref = "reason:provider-replay"
                evidence_refs = @("evidence:provider-failure")
                idempotency_key = "provider-replay-admin:provider-replay-candidate-1234"
                correlation_id = "corr-provider-replay"
                causation_id = "provider-failure-1"
                trace_id = "trace-provider-replay"
                expected_workflow_policy = "admin.workflow.provider_replay.v1"
            }
        )
        workflow_handoff_requests = @(
            [ordered]@{
                workflow_type = "REPAIR_APPROVAL"
                requester_service = "admin-service"
                target_service = "action-executor"
                target_operation = "PROVIDER_REPLAY_REQUEST"
                risk_level = "HIGH"
                target_ref_hash = "sha256:provider-failure"
                payload_schema_version = "admin.provider_replay_request.v1"
                payload_ref_hash = $payloadHash
                approval_policy_ref = "admin.workflow.provider_replay.v1"
                reason_ref = "reason:provider-replay"
                evidence_refs = @("evidence:provider-failure")
                idempotency_key = 'admin-workflow:${operation_id}'
                correlation_id = "corr-provider-replay"
                causation_id = '${operation_id}'
                trace_id = "trace-provider-replay"
            }
        )
        rows = @(
            [ordered]@{
                replay_candidate_id = "provider-replay-candidate-1234"
                replay_state = "AWAITING_ADMIN_WORKFLOW"
                tenant_id = "tenant-provider-replay"
                provider_failure_id = "provider-failure-1"
                execution_id = "exec-1"
                result_id = "result-1"
                proposal_id = "proposal-1"
                approval_id = "approval-1"
                prepared_audit_id = "audit-1"
                user_id_hash = "hash-user"
                skill_id = "skill-1"
                tool_name = "tool-1"
                resource_type = "conversation"
                resource_id_hash = "hash-resource"
                classification = "PROVIDER_UNAVAILABLE"
                status = "DLQ"
                retryable = $false
                retry_count = 3
                failure_ref_hash = "sha256:failure-ref"
                created_at = "2026-06-25T00:00:00Z"
            }
        )
    }

    $handoffPath = Join-Path $tempRoot "provider-replay-handoff.json"
    $pagePath = Join-Path $tempRoot "provider-replay-handoff-review.html"
    ($handoff | ConvertTo-Json -Depth 20) | Set-Content -LiteralPath $handoffPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
        -HandoffPath $handoffPath `
        -GeneratedBy "operator-a" `
        -PageID "provider-replay-handoff-review-page-1" `
        -OutputPath $pagePath
    if ($LASTEXITCODE -ne 0) {
        throw "write-provider-replay-handoff-review-page.ps1 failed"
    }

    $html = Get-Content -LiteralPath $pagePath -Raw
    foreach ($expected in @(
            "NexusIM Provider Replay Handoff Review",
            "PROVIDER_REPLAY_REQUEST",
            "REPAIR_APPROVAL",
            "RedriveProviderFailure",
            "provider-replay-candidate-1234",
            "admin.workflow.provider_replay.v1",
            "does_not_call_redrive_provider_failure",
            "handoff_sha256",
            "handoff_path_sha256"
        )) {
        if (-not $html.Contains($expected)) {
            throw "provider replay handoff review page missing expected low-sensitive content: $expected"
        }
    }

    foreach ($forbidden in @(
            $handoffPath,
            $pagePath,
            $tempRoot,
            $leakMarker,
            "raw-body",
            "debug_note",
            "provider_body"
        )) {
        if ($html.Contains($forbidden)) {
            throw "provider replay handoff review page leaked sensitive or local artifact content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-provider-replay-handoff-review.html"
    Invoke-WriterExpectFailure -HandoffPath $handoffPath -OutputPath $repoLocalOutput -FailureName "repository-local OutputPath"

    $badDirect = $handoff | ConvertTo-Json -Depth 20 | ConvertFrom-Json
    $badDirect.handoff_contract.direct_execution_allowed = $true
    $badDirectPath = Join-Path $tempRoot "bad-direct.json"
    ($badDirect | ConvertTo-Json -Depth 20) | Set-Content -LiteralPath $badDirectPath -Encoding UTF8
    Invoke-WriterExpectFailure -HandoffPath $badDirectPath -OutputPath (Join-Path $tempRoot "bad-direct.html") -FailureName "direct execution contract"

    $badHash = $handoff | ConvertTo-Json -Depth 20 | ConvertFrom-Json
    $badHash.admin_operation_requests[0].operation_payload_hash = "sha256:wrong"
    $badHashPath = Join-Path $tempRoot "bad-hash.json"
    ($badHash | ConvertTo-Json -Depth 20) | Set-Content -LiteralPath $badHashPath -Encoding UTF8
    Invoke-WriterExpectFailure -HandoffPath $badHashPath -OutputPath (Join-Path $tempRoot "bad-hash.html") -FailureName "payload hash mismatch"
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   provider replay handoff review page self-test"
