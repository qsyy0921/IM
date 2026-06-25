$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$writerPath = Join-Path $PSScriptRoot "write-workflow-compensation-execution-audit-append-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow compensation audit append manifest writer: $writerPath"
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

function New-ResultFixture {
    param([string]$Status = "SUCCEEDED")

    return [ordered]@{
        schema_version = "nexusim.workflow.compensation_execution_result.v1"
        result_manifest_id = "workflow-compensation-execution-result-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        manifest_is_execution = $false
        executes_compensation = $false
        records_decision = $false
        calls_downstream_service = $false
        source_invocation_sha256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        source_compensation_summary_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        workflow = [ordered]@{
            workflow_id = "wf-comp-audit-1"
            workflow_type = "COMPENSATION_REQUEST"
            target_service = "control-plane-service"
            target_operation = "CONFIG_ROLLBACK"
            target_ref_hash = "sha256:target"
            payload_schema_version = "admin.config_rollback.v1"
            payload_ref_hash = "sha256:payload"
            approval_policy_ref = "admin.workflow.compensation.v1"
            compensation_policy_ref = "admin.compensation.control_plane.v1"
        }
        compensation_result = [ordered]@{
            compensation_id = "wfc-comp-audit-1"
            source_step_id = "wfs-comp-audit-1"
            status = $Status
            downstream_service = "control-plane-service"
            downstream_request_ref = "config-rollback:prod:quota:v1"
            failure_class = ""
            public_error = ""
            created_at_unix_ms = 1000
            updated_at_unix_ms = 2000
            completed_at_unix_ms = 3000
        }
        required_checks = @(
            "source_invocation_manifest_verified",
            "list_compensations_summary_from_workflow_service_public_api",
            "compensation_row_matches_invocation_workflow_payload_target",
            "compensation_status_is_terminal",
            "result_manifest_contains_only_low_sensitive_refs"
        )
        execution_boundary = @(
            "result_manifest_is_not_execution",
            "does_not_call_downstream_service",
            "does_not_record_workflow_decision",
            "does_not_modify_workflow_or_compensation_rows",
            "workflow_service_compensation_executor_remains_final_execution_owner"
        )
    }
}

function Invoke-Writer {
    param(
        [string]$ResultPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$TenantID = "tenant-workflow-test",
        [string]$AuditManifestID = "workflow-compensation-audit-append-1",
        [string]$AuditRecordID = "workflow-service:audit:wfc-comp-audit-1:SUCCEEDED"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -ResultManifestPath $ResultPath `
        -GeneratedBy $GeneratedBy `
        -TenantID $TenantID `
        -OutputPath $OutputPath `
        -AuditManifestID $AuditManifestID `
        -AuditRecordID $AuditRecordID 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-audit-append-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $resultPath = Join-Path $tempRoot "workflow-compensation-execution-result-manifest.json"
    $auditPath = Join-Path $tempRoot "workflow-compensation-execution-audit-append.json"
    Write-JsonFile -Path $resultPath -Value (New-ResultFixture)

    Invoke-Writer -ResultPath $resultPath -OutputPath $auditPath

    $raw = Get-Content -LiteralPath $auditPath -Raw
    $manifest = $raw | ConvertFrom-Json
    if ($manifest.schema_version -ne "nexusim.audit.external_append.v1" -or
        $manifest.manifest_id -ne "workflow-compensation-audit-append-1" -or
        $manifest.source_manifest_id -ne "workflow-compensation-execution-result-1" -or
        $manifest.executes_append -ne $false -or
        $manifest.mutates_audit_service -ne $false -or
        $manifest.direct_append_allowed -ne $false -or
        $manifest.requires_operator_execution -ne $true -or
        $manifest.source_service -ne "workflow-service" -or
        $manifest.source_event_id -ne "wfc-comp-audit-1" -or
        $manifest.record_type -ne "WORKFLOW_COMPENSATION_EXECUTION" -or
        $manifest.action -ne "EXECUTE_COMPENSATION" -or
        $manifest.outcome -ne "SUCCEEDED" -or
        $manifest.reason_code -ne "WORKFLOW_COMPENSATION_EXECUTED" -or
        $manifest.auth_context_contract.tenant_id -ne "tenant-workflow-test") {
        throw "workflow compensation audit append manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_compensation_result_manifest_verified",
            "workflow_compensation_result_low_sensitive",
            "no_raw_compensation_payload",
            "compensation_status_terminal",
            "audit_service_append_only",
            "idempotency_key_present"
        )) {
        if (@($manifest.required_checks) -notcontains $expected) {
            throw "workflow compensation audit append manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "audit_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_execute_compensation",
            "does_not_record_workflow_decision",
            "does_not_modify_workflow_or_compensation_rows",
            "does_not_call_downstream_service"
        )) {
        if (@($manifest.execution_boundary) -notcontains $expected) {
            throw "workflow compensation audit append manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $resultPath,
            $auditPath,
            '"provider_body":',
            '"payload_body":',
            '"EvidencePack":',
            "raw:",
            "secret",
            "password",
            "credential",
            "postgres://"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow compensation audit append manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-compensation-audit-append.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -ResultPath $resultPath -OutputPath $repoLocalOutput
    }

    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-Writer -ResultPath $resultPath -OutputPath $auditPath -TenantID "tenant-token-secret"
    }

    $nonTerminalPath = Join-Path $tempRoot "non-terminal-result.json"
    Write-JsonFile -Path $nonTerminalPath -Value (New-ResultFixture -Status "EXECUTING")
    Invoke-ExpectFailure -Expected "must be terminal" -Script {
        Invoke-Writer -ResultPath $nonTerminalPath -OutputPath $auditPath
    }

    $sensitive = New-ResultFixture
    $sensitive.compensation_result.downstream_request_ref = "raw:provider-body"
    $sensitivePath = Join-Path $tempRoot "sensitive-result.json"
    Write-JsonFile -Path $sensitivePath -Value $sensitive
    Invoke-ExpectFailure -Expected "raw, secret" -Script {
        Invoke-Writer -ResultPath $sensitivePath -OutputPath $auditPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
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

Write-Host "OK   workflow compensation execution audit append manifest self-test"
