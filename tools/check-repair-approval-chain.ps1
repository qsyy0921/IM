$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$validatorPath = Join-Path $PSScriptRoot "validate-repair-approval-chain.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath, $validatorPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair approval chain test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-chain-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $planPath = Join-Path $tempRoot "plan.json"
    $requestPath = Join-Path $tempRoot "approval-request.json"
    $decisionPath = Join-Path $tempRoot "approval-decision.json"
    $summaryPath = Join-Path $tempRoot "approval-chain-summary.json"

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -Env "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON=do-not-copy-this-chain-value"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing approval chain test"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-chain-1" `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing approval chain test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-chain-1" `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing approval chain test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -OutputPath $summaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "validate-repair-approval-chain.ps1 failed"
    }

    $summaryRaw = Get-Content -LiteralPath $summaryPath -Raw
    $summary = $summaryRaw | ConvertFrom-Json
    if ($summary.schema_version -ne 1 -or
        $summary.valid -ne $true -or
        $summary.approval_id -ne "approval-chain-1" -or
        $summary.decision_id -ne "decision-chain-1" -or
        $summary.service -ne "delivery-service" -or
        $summary.mode -ne "projection-checkpoint-repair" -or
        $summary.mode_env -ne "NEXUSIM_DELIVERY_SERVICE_MODE" -or
        $summary.executes -ne $false) {
        throw "approval chain summary has unexpected fields."
    }
    foreach ($hashField in @("plan_sha256", "request_sha256", "decision_sha256")) {
        if ([string]::IsNullOrWhiteSpace([string]$summary.$hashField)) {
            throw "approval chain summary missing hash: $hashField"
        }
    }
    if ($summaryRaw.Contains("do-not-copy-this-chain-value")) {
        throw "approval chain summary leaked raw env value."
    }

    $tamperedPlan = Join-Path $tempRoot "tampered-plan.json"
    ($planJson -replace "projection-checkpoint-repair", "outbox-audit") | Set-Content -LiteralPath $tamperedPlan -Encoding UTF8
    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
            -PlanPath $tamperedPlan `
            -RequestPath $requestPath `
            -DecisionPath $decisionPath 2>$null | Out-Null
        $tamperedExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($tamperedExitCode -eq 0) {
        throw "approval chain validator should reject tampered plan file."
    }

    $unsupportedPlan = Join-Path $tempRoot "unsupported-plan.json"
    $unsupportedPlanJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing unsupported mode test"
    }
    $unsupportedPlanJson = $unsupportedPlanJson -replace "projection-checkpoint-repair", "not-in-catalog"
    $unsupportedPlanJson | Set-Content -LiteralPath $unsupportedPlan -Encoding UTF8
    $unsupportedRequest = Join-Path $tempRoot "unsupported-request.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $unsupportedPlan `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-chain-unsupported" `
        -OutputPath $unsupportedRequest
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing unsupported mode test"
    }
    $unsupportedDecision = Join-Path $tempRoot "unsupported-decision.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $unsupportedRequest `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-chain-unsupported" `
        -OutputPath $unsupportedDecision
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing unsupported mode test"
    }
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
            -PlanPath $unsupportedPlan `
            -RequestPath $unsupportedRequest `
            -DecisionPath $unsupportedDecision 2>$null | Out-Null
        $unsupportedExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($unsupportedExitCode -eq 0) {
        throw "approval chain validator should reject unsupported catalog mode."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair approval chain self-test"
