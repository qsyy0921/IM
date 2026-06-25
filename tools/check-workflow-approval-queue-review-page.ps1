$ErrorActionPreference = "Stop"

$pageWriter = Join-Path $PSScriptRoot "write-workflow-approval-queue-review-page.ps1"
if (-not (Test-Path -LiteralPath $pageWriter -PathType Leaf)) {
    throw "Missing workflow approval queue review page writer: $pageWriter"
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

function New-Workflow {
    param(
        [string]$WorkflowID,
        [string]$WorkflowType,
        [string]$RiskLevel = "HIGH",
        [string]$Status = "WAITING_DECISION",
        [string]$TargetService = "",
        [string]$TargetOperation = "",
        [string]$TargetRefHash = "",
        [string]$PayloadSchemaVersion = "",
        [string]$PayloadRefHash = "",
        [string]$ApprovalPolicyRef = "",
        [string]$CurrentStepID = "wfs-review-1"
    )

    return [ordered]@{
        workflow_id = $WorkflowID
        workflow_type = $WorkflowType
        risk_level = $RiskLevel
        status = $Status
        target_service = $TargetService
        target_operation = $TargetOperation
        target_ref_hash = $TargetRefHash
        payload_schema_version = $PayloadSchemaVersion
        payload_ref_hash = $PayloadRefHash
        approval_policy_ref = $ApprovalPolicyRef
        current_step_id = $CurrentStepID
        reason_ref = "reason-sha256:review"
    }
}

function Invoke-PageWriter {
    param(
        [string]$SummaryPath,
        [string]$OutputPath
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriter `
        -QueueSummaryPath $SummaryPath `
        -GeneratedBy "operator-a" `
        -PageID "workflow-approval-queue-review-page-1" `
        -OutputPath $OutputPath 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-approval-queue-review-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-approval-queue-review-secret"
    $operatorSummaryPath = Join-Path $tempRoot "operator-queues.json"
    $providerSummaryPath = Join-Path $tempRoot "provider-replay-queue.json"
    $operatorPagePath = Join-Path $tempRoot "operator-queues-review.html"
    $providerPagePath = Join-Path $tempRoot "provider-replay-review.html"

    $operatorSummary = [ordered]@{
        mode = "operator-queues"
        target = "127.0.0.1:10750"
        tenant_id = "tenant-workflow"
        operator_queues = @(
            [ordered]@{
                queue_id = "action-approval"
                workflow_type = "ACTION_APPROVAL"
                status = "WAITING_DECISION"
                workflow_count = 1
                workflows = @(
                    (New-Workflow -WorkflowID "wf_action_1" -WorkflowType "ACTION_APPROVAL" -TargetService "conversation-service" -TargetOperation "UPSERT_CONVERSATION_NOTE" -TargetRefHash "sha256:target" -PayloadSchemaVersion "agent.action.v1" -PayloadRefHash "sha256:payload" -ApprovalPolicyRef "agent.action.approval.v1")
                )
            },
            [ordered]@{
                queue_id = "provider-replay"
                workflow_type = "REPAIR_APPROVAL"
                status = "WAITING_DECISION"
                target_service = "action-executor"
                target_operation = "PROVIDER_REPLAY_REQUEST"
                approval_policy_ref = "admin.workflow.provider_replay.v1"
                workflow_count = 1
                workflows = @(
                    (New-Workflow -WorkflowID "wf_provider_replay_1" -WorkflowType "REPAIR_APPROVAL" -TargetService "action-executor" -TargetOperation "PROVIDER_REPLAY_REQUEST" -TargetRefHash "sha256:provider-failure" -PayloadSchemaVersion "admin.provider_replay_request.v1" -PayloadRefHash "sha256:provider-replay-payload" -ApprovalPolicyRef "admin.workflow.provider_replay.v1")
                )
            },
            [ordered]@{
                queue_id = "empty-compensation"
                workflow_type = "COMPENSATION_REQUEST"
                status = "WAITING_DECISION"
                workflow_count = 0
                workflows = @()
            }
        )
        checked_at = "2026-06-25T00:00:00Z"
        debug_note = $leakMarker
    }
    Write-JsonFile -Path $operatorSummaryPath -Value $operatorSummary
    Invoke-PageWriter -SummaryPath $operatorSummaryPath -OutputPath $operatorPagePath

    $providerSummary = [ordered]@{
        mode = "provider-replay-queue"
        target = "127.0.0.1:10750"
        tenant_id = "tenant-workflow"
        workflow_type = "REPAIR_APPROVAL"
        status = "WAITING_DECISION"
        target_service = "action-executor"
        target_operation = "PROVIDER_REPLAY_REQUEST"
        approval_policy_ref = "admin.workflow.provider_replay.v1"
        workflows = @(
            (New-Workflow -WorkflowID "wf_provider_replay_2" -WorkflowType "REPAIR_APPROVAL" -TargetService "action-executor" -TargetOperation "PROVIDER_REPLAY_REQUEST" -TargetRefHash "sha256:provider-failure-2" -PayloadSchemaVersion "admin.provider_replay_request.v1" -PayloadRefHash "sha256:provider-replay-payload-2" -ApprovalPolicyRef "admin.workflow.provider_replay.v1")
        )
        checked_at = "2026-06-25T00:00:00Z"
        debug_note = $leakMarker
    }
    Write-JsonFile -Path $providerSummaryPath -Value $providerSummary
    Invoke-PageWriter -SummaryPath $providerSummaryPath -OutputPath $providerPagePath

    $operatorHtml = Get-Content -LiteralPath $operatorPagePath -Raw
    $providerHtml = Get-Content -LiteralPath $providerPagePath -Raw
    foreach ($expected in @(
            "NexusIM Workflow Approval Queue Review",
            "action-approval",
            "provider-replay",
            "wf_action_1",
            "wf_provider_replay_1",
            "workflow-service.RecordWorkflowDecision",
            "page_records_decision",
            "False"
        )) {
        if (-not $operatorHtml.Contains($expected)) {
            throw "workflow approval queue review page missing expected operator content: $expected"
        }
    }
    foreach ($expected in @(
            "provider-replay",
            "wf_provider_replay_2",
            "admin.workflow.provider_replay.v1",
            "source_summary_sha256"
        )) {
        if (-not $providerHtml.Contains($expected)) {
            throw "workflow approval queue review page missing expected provider content: $expected"
        }
    }

    foreach ($text in @($operatorHtml, $providerHtml)) {
        foreach ($forbidden in @(
                $operatorSummaryPath,
                $providerSummaryPath,
                $tempRoot,
                $leakMarker,
                "https://",
                "provider_body",
                "raw:",
                "password"
            )) {
            if ($text.Contains($forbidden)) {
                throw "workflow approval queue review page leaked sensitive or local artifact content: $forbidden"
            }
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-approval-queue-review.html"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-PageWriter -SummaryPath $operatorSummaryPath -OutputPath $repoLocalOutput
    }

    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    $badStatus = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badStatus.operator_queues[0].workflows[0].status = "APPROVED"
    $badStatus | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badStatusPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "status must be WAITING_DECISION" -Script {
        Invoke-PageWriter -SummaryPath $badStatusPath -OutputPath (Join-Path $tempRoot "bad-status.html")
    }

    $badCountPath = Join-Path $tempRoot "bad-count.json"
    $badCount = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badCount.operator_queues[0].workflow_count = 7
    $badCount | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badCountPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "workflow_count does not match" -Script {
        Invoke-PageWriter -SummaryPath $badCountPath -OutputPath (Join-Path $tempRoot "bad-count.html")
    }

    $badDecisionPath = Join-Path $tempRoot "bad-decision.json"
    $badDecision = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badDecision | Add-Member -NotePropertyName "decision" -NotePropertyValue ([pscustomobject]@{ decision_id = "decision-1" }) -Force
    $badDecision | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badDecisionPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "recorded decision material" -Script {
        Invoke-PageWriter -SummaryPath $badDecisionPath -OutputPath (Join-Path $tempRoot "bad-decision.html")
    }

    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    $badRaw = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badRaw.operator_queues[0].workflows[0].payload_ref_hash = "https://provider.example/raw"
    $badRaw | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badRawPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-PageWriter -SummaryPath $badRawPath -OutputPath (Join-Path $tempRoot "bad-raw.html")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-approval-queue-review.html") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow approval queue review page self-test"
