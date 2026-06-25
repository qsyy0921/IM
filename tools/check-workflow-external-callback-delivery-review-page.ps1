$ErrorActionPreference = "Stop"

$deliveryPlanWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
$statusWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-status.ps1"
$redriveWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-redrive-plan.ps1"
$pageWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-review-page.ps1"
foreach ($path in @($deliveryPlanWriter, $statusWriter, $redriveWriter, $pageWriter)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback delivery review page dependency: $path"
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
    param([string]$LeakMarker)

    return [ordered]@{
        schema_version = "nexusim.workflow.external_decision_manifest.v1"
        workflow_id = "wf_callback_review_1"
        step_id = "wfs_callback_review_1"
        expected_workflow_type = "ACTION_APPROVAL"
        expected_status = "WAITING_DECISION"
        expected_target_service = "external-crm"
        expected_target_operation = "SYNC_APPROVAL_CALLBACK"
        expected_target_ref_hash = "sha256:target"
        expected_payload_schema_version = "external.callback_request.v1"
        expected_payload_ref_hash = "sha256:payload"
        expected_approval_policy_ref = "workflow.external_callback.v1"
        decision = ""
        decider_ref = ""
        decision_policy_ref = "workflow.external_callback.decision.v1"
        reason_ref = "reason-sha256:abc"
        evidence_refs = @("evidence:ticket-1")
        idempotency_key = "external-callback:wf_callback_review_1:wfs_callback_review_1"
        correlation_id = "corr:wf_callback_review_1"
        causation_id = "workflow:wf_callback_review_1"
        trace_id = "trace:wf_callback_review_1"
        debug_note = $LeakMarker
    }
}

function Invoke-DeliveryPlanWriter {
    param(
        [string]$ManifestPath,
        [string]$OutputPath
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $deliveryPlanWriter `
        -DecisionManifestPath $ManifestPath `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -DeliveryPlanID "workflow-external-callback-delivery-plan-review-1" `
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
        "-StatusID", "workflow-external-callback-delivery-status-review-1",
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
        [string]$OutputPath
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $redriveWriter `
        -DeliveryStatusPath $StatusPath `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -RedrivePlanID "workflow-external-callback-redrive-plan-review-1" `
        -RedriveQueueRef "queue:workflow-callback-redrive" `
        -RedriveReasonRef "reason-sha256:redrive" `
        -OperatorReviewRef "review:callback-redrive-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

function Invoke-PageWriter {
    param(
        [string]$PlanPath,
        [string]$StatusPath,
        [string]$OutputPath,
        [string]$RedrivePath = ""
    )
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $pageWriter,
        "-DeliveryPlanPath", $PlanPath,
        "-DeliveryStatusPath", $StatusPath,
        "-GeneratedBy", "operator-a",
        "-PageID", "workflow-external-callback-delivery-review-page-1",
        "-OutputPath", $OutputPath
    )
    if (-not [string]::IsNullOrWhiteSpace($RedrivePath)) {
        $args += @("-RedrivePlanPath", $RedrivePath)
    }
    $output = & powershell @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-external-callback-delivery-review-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-callback-delivery-review-secret"
    $manifestPath = Join-Path $tempRoot "workflow-external-decision-template.json"
    $planPath = Join-Path $tempRoot "workflow-external-callback-delivery-plan.json"
    $deliveredStatusPath = Join-Path $tempRoot "workflow-external-callback-delivered-status.json"
    $dlqStatusPath = Join-Path $tempRoot "workflow-external-callback-dlq-status.json"
    $redrivePath = Join-Path $tempRoot "workflow-external-callback-redrive-plan.json"
    $deliveredPagePath = Join-Path $tempRoot "workflow-external-callback-delivered-review.html"
    $dlqPagePath = Join-Path $tempRoot "workflow-external-callback-dlq-review.html"

    Write-JsonFile -Path $manifestPath -Value (New-DecisionTemplate -LeakMarker $leakMarker)
    Invoke-DeliveryPlanWriter -ManifestPath $manifestPath -OutputPath $planPath

    Invoke-StatusWriter `
        -PlanPath $planPath `
        -OutputPath $deliveredStatusPath `
        -DeliveryStatus "DELIVERED" `
        -AttemptNumber 1 `
        -DeliveryAttemptRef "attempt:callback-1" `
        -DeliveryResultRef "delivery-result:accepted"
    Invoke-PageWriter -PlanPath $planPath -StatusPath $deliveredStatusPath -OutputPath $deliveredPagePath

    Invoke-StatusWriter `
        -PlanPath $planPath `
        -OutputPath $dlqStatusPath `
        -DeliveryStatus "DLQ" `
        -AttemptNumber 3 `
        -DeliveryAttemptRef "attempt:callback-3" `
        -FailureClassRef "failure:retry-exhausted"
    Invoke-RedriveWriter -StatusPath $dlqStatusPath -OutputPath $redrivePath
    Invoke-PageWriter -PlanPath $planPath -StatusPath $dlqStatusPath -RedrivePath $redrivePath -OutputPath $dlqPagePath

    $deliveredHtml = Get-Content -LiteralPath $deliveredPagePath -Raw
    $dlqHtml = Get-Content -LiteralPath $dlqPagePath -Raw
    foreach ($expected in @(
            "NexusIM Workflow External Callback Delivery Review",
            "wf_callback_review_1",
            "workflow-service.RecordWorkflowDecision",
            "delivery_plan_sha256",
            "delivery_status_sha256",
            "delivered_status_is_not_decision",
            "review_calls_provider",
            "False"
        )) {
        if (-not $deliveredHtml.Contains($expected)) {
            throw "workflow external callback delivered review page missing expected low-sensitive content: $expected"
        }
    }
    foreach ($expected in @(
            "redrive_plan_sha256",
            "workflow-external-callback-redrive-plan-review-1",
            "queue:workflow-callback-redrive",
            "requires_existing_waiting_workflow"
        )) {
        if (-not $dlqHtml.Contains($expected)) {
            throw "workflow external callback DLQ review page missing expected low-sensitive content: $expected"
        }
    }

    foreach ($text in @($deliveredHtml, $dlqHtml)) {
        foreach ($forbidden in @(
                $manifestPath,
                $planPath,
                $deliveredStatusPath,
                $dlqStatusPath,
                $redrivePath,
                $tempRoot,
                $leakMarker,
                "https://",
                "provider_body",
                "raw:",
                "password"
            )) {
            if ($text.Contains($forbidden)) {
                throw "workflow external callback delivery review page leaked sensitive or local artifact content: $forbidden"
            }
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-delivery-review.html"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-PageWriter -PlanPath $planPath -StatusPath $deliveredStatusPath -OutputPath $repoLocalOutput
    }

    Invoke-ExpectFailure -Expected "must not include a redrive plan" -Script {
        Invoke-PageWriter -PlanPath $planPath -StatusPath $deliveredStatusPath -RedrivePath $redrivePath -OutputPath (Join-Path $tempRoot "bad-delivered-redrive.html")
    }

    Invoke-ExpectFailure -Expected "requires a redrive plan" -Script {
        Invoke-PageWriter -PlanPath $planPath -StatusPath $dlqStatusPath -OutputPath (Join-Path $tempRoot "bad-dlq-no-redrive.html")
    }

    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    $badStatus = Get-Content -LiteralPath $dlqStatusPath -Raw | ConvertFrom-Json
    $badStatus.source_delivery_plan_sha256 = "sha256:wrong"
    $badStatus | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badStatusPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "source delivery plan hash" -Script {
        Invoke-PageWriter -PlanPath $planPath -StatusPath $badStatusPath -RedrivePath $redrivePath -OutputPath (Join-Path $tempRoot "bad-source-hash.html")
    }

    $badRawStatusPath = Join-Path $tempRoot "bad-raw-status.json"
    $badRawStatus = Get-Content -LiteralPath $deliveredStatusPath -Raw | ConvertFrom-Json
    $badRawStatus.delivery_result_ref = "https://provider.example/callback"
    $badRawStatus | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $badRawStatusPath -Encoding UTF8
    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-PageWriter -PlanPath $planPath -StatusPath $badRawStatusPath -OutputPath (Join-Path $tempRoot "bad-raw.html")
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-delivery-review.html") -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback delivery review page self-test"
