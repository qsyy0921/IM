$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokePath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
$workflowInstructionWriterPath = Join-Path $PSScriptRoot "write-workflow-compensation-instruction-manifest.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath, $invokePath, $workflowInstructionWriterPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing approved repair invocation test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-approved-repair-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $planPath = Join-Path $tempRoot "plan.json"
    $requestPath = Join-Path $tempRoot "request.json"
    $decisionPath = Join-Path $tempRoot "decision.json"
    $summaryPath = Join-Path $tempRoot "summary.json"

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -ReasonFileEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE" `
        -ReasonFilePath "H:\NexusIM\operator-plans\projection-reason.txt"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing approved invocation test"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-invoke-1" `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing approved invocation test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-invoke-1" `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing approved invocation test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $invokePath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -OutputPath $summaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-approved-repair-operator.ps1 failed in preflight mode"
    }

    $summaryRaw = Get-Content -LiteralPath $summaryPath -Raw
    $summary = $summaryRaw | ConvertFrom-Json
    if ($summary.schema_version -ne 1 -or
        $summary.approval_id -ne "approval-invoke-1" -or
        $summary.decision_id -ne "decision-invoke-1" -or
        $summary.service -ne "delivery-service" -or
        $summary.mode -ne "projection-checkpoint-repair" -or
        $summary.execute_requested -ne $false -or
        $summary.executed -ne $false -or
        $summary.dry_run_requested -ne $true) {
        throw "approved invocation summary has unexpected fields."
    }
    if (@($summary.environment_keys) -notcontains "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE") {
        throw "approved invocation summary should include redacted environment keys."
    }
    if ($summaryRaw.Contains("projection-reason.txt")) {
        throw "approved invocation summary leaked raw env value."
    }

    $mutatingPlanPath = Join-Path $tempRoot "mutating-plan.json"
    $mutatingPlan = ($planJson | ConvertFrom-Json)
    $mutatingPlan.environment.PSObject.Properties.Remove("NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN")
    ($mutatingPlan | ConvertTo-Json -Depth 8) | Set-Content -LiteralPath $mutatingPlanPath -Encoding UTF8

    $mutatingRequestPath = Join-Path $tempRoot "mutating-request.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $mutatingPlanPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-invoke-mutating" `
        -OutputPath $mutatingRequestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing mutating invocation test"
    }
    $mutatingDecisionPath = Join-Path $tempRoot "mutating-decision.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $mutatingRequestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-invoke-mutating" `
        -OutputPath $mutatingDecisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing mutating invocation test"
    }
    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $invokePath `
            -PlanPath $mutatingPlanPath `
            -RequestPath $mutatingRequestPath `
            -DecisionPath $mutatingDecisionPath `
            -Execute 2>$null | Out-Null
        $mutatingExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($mutatingExitCode -eq 0) {
        throw "approved invocation should reject execution without dry-run env unless AllowMutating is set."
    }

    $workflowReasonPath = Join-Path $tempRoot "workflow-reason.txt"
    $workflowPayloadRefPath = Join-Path $tempRoot "workflow-payload-ref.txt"
    $workflowInstructionPath = Join-Path $tempRoot "workflow-compensation-instruction.json"
    "approved rollback reason" | Set-Content -LiteralPath $workflowReasonPath -Encoding UTF8
    "external rollback payload reference" | Set-Content -LiteralPath $workflowPayloadRefPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $workflowInstructionWriterPath `
        -OutputPath $workflowInstructionPath `
        -WorkflowID "wf_invoke_1" `
        -PayloadRefFile $workflowPayloadRefPath `
        -Environment "local" `
        -ConfigKind "API_GATEWAY_TENANT_QUOTA" `
        -BundleKey "tenant-a" `
        -TargetVersion "quota-v1" `
        -OperatorRef "operator:rollback" `
        -ReasonFile $workflowReasonPath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-compensation-instruction-manifest.ps1 failed while preparing approved invocation test"
    }

    $workflowPlanPath = Join-Path $tempRoot "workflow-plan.json"
    $workflowPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "workflow-service" `
        -Mode "compensation-instruction-import" `
        -Env "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID=tenant_1","NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE=$workflowInstructionPath"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing workflow approved invocation test"
    }
    $workflowPlanJson | Set-Content -LiteralPath $workflowPlanPath -Encoding UTF8

    $workflowRequestPath = Join-Path $tempRoot "workflow-request.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $workflowPlanPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-workflow-invoke-1" `
        -OutputPath $workflowRequestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing workflow approved invocation test"
    }

    $workflowDecisionPath = Join-Path $tempRoot "workflow-decision.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $workflowRequestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-workflow-invoke-1" `
        -OutputPath $workflowDecisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing workflow approved invocation test"
    }

    $workflowSummaryPath = Join-Path $tempRoot "workflow-summary.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $invokePath `
        -PlanPath $workflowPlanPath `
        -RequestPath $workflowRequestPath `
        -DecisionPath $workflowDecisionPath `
        -OutputPath $workflowSummaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-approved-repair-operator.ps1 failed workflow instruction manifest preflight"
    }
    $workflowSummaryRaw = Get-Content -LiteralPath $workflowSummaryPath -Raw
    $workflowSummary = $workflowSummaryRaw | ConvertFrom-Json
    if ($workflowSummary.service -ne "workflow-service" -or
        $workflowSummary.mode -ne "compensation-instruction-import" -or
        @($workflowSummary.preflight_checks).Count -ne 1 -or
        $workflowSummary.preflight_checks[0].name -ne "workflow_compensation_instruction_manifest" -or
        $workflowSummary.preflight_checks[0].instruction_count -ne 1) {
        throw "workflow approved invocation summary should include a validated instruction manifest preflight check."
    }
    if ($workflowSummaryRaw.Contains($workflowInstructionPath) -or
        $workflowSummaryRaw.Contains("approved rollback reason") -or
        $workflowSummaryRaw.Contains("external rollback payload reference")) {
        throw "workflow approved invocation summary leaked manifest path, reason, or payload ref body."
    }

    $badInstructionPath = Join-Path $tempRoot "workflow-compensation-instruction-bad.json"
    @'
{
  "instructions": [
    {
      "workflow_id": "wf_invoke_1",
      "payload_ref_hash": "sha256:payload",
      "environment": "local",
      "config_kind": "API_GATEWAY_TENANT_QUOTA",
      "bundle_key": "tenant-a",
      "target_version": "quota-v1",
      "operator_ref": "operator:rollback",
      "reason_ref": "reason-sha256:rollback",
      "raw_payload": "must not be embedded"
    }
  ]
}
'@ | Set-Content -LiteralPath $badInstructionPath -Encoding UTF8
    $badWorkflowPlanPath = Join-Path $tempRoot "workflow-plan-bad.json"
    $badWorkflowPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "workflow-service" `
        -Mode "compensation-instruction-import" `
        -Env "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID=tenant_1","NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE=$badInstructionPath"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing bad workflow approved invocation test"
    }
    $badWorkflowPlanJson | Set-Content -LiteralPath $badWorkflowPlanPath -Encoding UTF8
    $badWorkflowRequestPath = Join-Path $tempRoot "workflow-request-bad.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $badWorkflowPlanPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-workflow-invoke-bad" `
        -OutputPath $badWorkflowRequestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing bad workflow approved invocation test"
    }
    $badWorkflowDecisionPath = Join-Path $tempRoot "workflow-decision-bad.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $badWorkflowRequestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-workflow-invoke-bad" `
        -OutputPath $badWorkflowDecisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing bad workflow approved invocation test"
    }
    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $invokePath `
            -PlanPath $badWorkflowPlanPath `
            -RequestPath $badWorkflowRequestPath `
            -DecisionPath $badWorkflowDecisionPath 2>$null | Out-Null
        $badWorkflowExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($badWorkflowExitCode -eq 0) {
        throw "workflow approved invocation should reject invalid instruction manifest during preflight."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   approved repair invocation self-test"
