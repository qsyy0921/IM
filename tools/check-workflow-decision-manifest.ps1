$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-decision-manifest.ps1"
$validatorPath = Join-Path $PSScriptRoot "validate-workflow-decision-manifest.ps1"
foreach ($path in @($writerPath, $validatorPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow decision manifest tool dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-decision-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $reasonPath = Join-Path $tempRoot "reason.txt"
    $manifestPath = Join-Path $tempRoot "workflow-decision.json"
    "operator approved after checking external ticket" | Set-Content -LiteralPath $reasonPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -OutputPath $manifestPath `
        -WorkflowID "wf_manifest_1" `
        -StepID "wfs_manifest_1" `
        -Decision "APPROVE" `
        -DeciderRef "operator-a" `
        -DecisionPolicyRef "workflow.external-approval.v1" `
        -ReasonFile $reasonPath `
        -EvidenceRef "evidence:ticket-1","evidence:ticket-1","evidence:runbook-1" `
        -IdempotencyKey "external-approval:wf_manifest_1:wfs_manifest_1:approve:operator-a" `
        -CorrelationID "corr:wf_manifest_1" `
        -CausationID "approval:external-1" `
        -TraceID "trace:wf_manifest_1" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-decision-manifest.ps1 failed"
    }

    $summaryRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
        -ManifestPath $manifestPath `
        -ExpectedWorkflowID "wf_manifest_1" `
        -ExpectedStepID "wfs_manifest_1" `
        -ExpectedDecision "APPROVE"
    if ($LASTEXITCODE -ne 0) {
        throw "validate-workflow-decision-manifest.ps1 failed"
    }
    $summary = ($summaryRaw -join "`n") | ConvertFrom-Json
    if ($summary.workflow_id -ne "wf_manifest_1" -or $summary.step_id -ne "wfs_manifest_1" -or $summary.decision -ne "APPROVE") {
        throw "Workflow decision manifest summary has unexpected identity fields."
    }

    $manifestRaw = Get-Content -LiteralPath $manifestPath -Raw
    $manifest = $manifestRaw | ConvertFrom-Json
    if ($manifest.schema_version -ne "nexusim.workflow.decision_manifest.v1" -or
        $manifest.reason_ref -notmatch "^reason-sha256:[a-f0-9]{64}$" -or
        @($manifest.evidence_refs).Count -ne 2) {
        throw "Workflow decision manifest has unexpected normalized fields."
    }
    if ($manifestRaw.Contains("operator approved after checking external ticket") -or
        $manifestRaw.Contains("reason.txt")) {
        throw "Workflow decision manifest leaked reason text or local reason path."
    }

    $badOutput = ""
    $badExitCode = 0
    try {
        $badOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
            -OutputPath (Join-Path $tempRoot "bad.json") `
            -WorkflowID "wf_manifest_1" `
            -StepID "wfs_manifest_1" `
            -Decision "APPROVE" `
            -DeciderRef "bearer-token" 2>&1
        $badExitCode = $LASTEXITCODE
    } catch {
        $badOutput = $_.Exception.Message
        $badExitCode = 1
    }
    if ($badExitCode -eq 0) {
        throw "Workflow decision manifest writer should reject credential-like decider refs."
    }
    if (($badOutput -join "`n") -notmatch "low-sensitive") {
        throw "Workflow decision manifest writer rejection did not mention low-sensitive refs."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow decision manifest self-test"
