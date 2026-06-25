$ErrorActionPreference = "Stop"

$deliveryPlanWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
$statusWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-status.ps1"
$redriveWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-redrive-plan.ps1"
$batchInvocationWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-invocation.ps1"
$resultWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-result-manifest.ps1"
foreach ($path in @($deliveryPlanWriter, $statusWriter, $redriveWriter, $batchInvocationWriter, $resultWriter)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback batch redrive result dependency: $path"
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
        [string]$StepID,
        [string]$LeakMarker
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
        debug_note = $LeakMarker
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
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
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

function Invoke-ResultWriter {
    param(
        [string]$InvocationPath,
        [string]$SummaryRoot,
        [string]$OutputPath
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $resultWriter `
        -BatchInvocationPath $InvocationPath `
        -ExecutionSummaryRootPath $SummaryRoot `
        -GeneratedBy "operator-a" `
        -OutputPath $OutputPath `
        -ResultManifestID "workflow-external-callback-batch-redrive-result-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function New-ExecutionSummary {
    param(
        [object]$Redrive,
        [string]$TenantID = "tenant-workflow",
        [string]$DeliveryID = ""
    )
    if ([string]::IsNullOrWhiteSpace($DeliveryID)) {
        $DeliveryID = "delivery:$($Redrive.redrive_plan_id)"
    }
    return [ordered]@{
        schema_version = "nexusim.workflow.external_callback_redrive_execution_summary.v1"
        mode = "external-callback-delivery-redrive"
        generated_at = "2026-06-25T00:00:00Z"
        redrive_plan_id = $Redrive.redrive_plan_id
        redrive_plan_sha256 = $Redrive.redrive_plan_sha256
        source_delivery_status_sha256 = $Redrive.source_delivery_status_sha256
        source_delivery_plan_sha256 = $Redrive.source_delivery_plan_sha256
        tenant_id = $TenantID
        workflow_id = $Redrive.workflow_id
        step_id = $Redrive.step_id
        delivery_id = $DeliveryID
        target_service = $Redrive.target_service
        target_operation = $Redrive.target_operation
        target_ref_hash = $Redrive.target_ref_hash
        payload_schema_version = $Redrive.payload_schema_version
        payload_ref_hash = $Redrive.payload_ref_hash
        approval_policy_ref = $Redrive.approval_policy_ref
        decision_policy_ref = $Redrive.decision_policy_ref
        delivery_status = "PENDING"
        redrive_count = 1
        last_redrive_plan_sha256 = $Redrive.redrive_plan_sha256
        last_redrive_reason_ref = $Redrive.redrive_reason_ref
        outbox_event_type = "workflow.external_callback.redriven.v1"
        executed_redrive = $true
        records_decision = $false
        calls_provider = $false
        executes_target = $false
        mutates_delivery_fact = $true
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-callback-batch-redrive-result-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-callback-batch-redrive-result-secret"
    $manifestRoot = Join-Path $tempRoot "manifests"
    $planRoot = Join-Path $tempRoot "plans"
    $statusRoot = Join-Path $tempRoot "statuses"
    $redriveRoot = Join-Path $tempRoot "redrives"
    $summaryRoot = Join-Path $tempRoot "summaries"
    New-Item -ItemType Directory -Force -Path $manifestRoot, $planRoot, $statusRoot, $redriveRoot, $summaryRoot | Out-Null

    foreach ($case in @(
            @{ workflow = "wf_callback_result_retry"; step = "wfs_retry"; status = "RETRY_PENDING"; attempt = 1; failure = "failure:provider-unavailable"; next = "retry:next:wf_callback_result_retry" },
            @{ workflow = "wf_callback_result_dlq"; step = "wfs_dlq"; status = "DLQ"; attempt = 3; failure = "failure:retry-exhausted"; next = "" }
        )) {
        $manifestPath = Join-Path $manifestRoot "$($case.workflow).json"
        $planPath = Join-Path $planRoot "$($case.workflow)-plan.json"
        $statusPath = Join-Path $statusRoot "$($case.workflow)-status.json"
        $redrivePath = Join-Path $redriveRoot "$($case.workflow)-redrive.json"
        Write-JsonFile -Path $manifestPath -Value (New-DecisionTemplate -WorkflowID $case.workflow -StepID $case.step -LeakMarker $leakMarker)
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
    $invocation = Get-Content -LiteralPath $invocationPath -Raw | ConvertFrom-Json

    foreach ($redrive in @($invocation.redrives)) {
        Write-JsonFile -Path (Join-Path $summaryRoot "$($redrive.redrive_plan_id -replace ':', '_')-summary.json") -Value (New-ExecutionSummary -Redrive $redrive)
    }

    $resultPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-result.json"
    Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $summaryRoot -OutputPath $resultPath
    $resultRaw = Get-Content -LiteralPath $resultPath -Raw
    $result = $resultRaw | ConvertFrom-Json

    if ($result.schema_version -ne "nexusim.workflow.external_callback_batch_redrive_result.v1" -or
        $result.result_manifest_id -ne "workflow-external-callback-batch-redrive-result-1" -or
        $result.batch_invocation_id -ne "workflow-external-callback-batch-redrive-invocation-1" -or
        [int]$result.expected_redrive_count -ne 2 -or
        [int]$result.execution_summary_count -ne 2 -or
        [int]$result.result_count -ne 2 -or
        [bool]$result.manifest_is_execution -or
        [bool]$result.records_decision -or
        [bool]$result.calls_provider -or
        [bool]$result.executes_target -or
        [bool]$result.mutates_delivery_fact) {
        throw "workflow external callback batch redrive result manifest has unexpected fields."
    }
    foreach ($expected in @(
            "source_batch_invocation_manifest_verified",
            "one_execution_summary_per_redrive_plan",
            "execution_summary_matches_invocation_binding",
            "workflow_service_runtime_reported_executed_redrive",
            "delivery_fact_returned_to_pending",
            "redriven_outbox_event_declared",
            "result_manifest_contains_only_low_sensitive_refs"
        )) {
        if (@($result.required_checks) -notcontains $expected) {
            throw "workflow external callback batch redrive result missing required check: $expected"
        }
    }
    foreach ($forbidden in @(
            $manifestRoot,
            $planRoot,
            $statusRoot,
            $redriveRoot,
            $summaryRoot,
            $tempRoot,
            $leakMarker,
            "https://",
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($resultRaw.Contains($forbidden)) {
            throw "workflow external callback batch redrive result leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-batch-redrive-result.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $summaryRoot -OutputPath $repoLocalOutput
    }

    $missingRoot = Join-Path $tempRoot "missing-summary"
    New-Item -ItemType Directory -Force -Path $missingRoot | Out-Null
    $first = @($invocation.redrives)[0]
    Write-JsonFile -Path (Join-Path $missingRoot "only-one.json") -Value (New-ExecutionSummary -Redrive $first)
    Invoke-ExpectFailure -Expected "Missing redrive execution summary" -Script {
        Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $missingRoot -OutputPath (Join-Path $tempRoot "missing.json")
    }

    $duplicateRoot = Join-Path $tempRoot "duplicate-summary"
    New-Item -ItemType Directory -Force -Path $duplicateRoot | Out-Null
    foreach ($name in @("a.json", "b.json")) {
        Write-JsonFile -Path (Join-Path $duplicateRoot $name) -Value (New-ExecutionSummary -Redrive $first)
    }
    Invoke-ExpectFailure -Expected "Duplicate redrive execution summary" -Script {
        Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $duplicateRoot -OutputPath (Join-Path $tempRoot "duplicate.json")
    }

    $badHashRoot = Join-Path $tempRoot "bad-hash"
    New-Item -ItemType Directory -Force -Path $badHashRoot | Out-Null
    foreach ($redrive in @($invocation.redrives)) {
        $summary = New-ExecutionSummary -Redrive $redrive
        if ($redrive.redrive_plan_id -eq $first.redrive_plan_id) {
            $summary.redrive_plan_sha256 = "sha256:other-redrive-plan"
        }
        Write-JsonFile -Path (Join-Path $badHashRoot "$($redrive.redrive_plan_id -replace ':', '_').json") -Value $summary
    }
    Invoke-ExpectFailure -Expected "summary.redrive_plan_sha256 mismatch" -Script {
        Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $badHashRoot -OutputPath (Join-Path $tempRoot "bad-hash.json")
    }

    $notExecutedRoot = Join-Path $tempRoot "not-executed"
    New-Item -ItemType Directory -Force -Path $notExecutedRoot | Out-Null
    foreach ($redrive in @($invocation.redrives)) {
        $summary = New-ExecutionSummary -Redrive $redrive
        if ($redrive.redrive_plan_id -eq $first.redrive_plan_id) {
            $summary.executed_redrive = $false
        }
        Write-JsonFile -Path (Join-Path $notExecutedRoot "$($redrive.redrive_plan_id -replace ':', '_').json") -Value $summary
    }
    Invoke-ExpectFailure -Expected "executed_redrive=true" -Script {
        Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $notExecutedRoot -OutputPath (Join-Path $tempRoot "not-executed.json")
    }

    $rawRoot = Join-Path $tempRoot "raw-summary"
    New-Item -ItemType Directory -Force -Path $rawRoot | Out-Null
    foreach ($redrive in @($invocation.redrives)) {
        $summary = New-ExecutionSummary -Redrive $redrive
        if ($redrive.redrive_plan_id -eq $first.redrive_plan_id) {
            $summary.provider_body = "provider raw body"
        }
        Write-JsonFile -Path (Join-Path $rawRoot "$($redrive.redrive_plan_id -replace ':', '_').json") -Value $summary
    }
    Invoke-ExpectFailure -Expected "provider artifact" -Script {
        Invoke-ResultWriter -InvocationPath $invocationPath -SummaryRoot $rawRoot -OutputPath (Join-Path $tempRoot "raw.json")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-batch-redrive-result.json") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback batch redrive result manifest self-test"
