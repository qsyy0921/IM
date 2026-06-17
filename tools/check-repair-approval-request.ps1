$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$approvalWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"

foreach ($path in @($plannerPath, $approvalWriterPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair approval test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-approval-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $planPath = Join-Path $tempRoot "plan.json"
    $reasonPath = Join-Path $tempRoot "reason.txt"
    $approvalPath = Join-Path $tempRoot "approval.json"

    "operator needs to replay a resolved projection after external audit" | Set-Content -LiteralPath $reasonPath -Encoding UTF8

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -ReasonFileEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE" `
        -ReasonFilePath "H:\NexusIM\operator-plans\projection-reason.txt"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing approval test"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $approvalWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ReasonFile $reasonPath `
        -ApprovalID "approval-test-1" `
        -OutputPath $approvalPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed"
    }

    $approvalRaw = Get-Content -LiteralPath $approvalPath -Raw
    $approval = $approvalRaw | ConvertFrom-Json

    if ($approval.schema_version -ne 1 -or
        $approval.approval_id -ne "approval-test-1" -or
        $approval.status -ne "PENDING" -or
        $approval.service -ne "delivery-service" -or
        $approval.mode -ne "projection-checkpoint-repair" -or
        $approval.executes -ne $false) {
        throw "approval request has unexpected identity fields."
    }
    if (-not $approval.reason_present -or [string]::IsNullOrWhiteSpace([string]$approval.reason_sha256)) {
        throw "approval request should include reason presence and hash."
    }
    if ([string]::IsNullOrWhiteSpace([string]$approval.plan_sha256)) {
        throw "approval request should include plan hash."
    }
    $environmentKeys = @($approval.environment_keys)
    foreach ($expectedKey in @(
        "NEXUSIM_DELIVERY_SERVICE_MODE",
        "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN",
        "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE"
    )) {
        if ($environmentKeys -notcontains $expectedKey) {
            throw "approval request missing environment key: $expectedKey"
        }
    }
    if ($approvalRaw.Contains("projection-reason.txt") -or $approvalRaw.Contains("operator needs to replay")) {
        throw "approval request leaked raw env value or reason text."
    }

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $badActorOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $approvalWriterPath `
            -PlanPath $planPath `
            -RequestedBy "operator@example.com" 2>&1
        $badActorExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($badActorExitCode -eq 0 -or ($badActorOutput -join "`n") -notmatch "low-sensitive operator id") {
        throw "approval request should reject sensitive or personal RequestedBy values."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair approval request self-test"
