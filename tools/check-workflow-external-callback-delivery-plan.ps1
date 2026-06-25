$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-external-callback-delivery-plan.ps1"
if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
    throw "Missing workflow external callback delivery plan writer: $writerPath"
}

function Write-JsonFile {
    param(
        [string]$Path,
        [object]$Value
    )
    $Value | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Writer {
    param(
        [string]$ManifestPath,
        [string]$OutputPath,
        [string]$PreparedBy = "operator-a",
        [string]$CallbackEndpointRef = "endpoint:approval-provider-a"
    )

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -DecisionManifestPath $ManifestPath `
        -OutputPath $OutputPath `
        -PreparedBy $PreparedBy `
        -DeliveryPlanID "workflow-external-callback-delivery-plan-1" `
        -CallbackProviderRef "provider:approval-gateway-a" `
        -CallbackEndpointRef $CallbackEndpointRef `
        -DeliveryQueueRef "queue:workflow-callback-delivery" `
        -RetryPolicyRef "retry:workflow-callback-v1" `
        -BackoffPolicyRef "backoff:workflow-callback-exp-v1" `
        -CallbackTimeoutPolicyRef "timeout:workflow-callback-v1" `
        -MaxAttempts 4 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    $output | Out-Host
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
        [string]$LeakMarker = "do-not-leak-workflow-callback-provider-body"
    )

    return [ordered]@{
        schema_version = "nexusim.workflow.external_decision_manifest.v1"
        workflow_id = "wf_callback_1"
        step_id = "wfs_callback_1"
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
        idempotency_key = "external-callback:wf_callback_1:wfs_callback_1"
        correlation_id = "corr:wf_callback_1"
        causation_id = "workflow:wf_callback_1"
        trace_id = "trace:wf_callback_1"
        debug_note = $LeakMarker
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-external-callback-delivery-plan-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $leakMarker = "do-not-leak-workflow-callback-provider-body"
    $manifestPath = Join-Path $tempRoot "workflow-external-decision-template.json"
    $planPath = Join-Path $tempRoot "workflow-external-callback-delivery-plan.json"
    Write-JsonFile -Path $manifestPath -Value (New-DecisionTemplate -LeakMarker $leakMarker)

    Invoke-Writer -ManifestPath $manifestPath -OutputPath $planPath

    $raw = Get-Content -LiteralPath $planPath -Raw
    $plan = $raw | ConvertFrom-Json
    if ($plan.schema_version -ne "nexusim.workflow.external_callback_delivery_plan.v1" -or
        $plan.delivery_plan_id -ne "workflow-external-callback-delivery-plan-1" -or
        $plan.workflow_binding.workflow_id -ne "wf_callback_1" -or
        $plan.workflow_binding.step_id -ne "wfs_callback_1" -or
        $plan.workflow_binding.expected_status -ne "WAITING_DECISION" -or
        $plan.workflow_binding.expected_payload_ref_hash -ne "sha256:payload" -or
        $plan.callback_delivery_contract.owner -ne "workflow-service.external-callback-delivery" -or
        $plan.callback_delivery_contract.callback_endpoint_ref -ne "endpoint:approval-provider-a" -or
        $plan.callback_delivery_contract.raw_callback_url_allowed -ne $false -or
        $plan.callback_delivery_contract.delivery_plan_calls_provider -ne $false -or
        $plan.retry_contract.max_attempts -ne 4 -or
        $plan.final_decision_contract.final_decision_owner -ne "workflow-service.RecordWorkflowDecision" -or
        $plan.no_direct_execution -ne $true -or
        $plan.no_decision_recorded -ne $true -or
        $plan.does_not_call_provider -ne $true -or
        $plan.does_not_execute_target -ne $true) {
        throw "workflow external callback delivery plan has unexpected fields."
    }

    foreach ($expected in @(
            "external_decision_manifest_template_verified",
            "workflow_waiting_decision_verified",
            "decision_and_decider_are_empty",
            "endpoint_is_ref_not_raw_url",
            "retry_policy_is_explicit"
        )) {
        if (@($plan.preflight_checks) -notcontains $expected) {
            throw "workflow external callback delivery plan missing preflight check: $expected"
        }
    }
    foreach ($expected in @(
            "delivery_plan_does_not_record_decision",
            "external_system_must_return_explicit_decision_manifest",
            "record_decision_must_revalidate_workflow_binding"
        )) {
        if (@($plan.approval_boundary) -notcontains $expected) {
            throw "workflow external callback delivery plan missing approval boundary: $expected"
        }
    }
    foreach ($expected in @(
            "delivery_plan_is_not_execution",
            "does_not_call_target_service",
            "does_not_call_action_executor",
            "workflow_service_records_decision_only_after_binding_check"
        )) {
        if (@($plan.execution_boundary) -notcontains $expected) {
            throw "workflow external callback delivery plan missing execution boundary: $expected"
        }
    }

    foreach ($forbidden in @(
            $tempRoot,
            $manifestPath,
            $planPath,
            $leakMarker,
            "provider-body",
            "debug_note",
            "https://",
            "provider_body",
            "raw:",
            "password"
        )) {
        if ($raw -like "*$forbidden*") {
            throw "workflow external callback delivery plan leaked forbidden content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-delivery-plan.json"
    Invoke-ExpectFailure -Expected "must not be inside the repository" -Script {
        Invoke-Writer -ManifestPath $manifestPath -OutputPath $repoLocalOutput
    }

    Invoke-ExpectFailure -Expected "low-sensitive operator id" -Script {
        Invoke-Writer -ManifestPath $manifestPath -OutputPath $planPath -PreparedBy "operator-token-secret"
    }

    Invoke-ExpectFailure -Expected "low-sensitive repair identifier" -Script {
        Invoke-Writer -ManifestPath $manifestPath -OutputPath $planPath -CallbackEndpointRef "https://provider.example/callback"
    }

    $badDecided = New-DecisionTemplate
    $badDecided.decision = "APPROVE"
    $badDecided.decider_ref = "operator-a"
    $badDecidedPath = Join-Path $tempRoot "bad-decided.json"
    Write-JsonFile -Path $badDecidedPath -Value $badDecided
    Invoke-ExpectFailure -Expected "must not already contain a decision" -Script {
        Invoke-Writer -ManifestPath $badDecidedPath -OutputPath $planPath
    }

    $badStatus = New-DecisionTemplate
    $badStatus.expected_status = "TIMED_OUT"
    $badStatusPath = Join-Path $tempRoot "bad-status.json"
    Write-JsonFile -Path $badStatusPath -Value $badStatus
    Invoke-ExpectFailure -Expected "WAITING_DECISION" -Script {
        Invoke-Writer -ManifestPath $badStatusPath -OutputPath $planPath
    }

    Invoke-ExpectFailure -Expected "MaxAttempts must be" -Script {
        $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
            -DecisionManifestPath $manifestPath `
            -OutputPath $planPath `
            -PreparedBy "operator-a" `
            -CallbackProviderRef "provider:approval-gateway-a" `
            -CallbackEndpointRef "endpoint:approval-provider-a" `
            -DeliveryQueueRef "queue:workflow-callback-delivery" `
            -RetryPolicyRef "retry:workflow-callback-v1" `
            -BackoffPolicyRef "backoff:workflow-callback-exp-v1" `
            -CallbackTimeoutPolicyRef "timeout:workflow-callback-v1" `
            -MaxAttempts 0 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw (($output | Out-String).Trim())
        }
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-external-callback-delivery-plan.json"
    Remove-Item -LiteralPath $repoLocalOutput -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow external callback delivery plan self-test"
