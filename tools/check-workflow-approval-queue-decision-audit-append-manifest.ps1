$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$writerPath = Join-Path $PSScriptRoot "write-workflow-approval-queue-decision-audit-append-manifest.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow approval queue decision audit append manifest writer: $writerPath"
}

function Write-JsonFile {
    param(
        [string]$Path,
        [object]$Value
    )
    $Value | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $Path -Encoding UTF8
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

function New-ReviewSummaryFixture {
    return [ordered]@{
        schema_version = "nexusim.workflow.approval_queue_decision_result_review.v1"
        page_id = "workflow-approval-queue-decision-result-review-1"
        generated_at = "2026-06-25T00:00:00Z"
        generated_by = "operator-a"
        source_result_manifest_sha256 = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        result_manifest_id = "workflow-approval-queue-batch-decision-result-1"
        batch_decision_id = "workflow-approval-queue-batch-decision-1"
        tenant_id = "tenant-workflow"
        decision_count = 2
        decision_owner = "workflow-service.RecordWorkflowDecision"
        source_records_decision = $true
        source_called_workflow_service_runtime = $true
        source_calls_action_executor = $false
        source_executes_target = $false
        source_mutates_workflow_fact = $true
        review_page_calls_workflow_service = $false
        review_page_records_decision = $false
        review_page_calls_action_executor = $false
        review_page_executes_target = $false
        review_page_mutates_workflow_fact = $false
        items = @(
            [ordered]@{
                queue_id = "action-approval"
                workflow_id = "wf-approval-audit-1"
                step_id = "wfs-approval-audit-1"
                decision = "APPROVE"
                workflow_status = "APPROVED"
                decision_id = "decision:wf-approval-audit-1:wfs-approval-audit-1"
                decision_type = "APPROVE"
                replayed = $false
                source_decision_manifest_sha256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
                execution_summary_sha256 = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
                execution_summary_path_sha256 = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
            },
            [ordered]@{
                queue_id = "action-approval"
                workflow_id = "wf-approval-audit-2"
                step_id = "wfs-approval-audit-2"
                decision = "REJECT"
                workflow_status = "REJECTED"
                decision_id = "decision:wf-approval-audit-2:wfs-approval-audit-2"
                decision_type = "REJECT"
                replayed = $false
                source_decision_manifest_sha256 = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
                execution_summary_sha256 = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
                execution_summary_path_sha256 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
            }
        )
    }
}

function Invoke-Writer {
    param(
        [string]$SummaryPath,
        [string]$OutputPath,
        [string]$GeneratedBy = "operator-a",
        [string]$AuditManifestID = "workflow-approval-decision-audit-append-1",
        [string]$AuditRecordID = "workflow-service:audit:workflow-approval-queue-batch-decision-1"
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -ReviewSummaryPath $SummaryPath `
        -GeneratedBy $GeneratedBy `
        -OutputPath $OutputPath `
        -AuditManifestID $AuditManifestID `
        -AuditRecordID $AuditRecordID `
        -OccurredAt "2026-06-25T00:00:00Z" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-approval-decision-audit-append-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $summaryPath = Join-Path $tempRoot "workflow-decision-result-review.json"
    $auditPath = Join-Path $tempRoot "workflow-decision-audit-append.json"
    Write-JsonFile -Path $summaryPath -Value (New-ReviewSummaryFixture)

    Invoke-Writer -SummaryPath $summaryPath -OutputPath $auditPath

    $raw = Get-Content -LiteralPath $auditPath -Raw
    $manifest = $raw | ConvertFrom-Json
    if ($manifest.schema_version -ne "nexusim.audit.external_append.v1" -or
        $manifest.manifest_id -ne "workflow-approval-decision-audit-append-1" -or
        $manifest.source_manifest_id -ne "workflow-approval-queue-decision-result-review-1" -or
        $manifest.executes_append -ne $false -or
        $manifest.mutates_audit_service -ne $false -or
        $manifest.direct_append_allowed -ne $false -or
        $manifest.requires_operator_execution -ne $true -or
        $manifest.source_service -ne "workflow-service" -or
        $manifest.source_event_id -ne "workflow-approval-queue-batch-decision-1" -or
        $manifest.record_type -ne "WORKFLOW_APPROVAL_BATCH_DECISION" -or
        $manifest.action -ne "RECORD_WORKFLOW_DECISIONS" -or
        $manifest.outcome -ne "RECORDED" -or
        $manifest.reason_code -ne "WORKFLOW_APPROVAL_QUEUE_BATCH_DECISION_RECORDED" -or
        $manifest.auth_context_contract.tenant_id -ne "tenant-workflow") {
        throw "workflow approval queue decision audit append manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_decision_result_review_summary_verified",
            "workflow_service_recorded_decisions",
            "review_page_was_read_only",
            "action_executor_not_called",
            "target_action_not_executed",
            "audit_service_append_only",
            "idempotency_key_present"
        )) {
        if (@($manifest.required_checks) -notcontains $expected) {
            throw "workflow approval queue decision audit append manifest missing required check: $expected"
        }
    }
    foreach ($expected in @(
            "audit_manifest_is_not_audit_append_execution",
            "does_not_call_audit_service",
            "does_not_record_workflow_decision",
            "does_not_call_action_executor",
            "does_not_execute_target_action",
            "does_not_mutate_workflow_fact"
        )) {
        if (@($manifest.execution_boundary) -notcontains $expected) {
            throw "workflow approval queue decision audit append manifest missing execution boundary: $expected"
        }
    }
    foreach ($forbidden in @(
            $tempRoot,
            $summaryPath,
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
            throw "workflow approval queue decision audit append manifest leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path $repoRoot "tmp-workflow-approval-decision-audit-append.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -SummaryPath $summaryPath -OutputPath $repoLocalOutput
    }

    $badExecutorPath = Join-Path $tempRoot "bad-executor-summary.json"
    $badExecutor = New-ReviewSummaryFixture
    $badExecutor.source_calls_action_executor = $true
    Write-JsonFile -Path $badExecutorPath -Value $badExecutor
    Invoke-ExpectFailure -Expected "must not call action-executor" -Script {
        Invoke-Writer -SummaryPath $badExecutorPath -OutputPath $auditPath
    }

    $badOwnerPath = Join-Path $tempRoot "bad-owner-summary.json"
    $badOwner = New-ReviewSummaryFixture
    $badOwner.decision_owner = "admin-service.RecordWorkflowDecision"
    Write-JsonFile -Path $badOwnerPath -Value $badOwner
    Invoke-ExpectFailure -Expected "decision_owner must be workflow-service.RecordWorkflowDecision" -Script {
        Invoke-Writer -SummaryPath $badOwnerPath -OutputPath $auditPath
    }

    $badCountPath = Join-Path $tempRoot "bad-count-summary.json"
    $badCount = New-ReviewSummaryFixture
    $badCount.decision_count = 99
    Write-JsonFile -Path $badCountPath -Value $badCount
    Invoke-ExpectFailure -Expected "decision_count must match item count" -Script {
        Invoke-Writer -SummaryPath $badCountPath -OutputPath $auditPath
    }

    $badRawPath = Join-Path $tempRoot "bad-raw-summary.json"
    $badRawJson = (New-ReviewSummaryFixture | ConvertTo-Json -Depth 40)
    $badRawJson = $badRawJson -replace '"items":\s*\[', '"provider_body":"raw provider body","items":['
    Set-Content -LiteralPath $badRawPath -Value $badRawJson -Encoding UTF8
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-Writer -SummaryPath $badRawPath -OutputPath $auditPath
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $repoRoot "tmp-workflow-approval-decision-audit-append.json") -Force -ErrorAction SilentlyContinue
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

Write-Host "OK   workflow approval queue decision audit append manifest self-test"
