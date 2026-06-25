$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$writerPath = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-audit-append-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow external callback batch redrive audit append manifest writer: $writerPath"
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

function New-ResultManifestFixture {
    return [ordered]@{
        schema_version = "nexusim.workflow.external_callback_batch_redrive_result.v1"
        result_manifest_id = "workflow-external-callback-batch-redrive-result-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        source_batch_invocation_sha256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        source_batch_invocation_path_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        source_execution_summary_root_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
        batch_invocation_id = "workflow-external-callback-batch-redrive-invocation-1"
        expected_redrive_count = 2
        execution_summary_count = 2
        result_count = 2
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
            },
            [ordered]@{
                redrive_plan_id = "workflow-external-callback-redrive-plan-2"
                workflow_id = "wf-callback-audit-2"
                step_id = "wfs-callback-audit-2"
                delivery_id = "wfcd-callback-audit-2"
                tenant_id = "tenant-workflow"
                delivery_status = "PENDING"
                redrive_count = 1
                redrive_plan_sha256 = "sha256:7777777777777777777777777777777777777777777777777777777777777777"
                source_delivery_status_sha256 = "sha256:8888888888888888888888888888888888888888888888888888888888888888"
                source_delivery_plan_sha256 = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
                execution_summary_sha256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
                execution_summary_path_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
                target_service = "connector-service"
                target_operation = "CALLBACK_DELIVERY"
                payload_ref_hash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
                approval_policy_ref = "workflow.external_callback.v1"
                last_redrive_reason_ref = "workflow.redrive.reason.operator"
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

function Invoke-Writer {
    param(
        [string]$ResultManifestPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$TenantID = "tenant-workflow",
        [string]$AuditManifestID = "workflow-callback-redrive-audit-append-1",
        [string]$AuditRecordID = "workflow-service:audit:workflow-external-callback-batch-redrive-invocation-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -ResultManifestPath $ResultManifestPath `
        -GeneratedBy $GeneratedBy `
        -TenantID $TenantID `
        -OutputPath $OutputPath `
        -AuditManifestID $AuditManifestID `
        -AuditRecordID $AuditRecordID `
        -OccurredAt "2026-06-25T00:00:00Z" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-callback-redrive-audit-append-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $resultPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-result.json"
    $auditPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-audit-append.json"
    Write-JsonFile -Path $resultPath -Value (New-ResultManifestFixture)

    Invoke-Writer -ResultManifestPath $resultPath -OutputPath $auditPath

    $raw = Get-Content -LiteralPath $auditPath -Raw
    $manifest = $raw | ConvertFrom-Json
    if ($manifest.schema_version -ne "nexusim.audit.external_append.v1" -or
        $manifest.manifest_id -ne "workflow-callback-redrive-audit-append-1" -or
        $manifest.source_manifest_id -ne "workflow-external-callback-batch-redrive-result-1" -or
        $manifest.executes_append -ne $false -or
        $manifest.mutates_audit_service -ne $false -or
        $manifest.direct_append_allowed -ne $false -or
        $manifest.requires_operator_execution -ne $true -or
        $manifest.source_service -ne "workflow-service" -or
        $manifest.source_event_id -ne "workflow-external-callback-batch-redrive-invocation-1" -or
        $manifest.record_type -ne "WORKFLOW_EXTERNAL_CALLBACK_BATCH_REDRIVE" -or
        $manifest.action -ne "REDRIVE_EXTERNAL_CALLBACK_DELIVERIES" -or
        $manifest.outcome -ne "REDRIVEN" -or
        $manifest.reason_code -ne "WORKFLOW_EXTERNAL_CALLBACK_BATCH_REDRIVEN" -or
        $manifest.auth_context_contract.tenant_id -ne "tenant-workflow") {
        throw "workflow external callback batch redrive audit append manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_external_callback_batch_redrive_result_manifest_verified",
            "workflow_service_redrive_runtime_reported",
            "delivery_fact_returned_to_pending",
            "redriven_outbox_event_declared",
            "audit_service_append_only",
            "idempotency_key_present"
        )) {
        if (@($manifest.required_checks) -notcontains $expected) {
            throw "workflow external callback batch redrive audit append manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "audit_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_redrive_external_callback",
            "does_not_record_workflow_decision",
            "does_not_call_provider",
            "does_not_execute_target_action",
            "does_not_modify_delivery_rows"
        )) {
        if (@($manifest.execution_boundary) -notcontains $expected) {
            throw "workflow external callback batch redrive audit append manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $resultPath,
            $auditPath,
            '"provider_body":',
            '"payload_body":',
            '"raw_callback_url":',
            "EvidencePack",
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow external callback batch redrive audit append manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-callback-redrive-audit-append.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -ResultManifestPath $resultPath -OutputPath $repoLocalOutput
    }

    $badExecution = New-ResultManifestFixture
    $badExecution.manifest_is_execution = $true
    $badExecutionPath = Join-Path $tempRoot "bad-execution.json"
    Write-JsonFile -Path $badExecutionPath -Value $badExecution
    Invoke-ExpectFailure -Expected "must not be an execution" -Script {
        Invoke-Writer -ResultManifestPath $badExecutionPath -OutputPath $auditPath
    }

    $badTenant = New-ResultManifestFixture
    $badTenant.results[0].tenant_id = "tenant-other"
    $badTenantPath = Join-Path $tempRoot "bad-tenant.json"
    Write-JsonFile -Path $badTenantPath -Value $badTenant
    Invoke-ExpectFailure -Expected "tenant_id must match TenantID" -Script {
        Invoke-Writer -ResultManifestPath $badTenantPath -OutputPath $auditPath
    }

    $badStatus = New-ResultManifestFixture
    $badStatus.results[0].delivery_status = "FAILED"
    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    Write-JsonFile -Path $badStatusPath -Value $badStatus
    Invoke-ExpectFailure -Expected "delivery_status must be PENDING" -Script {
        Invoke-Writer -ResultManifestPath $badStatusPath -OutputPath $auditPath
    }

    $badRaw = New-ResultManifestFixture
    $badRaw.provider_body = "provider raw body"
    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-Writer -ResultManifestPath $badRawPath -OutputPath $auditPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $repoRoot "tmp-workflow-callback-redrive-audit-append.json") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback batch redrive audit append manifest self-test"
