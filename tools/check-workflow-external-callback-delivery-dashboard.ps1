$ErrorActionPreference = "Stop"

$deliveryPlanWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
$statusWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-status.ps1"
$redriveWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-redrive-plan.ps1"
$dashboardWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-dashboard.ps1"
foreach ($path in @($deliveryPlanWriter, $statusWriter, $redriveWriter, $dashboardWriter)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback delivery dashboard dependency: $path"
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
        [string]$DeliveryResultRef = "",
        [string]$FailureClassRef = "",
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
        "-RedrivePolicyRef", "workflow.external-callback-redrive.v1"
    )
    if (-not [string]::IsNullOrWhiteSpace($DeliveryResultRef)) {
        $args += @("-DeliveryResultRef", $DeliveryResultRef)
    }
    if (-not [string]::IsNullOrWhiteSpace($FailureClassRef)) {
        $args += @("-FailureClassRef", $FailureClassRef)
    }
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

function Invoke-DashboardWriter {
    param(
        [string]$StatusRoot,
        [string]$OutputPath,
        [string]$RedriveRoot = ""
    )
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $dashboardWriter,
        "-DeliveryStatusRootPath", $StatusRoot,
        "-GeneratedBy", "operator-a",
        "-DashboardID", "workflow-external-callback-delivery-dashboard-1",
        "-OutputPath", $OutputPath
    )
    if (-not [string]::IsNullOrWhiteSpace($RedriveRoot)) {
        $args += @("-RedrivePlanRootPath", $RedriveRoot)
    }
    $output = & powershell @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-callback-dashboard-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-callback-dashboard-secret"
    $manifestRoot = Join-Path $tempRoot "manifests"
    $planRoot = Join-Path $tempRoot "plans"
    $statusRoot = Join-Path $tempRoot "statuses"
    $redriveRoot = Join-Path $tempRoot "redrives"
    New-Item -ItemType Directory -Force -Path $manifestRoot, $planRoot, $statusRoot, $redriveRoot | Out-Null

    foreach ($case in @(
            @{ workflow = "wf_callback_dash_delivered"; step = "wfs_delivered"; status = "DELIVERED"; attempt = 1 },
            @{ workflow = "wf_callback_dash_retry"; step = "wfs_retry"; status = "RETRY_PENDING"; attempt = 1 },
            @{ workflow = "wf_callback_dash_dlq"; step = "wfs_dlq"; status = "DLQ"; attempt = 3 }
        )) {
        $manifestPath = Join-Path $manifestRoot "$($case.workflow).json"
        $planPath = Join-Path $planRoot "$($case.workflow)-plan.json"
        $statusPath = Join-Path $statusRoot "$($case.workflow)-status.json"
        Write-JsonFile -Path $manifestPath -Value (New-DecisionTemplate -WorkflowID $case.workflow -StepID $case.step -LeakMarker $leakMarker)
        Invoke-DeliveryPlanWriter -ManifestPath $manifestPath -OutputPath $planPath -PlanID "plan:$($case.workflow)"

        if ($case.status -eq "DELIVERED") {
            Invoke-StatusWriter `
                -PlanPath $planPath `
                -OutputPath $statusPath `
                -StatusID "status:$($case.workflow)" `
                -DeliveryStatus $case.status `
                -AttemptNumber $case.attempt `
                -DeliveryAttemptRef "attempt:$($case.workflow)" `
                -DeliveryResultRef "delivery-result:accepted"
        } elseif ($case.status -eq "RETRY_PENDING") {
            Invoke-StatusWriter `
                -PlanPath $planPath `
                -OutputPath $statusPath `
                -StatusID "status:$($case.workflow)" `
                -DeliveryStatus $case.status `
                -AttemptNumber $case.attempt `
                -DeliveryAttemptRef "attempt:$($case.workflow)" `
                -FailureClassRef "failure:provider-unavailable" `
                -NextRetryRef "retry:next:$($case.workflow)"
            Invoke-RedriveWriter -StatusPath $statusPath -OutputPath (Join-Path $redriveRoot "$($case.workflow)-redrive.json") -PlanID "redrive:$($case.workflow)"
        } else {
            Invoke-StatusWriter `
                -PlanPath $planPath `
                -OutputPath $statusPath `
                -StatusID "status:$($case.workflow)" `
                -DeliveryStatus $case.status `
                -AttemptNumber $case.attempt `
                -DeliveryAttemptRef "attempt:$($case.workflow)" `
                -FailureClassRef "failure:retry-exhausted"
            Invoke-RedriveWriter -StatusPath $statusPath -OutputPath (Join-Path $redriveRoot "$($case.workflow)-redrive.json") -PlanID "redrive:$($case.workflow)"
        }
    }

    $dashboardPath = Join-Path $tempRoot "workflow-external-callback-delivery-dashboard.html"
    Invoke-DashboardWriter -StatusRoot $statusRoot -RedriveRoot $redriveRoot -OutputPath $dashboardPath
    $html = Get-Content -LiteralPath $dashboardPath -Raw

    foreach ($expected in @(
            "NexusIM Workflow External Callback Delivery Dashboard",
            "delivery_status_count",
            "delivered_count",
            "retry_pending_count",
            "dlq_count",
            "redrive_candidate_count",
            "redrive_plan_count",
            "wf_callback_dash_delivered",
            "wf_callback_dash_retry",
            "wf_callback_dash_dlq",
            "redrive:wf_callback_dash_retry",
            "redrive:wf_callback_dash_dlq",
            "workflow-service.RecordWorkflowDecision",
            "dashboard_calls_provider",
            "False"
        )) {
        if (-not $html.Contains($expected)) {
            throw "workflow external callback delivery dashboard missing expected low-sensitive content: $expected"
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
        if ($html.Contains($forbidden)) {
            throw "workflow external callback delivery dashboard leaked sensitive or local artifact content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-dashboard.html"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-DashboardWriter -StatusRoot $statusRoot -RedriveRoot $redriveRoot -OutputPath $repoLocalOutput
    }

    $emptyRoot = Join-Path $tempRoot "empty"
    New-Item -ItemType Directory -Force -Path $emptyRoot | Out-Null
    Invoke-ExpectFailure -Expected "at least one delivery status JSON" -Script {
        Invoke-DashboardWriter -StatusRoot $emptyRoot -OutputPath (Join-Path $tempRoot "empty.html")
    }

    $badStatusPath = Join-Path $statusRoot "bad-raw-status.json"
    $badStatus = Get-Content -LiteralPath (Join-Path $statusRoot "wf_callback_dash_delivered-status.json") -Raw | ConvertFrom-Json
    $badStatus.delivery_result_ref = "https://provider.example/callback"
    $badStatus | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badStatusPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-DashboardWriter -StatusRoot $statusRoot -RedriveRoot $redriveRoot -OutputPath (Join-Path $tempRoot "bad-raw.html")
    }
    Remove-Item -LiteralPath $badStatusPath -Force

    $orphanStatusRoot = Join-Path $tempRoot "orphan-statuses"
    $orphanRoot = Join-Path $tempRoot "orphan-redrives"
    New-Item -ItemType Directory -Force -Path $orphanStatusRoot, $orphanRoot | Out-Null
    Copy-Item -LiteralPath (Join-Path $statusRoot "wf_callback_dash_delivered-status.json") -Destination (Join-Path $orphanStatusRoot "delivered-status.json")
    Copy-Item -LiteralPath (Join-Path $redriveRoot "wf_callback_dash_dlq-redrive.json") -Destination (Join-Path $orphanRoot "orphan-redrive.json")
    Invoke-ExpectFailure -Expected "without matching delivery status" -Script {
        Invoke-DashboardWriter -StatusRoot $orphanStatusRoot -RedriveRoot $orphanRoot -OutputPath (Join-Path $tempRoot "orphan.html")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-callback-dashboard.html") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback delivery dashboard self-test"
