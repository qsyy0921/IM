$ErrorActionPreference = "Stop"

$batchWriterPath = Join-Path $PSScriptRoot "write-workflow-approval-queue-batch-decision-manifest.ps1"
$runnerPath = Join-Path $PSScriptRoot "invoke-workflow-approval-queue-batch-decision.ps1"
$pageWriterPath = Join-Path $PSScriptRoot "write-workflow-approval-queue-decision-result-page.ps1"
foreach ($path in @($batchWriterPath, $runnerPath, $pageWriterPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow approval queue decision result page dependency: $path"
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

function New-OperatorQueueSummary {
    return [ordered]@{
        mode = "operator-queues"
        tenant_id = "tenant-workflow"
        checked_at = "2026-06-25T00:00:00Z"
        operator_queues = @(
            [ordered]@{
                queue_id = "action-approval"
                workflow_type = "ACTION_APPROVAL"
                status = "WAITING_DECISION"
                target_service = "action-executor"
                target_operation = "EXECUTE_APPROVED_ACTION"
                approval_policy_ref = "workflow.action-approval.v1"
                workflow_count = 2
                workflows = @(
                    [ordered]@{
                        workflow_id = "wf_batch_result_page_1"
                        workflow_type = "ACTION_APPROVAL"
                        status = "WAITING_DECISION"
                        current_step_id = "wfs_batch_result_page_1"
                        target_service = "action-executor"
                        target_operation = "EXECUTE_APPROVED_ACTION"
                        target_ref_hash = "sha256:target:result-page-1"
                        payload_schema_version = "agent.action.v1"
                        payload_ref_hash = "sha256:payload:result-page-1"
                        approval_policy_ref = "workflow.action-approval.v1"
                        reason_ref = "reason:result-page-1"
                    },
                    [ordered]@{
                        workflow_id = "wf_batch_result_page_2"
                        workflow_type = "ACTION_APPROVAL"
                        status = "WAITING_DECISION"
                        current_step_id = "wfs_batch_result_page_2"
                        target_service = "action-executor"
                        target_operation = "EXECUTE_APPROVED_ACTION"
                        target_ref_hash = "sha256:target:result-page-2"
                        payload_schema_version = "agent.action.v1"
                        payload_ref_hash = "sha256:payload:result-page-2"
                        approval_policy_ref = "workflow.action-approval.v1"
                        reason_ref = "reason:result-page-2"
                    }
                )
            }
        )
    }
}

function Invoke-BatchWriter {
    param(
        [string]$QueueSummaryPath,
        [string]$DecisionRoot,
        [string]$BatchPath,
        [string]$ReasonFile
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $batchWriterPath `
        -QueueSummaryPath $QueueSummaryPath `
        -OutputRootPath $DecisionRoot `
        -Decision APPROVE `
        -DeciderRef "operator-a" `
        -ReasonFile $ReasonFile `
        -BatchManifestPath $BatchPath `
        -BatchDecisionID "workflow-approval-queue-batch-decision-result-page-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Write-StubWorkflowOperator {
    param([string]$Path)

    $content = @'
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RemainingArgs
)

$ErrorActionPreference = "Stop"

function Read-ArgValue {
    param([string]$Name)
    for ($i = 0; $i -lt $RemainingArgs.Count; $i++) {
        if ($RemainingArgs[$i] -eq $Name -and ($i + 1) -lt $RemainingArgs.Count) {
            return [string]$RemainingArgs[$i + 1]
        }
    }
    return ""
}

if ((Read-ArgValue "-mode") -ne "record-decision") {
    exit 11
}
$manifestPath = Read-ArgValue "-decision-manifest"
if ([string]::IsNullOrWhiteSpace($manifestPath)) {
    exit 12
}
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$result = [ordered]@{
    mode = "record-decision"
    tenant_id = "tenant-workflow"
    workflow_id = [string]$manifest.workflow_id
    workflow = [ordered]@{
        workflow_id = [string]$manifest.workflow_id
        workflow_type = [string]$manifest.expected_workflow_type
        status = "APPROVED"
        current_step_id = [string]$manifest.step_id
        target_service = [string]$manifest.expected_target_service
        target_operation = [string]$manifest.expected_target_operation
        target_ref_hash = [string]$manifest.expected_target_ref_hash
        payload_schema_version = [string]$manifest.expected_payload_schema_version
        payload_ref_hash = [string]$manifest.expected_payload_ref_hash
        approval_policy_ref = [string]$manifest.expected_approval_policy_ref
    }
    decision = [ordered]@{
        decision_id = "decision:$($manifest.workflow_id):$($manifest.step_id)"
        workflow_id = [string]$manifest.workflow_id
        step_id = [string]$manifest.step_id
        decider_ref = [string]$manifest.decider_ref
        decision_type = [string]$manifest.decision
        decision_policy_ref = [string]$manifest.decision_policy_ref
        reason_ref = [string]$manifest.reason_ref
        evidence_refs = @($manifest.evidence_refs)
        created_at_unix_ms = 1782110000000
    }
    replayed = $false
    checked_at = "2026-06-25T00:00:00Z"
}
$result | ConvertTo-Json -Depth 20
'@
    Set-Content -LiteralPath $Path -Value $content -Encoding UTF8
}

function Invoke-BatchRunner {
    param(
        [string]$BatchPath,
        [string]$DecisionRoot,
        [string]$SummaryRoot,
        [string]$ResultPath,
        [string]$StubPath
    )

    $operatorArgsJson = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $StubPath) | ConvertTo-Json -Compress
    $operatorArgsBase64 = [System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($operatorArgsJson))
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $runnerPath `
        -BatchDecisionPath $BatchPath `
        -DecisionManifestRootPath $DecisionRoot `
        -WorkflowOperatorPath (Get-Command powershell).Source `
        -WorkflowOperatorArgumentsJsonBase64 $operatorArgsBase64 `
        -ExecutionSummaryRootPath $SummaryRoot `
        -GeneratedBy "operator-a" `
        -ResultManifestPath $ResultPath `
        -ResultManifestID "workflow-approval-queue-batch-decision-result-page-1" `
        -AllowMutating 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-PageWriter {
    param(
        [string]$ResultPath,
        [string]$HtmlPath,
        [string]$SummaryPath
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
        -ResultManifestPath $ResultPath `
        -GeneratedBy "operator-a" `
        -OutputPath $HtmlPath `
        -SummaryPath $SummaryPath `
        -PageID "workflow-approval-queue-decision-result-review-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    return ($output | Out-String)
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-approval-queue-decision-result-page-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $queuePath = Join-Path $tempRoot "workflow-operator-queues.json"
    $reasonFile = Join-Path $tempRoot "reason.txt"
    $decisionRoot = Join-Path $tempRoot "decision-manifests"
    $summaryRoot = Join-Path $tempRoot "execution-summaries"
    $batchPath = Join-Path $tempRoot "workflow-batch-decision.json"
    $resultPath = Join-Path $tempRoot "workflow-batch-decision-result.json"
    $htmlPath = Join-Path $tempRoot "workflow-batch-decision-result-review.html"
    $summaryPath = Join-Path $tempRoot "workflow-batch-decision-result-review.json"

    Write-JsonFile -Path $queuePath -Value (New-OperatorQueueSummary)
    Set-Content -LiteralPath $reasonFile -Value "operator reviewed batch decision result page" -Encoding UTF8
    Invoke-BatchWriter -QueueSummaryPath $queuePath -DecisionRoot $decisionRoot -BatchPath $batchPath -ReasonFile $reasonFile

    $stubPath = Join-Path $tempRoot "workflow-operator-stub.ps1"
    Write-StubWorkflowOperator -Path $stubPath
    Invoke-BatchRunner -BatchPath $batchPath -DecisionRoot $decisionRoot -SummaryRoot $summaryRoot -ResultPath $resultPath -StubPath $stubPath

    $pageOutput = Invoke-PageWriter -ResultPath $resultPath -HtmlPath $htmlPath -SummaryPath $summaryPath
    $summary = $pageOutput | ConvertFrom-Json
    $summaryFile = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    $html = Get-Content -LiteralPath $htmlPath -Raw

    if ($summary.schema_version -ne "nexusim.workflow.approval_queue_decision_result_review.v1" -or
        $summaryFile.schema_version -ne "nexusim.workflow.approval_queue_decision_result_review.v1" -or
        [int]$summary.decision_count -ne 2 -or
        $summary.decision_owner -ne "workflow-service.RecordWorkflowDecision" -or
        [bool]$summary.review_page_calls_workflow_service -or
        [bool]$summary.review_page_records_decision -or
        [bool]$summary.review_page_calls_action_executor -or
        [bool]$summary.review_page_executes_target) {
        throw "workflow approval queue decision result review summary has unexpected fields."
    }
    if (-not $html.Contains("Workflow Approval Queue Decision Result Review") -or
        -not $html.Contains("wf_batch_result_page_1") -or
        -not $html.Contains("decision:wf_batch_result_page_1:wfs_batch_result_page_1") -or
        -not $html.Contains("workflow-service.RecordWorkflowDecision")) {
        throw "workflow approval queue decision result review page is missing expected content."
    }

    $combined = $pageOutput + $html + (Get-Content -LiteralPath $summaryPath -Raw)
    foreach ($forbidden in @(
            $tempRoot,
            $stubPath,
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($combined.Contains($forbidden)) {
            throw "workflow approval queue decision result review leaked forbidden content: $forbidden"
        }
    }

    $badExecutorPath = Join-Path $tempRoot "bad-calls-action-executor.json"
    $badExecutor = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
    $badExecutor.calls_action_executor = $true
    Write-JsonFile -Path $badExecutorPath -Value $badExecutor
    Invoke-ExpectFailure -Expected "must not call action-executor" -Script {
        Invoke-PageWriter -ResultPath $badExecutorPath -HtmlPath (Join-Path $tempRoot "bad-executor.html") -SummaryPath (Join-Path $tempRoot "bad-executor-review.json")
    }

    $badDecisionPath = Join-Path $tempRoot "bad-decision-type.json"
    $badDecision = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
    $badDecision.items[0].decision_type = "REJECT"
    Write-JsonFile -Path $badDecisionPath -Value $badDecision
    Invoke-ExpectFailure -Expected "must match decision_type" -Script {
        Invoke-PageWriter -ResultPath $badDecisionPath -HtmlPath (Join-Path $tempRoot "bad-decision.html") -SummaryPath (Join-Path $tempRoot "bad-decision-review.json")
    }

    $badRawPath = Join-Path $tempRoot "bad-raw.json"
    $badRaw = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
    $badRaw.items[0] | Add-Member -NotePropertyName "provider_body" -NotePropertyValue "raw provider body"
    Write-JsonFile -Path $badRawPath -Value $badRaw
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-PageWriter -ResultPath $badRawPath -HtmlPath (Join-Path $tempRoot "bad-raw.html") -SummaryPath (Join-Path $tempRoot "bad-raw-review.json")
    }

    $repoLocalHtml = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-decision-result-review.html"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-PageWriter -ResultPath $resultPath -HtmlPath $repoLocalHtml -SummaryPath (Join-Path $tempRoot "repo-local-review.json")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-decision-result-review.html") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow approval queue decision result review page self-test"
