$ErrorActionPreference = "Stop"

$deliveryPlanWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
$statusWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-status.ps1"
$redriveWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-redrive-plan.ps1"
$batchInvocationWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-invocation.ps1"
$runner = Join-Path $PSScriptRoot "invoke-workflow-external-callback-batch-redrive.ps1"
foreach ($path in @($deliveryPlanWriter, $statusWriter, $redriveWriter, $batchInvocationWriter, $runner)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback batch redrive runner dependency: $path"
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

function New-DecisionTemplate {
    param(
        [string]$WorkflowID,
        [string]$StepID
    )

    return [ordered]@{
        schema_version = "nexusim.workflow.external_decision_manifest.v1"
        workflow_id = $WorkflowID
        step_id = $StepID
        expected_workflow_type = "ACTION_APPROVAL"
        expected_status = "WAITING_DECISION"
        expected_target_service = "external-crm"
        expected_target_operation = "SYNC_APPROVAL_CALLBACK"
        expected_target_ref_hash = "sha256:target:$WorkflowID"
        expected_payload_schema_version = "external.callback_request.v1"
        expected_payload_ref_hash = "sha256:payload:$WorkflowID"
        expected_approval_policy_ref = "workflow.external_callback.v1"
        decision = ""
        decider_ref = ""
        decision_policy_ref = "workflow.external_callback.decision.v1"
        reason_ref = "reason-sha256:$WorkflowID"
        evidence_refs = @("evidence:$WorkflowID")
        idempotency_key = "external-callback:${WorkflowID}:${StepID}"
        correlation_id = "corr:$WorkflowID"
        causation_id = "workflow:$WorkflowID"
        trace_id = "trace:$WorkflowID"
    }
}

function Invoke-DeliveryPlanWriter {
    param(
        [string]$ManifestPath,
        [string]$OutputPath,
        [string]$PlanID
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $deliveryPlanWriter `
        -DecisionManifestPath $ManifestPath `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -DeliveryPlanID $PlanID `
        -CallbackProviderRef "provider:approval-gateway-a" `
        -CallbackEndpointRef "endpoint:approval-provider-a" `
        -DeliveryQueueRef "queue:workflow-callback-delivery" `
        -RetryPolicyRef "retry:workflow-callback-v1" `
        -BackoffPolicyRef "backoff:workflow-callback-exp-v1" `
        -CallbackTimeoutPolicyRef "timeout:workflow-callback-v1" `
        -MaxAttempts 3 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-StatusWriter {
    param(
        [string]$PlanPath,
        [string]$OutputPath,
        [string]$StatusID,
        [string]$DeliveryStatus,
        [int]$AttemptNumber,
        [string]$DeliveryAttemptRef,
        [string]$FailureClassRef,
        [string]$NextRetryRef = ""
    )
    $args = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass",
        "-File", $statusWriter,
        "-DeliveryPlanPath", $PlanPath,
        "-OutputPath", $OutputPath,
        "-ReportedBy", "operator-a",
        "-StatusID", $StatusID,
        "-DeliveryStatus", $DeliveryStatus,
        "-AttemptNumber", $AttemptNumber,
        "-DeliveryAttemptRef", $DeliveryAttemptRef,
        "-FailureClassRef", $FailureClassRef,
        "-RedrivePolicyRef", "workflow.external-callback-redrive.v1"
    )
    if (-not [string]::IsNullOrWhiteSpace($NextRetryRef)) {
        $args += @("-NextRetryRef", $NextRetryRef)
    }
    $output = & powershell @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-RedriveWriter {
    param(
        [string]$StatusPath,
        [string]$OutputPath,
        [string]$PlanID
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $redriveWriter `
        -DeliveryStatusPath $StatusPath `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -RedrivePlanID $PlanID `
        -RedriveQueueRef "queue:workflow-callback-redrive" `
        -RedriveReasonRef "reason-sha256:redrive" `
        -OperatorReviewRef "review:callback-redrive-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-BatchInvocationWriter {
    param(
        [string]$RedriveRoot,
        [string]$OutputPath
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $batchInvocationWriter `
        -RedrivePlanRootPath $RedriveRoot `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -InvocationID "workflow-external-callback-batch-redrive-invocation-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-BatchRunner {
    param(
        [string]$InvocationPath,
        [string]$PlanRoot,
        [string]$SummaryRoot,
        [string]$ResultPath,
        [string]$StubPath,
        [switch]$AllowMutating
    )

    $workflowArgsJson = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $StubPath) | ConvertTo-Json -Compress
    $workflowArgsBase64 = [System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($workflowArgsJson))
    $args = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass",
        "-File", $runner,
        "-BatchInvocationPath", $InvocationPath,
        "-RedrivePlanRootPath", $PlanRoot,
        "-WorkflowServicePath", (Get-Command powershell).Source,
        "-WorkflowServiceArgumentsJsonBase64", $workflowArgsBase64,
        "-ExecutionSummaryRootPath", $SummaryRoot,
        "-GeneratedBy", "operator-a",
        "-TenantID", "tenant-workflow",
        "-ResultManifestPath", $ResultPath,
        "-ResultManifestID", "workflow-external-callback-batch-redrive-result-1"
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

function Write-StubRuntime {
    param(
        [string]$Path,
        [string]$Mode
    )

    $content = @'
$ErrorActionPreference = "Stop"

function Get-FileSha256Ref {
    param([string]$Path)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path)))
    } finally {
        $sha.Dispose()
    }
    return "sha256:" + (-join ($hash | ForEach-Object { $_.ToString("x2") }))
}

if ($env:NEXUSIM_WORKFLOW_SERVICE_MODE -ne "external-callback-delivery-redrive") {
    exit 11
}
if ([string]::IsNullOrWhiteSpace($env:NEXUSIM_PG_DSN)) {
    exit 12
}

$mode = "__MODE__"
if ($mode -eq "fail") {
    exit 7
}
if ($mode -eq "no-summary") {
    exit 0
}

$planPath = $env:NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE
$summaryPath = $env:NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_SUMMARY_FILE
$tenantID = $env:NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID
$plan = Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
$summary = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_redrive_execution_summary.v1"
    mode = "external-callback-delivery-redrive"
    generated_at = "2026-06-25T00:00:00Z"
    redrive_plan_id = [string]$plan.redrive_plan_id
    redrive_plan_sha256 = Get-FileSha256Ref -Path $planPath
    source_delivery_status_sha256 = [string]$plan.source_delivery_status_sha256
    source_delivery_plan_sha256 = [string]$plan.source_delivery_plan_sha256
    tenant_id = $tenantID
    workflow_id = [string]$plan.workflow_binding.workflow_id
    step_id = [string]$plan.workflow_binding.step_id
    delivery_id = "delivery:$($plan.redrive_plan_id)"
    target_service = [string]$plan.workflow_binding.expected_target_service
    target_operation = [string]$plan.workflow_binding.expected_target_operation
    target_ref_hash = [string]$plan.workflow_binding.expected_target_ref_hash
    payload_schema_version = [string]$plan.workflow_binding.expected_payload_schema_version
    payload_ref_hash = [string]$plan.workflow_binding.expected_payload_ref_hash
    approval_policy_ref = [string]$plan.workflow_binding.expected_approval_policy_ref
    decision_policy_ref = [string]$plan.workflow_binding.decision_policy_ref
    delivery_status = "PENDING"
    redrive_count = 1
    last_redrive_plan_sha256 = Get-FileSha256Ref -Path $planPath
    last_redrive_reason_ref = [string]$plan.redrive_contract.redrive_reason_ref
    outbox_event_type = "workflow.external_callback.redriven.v1"
    executed_redrive = $true
    records_decision = $false
    calls_provider = $false
    executes_target = $false
    mutates_delivery_fact = $true
}
if ($mode -eq "raw-summary") {
    $summary.provider_body = "provider raw body"
}
$parent = Split-Path -Parent $summaryPath
if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
}
$summary | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
'@
    $content = $content.Replace("__MODE__", $Mode)
    Set-Content -LiteralPath $Path -Value $content -Encoding UTF8
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-callback-batch-redrive-runner-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
$oldPGDSN = $env:NEXUSIM_PG_DSN
try {
    $manifestRoot = Join-Path $tempRoot "manifests"
    $planRoot = Join-Path $tempRoot "plans"
    $statusRoot = Join-Path $tempRoot "statuses"
    $redriveRoot = Join-Path $tempRoot "redrives"
    $summaryRoot = Join-Path $tempRoot "summaries"
    New-Item -ItemType Directory -Force -Path $manifestRoot, $planRoot, $statusRoot, $redriveRoot, $summaryRoot | Out-Null

    foreach ($case in @(
            @{ workflow = "wf_callback_runner_retry"; step = "wfs_retry"; status = "RETRY_PENDING"; attempt = 1; failure = "failure:provider-unavailable"; next = "retry:next:wf_callback_runner_retry" },
            @{ workflow = "wf_callback_runner_dlq"; step = "wfs_dlq"; status = "DLQ"; attempt = 3; failure = "failure:retry-exhausted"; next = "" }
        )) {
        $manifestPath = Join-Path $manifestRoot "$($case.workflow).json"
        $planPath = Join-Path $planRoot "$($case.workflow)-plan.json"
        $statusPath = Join-Path $statusRoot "$($case.workflow)-status.json"
        $redrivePath = Join-Path $redriveRoot "$($case.workflow)-redrive.json"
        Write-JsonFile -Path $manifestPath -Value (New-DecisionTemplate -WorkflowID $case.workflow -StepID $case.step)
        Invoke-DeliveryPlanWriter -ManifestPath $manifestPath -OutputPath $planPath -PlanID "plan:$($case.workflow)"
        Invoke-StatusWriter `
            -PlanPath $planPath `
            -OutputPath $statusPath `
            -StatusID "status:$($case.workflow)" `
            -DeliveryStatus $case.status `
            -AttemptNumber $case.attempt `
            -DeliveryAttemptRef "attempt:$($case.workflow)" `
            -FailureClassRef $case.failure `
            -NextRetryRef $case.next
        Invoke-RedriveWriter -StatusPath $statusPath -OutputPath $redrivePath -PlanID "redrive:$($case.workflow)"
    }

    $invocationPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-invocation.json"
    Invoke-BatchInvocationWriter -RedriveRoot $redriveRoot -OutputPath $invocationPath

    $stubPath = Join-Path $tempRoot "workflow-service-stub.ps1"
    Write-StubRuntime -Path $stubPath -Mode "success"
    $resultPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-result.json"
    $env:NEXUSIM_PG_DSN = "postgres://test-redacted"
    $runnerOutput = Invoke-BatchRunner `
        -InvocationPath $invocationPath `
        -PlanRoot $redriveRoot `
        -SummaryRoot $summaryRoot `
        -ResultPath $resultPath `
        -StubPath $stubPath `
        -AllowMutating
    $runnerSummary = $runnerOutput | ConvertFrom-Json
    $result = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json

    if ($runnerSummary.schema_version -ne "nexusim.workflow.external_callback_batch_redrive_runner.v1" -or
        [int]$runnerSummary.redrive_count -ne 2 -or
        $runnerSummary.mode -ne "external-callback-delivery-redrive" -or
        $runnerSummary.tenant_id -ne "tenant-workflow" -or
        [bool]$runnerSummary.called_workflow_service_runtime -ne $true -or
        [bool]$runnerSummary.records_decision -or
        [bool]$runnerSummary.calls_provider -or
        [bool]$runnerSummary.executes_target -or
        [bool]$runnerSummary.mutates_delivery_fact -ne $true) {
        throw "workflow external callback batch redrive runner summary has unexpected fields."
    }
    if ($result.schema_version -ne "nexusim.workflow.external_callback_batch_redrive_result.v1" -or
        [int]$result.result_count -ne 2) {
        throw "workflow external callback batch redrive runner did not write a valid result manifest."
    }

    $runnerRaw = $runnerOutput + (Get-Content -LiteralPath $resultPath -Raw)
    foreach ($forbidden in @(
            $tempRoot,
            $stubPath,
            "postgres://test-redacted",
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($runnerRaw.Contains($forbidden)) {
            throw "workflow external callback batch redrive runner leaked forbidden content: $forbidden"
        }
    }

    Invoke-ExpectFailure -Expected "without -AllowMutating" -Script {
        Invoke-BatchRunner `
            -InvocationPath $invocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot (Join-Path $tempRoot "no-allow") `
            -ResultPath (Join-Path $tempRoot "no-allow.json") `
            -StubPath $stubPath
    }

    $env:NEXUSIM_PG_DSN = ""
    Invoke-ExpectFailure -Expected "NEXUSIM_PG_DSN is required" -Script {
        Invoke-BatchRunner `
            -InvocationPath $invocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot (Join-Path $tempRoot "no-dsn") `
            -ResultPath (Join-Path $tempRoot "no-dsn.json") `
            -StubPath $stubPath `
            -AllowMutating
    }
    $env:NEXUSIM_PG_DSN = "postgres://test-redacted"

    $badInvocationPath = Join-Path $tempRoot "bad-invocation.json"
    $badInvocation = Get-Content -LiteralPath $invocationPath -Raw | ConvertFrom-Json
    $badInvocation.redrives[0].redrive_plan_sha256 = "sha256:missing-plan"
    Write-JsonFile -Path $badInvocationPath -Value $badInvocation
    Invoke-ExpectFailure -Expected "Missing redrive plan file" -Script {
        Invoke-BatchRunner `
            -InvocationPath $badInvocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot (Join-Path $tempRoot "bad-plan") `
            -ResultPath (Join-Path $tempRoot "bad-plan.json") `
            -StubPath $stubPath `
            -AllowMutating
    }

    $noSummaryStub = Join-Path $tempRoot "workflow-service-no-summary.ps1"
    Write-StubRuntime -Path $noSummaryStub -Mode "no-summary"
    Invoke-ExpectFailure -Expected "did not write execution summary" -Script {
        Invoke-BatchRunner `
            -InvocationPath $invocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot (Join-Path $tempRoot "no-summary") `
            -ResultPath (Join-Path $tempRoot "no-summary.json") `
            -StubPath $noSummaryStub `
            -AllowMutating
    }

    $failStub = Join-Path $tempRoot "workflow-service-fail.ps1"
    Write-StubRuntime -Path $failStub -Mode "fail"
    Invoke-ExpectFailure -Expected "exit_code=7" -Script {
        Invoke-BatchRunner `
            -InvocationPath $invocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot (Join-Path $tempRoot "runtime-fail") `
            -ResultPath (Join-Path $tempRoot "runtime-fail.json") `
            -StubPath $failStub `
            -AllowMutating
    }

    $rawStub = Join-Path $tempRoot "workflow-service-raw.ps1"
    Write-StubRuntime -Path $rawStub -Mode "raw-summary"
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-BatchRunner `
            -InvocationPath $invocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot (Join-Path $tempRoot "raw-summary") `
            -ResultPath (Join-Path $tempRoot "raw-summary.json") `
            -StubPath $rawStub `
            -AllowMutating
    }

    $repoLocalSummary = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-batch-redrive-runner"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-BatchRunner `
            -InvocationPath $invocationPath `
            -PlanRoot $redriveRoot `
            -SummaryRoot $repoLocalSummary `
            -ResultPath (Join-Path $tempRoot "repo-local.json") `
            -StubPath $stubPath `
            -AllowMutating
    }
} finally {
    $env:NEXUSIM_PG_DSN = $oldPGDSN
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-batch-redrive-runner") -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback batch redrive runner self-test"
