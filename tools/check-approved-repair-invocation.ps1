$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokePath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath, $invokePath)) {
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
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   approved repair invocation self-test"
