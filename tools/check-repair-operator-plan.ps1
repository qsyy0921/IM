$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"

if (-not (Test-Path -LiteralPath $plannerPath -PathType Leaf)) {
    throw "Missing repair operator plan writer: $plannerPath"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-operator-plan-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $messagePlanPath = Join-Path $tempRoot "message-plan.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "message-service" `
        -Mode "outbox-repair-cleanup" `
        -DryRun `
        -OutputEnv "NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT" `
        -OutputPath "H:\NexusIM\operator-plans\message-cleanup.json" `
        -PlanOutputPath $messagePlanPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for message-service"
    }

    $messagePlan = Get-Content -LiteralPath $messagePlanPath -Raw | ConvertFrom-Json
    if ($messagePlan.executes -ne $false -or $messagePlan.service -ne "message-service" -or $messagePlan.mode -ne "outbox-repair-cleanup") {
        throw "message-service repair operator plan has unexpected identity fields."
    }
    if ($messagePlan.environment.NEXUSIM_MESSAGE_SERVICE_MODE -ne "outbox-repair-cleanup" -or
        $messagePlan.environment.NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_DRY_RUN -ne "true" -or
        $messagePlan.environment.NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT -ne "H:\NexusIM\operator-plans\message-cleanup.json") {
        throw "message-service repair operator plan has unexpected environment."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $ambiguousOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service "delivery-service" `
            -Mode "projection-checkpoint-repair" `
            -DryRun 2>&1
        $ambiguousExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($ambiguousExitCode -eq 0 -or ($ambiguousOutput -join "`n") -notmatch "DryRun requires DryRunEnv") {
        throw "delivery-service ambiguous dry-run plan should require explicit DryRunEnv."
    }

    $deliveryPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -ReasonFileEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE" `
        -ReasonFilePath "H:\NexusIM\operator-plans\projection-reason.txt" `
        -Env "NEXUSIM_DELIVERY_PROJECTION_REPAIR_OPERATOR=manual"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for delivery-service"
    }
    $deliveryPlan = $deliveryPlanJson | ConvertFrom-Json
    if ($deliveryPlan.environment.NEXUSIM_DELIVERY_SERVICE_MODE -ne "projection-checkpoint-repair" -or
        $deliveryPlan.environment.NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN -ne "true" -or
        $deliveryPlan.environment.NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE -ne "H:\NexusIM\operator-plans\projection-reason.txt" -or
        $deliveryPlan.environment.NEXUSIM_DELIVERY_PROJECTION_REPAIR_OPERATOR -ne "manual") {
        throw "delivery-service repair operator plan has unexpected environment."
    }

    $receiptPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "receipt-service" `
        -Mode "outbox-repair" `
        -ReasonFilePath "H:\NexusIM\operator-plans\receipt-reason.txt"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for receipt-service reason file"
    }
    $receiptPlan = $receiptPlanJson | ConvertFrom-Json
    if ($receiptPlan.environment.NEXUSIM_RECEIPT_SERVICE_MODE -ne "outbox-repair" -or
        $receiptPlan.environment.NEXUSIM_RECEIPT_OUTBOX_REPAIR_REASON_FILE -ne "H:\NexusIM\operator-plans\receipt-reason.txt") {
        throw "receipt-service repair operator plan has unexpected reason-file environment."
    }

    $policyPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "policy-service" `
        -Mode "tenant-quota-set" `
        -OutputEnv "NEXUSIM_POLICY_TENANT_QUOTA_SET_OUTPUT" `
        -OutputPath "H:\NexusIM\operator-plans\policy-quota-set.json" `
        -Env "NEXUSIM_POLICY_TENANT_QUOTA_SET_TENANT_ID=tenant_1"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for policy-service tenant-quota-set"
    }
    $policyPlan = $policyPlanJson | ConvertFrom-Json
    if ($policyPlan.environment.NEXUSIM_POLICY_SERVICE_MODE -ne "tenant-quota-set" -or
        $policyPlan.environment.NEXUSIM_POLICY_TENANT_QUOTA_SET_OUTPUT -ne "H:\NexusIM\operator-plans\policy-quota-set.json" -or
        $policyPlan.environment.NEXUSIM_POLICY_TENANT_QUOTA_SET_TENANT_ID -ne "tenant_1") {
        throw "policy-service tenant-quota-set repair operator plan has unexpected environment."
    }

    $workflowPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "workflow-service" `
        -Mode "compensation-instruction-import" `
        -Env "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID=tenant_1","NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE=H:\NexusIM\operator-plans\workflow-compensation-instruction.json"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for workflow-service compensation-instruction-import"
    }
    $workflowPlan = $workflowPlanJson | ConvertFrom-Json
    if ($workflowPlan.environment.NEXUSIM_WORKFLOW_SERVICE_MODE -ne "compensation-instruction-import" -or
        $workflowPlan.environment.NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID -ne "tenant_1" -or
        $workflowPlan.environment.NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE -ne "H:\NexusIM\operator-plans\workflow-compensation-instruction.json") {
        throw "workflow-service compensation-instruction-import repair operator plan has unexpected environment."
    }

    $actionPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "action-executor" `
        -Mode "provider-failure-redrive-plan" `
        -OutputEnv "NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_PLAN_OUTPUT" `
        -OutputPath "H:\NexusIM\operator-plans\action-provider-redrive-plan.json" `
        -DryRun `
        -ReasonFilePath "H:\NexusIM\operator-plans\action-provider-redrive-reason.txt" `
        -Env "NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_TENANT_ID=tenant_1","NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_STATUS=DLQ"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed for action-executor provider-failure-redrive-plan"
    }
    $actionPlan = $actionPlanJson | ConvertFrom-Json
    if ($actionPlan.environment.NEXUSIM_ACTION_EXECUTOR_MODE -ne "provider-failure-redrive-plan" -or
        $actionPlan.environment.NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_PLAN_OUTPUT -ne "H:\NexusIM\operator-plans\action-provider-redrive-plan.json" -or
        $actionPlan.environment.NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_DRY_RUN -ne "true" -or
        $actionPlan.environment.NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE -ne "H:\NexusIM\operator-plans\action-provider-redrive-reason.txt" -or
        $actionPlan.environment.NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_TENANT_ID -ne "tenant_1" -or
        $actionPlan.environment.NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_STATUS -ne "DLQ") {
        throw "action-executor provider-failure-redrive-plan has unexpected environment."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $sensitiveKeyOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service "delivery-service" `
            -Mode "projection-checkpoint-repair" `
            -Env "NEXUSIM_OPERATOR_SECRET=do-not-store" 2>&1
        $sensitiveKeyExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($sensitiveKeyExitCode -eq 0 -or ($sensitiveKeyOutput -join "`n") -notmatch "potentially sensitive Env key") {
        throw "repair operator plan should reject sensitive ad-hoc env keys."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $duplicateReasonFileOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service "receipt-service" `
            -Mode "outbox-repair" `
            -ReasonFilePath "H:\NexusIM\operator-plans\receipt-reason.txt" `
            -Env "NEXUSIM_RECEIPT_OUTBOX_REPAIR_REASON_FILE=H:\NexusIM\operator-plans\other.txt" 2>&1
        $duplicateReasonFileExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($duplicateReasonFileExitCode -eq 0 -or ($duplicateReasonFileOutput -join "`n") -notmatch "duplicates a catalog-managed environment key") {
        throw "repair operator plan should reject duplicate reason-file env values."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $directReasonOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service "delivery-service" `
            -Mode "projection-checkpoint-repair" `
            -Env "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON=do-not-copy-this-reason" 2>&1
        $directReasonExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($directReasonExitCode -eq 0 -or ($directReasonOutput -join "`n") -notmatch "raw operator reason") {
        throw "repair operator plan should reject direct *_REASON env values."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $sensitiveValueOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
            -Service "delivery-service" `
            -Mode "projection-checkpoint-repair" `
            -Env "NEXUSIM_OPERATOR_NOTE=Bearer abc.def.ghi" 2>&1
        $sensitiveValueExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($sensitiveValueExitCode -eq 0 -or ($sensitiveValueOutput -join "`n") -notmatch "potentially sensitive Env value") {
        throw "repair operator plan should reject sensitive ad-hoc env values."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair operator plan writer self-test"
