$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-compensation-execution-result-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow compensation execution result manifest writer: $writerPath"
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
        schema_version = "nexusim.workflow.compensation_execution_invocation.v1"
        invocation_id = "workflow-compensation-execution-invocation-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        manifest_is_execution = $false
        executes_compensation = $false
        requires_explicit_operator_execution = $true
        workflow = [ordered]@{
            workflow_id = "wf-comp-exec-1"
            workflow_type = "COMPENSATION_REQUEST"
            status = "COMPENSATION_PENDING"
            target_service = "control-plane-service"
            target_operation = "CONFIG_ROLLBACK"
            target_ref_hash = "sha256:target"
            payload_schema_version = "admin.config_rollback.v1"
            payload_ref_hash = "sha256:payload"
            approval_policy_ref = "admin.workflow.compensation.v1"
            compensation_policy_ref = "admin.compensation.control_plane.v1"
            current_step_id = "wfs-comp-exec-1"
        }
        service_runtime_contract = [ordered]@{
            owner = "workflow-service.compensation-executor"
            mode_env = "NEXUSIM_WORKFLOW_SERVICE_MODE"
            mode_env_value = "compensation-executor"
        }
        required_checks = @(
            "readiness_manifest_verified",
            "workflow_still_compensation_pending_before_executor_start"
        )
        execution_boundary = @(
            "invocation_manifest_is_not_execution",
            "workflow_service_compensation_executor_remains_final_execution_owner"
        )
    }
}

function New-SummaryFixture {
    return [ordered]@{
        mode = "list-compensations"
        target = "127.0.0.1:10750"
        tenant_id = "tenant-workflow-test"
        workflow_id = "wf-comp-exec-1"
        compensations = @(
            [ordered]@{
                compensation_id = "wfc-wf-comp-exec-1"
                workflow_id = "wf-comp-exec-1"
                source_step_id = "wfs-comp-exec-1"
                target_service = "control-plane-service"
                target_operation = "CONFIG_ROLLBACK"
                target_ref_hash = "sha256:target"
                payload_schema_version = "admin.config_rollback.v1"
                payload_ref_hash = "sha256:payload"
                compensation_policy_ref = "admin.compensation.control_plane.v1"
                reason_ref = "reason-sha256:rollback"
                downstream_service = "control-plane-service"
                downstream_request_ref = "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1"
                status = "SUCCEEDED"
                created_at_unix_ms = 1000
                updated_at_unix_ms = 2000
                completed_at_unix_ms = 3000
            }
        )
        checked_at = "2026-06-25T00:00:00Z"
    }
}

function Invoke-Writer {
    param(
        [string]$InvocationPath,
        [string]$SummaryPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$ResultManifestID = "workflow-compensation-execution-result-1"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -InvocationPath $InvocationPath `
        -CompensationSummaryPath $SummaryPath `
        -GeneratedBy $GeneratedBy `
        -OutputPath $OutputPath `
        -ResultManifestID $ResultManifestID 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-result-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $invocationPath = Join-Path $tempRoot "workflow-compensation-execution-invocation.json"
    $summaryPath = Join-Path $tempRoot "workflow-compensation-summary.json"
    $resultPath = Join-Path $tempRoot "workflow-compensation-execution-result-manifest.json"
    Write-JsonFile -Path $invocationPath -Value (New-InvocationFixture)
    Write-JsonFile -Path $summaryPath -Value (New-SummaryFixture)

    Invoke-Writer -InvocationPath $invocationPath -SummaryPath $summaryPath -OutputPath $resultPath

    $raw = Get-Content -LiteralPath $resultPath -Raw
    $result = $raw | ConvertFrom-Json
    if ($result.schema_version -ne "nexusim.workflow.compensation_execution_result.v1" -or
        $result.result_manifest_id -ne "workflow-compensation-execution-result-1" -or
        $result.manifest_is_execution -ne $false -or
        $result.executes_compensation -ne $false -or
        $result.records_decision -ne $false -or
        $result.calls_downstream_service -ne $false -or
        $result.workflow.workflow_id -ne "wf-comp-exec-1" -or
        $result.compensation_result.compensation_id -ne "wfc-wf-comp-exec-1" -or
        $result.compensation_result.status -ne "SUCCEEDED" -or
        $result.compensation_result.downstream_request_ref -ne "config-rollback:prod:API_GATEWAY_TENANT_QUOTA:tenant-a:v1") {
        throw "workflow compensation execution result manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_invocation_manifest_verified",
            "list_compensations_summary_from_workflow_service_public_api",
            "compensation_row_matches_invocation_workflow_payload_target",
            "compensation_status_is_terminal",
            "result_manifest_contains_only_low_sensitive_refs"
        )) {
        if (@($result.required_checks) -notcontains $expected) {
            throw "workflow compensation execution result manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "result_manifest_is_not_execution",
            "does_not_call_downstream_service",
            "does_not_record_workflow_decision",
            "does_not_modify_workflow_or_compensation_rows",
            "workflow_service_compensation_executor_remains_final_execution_owner"
        )) {
        if (@($result.execution_boundary) -notcontains $expected) {
            throw "workflow compensation execution result manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $invocationPath,
            $summaryPath,
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
            throw "workflow compensation execution result manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-execution-result.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -InvocationPath $invocationPath -SummaryPath $summaryPath -OutputPath $repoLocalOutput
    }

    Invoke-ExpectFailure -Expected "low-sensitive operator id" -Script {
        Invoke-Writer -InvocationPath $invocationPath -SummaryPath $summaryPath -OutputPath $resultPath -GeneratedBy "operator-token-secret"
    }

    $nonTerminal = New-SummaryFixture
    $nonTerminal.compensations[0].status = "EXECUTING"
    $nonTerminalPath = Join-Path $tempRoot "non-terminal.json"
    Write-JsonFile -Path $nonTerminalPath -Value $nonTerminal
    Invoke-ExpectFailure -Expected "SUCCEEDED or FAILED" -Script {
        Invoke-Writer -InvocationPath $invocationPath -SummaryPath $nonTerminalPath -OutputPath $resultPath
    }

    $mismatch = New-SummaryFixture
    $mismatch.workflow_id = "wf-other"
    $mismatch.compensations[0].workflow_id = "wf-other"
    $mismatchPath = Join-Path $tempRoot "mismatch.json"
    Write-JsonFile -Path $mismatchPath -Value $mismatch
    Invoke-ExpectFailure -Expected "workflow_id does not match" -Script {
        Invoke-Writer -InvocationPath $invocationPath -SummaryPath $mismatchPath -OutputPath $resultPath
    }

    $sensitive = New-SummaryFixture
    $sensitive.compensations[0].downstream_request_ref = "raw:provider-body"
    $sensitivePath = Join-Path $tempRoot "sensitive.json"
    Write-JsonFile -Path $sensitivePath -Value $sensitive
    Invoke-ExpectFailure -Expected "raw, secret" -Script {
        Invoke-Writer -InvocationPath $invocationPath -SummaryPath $sensitivePath -OutputPath $resultPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation execution result manifest self-test"
