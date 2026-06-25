$ErrorActionPreference = "Stop"

$deliveryPlanWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
$statusWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-status.ps1"
$redriveWriter = Join-Path $PSScriptRoot "write-workflow-external-callback-redrive-plan.ps1"
foreach ($path in @($deliveryPlanWriter, $statusWriter, $redriveWriter)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow external callback tool dependency: $path"
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
    return [ordered]@{
        schema_version = "nexusim.workflow.external_decision_manifest.v1"
        workflow_id = "wf_callback_status_1"
        step_id = "wfs_callback_status_1"
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
        idempotency_key = "external-callback:wf_callback_status_1:wfs_callback_status_1"
        correlation_id = "corr:wf_callback_status_1"
        causation_id = "workflow:wf_callback_status_1"
        trace_id = "trace:wf_callback_status_1"
        debug_note = "do-not-leak-workflow-callback-delivery-status"
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
        -DeliveryPlanID "workflow-external-callback-delivery-plan-1" `
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
    $output | Out-Host
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
        "-StatusID", "workflow-external-callback-delivery-status-1",
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
    $output | Out-Host
}

function Invoke-RedriveWriter {
    param(
        [string]$StatusPath,
        [string]$OutputPath,
        [string]$RedriveQueueRef = "queue:workflow-callback-redrive",
        [string]$RedriveReasonRef = "reason-sha256:redrive"
    )
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $redriveWriter `
        -DeliveryStatusPath $StatusPath `
        -OutputPath $OutputPath `
        -PreparedBy "operator-a" `
        -RedrivePlanID "workflow-external-callback-redrive-plan-1" `
        -RedriveQueueRef $RedriveQueueRef `
        -RedriveReasonRef $RedriveReasonRef `
        -OperatorReviewRef "review:callback-redrive-1" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-external-callback-delivery-status-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $manifestPath = Join-Path $tempRoot "workflow-external-decision-template.json"
    $planPath = Join-Path $tempRoot "workflow-external-callback-delivery-plan.json"
    $deliveredStatusPath = Join-Path $tempRoot "workflow-external-callback-delivered-status.json"
    $retryStatusPath = Join-Path $tempRoot "workflow-external-callback-retry-status.json"
    $dlqStatusPath = Join-Path $tempRoot "workflow-external-callback-dlq-status.json"
    $redrivePath = Join-Path $tempRoot "workflow-external-callback-redrive-plan.json"
    Write-JsonFile -Path $manifestPath -Value (New-DecisionTemplate)
    Invoke-DeliveryPlanWriter -ManifestPath $manifestPath -OutputPath $planPath

    Invoke-StatusWriter `
        -PlanPath $planPath `
        -OutputPath $deliveredStatusPath `
        -DeliveryStatus "DELIVERED" `
        -AttemptNumber 1 `
        -DeliveryAttemptRef "attempt:callback-1" `
        -DeliveryResultRef "delivery-result:accepted"
    $deliveredRaw = Get-Content -LiteralPath $deliveredStatusPath -Raw
    $delivered = $deliveredRaw | ConvertFrom-Json
    if ($delivered.schema_version -ne "nexusim.workflow.external_callback_delivery_status.v1" -or
        $delivered.delivery_status -ne "DELIVERED" -or
        $delivered.workflow_binding.workflow_id -ne "wf_callback_status_1" -or
        $delivered.status_contract.delivered_status_is_not_decision -ne $true -or
        $delivered.no_decision_recorded -ne $true -or
        $delivered.does_not_call_provider -ne $true) {
        throw "workflow external callback delivered status has unexpected fields."
    }

    Invoke-StatusWriter `
        -PlanPath $planPath `
        -OutputPath $retryStatusPath `
        -DeliveryStatus "RETRY_PENDING" `
        -AttemptNumber 2 `
        -DeliveryAttemptRef "attempt:callback-2" `
        -FailureClassRef "failure:provider-timeout" `
        -NextRetryRef "retry-at:slot-3"
    $retry = Get-Content -LiteralPath $retryStatusPath -Raw | ConvertFrom-Json
    if ($retry.delivery_status -ne "RETRY_PENDING" -or
        $retry.failure_class_ref -ne "failure:provider-timeout" -or
        $retry.next_retry_ref -ne "retry-at:slot-3") {
        throw "workflow external callback retry status has unexpected fields."
    }

    Invoke-StatusWriter `
        -PlanPath $planPath `
        -OutputPath $dlqStatusPath `
        -DeliveryStatus "DLQ" `
        -AttemptNumber 3 `
        -DeliveryAttemptRef "attempt:callback-3" `
        -FailureClassRef "failure:retry-exhausted"
    $dlq = Get-Content -LiteralPath $dlqStatusPath -Raw | ConvertFrom-Json
    if ($dlq.delivery_status -ne "DLQ" -or
        $dlq.attempt_number -ne 3 -or
        $dlq.max_attempts -ne 3) {
        throw "workflow external callback DLQ status has unexpected fields."
    }

    Invoke-RedriveWriter -StatusPath $dlqStatusPath -OutputPath $redrivePath
    $redriveRaw = Get-Content -LiteralPath $redrivePath -Raw
    $redrive = $redriveRaw | ConvertFrom-Json
    if ($redrive.schema_version -ne "nexusim.workflow.external_callback_redrive_plan.v1" -or
        $redrive.redrive_source.delivery_status -ne "DLQ" -or
        $redrive.workflow_binding.workflow_id -ne "wf_callback_status_1" -or
        $redrive.redrive_contract.redrive_queue_ref -ne "queue:workflow-callback-redrive" -or
        $redrive.redrive_contract.redrive_plan_calls_provider -ne $false -or
        $redrive.no_decision_recorded -ne $true -or
        $redrive.does_not_execute_target -ne $true) {
        throw "workflow external callback redrive plan has unexpected fields."
    }

    foreach ($text in @($deliveredRaw, $redriveRaw)) {
        foreach ($forbidden in @(
                $tempRoot,
                $manifestPath,
                $planPath,
                "do-not-leak-workflow-callback-delivery-status",
                "https://",
                "provider_body",
                "raw:",
                "password"
            )) {
            if ($text -like "*$forbidden*") {
                throw "workflow external callback status/redrive leaked forbidden content: $forbidden"
            }
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-delivery-status.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-StatusWriter -PlanPath $planPath -OutputPath $repoLocalOutput -DeliveryStatus "DELIVERED" -AttemptNumber 1 -DeliveryAttemptRef "attempt:callback-1" -DeliveryResultRef "delivery-result:accepted"
    }

    Invoke-ExpectFailure -Expected "low-sensitive operator id" -Script {
        $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $statusWriter `
            -DeliveryPlanPath $planPath `
            -OutputPath $deliveredStatusPath `
            -ReportedBy "operator-token-secret" `
            -DeliveryStatus "DELIVERED" `
            -AttemptNumber 1 `
            -DeliveryAttemptRef "attempt:callback-1" `
            -DeliveryResultRef "delivery-result:accepted" 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw (($output | Out-String).Trim())
        }
    }

    Invoke-ExpectFailure -Expected "FailureClassRef" -Script {
        Invoke-StatusWriter -PlanPath $planPath -OutputPath $retryStatusPath -DeliveryStatus "RETRY_PENDING" -AttemptNumber 1 -DeliveryAttemptRef "attempt:callback-x"
    }

    Invoke-ExpectFailure -Expected "below max_attempts" -Script {
        Invoke-StatusWriter -PlanPath $planPath -OutputPath $retryStatusPath -DeliveryStatus "RETRY_PENDING" -AttemptNumber 3 -DeliveryAttemptRef "attempt:callback-x" -FailureClassRef "failure:timeout" -NextRetryRef "retry-at:slot-4"
    }

    Invoke-ExpectFailure -Expected "reach max_attempts" -Script {
        Invoke-StatusWriter -PlanPath $planPath -OutputPath $dlqStatusPath -DeliveryStatus "DLQ" -AttemptNumber 2 -DeliveryAttemptRef "attempt:callback-x" -FailureClassRef "failure:timeout"
    }

    Invoke-ExpectFailure -Expected "RETRY_PENDING or DLQ" -Script {
        Invoke-RedriveWriter -StatusPath $deliveredStatusPath -OutputPath $redrivePath
    }

    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-RedriveWriter -StatusPath $dlqStatusPath -OutputPath $redrivePath -RedriveQueueRef "https://provider.example/redrive"
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    foreach ($path in @(
            (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-delivery-status.json"),
            (Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-redrive-plan.json")
        )) {
        Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
    }
}

Write-Host "OK   workflow external callback delivery status and redrive self-test"
