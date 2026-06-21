$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokerPath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
$bundleWriterPath = Join-Path $PSScriptRoot "write-repair-audit-bundle.ps1"
$pageWriterPath = Join-Path $PSScriptRoot "write-repair-approval-review-page.ps1"
$instructionWriterPath = Join-Path $PSScriptRoot "write-workflow-compensation-instruction-manifest.ps1"

foreach ($path in @(
        $plannerPath,
        $requestWriterPath,
        $decisionWriterPath,
        $invokerPath,
        $bundleWriterPath,
        $pageWriterPath,
        $instructionWriterPath
    )) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair approval review page test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-review-page-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $payloadRef = Join-Path $tempRoot "payload-ref.txt"
    $instructionReason = Join-Path $tempRoot "instruction-reason.txt"
    $requestReason = Join-Path $tempRoot "request-reason.txt"
    $decisionReason = Join-Path $tempRoot "decision-reason.txt"
    $bundleReason = Join-Path $tempRoot "bundle-reason.txt"
    $leakMarker = "do-not-leak-approval-review-secret-body"
    "external-payload-ref-$leakMarker" | Set-Content -LiteralPath $payloadRef -Encoding UTF8
    "instruction reason $leakMarker" | Set-Content -LiteralPath $instructionReason -Encoding UTF8
    "request reason $leakMarker" | Set-Content -LiteralPath $requestReason -Encoding UTF8
    "decision reason $leakMarker" | Set-Content -LiteralPath $decisionReason -Encoding UTF8
    "bundle reason $leakMarker" | Set-Content -LiteralPath $bundleReason -Encoding UTF8

    $manifestPath = Join-Path $tempRoot "workflow-instruction.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $instructionWriterPath `
        -OutputPath $manifestPath `
        -WorkflowID "wf-review-1" `
        -PayloadRefFile $payloadRef `
        -Environment "local" `
        -ConfigKind "API_GATEWAY_TENANT_QUOTA" `
        -BundleKey "tenant-a" `
        -TargetVersion "quota-v1" `
        -OperatorRef "operator:rollback" `
        -ReasonFile $instructionReason | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-compensation-instruction-manifest.ps1 failed"
    }

    $planPath = Join-Path $tempRoot "plan.json"
    $requestPath = Join-Path $tempRoot "approval-request.json"
    $decisionPath = Join-Path $tempRoot "approval-decision.json"
    $invokePath = Join-Path $tempRoot "invoke-summary.json"
    $bundlePath = Join-Path $tempRoot "audit-bundle.json"
    $pagePath = Join-Path $tempRoot "approval-review.html"

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
        -ApprovalID "approval-review-1" `
        -ReasonFile $requestReason `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-review-1" `
        -ReasonFile $decisionReason `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $invokerPath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -OutputPath $invokePath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-approved-repair-operator.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleWriterPath `
        -EvidencePath "$planPath,$requestPath,$decisionPath,$invokePath" `
        -GeneratedBy "operator-a" `
        -ReasonFile $bundleReason `
        -OutputPath $bundlePath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-audit-bundle.ps1 failed"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -GeneratedBy "operator-a" `
        -PageID "repair-review-page-1" `
        -InvocationSummaryPath $invokePath `
        -AuditBundlePath $bundlePath `
        -OutputPath $pagePath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-review-page.ps1 failed"
    }

    $html = Get-Content -LiteralPath $pagePath -Raw
    foreach ($expected in @(
            "NexusIM Repair Approval Review",
            "workflow-service",
            "compensation-instruction-import",
            "approval-review-1",
            "decision-review-1",
            "workflow_compensation_instruction_manifest",
            "manifest_sha256",
            "manifest_path_sha256",
            "Audit Bundle Summary"
        )) {
        if (-not $html.Contains($expected)) {
            throw "approval review page missing expected low-sensitive content: $expected"
        }
    }

    foreach ($forbidden in @(
            $manifestPath,
            $payloadRef,
            $instructionReason,
            $requestReason,
            $decisionReason,
            $bundleReason,
            $leakMarker
        )) {
        if ($html.Contains($forbidden)) {
            throw "approval review page leaked sensitive or local artifact content: $forbidden"
        }
    }

    $repoLocalOutput = Join-Path (Split-Path -Parent $PSScriptRoot) "tmp-repair-review.html"
    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $pageWriterPath `
            -PlanPath $planPath `
            -RequestPath $requestPath `
            -DecisionPath $decisionPath `
            -GeneratedBy "operator-a" `
            -OutputPath $repoLocalOutput 2>$null | Out-Null
        $repoOutputExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($repoOutputExitCode -eq 0) {
        throw "write-repair-approval-review-page.ps1 should reject repository-local OutputPath."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair approval review page self-test"
