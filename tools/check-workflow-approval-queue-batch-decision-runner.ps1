$ErrorActionPreference = "Stop"

$batchWriterPath = Join-Path $PSScriptRoot "write-workflow-approval-queue-batch-decision-manifest.ps1"
$runnerPath = Join-Path $PSScriptRoot "invoke-workflow-approval-queue-batch-decision.ps1"
foreach ($path in @($batchWriterPath, $runnerPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow approval queue batch decision runner dependency: $path"
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
                        workflow_id = "wf_batch_decision_runner_1"
                        workflow_type = "ACTION_APPROVAL"
                        status = "WAITING_DECISION"
                        current_step_id = "wfs_batch_decision_runner_1"
                        target_service = "action-executor"
                        target_operation = "EXECUTE_APPROVED_ACTION"
                        target_ref_hash = "sha256:target:runner-1"
                        payload_schema_version = "agent.action.v1"
                        payload_ref_hash = "sha256:payload:runner-1"
                        approval_policy_ref = "workflow.action-approval.v1"
                        reason_ref = "reason:runner-1"
                    },
                    [ordered]@{
                        workflow_id = "wf_batch_decision_runner_2"
                        workflow_type = "ACTION_APPROVAL"
                        status = "WAITING_DECISION"
                        current_step_id = "wfs_batch_decision_runner_2"
                        target_service = "action-executor"
                        target_operation = "EXECUTE_APPROVED_ACTION"
                        target_ref_hash = "sha256:target:runner-2"
                        payload_schema_version = "agent.action.v1"
                        payload_ref_hash = "sha256:payload:runner-2"
                        approval_policy_ref = "workflow.action-approval.v1"
                        reason_ref = "reason:runner-2"
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
        -BatchDecisionID "workflow-approval-queue-batch-decision-runner-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Write-StubWorkflowOperator {
    param(
        [string]$Path,
        [string]$Mode
    )

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

$stubMode = "__MODE__"
if ($stubMode -eq "fail") {
    exit 7
}

$commandMode = Read-ArgValue "-mode"
if ($commandMode -ne "record-decision") {
    exit 11
}
$manifestPath = Read-ArgValue "-decision-manifest"
if ([string]::IsNullOrWhiteSpace($manifestPath)) {
    exit 12
}
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
if ($stubMode -eq "no-json") {
    Write-Host "not-json"
    exit 0
}

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
if ($stubMode -eq "raw") {
    $result.provider_body = "provider raw body"
}
$result | ConvertTo-Json -Depth 20
'@
    $content = $content.Replace("__MODE__", $Mode)
    Set-Content -LiteralPath $Path -Value $content -Encoding UTF8
}

function Invoke-BatchRunner {
    param(
        [string]$BatchPath,
        [string]$DecisionRoot,
        [string]$SummaryRoot,
        [string]$ResultPath,
        [string]$StubPath,
        [switch]$AllowMutating
    )

    $operatorArgsJson = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $StubPath) | ConvertTo-Json -Compress
    $operatorArgsBase64 = [System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($operatorArgsJson))
    $args = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass",
        "-File", $runnerPath,
        "-BatchDecisionPath", $BatchPath,
        "-DecisionManifestRootPath", $DecisionRoot,
        "-WorkflowOperatorPath", (Get-Command powershell).Source,
        "-WorkflowOperatorArgumentsJsonBase64", $operatorArgsBase64,
        "-ExecutionSummaryRootPath", $SummaryRoot,
        "-GeneratedBy", "operator-a",
        "-ResultManifestPath", $ResultPath,
        "-ResultManifestID", "workflow-approval-queue-batch-decision-result-1"
    )
    if ($AllowMutating) {
        $args += "-AllowMutating"
    }
    $output = & powershell @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    return ($output | Out-String)
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-approval-queue-batch-decision-runner-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $queuePath = Join-Path $tempRoot "workflow-operator-queues.json"
    $reasonFile = Join-Path $tempRoot "reason.txt"
    $decisionRoot = Join-Path $tempRoot "decision-manifests"
    $summaryRoot = Join-Path $tempRoot "execution-summaries"
    $batchPath = Join-Path $tempRoot "workflow-batch-decision.json"
    $resultPath = Join-Path $tempRoot "workflow-batch-decision-result.json"

    Write-JsonFile -Path $queuePath -Value (New-OperatorQueueSummary)
    Set-Content -LiteralPath $reasonFile -Value "operator reviewed batch decision request" -Encoding UTF8
    Invoke-BatchWriter -QueueSummaryPath $queuePath -DecisionRoot $decisionRoot -BatchPath $batchPath -ReasonFile $reasonFile

    $stubPath = Join-Path $tempRoot "workflow-operator-stub.ps1"
    Write-StubWorkflowOperator -Path $stubPath -Mode "success"
    $runnerOutput = Invoke-BatchRunner `
        -BatchPath $batchPath `
        -DecisionRoot $decisionRoot `
        -SummaryRoot $summaryRoot `
        -ResultPath $resultPath `
        -StubPath $stubPath `
        -AllowMutating
    $runnerSummary = $runnerOutput | ConvertFrom-Json
    $result = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json

    if ($runnerSummary.schema_version -ne "nexusim.workflow.approval_queue_batch_decision_result.v1" -or
        [int]$runnerSummary.decision_count -ne 2 -or
        [bool]$runnerSummary.called_workflow_service_runtime -ne $true -or
        [bool]$runnerSummary.records_decision -ne $true -or
        [bool]$runnerSummary.calls_action_executor -or
        [bool]$runnerSummary.executes_target -or
        [bool]$runnerSummary.mutates_workflow_fact -ne $true) {
        throw "workflow approval queue batch decision runner summary has unexpected fields."
    }
    if ($result.schema_version -ne "nexusim.workflow.approval_queue_batch_decision_result.v1" -or
        [int]$result.decision_count -ne 2) {
        throw "workflow approval queue batch decision runner did not write a valid result manifest."
    }
    foreach ($item in @($result.items)) {
        if ($item.decision -ne "APPROVE" -or $item.decision_type -ne "APPROVE" -or $item.workflow_status -ne "APPROVED") {
            throw "workflow approval queue batch decision result item mismatch."
        }
    }

    $runnerRaw = $runnerOutput + (Get-Content -LiteralPath $resultPath -Raw)
    foreach ($forbidden in @(
            $tempRoot,
            $stubPath,
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($runnerRaw.Contains($forbidden)) {
            throw "workflow approval queue batch decision runner leaked forbidden content: $forbidden"
        }
    }

    Invoke-ExpectFailure -Expected "without -AllowMutating" -Script {
        Invoke-BatchRunner `
            -BatchPath $batchPath `
            -DecisionRoot $decisionRoot `
            -SummaryRoot (Join-Path $tempRoot "no-allow") `
            -ResultPath (Join-Path $tempRoot "no-allow.json") `
            -StubPath $stubPath
    }

    $repoLocalSummary = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-batch-decision-runner"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-BatchRunner `
            -BatchPath $batchPath `
            -DecisionRoot $decisionRoot `
            -SummaryRoot $repoLocalSummary `
            -ResultPath (Join-Path $tempRoot "repo-local.json") `
            -StubPath $stubPath `
            -AllowMutating
    }

    $badBatchPath = Join-Path $tempRoot "bad-records-decision.json"
    $badBatch = Get-Content -LiteralPath $batchPath -Raw | ConvertFrom-Json
    $badBatch.records_decision = $true
    Write-JsonFile -Path $badBatchPath -Value $badBatch
    Invoke-ExpectFailure -Expected "must not record decisions" -Script {
        Invoke-BatchRunner `
            -BatchPath $badBatchPath `
            -DecisionRoot $decisionRoot `
            -SummaryRoot (Join-Path $tempRoot "bad-records") `
            -ResultPath (Join-Path $tempRoot "bad-records.json") `
            -StubPath $stubPath `
            -AllowMutating
    }

    $missingManifestPath = Join-Path $tempRoot "missing-manifest.json"
    $missingManifestBatch = Get-Content -LiteralPath $batchPath -Raw | ConvertFrom-Json
    $missingManifestBatch.items[0].decision_manifest_sha256 = "sha256:missing-manifest"
    Write-JsonFile -Path $missingManifestPath -Value $missingManifestBatch
    Invoke-ExpectFailure -Expected "Missing decision manifest file" -Script {
        Invoke-BatchRunner `
            -BatchPath $missingManifestPath `
            -DecisionRoot $decisionRoot `
            -SummaryRoot (Join-Path $tempRoot "missing-summary") `
            -ResultPath (Join-Path $tempRoot "missing-result.json") `
            -StubPath $stubPath `
            -AllowMutating
    }

    $failStub = Join-Path $tempRoot "workflow-operator-fail.ps1"
    Write-StubWorkflowOperator -Path $failStub -Mode "fail"
    Invoke-ExpectFailure -Expected "runtime failed" -Script {
        Invoke-BatchRunner `
            -BatchPath $batchPath `
            -DecisionRoot $decisionRoot `
            -SummaryRoot (Join-Path $tempRoot "runtime-fail") `
            -ResultPath (Join-Path $tempRoot "runtime-fail.json") `
            -StubPath $failStub `
            -AllowMutating
    }

    $rawStub = Join-Path $tempRoot "workflow-operator-raw.ps1"
    Write-StubWorkflowOperator -Path $rawStub -Mode "raw"
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-BatchRunner `
            -BatchPath $batchPath `
            -DecisionRoot $decisionRoot `
            -SummaryRoot (Join-Path $tempRoot "runtime-raw") `
            -ResultPath (Join-Path $tempRoot "runtime-raw.json") `
            -StubPath $rawStub `
            -AllowMutating
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-batch-decision-runner") -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow approval queue batch decision runner self-test"
