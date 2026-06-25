$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$writerPath = Join-Path $PSScriptRoot "write-provider-replay-redrive-audit-append-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing provider replay redrive audit append manifest writer: $writerPath"
}

function Write-JsonFile {
    param(
        [string]$Path,
        [object]$Value
    )
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 30), $utf8NoBom)
}

function Invoke-ExpectFailure {
    param(
        [string]$Expected,
        [scriptblock]$Script
    )
    try {
        & $Script
    } catch {
        if ($_.Exception.Message -like "*$Expected*") {
            return
        }
        throw "Expected failure containing '$Expected', got: $($_.Exception.Message)"
    }
    throw "Expected failure containing '$Expected', but command succeeded."
}

function New-ResultManifestFixture {
    param(
        [string]$ResultManifestID = "provider-replay-redrive-result-1"
    )

    return [ordered]@{
        schema_version = "nexusim.action_executor.provider_replay_redrive_result.v1"
        result_manifest_id = $ResultManifestID
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        source_invocation_sha256 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
        source_execution_summary_sha256 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
        source_invocation_path_sha256 = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
        source_execution_summary_path_sha256 = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
        manifest_is_execution = $false
        executes_redrive = $false
        mutates_provider_failure = $false
        appends_audit = $false
        source_dlq_immutable = $true
        invocation = [ordered]@{
            manifest_id = "provider-replay-redrive-invocation-1"
            provider_failure_id = "provider-failure-1"
            replay_candidate_id = "provider-replay-candidate-1"
            admin_operation_id = "admop-provider-replay-1"
            workflow_id = "workflow-provider-replay-1"
            workflow_step_id = "workflow-step-provider-replay-1"
            proposal_id = "proposal-fresh-1"
            approval_id = "approval-fresh-1"
            prepared_audit_id = "audit-fresh-1"
            skill_id = "skill.demo"
            tool_name = "nexusim.local.echo"
            resource_type = "conversation"
            resource_id_hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            new_input_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
            reason_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
            trace_id = "trace-redrive-1"
        }
        execution_request = [ordered]@{
            tenant_id = "tenant-demo"
            operator_user_ref = "operator-a"
            operator_device_ref = "operator-device-a"
            proposal_id = "proposal-fresh-1"
            approval_id = "approval-fresh-1"
            prepared_audit_id = "audit-fresh-1"
            skill_id = "skill.demo"
            tool_name = "nexusim.local.echo"
            resource_type = "conversation"
            resource_id_hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
            new_input_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
            reason_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
            idempotency_key = "provider-replay-redrive:provider-replay-candidate-1:approval-fresh-1"
        }
        execution_result = [ordered]@{
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
            response_reason_sha256 = ""
        }
        required_checks = @(
            "source_invocation_manifest_verified",
            "execution_summary_matches_invocation",
            "action_executor_reported_executed_redrive",
            "redrive_result_ref_present_or_status_recorded",
            "source_dlq_remains_immutable",
            "operator_must_append_external_audit_separately_if_needed"
        )
        forbidden_contents = @(
            "payload_material",
            "provider_artifact_material",
            "operator_reason_material",
            "evidence_text_material",
            "filesystem_path_material",
            "auth_material"
        )
        execution_boundary = @(
            "result_manifest_is_not_redrive_execution",
            "does_not_call_action_executor",
            "does_not_append_audit_record",
            "does_not_modify_provider_failure_or_dlq",
            "does_not_create_admin_or_workflow_decision"
        )
        note = "Low-sensitive provider replay redrive result manifest."
    }
}

function Invoke-Writer {
    param(
        [string]$ResultManifestPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$AuditManifestID = "action-executor-audit-append-1",
        [string]$AuditRecordID = "action-executor:audit:execution-redrive-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -ResultManifestPath $ResultManifestPath `
        -GeneratedBy $GeneratedBy `
        -OutputPath $OutputPath `
        -AuditManifestID $AuditManifestID `
        -AuditRecordID $AuditRecordID `
        -OccurredAt "2026-06-25T03:00:00Z" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

function Invoke-AuditAppendPreflight {
    param([string]$AuditManifestPath)

    Push-Location $repoRoot
    try {
        $output = & go run ./loadtest/actionexecutor `
            -mode external-audit-append `
            -audit-manifest $AuditManifestPath `
            -operator-user-id operator-user `
            -operator-device-id operator-device `
            -request-id request-provider-replay-audit-append 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw (($output | Out-String).Trim())
        }
        $output | Out-Host
    } finally {
        Pop-Location
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-provider-replay-audit-append-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $resultPath = Join-Path $tempRoot "provider-replay-redrive-result-manifest.json"
    $auditPath = Join-Path $tempRoot "provider-replay-redrive-audit-append.json"
    Write-JsonFile -Path $resultPath -Value (New-ResultManifestFixture)

    Invoke-Writer -ResultManifestPath $resultPath -OutputPath $auditPath

    $raw = Get-Content -LiteralPath $auditPath -Raw
    $audit = $raw | ConvertFrom-Json
    if ($audit.schema_version -ne "nexusim.action_executor.external_audit_append.v1" -or
        $audit.manifest_id -ne "action-executor-audit-append-1" -or
        $audit.source_manifest_id -ne "provider-replay-redrive-result-1" -or
        $audit.executes_append -ne $false -or
        $audit.mutates_audit_service -ne $false -or
        $audit.direct_append_allowed -ne $false -or
        $audit.requires_operator_execution -ne $true -or
        $audit.source_service -ne "action-executor" -or
        $audit.source_event_id -ne "execution-redrive-1" -or
        $audit.record_type -ne "ACTION_PROVIDER_REDRIVE" -or
        $audit.action -ne "REDRIVE_PROVIDER_FAILURE" -or
        $audit.attributes_json.proposal_id -ne "proposal-fresh-1" -or
        $audit.attributes_json.payload_schema_version -ne "nexusim.action_executor.provider_replay_redrive_result.v1") {
        throw "provider replay audit append manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_execution_audit_low_sensitive",
            "no_raw_provider_artifacts",
            "audit_service_append_only",
            "idempotency_key_present",
            "provider_replay_result_manifest_verified",
            "source_result_manifest_bound_by_sha256"
        )) {
        if (@($audit.required_checks) -notcontains $expected) {
            throw "provider replay audit append manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "audit_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_call_action_executor",
            "does_not_modify_provider_failure_or_dlq",
            "does_not_create_admin_or_workflow_decision"
        )) {
        if (@($audit.execution_boundary) -notcontains $expected) {
            throw "provider replay audit append manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $resultPath,
            $auditPath,
            '"provider_body":',
            '"provider_error_body":',
            '"raw_provider_input":',
            '"raw_provider_output":',
            "EvidencePack",
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "provider replay audit append manifest leaked forbidden content: $forbidden"
        }
    }

    Invoke-AuditAppendPreflight -AuditManifestPath $auditPath

    $repoLocalOutput = Join-Path $repoRoot "tmp-provider-replay-audit-append.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -ResultManifestPath $resultPath -OutputPath $repoLocalOutput
    }

    $alreadyAppended = New-ResultManifestFixture
    $alreadyAppended.appends_audit = $true
    $alreadyAppendedPath = Join-Path $tempRoot "bad-appends-audit.json"
    Write-JsonFile -Path $alreadyAppendedPath -Value $alreadyAppended
    Invoke-ExpectFailure -Expected "already appended" -Script {
        Invoke-Writer -ResultManifestPath $alreadyAppendedPath -OutputPath $auditPath
    }

    $notExecuted = New-ResultManifestFixture
    $notExecuted.execution_result.executed = $false
    $notExecutedPath = Join-Path $tempRoot "bad-not-executed.json"
    Write-JsonFile -Path $notExecutedPath -Value $notExecuted
    Invoke-ExpectFailure -Expected "execution_result.executed must be true" -Script {
        Invoke-Writer -ResultManifestPath $notExecutedPath -OutputPath $auditPath
    }

    $requestMismatch = New-ResultManifestFixture
    $requestMismatch.execution_request.new_input_sha256 = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
    $requestMismatchPath = Join-Path $tempRoot "bad-request-mismatch.json"
    Write-JsonFile -Path $requestMismatchPath -Value $requestMismatch
    Invoke-ExpectFailure -Expected "execution_request.new_input_sha256 mismatch" -Script {
        Invoke-Writer -ResultManifestPath $requestMismatchPath -OutputPath $auditPath
    }

    $missingCheck = New-ResultManifestFixture
    $missingCheck.required_checks = @("source_invocation_manifest_verified")
    $missingCheckPath = Join-Path $tempRoot "bad-missing-check.json"
    Write-JsonFile -Path $missingCheckPath -Value $missingCheck
    Invoke-ExpectFailure -Expected "result.required_checks must contain execution_summary_matches_invocation" -Script {
        Invoke-Writer -ResultManifestPath $missingCheckPath -OutputPath $auditPath
    }

    $badRaw = New-ResultManifestFixture
    $badRaw.execution_result.provider_body = "provider raw body"
    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-Writer -ResultManifestPath $badRawPath -OutputPath $auditPath
    }

    $badAudit = $audit
    $badAudit.attributes_json.status = "FAILED"
    $badAuditPath = Join-Path $tempRoot "bad-audit-hash.json"
    Write-JsonFile -Path $badAuditPath -Value $badAudit
    Invoke-ExpectFailure -Expected "attributes_json sha256" -Script {
        Invoke-AuditAppendPreflight -AuditManifestPath $badAuditPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path $repoRoot "tmp-provider-replay-audit-append.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   provider replay redrive audit append manifest self-test"
