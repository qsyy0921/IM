$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair approval decision test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-decision-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $planPath = Join-Path $tempRoot "plan.json"
    $requestPath = Join-Path $tempRoot "approval-request.json"
    $decisionReasonPath = Join-Path $tempRoot "decision-reason.txt"
    $decisionPath = Join-Path $tempRoot "approval-decision.json"

    "operator approver checked external ticket and approves replay" | Set-Content -LiteralPath $decisionReasonPath -Encoding UTF8

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -ReasonFileEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE" `
        -ReasonFilePath "H:\NexusIM\operator-plans\projection-reason.txt"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing approval decision test"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-test-2" `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing approval decision test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -ReasonFile $decisionReasonPath `
        -DecisionID "decision-test-1" `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed"
    }

    $requestRaw = Get-Content -LiteralPath $requestPath -Raw
    $request = $requestRaw | ConvertFrom-Json
    $decisionRaw = Get-Content -LiteralPath $decisionPath -Raw
    $decision = $decisionRaw | ConvertFrom-Json

    if ($decision.schema_version -ne 1 -or
        $decision.decision_id -ne "decision-test-1" -or
        $decision.approval_id -ne "approval-test-2" -or
        $decision.status -ne "APPROVED" -or
        $decision.service -ne "delivery-service" -or
        $decision.mode -ne "projection-checkpoint-repair" -or
        $decision.executes -ne $false) {
        throw "approval decision has unexpected identity fields."
    }
    if ([string]$decision.plan_sha256 -ne [string]$request.plan_sha256) {
        throw "approval decision should preserve request plan hash."
    }
    if ([string]::IsNullOrWhiteSpace([string]$decision.request_sha256)) {
        throw "approval decision should include request hash."
    }
    if (-not $decision.reason_present -or [string]::IsNullOrWhiteSpace([string]$decision.reason_sha256)) {
        throw "approval decision should include decision reason presence and hash."
    }
    if ($decisionRaw.Contains("projection-reason.txt") -or $decisionRaw.Contains("operator approver checked external ticket")) {
        throw "approval decision leaked raw env value or decision reason text."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $badActorOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
            -RequestPath $requestPath `
            -Decision "APPROVED" `
            -DecidedBy "approver-token" 2>&1
        $badActorExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($badActorExitCode -eq 0 -or ($badActorOutput -join "`n") -notmatch "low-sensitive operator id") {
        throw "approval decision should reject credential-like DecidedBy values."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair approval decision self-test"
