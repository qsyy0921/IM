$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokerPath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
$instructionWriterPath = Join-Path $PSScriptRoot "write-workflow-compensation-instruction-manifest.ps1"
$pageWriterPath = Join-Path $PSScriptRoot "write-workflow-compensation-instruction-approval-page.ps1"

foreach ($path in @(
        $plannerPath,
        $requestWriterPath,
        $decisionWriterPath,
        $invokerPath,
        $instructionWriterPath,
        $pageWriterPath
    )) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow compensation instruction approval page test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-instruction-approval-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

function Invoke-PageWriterExpectFailure {
    param(
        [string]$PlanPath,
        [string]$RequestPath,
        [string]$DecisionPath,
        [string]$InstructionManifestPath,
        [string]$OutputPath,
        [string]$FailureName,
        [string]$InvocationSummaryPath = ""
    )

    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $args = @(
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-File",
            $pageWriterPath,
            "-PlanPath",
            $PlanPath,
            "-RequestPath",
            $RequestPath,
            "-DecisionPath",
            $DecisionPath,
            "-InstructionManifestPath",
            $InstructionManifestPath,
            "-GeneratedBy",
            "operator-a",
            "-PageID",
            "workflow-comp-instruction-approval-page-1",
            "-OutputPath",
            $OutputPath
        )
        if (-not [string]::IsNullOrWhiteSpace($InvocationSummaryPath)) {
            $args += @("-InvocationSummaryPath", $InvocationSummaryPath)
        }
        & powershell @args 2>$null | Out-Null
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($exitCode -eq 0) {
        throw "$FailureName should have failed."
    }
}

try {
    $leakMarker = "do-not-leak-workflow-compensation-instruction-secret"
    $payloadRef = Join-Path $tempRoot "payload-ref.txt"
    $instructionReason = Join-Path $tempRoot "instruction-reason.txt"
    $requestReason = Join-Path $tempRoot "request-reason.txt"
    $decisionReason = Join-Path $tempRoot "decision-reason.txt"
    "external-payload-ref-$leakMarker" | Set-Content -LiteralPath $payloadRef -Encoding UTF8
    "instruction reason $leakMarker" | Set-Content -LiteralPath $instructionReason -Encoding UTF8
    "request reason $leakMarker" | Set-Content -LiteralPath $requestReason -Encoding UTF8
    "decision reason $leakMarker" | Set-Content -LiteralPath $decisionReason -Encoding UTF8

    $manifestPath = Join-Path $tempRoot "workflow-compensation-instruction.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $instructionWriterPath `
        -OutputPath $manifestPath `
        -WorkflowID "wf-comp-approval-1" `
        -PayloadRefFile $payloadRef `
        -Environment "local" `
        -ConfigKind "API_GATEWAY_TENANT_QUOTA" `
        -BundleKey "tenant-a" `
        -TargetVersion "quota-v2" `
        -OperatorRef "operator:rollback" `
        -ReasonFile $instructionReason | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-compensation-instruction-manifest.ps1 failed"
    }

    $planPath = Join-Path $tempRoot "plan.json"
    $requestPath = Join-Path $tempRoot "approval-request.json"
    $decisionPath = Join-Path $tempRoot "approval-decision.json"
    $invocationPath = Join-Path $tempRoot "invocation-summary.json"
    $pagePath = Join-Path $tempRoot "workflow-compensation-instruction-approval.html"

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "workflow-service" `
        -Mode "compensation-instruction-import" `
        -Env "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID=tenant-a,NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE=$manifestPath"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ApprovalID "wf-comp-approval-1" `
        -ReasonFile $requestReason `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "wf-comp-decision-1" `
        -ReasonFile $decisionReason `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $invokerPath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -OutputPath $invocationPath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-approved-repair-operator.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -InstructionManifestPath $manifestPath `
        -GeneratedBy "operator-a" `
        -PageID "workflow-comp-instruction-approval-page-1" `
        -InvocationSummaryPath $invocationPath `
        -OutputPath $pagePath
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-compensation-instruction-approval-page.ps1 failed"
    }

    $html = Get-Content -LiteralPath $pagePath -Raw
    foreach ($expected in @(
            "NexusIM Workflow Compensation Instruction Approval",
            "workflow-service",
            "compensation-instruction-import",
            "wf-comp-approval-1",
            "wf-comp-decision-1",
            "API_GATEWAY_TENANT_QUOTA",
            "quota-v2",
            "operator:rollback",
            "final_compensation_execution_owner",
            "instruction_manifest_sha256",
            "instruction_manifest_path_sha256",
            "does_not_execute_compensation"
        )) {
        if (-not $html.Contains($expected)) {
            throw "workflow compensation instruction approval page missing expected low-sensitive content: $expected"
        }
    }

    foreach ($forbidden in @(
            $manifestPath,
            $payloadRef,
            $instructionReason,
            $requestReason,
            $decisionReason,
            $tempRoot,
            $leakMarker,
            "external-payload-ref",
            "instruction reason",
            "request reason",
            "decision reason"
        )) {
        if ($html.Contains($forbidden)) {
            throw "workflow compensation instruction approval page leaked sensitive or local artifact content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-workflow-compensation-instruction-approval.html"
    Invoke-PageWriterExpectFailure `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -InstructionManifestPath $manifestPath `
        -OutputPath $repoLocalOutput `
        -InvocationSummaryPath $invocationPath `
        -FailureName "repository-local OutputPath"

    $badPlanPath = Join-Path $tempRoot "bad-plan.json"
    $badPlan = Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
    $badPlan.mode = "timer-worker"
    $badPlan.environment.NEXUSIM_WORKFLOW_SERVICE_MODE = "timer-worker"
    $badPlan | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $badPlanPath -Encoding UTF8
    Invoke-PageWriterExpectFailure `
        -PlanPath $badPlanPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -InstructionManifestPath $manifestPath `
        -OutputPath (Join-Path $tempRoot "bad-mode.html") `
        -FailureName "wrong workflow mode"

    $badManifestPath = Join-Path $tempRoot "other-manifest.json"
    Copy-Item -LiteralPath $manifestPath -Destination $badManifestPath
    Invoke-PageWriterExpectFailure `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -InstructionManifestPath $badManifestPath `
        -OutputPath (Join-Path $tempRoot "bad-manifest-path.html") `
        -FailureName "manifest path mismatch"

    $badInvocationPath = Join-Path $tempRoot "bad-invocation.json"
    $badInvocation = Get-Content -LiteralPath $invocationPath -Raw | ConvertFrom-Json
    $badInvocation.preflight_checks[0].manifest_sha256 = "bad"
    $badInvocation | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $badInvocationPath -Encoding UTF8
    Invoke-PageWriterExpectFailure `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -InstructionManifestPath $manifestPath `
        -OutputPath (Join-Path $tempRoot "bad-invocation.html") `
        -InvocationSummaryPath $badInvocationPath `
        -FailureName "invocation manifest hash mismatch"
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation instruction approval page self-test"
