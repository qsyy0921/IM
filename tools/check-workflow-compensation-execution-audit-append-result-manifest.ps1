$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$writerPath = Join-Path $PSScriptRoot "write-workflow-compensation-execution-audit-append-result-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow compensation audit append result manifest writer: $writerPath"
}

function Write-JsonFile {
    param([string]$Path, [object]$Value)
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, ($Value | ConvertTo-Json -Depth 30), $utf8NoBom)
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
        downstream_request_ref = "config-rollback:prod:quota:v1"
        downstream_service = "control-plane-service"
        event_type = "WORKFLOW_COMPENSATION_EXECUTION"
        failure_class = ""
        operation_id = "wf-comp-audit-1"
        operation_type = "CONFIG_ROLLBACK"
        payload_hash = "sha256:payload"
        payload_schema_version = "admin.config_rollback.v1"
        policy_decision_id = "admin.workflow.compensation.v1"
        reason_ref = "admin.compensation.control_plane.v1"
        source_ref = "wfc-comp-audit-1"
        status = "SUCCEEDED"
        target_ref_hash = "sha256:target"
    }
    $attributesJSON = ConvertTo-CanonicalFlatJson -Values $attributes
    return [ordered]@{
        schema_version = "nexusim.audit.external_append.v1"
        manifest_id = "workflow-compensation-audit-append-1"
        source_manifest_id = "workflow-compensation-execution-result-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        executes_append = $false
        mutates_audit_service = $false
        direct_append_allowed = $false
        requires_operator_execution = $true
        audit_stream = "security"
        source_service = "workflow-service"
        source_event_id = "wfc-comp-audit-1"
        record_type = "WORKFLOW_COMPENSATION_EXECUTION"
        actor_ref = "service:workflow-service"
        subject_ref = "workflow:wf-comp-audit-1"
        resource_ref = "workflow:wf-comp-audit-1:compensation:wfc-comp-audit-1"
        action = "EXECUTE_COMPENSATION"
        outcome = "SUCCEEDED"
        reason_code = "WORKFLOW_COMPENSATION_EXECUTED"
        risk_level = "HIGH"
        occurred_at_unix_ms = 3000
        attributes_json = ($attributesJSON | ConvertFrom-Json)
        attributes_sha256 = Get-StringSha256Ref -Value $attributesJSON
        idempotency_key = "workflow-service:audit:wfc-comp-audit-1:SUCCEEDED"
        correlation_id = "wf-comp-audit-1"
        causation_id = "workflow-compensation-execution-result-1"
        trace_id = ""
        auth_context_contract = [ordered]@{
            tenant_id = "tenant-workflow-test"
            trace_id = ""
        }
        source_result_manifest_sha256 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
        source_result_manifest_path_sha256 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
        execution_boundary = @(
            "audit_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_execute_compensation",
            "does_not_record_workflow_decision",
            "does_not_modify_workflow_or_compensation_rows",
            "does_not_call_downstream_service"
        )
        required_checks = @(
            "source_compensation_result_manifest_verified",
            "workflow_compensation_result_low_sensitive",
            "no_raw_compensation_payload",
            "compensation_status_terminal",
            "audit_service_append_only",
            "idempotency_key_present"
        )
        forbidden_contents = @("raw_payload", "payload_body", "provider_body")
    }
}

function New-AuditAppendResultFixture {
    return [ordered]@{
        mode = "external-audit-append"
        audit_target = "127.0.0.1:10700"
        manifest_id = "workflow-compensation-audit-append-1"
        source_manifest_id = "workflow-compensation-execution-result-1"
        request = [ordered]@{
            tenant_id = "tenant-workflow-test"
            user_id = "operator-user"
            device_id = "operator-device"
            audit_stream = "security"
            source_service = "workflow-service"
            source_event_id = "wfc-comp-audit-1"
            record_type = "WORKFLOW_COMPENSATION_EXECUTION"
            resource_ref = "workflow:wf-comp-audit-1:compensation:wfc-comp-audit-1"
            action = "EXECUTE_COMPENSATION"
            outcome = "SUCCEEDED"
            reason_code = "WORKFLOW_COMPENSATION_EXECUTED"
            risk_level = "HIGH"
            attributes_sha256 = (New-AuditManifestFixture).attributes_sha256
            idempotency_key = "workflow-service:audit:wfc-comp-audit-1:SUCCEEDED"
            correlation_id = "wf-comp-audit-1"
            causation_id = "workflow-compensation-execution-result-1"
            trace_id = ""
            occurred_at_unix_ms = 3000
        }
        response = [ordered]@{
            audit_id = "audit-workflow-compensation-1"
            record_hash = "record-hash-workflow-1"
            previous_record_hash = "previous-hash-workflow-1"
            idempotency_key = "workflow-service:audit:wfc-comp-audit-1:SUCCEEDED"
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
        [string]$ResultManifestID = "workflow-compensation-audit-append-result-1"
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-audit-append-result-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $auditManifestPath = Join-Path $tempRoot "workflow-compensation-execution-audit-append.json"
    $auditAppendResultPath = Join-Path $tempRoot "workflow-compensation-execution-audit-append-execution.json"
    $resultManifestPath = Join-Path $tempRoot "workflow-compensation-execution-audit-append-result-manifest.json"
    Write-JsonFile -Path $auditManifestPath -Value (New-AuditManifestFixture)
    Write-JsonFile -Path $auditAppendResultPath -Value (New-AuditAppendResultFixture)

    Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $resultManifestPath

    $raw = Get-Content -LiteralPath $resultManifestPath -Raw
    $result = $raw | ConvertFrom-Json
    if ($result.schema_version -ne "nexusim.workflow.compensation_audit_append_result.v1" -or
        $result.result_manifest_id -ne "workflow-compensation-audit-append-result-1" -or
        $result.manifest_is_execution -ne $false -or
        $result.executes_append -ne $false -or
        $result.mutates_audit_service -ne $false -or
        $result.executes_compensation -ne $false -or
        $result.records_decision -ne $false -or
        $result.calls_downstream_service -ne $false -or
        $result.audit_append_manifest.manifest_id -ne "workflow-compensation-audit-append-1" -or
        $result.audit_append_result.audit_id -ne "audit-workflow-compensation-1" -or
        $result.audit_append_result.executed_append -ne $true) {
        throw "workflow compensation audit append result manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_audit_append_manifest_verified",
            "audit_append_summary_matches_manifest",
            "audit_service_reported_audit_record",
            "workflow_compensation_result_manifest_bound_by_sha256",
            "workflow_compensation_audit_low_sensitive",
            "operator_keeps_result_manifest_external"
        )) {
        if (@($result.required_checks) -notcontains $expected) {
            throw "workflow compensation audit append result manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "result_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_execute_compensation",
            "does_not_record_workflow_decision",
            "does_not_modify_workflow_or_compensation_rows",
            "does_not_call_downstream_service"
        )) {
        if (@($result.execution_boundary) -notcontains $expected) {
            throw "workflow compensation audit append result manifest missing execution boundary: $expected"
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
            "EvidencePack",
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow compensation audit append result manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-compensation-audit-append-result.json"
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
    $manifestMismatch.manifest_id = "workflow-compensation-audit-append-other"
    $manifestMismatchPath = Join-Path $tempRoot "bad-manifest-mismatch.json"
    Write-JsonFile -Path $manifestMismatchPath -Value $manifestMismatch
    Invoke-ExpectFailure -Expected "audit_result.manifest_id mismatch" -Script {
        Invoke-Writer -AuditManifestPath $auditManifestPath -AuditAppendResultPath $manifestMismatchPath -OutputPath $resultManifestPath
    }

    $requestMismatch = New-AuditAppendResultFixture
    $requestMismatch.request.attributes_sha256 = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
    $requestMismatchPath = Join-Path $tempRoot "bad-request-mismatch.json"
    Write-JsonFile -Path $requestMismatchPath -Value $requestMismatch
    Invoke-ExpectFailure -Expected "request.attributes_sha256 mismatch" -Script {
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
    $badAuditManifest.attributes_json.status = "FAILED"
    $badAuditManifestPath = Join-Path $tempRoot "bad-audit-manifest.json"
    Write-JsonFile -Path $badAuditManifestPath -Value $badAuditManifest
    Invoke-ExpectFailure -Expected "audit_manifest.attributes_json_sha256 mismatch" -Script {
        Invoke-Writer -AuditManifestPath $badAuditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $resultManifestPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-compensation-audit-append-result.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation audit append result manifest self-test"
