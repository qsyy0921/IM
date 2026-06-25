$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$writerPath = Join-Path $PSScriptRoot "write-workflow-approval-queue-decision-audit-append-result-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow approval decision audit append result manifest writer: $writerPath"
}

function Write-JsonFile {
    param([string]$Path, [object]$Value)
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 40), $utf8NoBom)
}

function Invoke-ExpectFailure {
    param([scriptblock]$Script, [string]$Expected)
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

function Get-StringSha256Ref {
    param([string]$Value)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($Value))
    } finally {
        $sha.Dispose()
    }
    return "sha256:" + (-join ($hash | ForEach-Object { $_.ToString("x2") }))
}

function ConvertTo-CanonicalFlatJson {
    param([hashtable]$Values)
    $parts = @()
    foreach ($key in @($Values.Keys | Sort-Object)) {
        $encodedKey = ConvertTo-Json ([string]$key) -Compress
        $encodedValue = ConvertTo-Json ([string]$Values[$key]) -Compress
        $parts += ("{0}:{1}" -f $encodedKey, $encodedValue)
    }
    return "{" + ($parts -join ",") + "}"
}

function New-AuditManifestFixture {
    $attributes = [ordered]@{
        batch_decision_id = "workflow-approval-queue-batch-decision-1"
        decision_count = "2"
        decision_histogram_sha256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        decision_owner = "workflow-service.RecordWorkflowDecision"
        decision_refs_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        event_type = "WORKFLOW_APPROVAL_QUEUE_BATCH_DECISION"
        result_manifest_id = "workflow-approval-queue-batch-decision-result-1"
        review_page_id = "workflow-approval-queue-decision-result-review-1"
        review_summary_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
        source_result_manifest_sha256 = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
    }
    $attributesJSON = ConvertTo-CanonicalFlatJson -Values $attributes
    return [ordered]@{
        schema_version = "nexusim.audit.external_append.v1"
        manifest_id = "workflow-approval-decision-audit-append-1"
        source_manifest_id = "workflow-approval-queue-decision-result-review-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        executes_append = $false
        mutates_audit_service = $false
        direct_append_allowed = $false
        requires_operator_execution = $true
        audit_stream = "security"
        source_service = "workflow-service"
        source_event_id = "workflow-approval-queue-batch-decision-1"
        record_type = "WORKFLOW_APPROVAL_BATCH_DECISION"
        actor_ref = "service:workflow-service"
        subject_ref = "tenant:tenant-workflow"
        resource_ref = "workflow-approval-batch:workflow-approval-queue-batch-decision-1"
        action = "RECORD_WORKFLOW_DECISIONS"
        outcome = "RECORDED"
        reason_code = "WORKFLOW_APPROVAL_QUEUE_BATCH_DECISION_RECORDED"
        risk_level = "HIGH"
        occurred_at_unix_ms = 3000
        attributes_json = ($attributesJSON | ConvertFrom-Json)
        attributes_sha256 = Get-StringSha256Ref -Value $attributesJSON
        idempotency_key = "workflow-service:audit:workflow-approval-queue-batch-decision-1"
        correlation_id = "workflow-approval-queue-batch-decision-1"
        causation_id = "workflow-approval-queue-batch-decision-result-1"
        trace_id = ""
        auth_context_contract = [ordered]@{
            tenant_id = "tenant-workflow"
            trace_id = ""
        }
        source_review_summary_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
        source_review_summary_path_sha256 = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
        execution_boundary = @(
            "audit_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_record_workflow_decision",
            "does_not_call_action_executor",
            "does_not_execute_target_action",
            "does_not_mutate_workflow_fact"
        )
        required_checks = @(
            "source_decision_result_review_summary_verified",
            "workflow_service_recorded_decisions",
            "review_page_was_read_only",
            "action_executor_not_called",
            "target_action_not_executed",
            "audit_service_append_only",
            "idempotency_key_present"
        )
        forbidden_contents = @("raw_payload", "payload_body", "provider_body")
    }
}

function New-AuditAppendResultFixture {
    $manifest = New-AuditManifestFixture
    return [ordered]@{
        mode = "external-audit-append"
        audit_target = "127.0.0.1:10700"
        manifest_id = $manifest.manifest_id
        source_manifest_id = $manifest.source_manifest_id
        request = [ordered]@{
            tenant_id = "tenant-workflow"
            user_id = "operator-user"
            device_id = "operator-device"
            audit_stream = "security"
            source_service = "workflow-service"
            source_event_id = "workflow-approval-queue-batch-decision-1"
            record_type = "WORKFLOW_APPROVAL_BATCH_DECISION"
            resource_ref = "workflow-approval-batch:workflow-approval-queue-batch-decision-1"
            action = "RECORD_WORKFLOW_DECISIONS"
            outcome = "RECORDED"
            reason_code = "WORKFLOW_APPROVAL_QUEUE_BATCH_DECISION_RECORDED"
            risk_level = "HIGH"
            attributes_sha256 = $manifest.attributes_sha256
            idempotency_key = "workflow-service:audit:workflow-approval-queue-batch-decision-1"
            correlation_id = "workflow-approval-queue-batch-decision-1"
            causation_id = "workflow-approval-queue-batch-decision-result-1"
            trace_id = ""
            occurred_at_unix_ms = 3000
        }
        response = [ordered]@{
            audit_id = "audit-workflow-approval-decision-1"
            record_hash = "record-hash-workflow-approval-decision-1"
            previous_record_hash = "previous-hash-workflow-approval-decision-1"
            idempotency_key = "workflow-service:audit:workflow-approval-queue-batch-decision-1"
        }
        executed_append = $true
        verified = @(
            "external_audit_manifest_contract_valid",
            "attributes_json_sha256_matches",
            "attributes_json_low_sensitive_keys_only",
            "audit_service_append_only"
        )
        checked_at = "2026-06-25T03:01:00Z"
    }
}

function Invoke-Writer {
    param(
        [string]$AuditManifestPath,
        [string]$AuditAppendResultPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$ResultManifestID = "workflow-approval-decision-audit-append-result-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -AuditManifestPath $AuditManifestPath `
        -AuditAppendResultPath $AuditAppendResultPath `
        -GeneratedBy $GeneratedBy `
        -OutputPath $OutputPath `
        -ResultManifestID $ResultManifestID 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-approval-decision-audit-append-result-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $auditManifestPath = Join-Path $tempRoot "workflow-approval-decision-audit-append.json"
    $auditAppendResultPath = Join-Path $tempRoot "workflow-approval-decision-audit-append-execution.json"
    $resultManifestPath = Join-Path $tempRoot "workflow-approval-decision-audit-append-result-manifest.json"
    Write-JsonFile -Path $auditManifestPath -Value (New-AuditManifestFixture)
    Write-JsonFile -Path $auditAppendResultPath -Value (New-AuditAppendResultFixture)

    Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $resultManifestPath

    $raw = Get-Content -LiteralPath $resultManifestPath -Raw
    $result = $raw | ConvertFrom-Json
    if ($result.schema_version -ne "nexusim.workflow.approval_queue_decision_audit_append_result.v1" -or
        $result.result_manifest_id -ne "workflow-approval-decision-audit-append-result-1" -or
        $result.manifest_is_execution -ne $false -or
        $result.executes_append -ne $false -or
        $result.mutates_audit_service -ne $false -or
        $result.records_decision -ne $false -or
        $result.calls_action_executor -ne $false -or
        $result.executes_target_action -ne $false -or
        $result.mutates_workflow_fact -ne $false -or
        $result.audit_append_manifest.manifest_id -ne "workflow-approval-decision-audit-append-1" -or
        $result.audit_append_result.audit_id -ne "audit-workflow-approval-decision-1" -or
        $result.audit_append_result.executed_append -ne $true) {
        throw "workflow approval decision audit append result manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_audit_append_manifest_verified",
            "audit_append_summary_matches_manifest",
            "audit_service_reported_audit_record",
            "workflow_approval_decision_result_review_bound_by_sha256",
            "workflow_approval_decision_audit_low_sensitive",
            "operator_keeps_result_manifest_external"
        )) {
        if (@($result.required_checks) -notcontains $expected) {
            throw "workflow approval decision audit append result manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "result_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_record_workflow_decision",
            "does_not_call_action_executor",
            "does_not_execute_target_action",
            "does_not_mutate_workflow_fact"
        )) {
        if (@($result.execution_boundary) -notcontains $expected) {
            throw "workflow approval decision audit append result manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $auditManifestPath,
            $auditAppendResultPath,
            $resultManifestPath,
            '"attributes_json":',
            '"provider_body":',
            '"payload_body":',
            '"raw_payload":',
            '"EvidencePack":',
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow approval decision audit append result manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-approval-decision-audit-append-result.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $repoLocalOutput
    }

    $preflightOnly = New-AuditAppendResultFixture
    $preflightOnly.executed_append = $false
    $preflightOnly.Remove("response")
    $preflightOnlyPath = Join-Path $tempRoot "bad-preflight-only.json"
    Write-JsonFile -Path $preflightOnlyPath -Value $preflightOnly
    Invoke-ExpectFailure -Expected "executed_append=true" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $preflightOnlyPath -OutputPath $resultManifestPath
    }

    $manifestMismatch = New-AuditAppendResultFixture
    $manifestMismatch.manifest_id = "workflow-approval-decision-audit-append-other"
    $manifestMismatchPath = Join-Path $tempRoot "bad-manifest-mismatch.json"
    Write-JsonFile -Path $manifestMismatchPath -Value $manifestMismatch
    Invoke-ExpectFailure -Expected "audit_result.manifest_id mismatch" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $manifestMismatchPath -OutputPath $resultManifestPath
    }

    $requestMismatch = New-AuditAppendResultFixture
    $requestMismatch.request.action = "EXECUTE_TARGET"
    $requestMismatchPath = Join-Path $tempRoot "bad-request-mismatch.json"
    Write-JsonFile -Path $requestMismatchPath -Value $requestMismatch
    Invoke-ExpectFailure -Expected "request.action mismatch" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $requestMismatchPath -OutputPath $resultManifestPath
    }

    $responseMismatch = New-AuditAppendResultFixture
    $responseMismatch.response.idempotency_key = "workflow-service:audit:other"
    $responseMismatchPath = Join-Path $tempRoot "bad-response-mismatch.json"
    Write-JsonFile -Path $responseMismatchPath -Value $responseMismatch
    Invoke-ExpectFailure -Expected "response.idempotency_key mismatch" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $responseMismatchPath -OutputPath $resultManifestPath
    }

    $badRaw = New-AuditAppendResultFixture
    $badRaw.provider_body = "provider raw body"
    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $badRawPath -OutputPath $resultManifestPath
    }

    $badAuditManifest = New-AuditManifestFixture
    $badAuditManifest.attributes_json.decision_count = "99"
    $badAuditManifestPath = Join-Path $tempRoot "bad-audit-manifest.json"
    Write-JsonFile -Path $badAuditManifestPath -Value $badAuditManifest
    Invoke-ExpectFailure -Expected "audit_manifest.attributes_json_sha256 mismatch" -Script {
        Invoke-Writer -AuditManifestPath $badAuditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $resultManifestPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $repoRoot "tmp-workflow-approval-decision-audit-append-result.json") -Force -ErrorAction SilentlyContinue
}

Push-Location $repoRoot
try {
    & go test ./loadtest/actionexecutor -run ExternalAuditAppend -count=1
    if ($LASTEXITCODE -ne 0) {
        throw "go test ./loadtest/actionexecutor -run ExternalAuditAppend failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

Write-Host "OK   workflow approval decision audit append result manifest self-test"
