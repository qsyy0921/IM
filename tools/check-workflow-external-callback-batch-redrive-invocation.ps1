$ErrorActionPreference = "Stop"

$deliveryPlanWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
$statusWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-status.ps1"
$redriveWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-redrive-plan.ps1"
$dashboardWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-dashboard.ps1"
$batchWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-invocation.ps1"
foreach ($path in @($deliveryPlanWriter, $statusWriter, $redriveWriter, $dashboardWriter, $batchWriter)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback batch redrive invocation dependency: $path"
    }
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
        [string]$PlanID,
        [string]$ReasonRef = "reason-sha256:redrive"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $redriveWriter `
        -DeliveryStatusPath $StatusPath `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -RedrivePlanID $PlanID `
        -RedriveQueueRef "queue:workflow-callback-redrive" `
        -RedriveReasonRef $ReasonRef `
        -OperatorReviewRef "review:callback-redrive-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-DashboardWriter {
    param(
        [string]$StatusRoot,
        [string]$RedriveRoot,
        [string]$OutputPath
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $dashboardWriter `
        -DeliveryStatusRootPath $StatusRoot `
        -RedrivePlanRootPath $RedriveRoot `
        -GeneratedBy "operator-a" `
        -DashboardID "workflow-external-callback-delivery-dashboard-1" `
        -OutputPath $OutputPath 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-BatchWriter {
    param(
        [string]$RedriveRoot,
        [string]$OutputPath,
        [string]$DashboardPath = ""
    )
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $batchWriter,
        "-RedrivePlanRootPath", $RedriveRoot,
        "-OutputPath", $OutputPath,
        "-PreparedBy", "operator-a",
        "-InvocationID", "workflow-external-callback-batch-redrive-invocation-1"
    )
    if (-not [string]::IsNullOrWhiteSpace($DashboardPath)) {
        $args += @("-DashboardPath", $DashboardPath)
    }
    $output = & powershell @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-callback-batch-redrive-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-callback-batch-redrive-secret"
    $manifestRoot = Join-Path $tempRoot "manifests"
    $planRoot = Join-Path $tempRoot "plans"
    $statusRoot = Join-Path $tempRoot "statuses"
    $redriveRoot = Join-Path $tempRoot "redrives"
    New-Item -ItemType Directory -Force -Path $manifestRoot, $planRoot, $statusRoot, $redriveRoot | Out-Null

    foreach ($case in @(
            @{ workflow = "wf_callback_batch_retry"; step = "wfs_retry"; status = "RETRY_PENDING"; attempt = 1; failure = "failure:provider-unavailable"; next = "retry:next:wf_callback_batch_retry" },
            @{ workflow = "wf_callback_batch_dlq"; step = "wfs_dlq"; status = "DLQ"; attempt = 3; failure = "failure:retry-exhausted"; next = "" }
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

    $dashboardPath = Join-Path $tempRoot "workflow-external-callback-delivery-dashboard.html"
    Invoke-DashboardWriter -StatusRoot $statusRoot -RedriveRoot $redriveRoot -OutputPath $dashboardPath

    $batchPath = Join-Path $tempRoot "workflow-external-callback-batch-redrive-invocation.json"
    Invoke-BatchWriter -RedriveRoot $redriveRoot -DashboardPath $dashboardPath -OutputPath $batchPath
    $batch = Get-Content -LiteralPath $batchPath -Raw | ConvertFrom-Json
    $batchJson = Get-Content -LiteralPath $batchPath -Raw

    if ($batch.schema_version -ne "nexusim.workflow.external_callback_batch_redrive_invocation.v1") {
        throw "Unexpected batch redrive invocation schema_version."
    }
    if ([int]$batch.redrive_count -ne 2) {
        throw "Expected two redrive entries."
    }
    if ($batch.runtime_contract.service -ne "workflow-service" -or $batch.runtime_contract.mode -ne "external-callback-delivery-redrive") {
        throw "Unexpected runtime contract."
    }
    if ([bool]$batch.runtime_contract.batch_invocation_calls_service -or
        [bool]$batch.runtime_contract.batch_invocation_records_decision -or
        [bool]$batch.runtime_contract.batch_invocation_calls_provider -or
        [bool]$batch.runtime_contract.batch_invocation_executes_target) {
        throw "Batch redrive invocation must not execute anything."
    }
    if (-not [bool]$batch.runtime_contract.requires_one_runtime_call_per_redrive_plan) {
        throw "Batch redrive invocation must require one runtime call per redrive plan."
    }

    foreach ($expected in @(
            "wf_callback_batch_retry",
            "wf_callback_batch_dlq",
            "redrive:wf_callback_batch_retry",
            "redrive:wf_callback_batch_dlq",
            "workflow-service.RecordWorkflowDecision",
            "source_dashboard_sha256",
            "redrive_plan_path_sha256",
            "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE"
        )) {
        if (-not $batchJson.Contains($expected)) {
            throw "workflow external callback batch redrive invocation missing expected content: $expected"
        }
    }

    foreach ($forbidden in @(
            $manifestRoot,
            $planRoot,
            $statusRoot,
            $redriveRoot,
            $tempRoot,
            $leakMarker,
            "https://",
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($batchJson.Contains($forbidden)) {
            throw "workflow external callback batch redrive invocation leaked sensitive or local artifact content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-batch-redrive-invocation.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-BatchWriter -RedriveRoot $redriveRoot -DashboardPath $dashboardPath -OutputPath $repoLocalOutput
    }

    $emptyRoot = Join-Path $tempRoot "empty"
    New-Item -ItemType Directory -Force -Path $emptyRoot | Out-Null
    Invoke-ExpectFailure -Expected "at least one redrive plan JSON" -Script {
        Invoke-BatchWriter -RedriveRoot $emptyRoot -OutputPath (Join-Path $tempRoot "empty.json")
    }

    $duplicateRoot = Join-Path $tempRoot "duplicate"
    New-Item -ItemType Directory -Force -Path $duplicateRoot | Out-Null
    Copy-Item -LiteralPath (Join-Path $redriveRoot "wf_callback_batch_retry-redrive.json") -Destination (Join-Path $duplicateRoot "a.json")
    Copy-Item -LiteralPath (Join-Path $redriveRoot "wf_callback_batch_retry-redrive.json") -Destination (Join-Path $duplicateRoot "b.json")
    Invoke-ExpectFailure -Expected "Duplicate redrive_plan_id" -Script {
        Invoke-BatchWriter -RedriveRoot $duplicateRoot -OutputPath (Join-Path $tempRoot "duplicate.json")
    }

    $badRawRoot = Join-Path $tempRoot "bad-raw"
    New-Item -ItemType Directory -Force -Path $badRawRoot | Out-Null
    $badRaw = Get-Content -LiteralPath (Join-Path $redriveRoot "wf_callback_batch_retry-redrive.json") -Raw | ConvertFrom-Json
    $badRaw.redrive_contract.redrive_reason_ref = "https://provider.example/callback"
    $badRaw | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath (Join-Path $badRawRoot "bad.json") -Encoding UTF8
    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-BatchWriter -RedriveRoot $badRawRoot -OutputPath (Join-Path $tempRoot "bad-raw.json")
    }

    $badStatusRoot = Join-Path $tempRoot "bad-status"
    New-Item -ItemType Directory -Force -Path $badStatusRoot | Out-Null
    $badStatus = Get-Content -LiteralPath (Join-Path $redriveRoot "wf_callback_batch_dlq-redrive.json") -Raw | ConvertFrom-Json
    $badStatus.redrive_source.delivery_status = "DELIVERED"
    $badStatus | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath (Join-Path $badStatusRoot "bad.json") -Encoding UTF8
    Invoke-ExpectFailure -Expected "RETRY_PENDING or DLQ" -Script {
        Invoke-BatchWriter -RedriveRoot $badStatusRoot -OutputPath (Join-Path $tempRoot "bad-status.json")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-batch-redrive-invocation.json") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback batch redrive invocation self-test"
