$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$auditWriterPath = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-audit-append-manifest.ps1"
$resultWriterPath = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-audit-append-result-manifest.ps1"
foreach ($path in @($auditWriterPath, $resultWriterPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback audit append result dependency: $path"
    }
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

function New-RedriveResultManifestFixture {
    return [ordered]@{
        schema_version = "nexusim.workflow.external_callback_batch_redrive_result.v1"
        result_manifest_id = "workflow-external-callback-batch-redrive-result-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        source_batch_invocation_sha256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        source_batch_invocation_path_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        source_execution_summary_root_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
        batch_invocation_id = "workflow-external-callback-batch-redrive-invocation-1"
        expected_redrive_count = 1
        execution_summary_count = 1
        result_count = 1
        manifest_is_execution = $false
        records_decision = $false
        calls_provider = $false
        executes_target = $false
        mutates_delivery_fact = $false
        runtime_contract = [ordered]@{
            service = "workflow-service"
            mode = "external-callback-delivery-redrive"
            plan_env = "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE"
            summary_env = "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_SUMMARY_FILE"
            result_manifest_calls_service = $false
            result_manifest_records_decision = $false
            result_manifest_calls_provider = $false
            result_manifest_executes_target = $false
        }
        results = @(
            [ordered]@{
                redrive_plan_id = "workflow-external-callback-redrive-plan-1"
                workflow_id = "wf-callback-audit-1"
                step_id = "wfs-callback-audit-1"
                delivery_id = "wfcd-callback-audit-1"
                tenant_id = "tenant-workflow"
                delivery_status = "PENDING"
                redrive_count = 2
                redrive_plan_sha256 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
                source_delivery_status_sha256 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
                source_delivery_plan_sha256 = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
                execution_summary_sha256 = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
                execution_summary_path_sha256 = "sha256:5555555555555555555555555555555555555555555555555555555555555555"
                target_service = "connector-service"
                target_operation = "CALLBACK_DELIVERY"
                payload_ref_hash = "sha256:6666666666666666666666666666666666666666666666666666666666666666"
                approval_policy_ref = "workflow.external_callback.v1"
                last_redrive_reason_ref = "workflow.redrive.reason.timeout"
                outbox_event_type = "workflow.external_callback.redriven.v1"
            }
        )
        required_checks = @(
            "source_batch_invocation_manifest_verified",
            "one_execution_summary_per_redrive_plan",
            "execution_summary_matches_invocation_binding",
            "workflow_service_runtime_reported_executed_redrive",
            "delivery_fact_returned_to_pending",
            "redriven_outbox_event_declared",
            "result_manifest_contains_only_low_sensitive_refs"
        )
        execution_boundary = @(
            "result_manifest_is_not_redrive_execution",
            "does_not_call_workflow_service",
            "does_not_record_workflow_decision",
            "does_not_call_provider",
            "does_not_execute_target_action",
            "does_not_modify_delivery_rows"
        )
        forbidden_contents = @("raw_callback_url", "provider_response_material")
    }
}

function New-AuditAppendResultFixture {
    param([object]$AuditManifest)
    return [ordered]@{
        mode = "external-audit-append"
        audit_target = "127.0.0.1:10700"
        manifest_id = $AuditManifest.manifest_id
        source_manifest_id = $AuditManifest.source_manifest_id
        request = [ordered]@{
            tenant_id = $AuditManifest.auth_context_contract.tenant_id
            user_id = "operator-user"
            device_id = "operator-device"
            audit_stream = $AuditManifest.audit_stream
            source_service = $AuditManifest.source_service
            source_event_id = $AuditManifest.source_event_id
            record_type = $AuditManifest.record_type
            resource_ref = $AuditManifest.resource_ref
            action = $AuditManifest.action
            outcome = $AuditManifest.outcome
            reason_code = $AuditManifest.reason_code
            risk_level = $AuditManifest.risk_level
            attributes_sha256 = $AuditManifest.attributes_sha256
            idempotency_key = $AuditManifest.idempotency_key
            correlation_id = $AuditManifest.correlation_id
            causation_id = $AuditManifest.causation_id
            trace_id = $AuditManifest.trace_id
            occurred_at_unix_ms = $AuditManifest.occurred_at_unix_ms
        }
        response = [ordered]@{
            audit_id = "audit-workflow-callback-redrive-1"
            record_hash = "record-hash-workflow-callback-redrive-1"
            previous_record_hash = "previous-hash-workflow-callback-redrive-1"
            idempotency_key = $AuditManifest.idempotency_key
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

function Invoke-AuditWriter {
    param([string]$ResultManifestPath, [string]$OutputPath)
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $auditWriterPath `
        -ResultManifestPath $ResultManifestPath `
        -GeneratedBy "operator-a" `
        -TenantID "tenant-workflow" `
        -OutputPath $OutputPath `
        -AuditManifestID "workflow-callback-redrive-audit-append-1" `
        -AuditRecordID "workflow-service:audit:workflow-external-callback-batch-redrive-invocation-1" `
        -OccurredAt "2026-06-25T00:00:00Z" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

function Invoke-ResultWriter {
    param(
        [string]$AuditManifestPath,
        [string]$AuditAppendResultPath,
        [string]$OutputPath,
        [string]$ResultManifestID = "workflow-callback-redrive-audit-append-result-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $resultWriterPath `
        -AuditManifestPath $AuditManifestPath `
        -AuditAppendResultPath $AuditAppendResultPath `
        -GeneratedBy "operator-a" `
        -OutputPath $OutputPath `
        -ResultManifestID $ResultManifestID 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-callback-redrive-audit-append-result-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $redriveResultPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-result.json"
    $auditManifestPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-audit-append.json"
    $auditAppendResultPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-audit-append-execution.json"
    $resultManifestPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-audit-append-result.json"
    Write-JsonFile -Path $redriveResultPath -Value (New-RedriveResultManifestFixture)
    Invoke-AuditWriter -ResultManifestPath $redriveResultPath -OutputPath $auditManifestPath
    $auditManifest = Get-Content -LiteralPath $auditManifestPath -Raw | ConvertFrom-Json
    Write-JsonFile -Path $auditAppendResultPath -Value (New-AuditAppendResultFixture -AuditManifest $auditManifest)

    Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $resultManifestPath

    $raw = Get-Content -LiteralPath $resultManifestPath -Raw
    $result = $raw | ConvertFrom-Json
    if ($result.schema_version -ne "nexusim.workflow.external_callback_batch_redrive_audit_append_result.v1" -or
        $result.result_manifest_id -ne "workflow-callback-redrive-audit-append-result-1" -or
        $result.manifest_is_execution -ne $false -or
        $result.executes_append -ne $false -or
        $result.mutates_audit_service -ne $false -or
        $result.redrives_external_callback -ne $false -or
        $result.records_decision -ne $false -or
        $result.calls_provider -ne $false -or
        $result.executes_target_action -ne $false -or
        $result.mutates_delivery_fact -ne $false -or
        $result.audit_append_manifest.manifest_id -ne "workflow-callback-redrive-audit-append-1" -or
        $result.audit_append_result.audit_id -ne "audit-workflow-callback-redrive-1" -or
        $result.audit_append_result.executed_append -ne $true) {
        throw "workflow external callback batch redrive audit append result manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_audit_append_manifest_verified",
            "audit_append_summary_matches_manifest",
            "audit_service_reported_audit_record",
            "workflow_external_callback_batch_redrive_result_bound_by_sha256",
            "workflow_external_callback_batch_redrive_audit_low_sensitive",
            "operator_keeps_result_manifest_external"
        )) {
        if (@($result.required_checks) -notcontains $expected) {
            throw "workflow external callback batch redrive audit append result manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "result_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_redrive_external_callback",
            "does_not_record_workflow_decision",
            "does_not_call_provider",
            "does_not_execute_target_action",
            "does_not_modify_delivery_rows"
        )) {
        if (@($result.execution_boundary) -notcontains $expected) {
            throw "workflow external callback batch redrive audit append result manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $redriveResultPath,
            $auditManifestPath,
            $auditAppendResultPath,
            $resultManifestPath,
            '"attributes_json":',
            '"provider_body":',
            '"raw_callback_url":',
            "EvidencePack",
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow external callback batch redrive audit append result manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-callback-redrive-audit-append-result.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $auditAppendResultPath -OutputPath $repoLocalOutput
    }

    $preflightOnly = New-AuditAppendResultFixture -AuditManifest $auditManifest
    $preflightOnly.executed_append = $false
    $preflightOnly.Remove("response")
    $preflightOnlyPath = Join-Path $tempRoot "bad-preflight-only.json"
    Write-JsonFile -Path $preflightOnlyPath -Value $preflightOnly
    Invoke-ExpectFailure -Expected "executed_append=true" -Script {
        Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $preflightOnlyPath -OutputPath $resultManifestPath
    }

    $manifestMismatch = New-AuditAppendResultFixture -AuditManifest $auditManifest
    $manifestMismatch.manifest_id = "workflow-callback-redrive-audit-append-other"
    $manifestMismatchPath = Join-Path $tempRoot "bad-manifest-mismatch.json"
    Write-JsonFile -Path $manifestMismatchPath -Value $manifestMismatch
    Invoke-ExpectFailure -Expected "audit_result.manifest_id mismatch" -Script {
        Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $manifestMismatchPath -OutputPath $resultManifestPath
    }

    $requestMismatch = New-AuditAppendResultFixture -AuditManifest $auditManifest
    $requestMismatch.request.outcome = "FAILED"
    $requestMismatchPath = Join-Path $tempRoot "bad-request-mismatch.json"
    Write-JsonFile -Path $requestMismatchPath -Value $requestMismatch
    Invoke-ExpectFailure -Expected "request.outcome mismatch" -Script {
        Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $requestMismatchPath -OutputPath $resultManifestPath
    }

    $responseMismatch = New-AuditAppendResultFixture -AuditManifest $auditManifest
    $responseMismatch.response.idempotency_key = "workflow-service:audit:other"
    $responseMismatchPath = Join-Path $tempRoot "bad-response-mismatch.json"
    Write-JsonFile -Path $responseMismatchPath -Value $responseMismatch
    Invoke-ExpectFailure -Expected "response.idempotency_key mismatch" -Script {
        Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $responseMismatchPath -OutputPath $resultManifestPath
    }

    $badRaw = New-AuditAppendResultFixture -AuditManifest $auditManifest
    $badRaw.provider_body = "provider raw body"
    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-ResultWriter -AuditManifestPath $auditManifestPath -AuditAppendResultPath $badRawPath -OutputPath $resultManifestPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $repoRoot "tmp-workflow-callback-redrive-audit-append-result.json") -Force -ErrorAction SilentlyContinue
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

Write-Host "OK   workflow external callback batch redrive audit append result manifest self-test"
