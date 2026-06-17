$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokePath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
$batchWriterPath = Join-Path $PSScriptRoot "write-repair-batch-manifest.ps1"
$batchValidatorPath = Join-Path $PSScriptRoot "validate-repair-batch-manifest.ps1"
$batchInvokerPath = Join-Path $PSScriptRoot "invoke-repair-batch-manifest.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath, $invokePath, $batchWriterPath, $batchValidatorPath, $batchInvokerPath)) {
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
        [string]$OutputName,
        [bool]$UseDryRun = $true
    )

    $planPath = Join-Path $tempRoot "$OutputName-plan.json"
    $requestPath = Join-Path $tempRoot "$OutputName-request.json"
    $decisionPath = Join-Path $tempRoot "$OutputName-decision.json"
    $summaryPath = Join-Path $tempRoot "$OutputName-summary.json"

    $plannerArgs = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $plannerPath,
        "-Service", "delivery-service",
        "-Mode", $Mode,
        "-Env", "NEXUSIM_DELIVERY_BATCH_TEST_REF=do-not-copy-batch-value"
    )
    if ($UseDryRun) {
        $plannerArgs += @("-DryRun", "-DryRunEnv", $DryRunEnv)
    }
    $planJson = & powershell @plannerArgs
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
    $summaryMutating = New-ApprovedInvocationSummary `
        -Mode "projection-failure-cleanup" `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_DRY_RUN" `
        -ApprovalID "approval-batch-mutating" `
        -DecisionID "decision-batch-mutating" `
        -OutputName "mutating" `
        -UseDryRun $false

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
            [string]::IsNullOrWhiteSpace([string]$item.plan_path) -or
            [string]::IsNullOrWhiteSpace([string]$item.request_path) -or
            [string]::IsNullOrWhiteSpace([string]$item.decision_path) -or
            [string]::IsNullOrWhiteSpace([string]$item.plan_sha256) -or
            [string]::IsNullOrWhiteSpace([string]$item.request_sha256) -or
            [string]::IsNullOrWhiteSpace([string]$item.decision_sha256)) {
            throw "repair batch manifest item is missing expected hashes."
        }
        if (@($item.environment_keys) -notcontains "NEXUSIM_DELIVERY_BATCH_TEST_REF") {
            throw "repair batch manifest should preserve redacted environment key names."
        }
    }

    $validationPath = Join-Path $tempRoot "batch-validation.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchValidatorPath `
        -ManifestPath $manifestPath `
        -OutputPath $validationPath
    if ($LASTEXITCODE -ne 0) {
        throw "validate-repair-batch-manifest.ps1 failed"
    }
    $validationRaw = Get-Content -LiteralPath $validationPath -Raw
    $validation = $validationRaw | ConvertFrom-Json
    if ($validation.schema_version -ne 1 -or
        $validation.batch_id -ne "repair-batch-test" -or
        $validation.item_count -ne 2 -or
        $validation.valid -ne $true -or
        $validation.executes -ne $false) {
        throw "repair batch manifest validation output has unexpected fields."
    }
    if ($validationRaw.Contains("do-not-copy-batch-value") -or $validationRaw.Contains("do-not-copy-batch-reason")) {
        throw "repair batch manifest validation leaked raw environment value or reason text."
    }

    $batchInvokePath = Join-Path $tempRoot "batch-invoke.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchInvokerPath `
        -ManifestPath $manifestPath `
        -OutputPath $batchInvokePath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-repair-batch-manifest.ps1 failed in preflight mode"
    }
    $batchInvokeRaw = Get-Content -LiteralPath $batchInvokePath -Raw
    $batchInvoke = $batchInvokeRaw | ConvertFrom-Json
    if ($batchInvoke.schema_version -ne 1 -or
        $batchInvoke.batch_id -ne "repair-batch-test" -or
        $batchInvoke.item_count -ne 2 -or
        $batchInvoke.execute_requested -ne $false -or
        $batchInvoke.executed_count -ne 0 -or
        $batchInvoke.failed_count -ne 0 -or
        $batchInvoke.all_items_dry_run -ne $true) {
        throw "repair batch invocation preflight output has unexpected fields."
    }
    if ($batchInvokeRaw.Contains("do-not-copy-batch-value") -or $batchInvokeRaw.Contains("do-not-copy-batch-reason")) {
        throw "repair batch invocation preflight leaked raw environment value or reason text."
    }

    $mutatingManifestPath = Join-Path $tempRoot "batch-manifest-mutating.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchWriterPath `
        -InvocationSummaryPath $summaryA,$summaryMutating `
        -RequestedBy "operator-a" `
        -BatchID "repair-batch-mutating-test" `
        -OutputPath $mutatingManifestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-batch-manifest.ps1 failed while preparing mutating batch test"
    }

    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $batchInvokerPath `
            -ManifestPath $mutatingManifestPath `
            -Execute 2>$null | Out-Null
        $mutatingBatchExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($mutatingBatchExitCode -eq 0) {
        throw "repair batch invocation should reject mixed mutating execution before running any item unless AllowMutating is set."
    }

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

    $tamperedManifestPath = Join-Path $tempRoot "batch-manifest-tampered.json"
    $tamperedManifest = $manifestRaw | ConvertFrom-Json
    $tamperedManifest.items[0].plan_sha256 = "tampered"
    ($tamperedManifest | ConvertTo-Json -Depth 10) | Set-Content -LiteralPath $tamperedManifestPath -Encoding UTF8
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $batchValidatorPath `
            -ManifestPath $tamperedManifestPath 2>$null | Out-Null
        $tamperedExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($tamperedExitCode -eq 0) {
        throw "repair batch manifest validator should reject tampered summary metadata."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair batch manifest self-test"
