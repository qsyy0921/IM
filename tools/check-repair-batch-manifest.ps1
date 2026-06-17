$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokePath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
$batchWriterPath = Join-Path $PSScriptRoot "write-repair-batch-manifest.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath, $invokePath, $batchWriterPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair batch manifest test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-batch-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

function New-ApprovedInvocationSummary {
    param(
        [string]$Mode,
        [string]$DryRunEnv,
        [string]$ApprovalID,
        [string]$DecisionID,
        [string]$OutputName
    )

    $planPath = Join-Path $tempRoot "$OutputName-plan.json"
    $requestPath = Join-Path $tempRoot "$OutputName-request.json"
    $decisionPath = Join-Path $tempRoot "$OutputName-decision.json"
    $summaryPath = Join-Path $tempRoot "$OutputName-summary.json"

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode $Mode `
        -DryRun `
        -DryRunEnv $DryRunEnv `
        -Env "NEXUSIM_DELIVERY_BATCH_TEST_SECRET=do-not-copy-batch-value"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing repair batch test"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ApprovalID $ApprovalID `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing repair batch test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID $DecisionID `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing repair batch test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $invokePath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -OutputPath $summaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-approved-repair-operator.ps1 failed while preparing repair batch test"
    }

    return $summaryPath
}

try {
    $summaryA = New-ApprovedInvocationSummary `
        -Mode "projection-checkpoint-repair" `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -ApprovalID "approval-batch-1" `
        -DecisionID "decision-batch-1" `
        -OutputName "one"
    $summaryB = New-ApprovedInvocationSummary `
        -Mode "projection-failure-resolve" `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_DRY_RUN" `
        -ApprovalID "approval-batch-2" `
        -DecisionID "decision-batch-2" `
        -OutputName "two"

    $reasonPath = Join-Path $tempRoot "batch-reason.txt"
    "batch reason do-not-copy-batch-reason" | Set-Content -LiteralPath $reasonPath -Encoding UTF8
    $manifestPath = Join-Path $tempRoot "batch-manifest.json"

    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchWriterPath `
        -InvocationSummaryPath $summaryA,$summaryB `
        -RequestedBy "operator-a" `
        -BatchID "repair-batch-test" `
        -ReasonFile $reasonPath `
        -OutputPath $manifestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-batch-manifest.ps1 failed"
    }

    $manifestRaw = Get-Content -LiteralPath $manifestPath -Raw
    $manifest = $manifestRaw | ConvertFrom-Json
    if ($manifest.schema_version -ne 1 -or
        $manifest.batch_id -ne "repair-batch-test" -or
        $manifest.item_count -ne 2 -or
        $manifest.executes -ne $false -or
        $manifest.reason_present -ne $true) {
        throw "repair batch manifest has unexpected top-level fields."
    }
    if ($manifestRaw.Contains("do-not-copy-batch-value") -or $manifestRaw.Contains("do-not-copy-batch-reason")) {
        throw "repair batch manifest leaked raw environment value or reason text."
    }

    $items = @($manifest.items)
    if ($items.Count -ne 2) {
        throw "repair batch manifest should contain exactly two items."
    }
    if (@($items.service | Select-Object -Unique).Count -ne 1 -or $items[0].service -ne "delivery-service") {
        throw "repair batch manifest should preserve item service metadata."
    }
    if (@($items.approval_id) -notcontains "approval-batch-1" -or @($items.approval_id) -notcontains "approval-batch-2") {
        throw "repair batch manifest should preserve approval ids."
    }
    foreach ($item in $items) {
        if ([string]::IsNullOrWhiteSpace([string]$item.summary_sha256) -or
            [string]::IsNullOrWhiteSpace([string]$item.plan_sha256) -or
            [string]::IsNullOrWhiteSpace([string]$item.request_sha256) -or
            [string]::IsNullOrWhiteSpace([string]$item.decision_sha256)) {
            throw "repair batch manifest item is missing expected hashes."
        }
        if (@($item.environment_keys) -notcontains "NEXUSIM_DELIVERY_BATCH_TEST_SECRET") {
            throw "repair batch manifest should preserve redacted environment key names."
        }
    }

    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $batchWriterPath `
            -InvocationSummaryPath $summaryA,$summaryA `
            -RequestedBy "operator-a" 2>$null | Out-Null
        $duplicateExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($duplicateExitCode -eq 0) {
        throw "repair batch manifest should reject duplicate summary files."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair batch manifest self-test"
