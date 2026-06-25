$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-approval-queue-batch-decision-manifest.ps1"
$validatorPath = Join-Path $PSScriptRoot "validate-workflow-decision-manifest.ps1"
foreach ($path in @($writerPath, $validatorPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow approval queue batch decision dependency: $path"
    }
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

function Invoke-BatchWriter {
    param(
        [string]$SummaryPath,
        [string]$OutputRoot,
        [string]$BatchManifestPath,
        [string]$Decision = "APPROVE",
        [string]$DeciderRef = "operator-a",
        [string]$ReasonFile = ""
    )

    $args = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass",
        "-File", $writerPath,
        "-QueueSummaryPath", $SummaryPath,
        "-OutputRootPath", $OutputRoot,
        "-BatchManifestPath", $BatchManifestPath,
        "-BatchDecisionID", "workflow-approval-queue-batch-decision-1",
        "-Decision", $Decision,
        "-DeciderRef", $DeciderRef,
        "-DecisionPolicyRef", "workflow.external-approval.v1",
        "-EvidenceRef", "evidence:operator-review-1,evidence:ticket-1"
    )
    if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
        $args += @("-ReasonFile", $ReasonFile)
    } else {
        $args += @("-ReasonRef", "reason-sha256:operator-review")
    }
    $output = & powershell @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Assert-ValidDecisionManifest {
    param(
        [string]$Path,
        [string]$WorkflowID,
        [string]$StepID,
        [string]$Decision
    )

    $summaryRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
        -ManifestPath $Path `
        -ExpectedWorkflowID $WorkflowID `
        -ExpectedStepID $StepID `
        -ExpectedDecision $Decision
    if ($LASTEXITCODE -ne 0) {
        throw "generated workflow decision manifest failed validation: $Path"
    }
    $summary = ($summaryRaw -join "`n") | ConvertFrom-Json
    if ($summary.workflow_id -ne $WorkflowID -or $summary.step_id -ne $StepID -or $summary.decision -ne $Decision) {
        throw "workflow decision manifest summary mismatch for $WorkflowID/$StepID."
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-batch-decision-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $operatorSummaryPath = Join-Path $tempRoot "operator-queues.json"
    $providerSummaryPath = Join-Path $tempRoot "provider-replay-queue.json"
    $reasonPath = Join-Path $tempRoot "operator-reason.txt"
    "operator reviewed approved queue" | Set-Content -LiteralPath $reasonPath -Encoding UTF8

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
                    (New-Workflow -WorkflowID "wf_action_batch_1" -WorkflowType "ACTION_APPROVAL" -TargetService "conversation-service" -TargetOperation "UPSERT_CONVERSATION_NOTE" -TargetRefHash "sha256:target" -PayloadSchemaVersion "agent.action.v1" -PayloadRefHash "sha256:payload" -ApprovalPolicyRef "agent.action.approval.v1" -CurrentStepID "wfs_action_batch_1")
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
                    (New-Workflow -WorkflowID "wf_provider_batch_1" -WorkflowType "REPAIR_APPROVAL" -TargetService "action-executor" -TargetOperation "PROVIDER_REPLAY_REQUEST" -TargetRefHash "sha256:provider-failure" -PayloadSchemaVersion "admin.provider_replay_request.v1" -PayloadRefHash "sha256:provider-replay-payload" -ApprovalPolicyRef "admin.workflow.provider_replay.v1" -CurrentStepID "wfs_provider_batch_1")
                )
            }
        )
        checked_at = "2026-06-25T00:00:00Z"
    }
    Write-JsonFile -Path $operatorSummaryPath -Value $operatorSummary

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
            (New-Workflow -WorkflowID "wf_provider_batch_2" -WorkflowType "REPAIR_APPROVAL" -TargetService "action-executor" -TargetOperation "PROVIDER_REPLAY_REQUEST" -TargetRefHash "sha256:provider-failure-2" -PayloadSchemaVersion "admin.provider_replay_request.v1" -PayloadRefHash "sha256:provider-replay-payload-2" -ApprovalPolicyRef "admin.workflow.provider_replay.v1" -CurrentStepID "wfs_provider_batch_2")
        )
        checked_at = "2026-06-25T00:00:00Z"
    }
    Write-JsonFile -Path $providerSummaryPath -Value $providerSummary

    $operatorOutputRoot = Join-Path $tempRoot "operator-output"
    $operatorBatchPath = Join-Path $tempRoot "operator-batch.json"
    Invoke-BatchWriter -SummaryPath $operatorSummaryPath -OutputRoot $operatorOutputRoot -BatchManifestPath $operatorBatchPath -Decision "APPROVE" -ReasonFile $reasonPath
    $operatorBatchRaw = Get-Content -LiteralPath $operatorBatchPath -Raw
    $operatorBatch = $operatorBatchRaw | ConvertFrom-Json
    if ($operatorBatch.schema_version -ne "nexusim.workflow.approval_queue_batch_decision.v1" -or
        $operatorBatch.batch_decision_id -ne "workflow-approval-queue-batch-decision-1" -or
        [int]$operatorBatch.decision_count -ne 2 -or
        [bool]$operatorBatch.records_decision -or
        [bool]$operatorBatch.calls_workflow_service -or
        [bool]$operatorBatch.calls_action_executor -or
        [bool]$operatorBatch.executes_target -or
        [bool]$operatorBatch.requires_record_workflow_decision -ne $true) {
        throw "workflow approval queue batch decision manifest has unexpected fields."
    }
    foreach ($item in @($operatorBatch.items)) {
        $decisionPath = Join-Path $operatorOutputRoot (($item.workflow_id + "-" + $item.step_id).Replace(":", "_") + "-decision.json")
        Assert-ValidDecisionManifest -Path $decisionPath -WorkflowID $item.workflow_id -StepID $item.step_id -Decision "APPROVE"
    }

    $providerOutputRoot = Join-Path $tempRoot "provider-output"
    $providerBatchPath = Join-Path $tempRoot "provider-batch.json"
    Invoke-BatchWriter -SummaryPath $providerSummaryPath -OutputRoot $providerOutputRoot -BatchManifestPath $providerBatchPath -Decision "REJECT"
    $providerBatch = Get-Content -LiteralPath $providerBatchPath -Raw | ConvertFrom-Json
    if ([int]$providerBatch.decision_count -ne 1 -or $providerBatch.items[0].decision -ne "REJECT") {
        throw "provider replay batch decision manifest has unexpected fields."
    }

    $combinedRaw = $operatorBatchRaw + (Get-Content -LiteralPath $providerBatchPath -Raw)
    foreach ($forbidden in @(
            $tempRoot,
            $operatorSummaryPath,
            $providerSummaryPath,
            $reasonPath,
            "operator reviewed approved queue",
            "https://",
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($combinedRaw.Contains($forbidden)) {
            throw "workflow approval queue batch decision leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-batch-decision"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-BatchWriter -SummaryPath $operatorSummaryPath -OutputRoot $repoLocalOutput -BatchManifestPath (Join-Path $tempRoot "repo-local.json")
    }

    $badDecisionPath = Join-Path $tempRoot "bad-decision.json"
    $badDecision = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badDecision | Add-Member -NotePropertyName "decision" -NotePropertyValue ([pscustomobject]@{ decision_id = "decision-1" }) -Force
    Write-JsonFile -Path $badDecisionPath -Value $badDecision
    Invoke-ExpectFailure -Expected "recorded decision material" -Script {
        Invoke-BatchWriter -SummaryPath $badDecisionPath -OutputRoot (Join-Path $tempRoot "bad-decision-output") -BatchManifestPath (Join-Path $tempRoot "bad-decision-batch.json")
    }

    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    $badStatus = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badStatus.operator_queues[0].workflows[0].status = "APPROVED"
    Write-JsonFile -Path $badStatusPath -Value $badStatus
    Invoke-ExpectFailure -Expected "status must be WAITING_DECISION" -Script {
        Invoke-BatchWriter -SummaryPath $badStatusPath -OutputRoot (Join-Path $tempRoot "bad-status-output") -BatchManifestPath (Join-Path $tempRoot "bad-status-batch.json")
    }

    $badBindingPath = Join-Path $tempRoot "bad-binding.json"
    $badBinding = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badBinding.operator_queues[1].workflows[0].target_operation = "OTHER_OPERATION"
    Write-JsonFile -Path $badBindingPath -Value $badBinding
    Invoke-ExpectFailure -Expected "target_operation does not match" -Script {
        Invoke-BatchWriter -SummaryPath $badBindingPath -OutputRoot (Join-Path $tempRoot "bad-binding-output") -BatchManifestPath (Join-Path $tempRoot "bad-binding-batch.json")
    }

    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    $badRaw = Get-Content -LiteralPath $operatorSummaryPath -Raw | ConvertFrom-Json
    $badRaw.operator_queues[0].workflows[0].payload_ref_hash = "https://provider.example/raw"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "QueueSummaryPath contains raw" -Script {
        Invoke-BatchWriter -SummaryPath $badRawPath -OutputRoot (Join-Path $tempRoot "bad-raw-output") -BatchManifestPath (Join-Path $tempRoot "bad-raw-batch.json")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-batch-decision") -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow approval queue batch decision manifest self-test"
