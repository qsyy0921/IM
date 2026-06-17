$ErrorActionPreference = "Stop"

$plannerPath = Join-Path $PSScriptRoot "write-repair-operator-plan.ps1"
$requestWriterPath = Join-Path $PSScriptRoot "write-repair-approval-request.ps1"
$decisionWriterPath = Join-Path $PSScriptRoot "write-repair-approval-decision.ps1"
$invokePath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
$batchWriterPath = Join-Path $PSScriptRoot "write-repair-batch-manifest.ps1"
$batchValidatorPath = Join-Path $PSScriptRoot "validate-repair-batch-manifest.ps1"
$batchInvokerPath = Join-Path $PSScriptRoot "invoke-repair-batch-manifest.ps1"
$bundleWriterPath = Join-Path $PSScriptRoot "write-repair-audit-bundle.ps1"
$bundleValidatorPath = Join-Path $PSScriptRoot "validate-repair-audit-bundle.ps1"

foreach ($path in @($plannerPath, $requestWriterPath, $decisionWriterPath, $invokePath, $batchWriterPath, $batchValidatorPath, $batchInvokerPath, $bundleWriterPath, $bundleValidatorPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair audit bundle test dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-audit-bundle-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null

try {
    $planPath = Join-Path $tempRoot "plan.json"
    $requestPath = Join-Path $tempRoot "request.json"
    $decisionPath = Join-Path $tempRoot "decision.json"
    $summaryPath = Join-Path $tempRoot "summary.json"
    $batchPath = Join-Path $tempRoot "batch.json"
    $batchValidationPath = Join-Path $tempRoot "batch-validation.json"
    $batchInvocationPath = Join-Path $tempRoot "batch-invocation.json"
    $bundlePath = Join-Path $tempRoot "audit-bundle.json"
    $reasonPath = Join-Path $tempRoot "bundle-reason.txt"

    $planJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $plannerPath `
        -Service "delivery-service" `
        -Mode "projection-checkpoint-repair" `
        -DryRun `
        -DryRunEnv "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN" `
        -Env "NEXUSIM_REPAIR_AUDIT_BUNDLE_REF=do-not-copy-audit-bundle-value"
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-operator-plan.ps1 failed while preparing repair audit bundle test"
    }
    $planJson | Set-Content -LiteralPath $planPath -Encoding UTF8

    & powershell -NoProfile -ExecutionPolicy Bypass -File $requestWriterPath `
        -PlanPath $planPath `
        -RequestedBy "operator-a" `
        -ApprovalID "approval-audit-bundle-1" `
        -OutputPath $requestPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-request.ps1 failed while preparing repair audit bundle test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionWriterPath `
        -RequestPath $requestPath `
        -Decision "APPROVED" `
        -DecidedBy "approver-a" `
        -DecisionID "decision-audit-bundle-1" `
        -OutputPath $decisionPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-approval-decision.ps1 failed while preparing repair audit bundle test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $invokePath `
        -PlanPath $planPath `
        -RequestPath $requestPath `
        -DecisionPath $decisionPath `
        -OutputPath $summaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-approved-repair-operator.ps1 failed while preparing repair audit bundle test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchWriterPath `
        -InvocationSummaryPath $summaryPath `
        -RequestedBy "operator-a" `
        -BatchID "repair-batch-audit-bundle" `
        -OutputPath $batchPath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-batch-manifest.ps1 failed while preparing repair audit bundle test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchValidatorPath `
        -ManifestPath $batchPath `
        -OutputPath $batchValidationPath
    if ($LASTEXITCODE -ne 0) {
        throw "validate-repair-batch-manifest.ps1 failed while preparing repair audit bundle test"
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File $batchInvokerPath `
        -ManifestPath $batchPath `
        -OutputPath $batchInvocationPath
    if ($LASTEXITCODE -ne 0) {
        throw "invoke-repair-batch-manifest.ps1 failed while preparing repair audit bundle test"
    }

    "operator reason do-not-copy-audit-bundle-reason" | Set-Content -LiteralPath $reasonPath -Encoding UTF8
    & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleWriterPath `
        -EvidencePath $planPath,$requestPath,$decisionPath,$summaryPath,$batchPath,$batchValidationPath,$batchInvocationPath `
        -GeneratedBy "operator-a" `
        -BundleID "repair-audit-bundle-test" `
        -ReasonFile $reasonPath `
        -OutputPath $bundlePath
    if ($LASTEXITCODE -ne 0) {
        throw "write-repair-audit-bundle.ps1 failed"
    }

    $bundleRaw = Get-Content -LiteralPath $bundlePath -Raw
    $bundle = $bundleRaw | ConvertFrom-Json
    if ($bundle.schema_version -ne 1 -or
        $bundle.bundle_id -ne "repair-audit-bundle-test" -or
        $bundle.file_count -ne 7 -or
        $bundle.reason_present -ne $true) {
        throw "repair audit bundle has unexpected top-level fields."
    }
    if ($bundleRaw.Contains("do-not-copy-audit-bundle-value") -or $bundleRaw.Contains("do-not-copy-audit-bundle-reason")) {
        throw "repair audit bundle leaked raw environment value or reason text."
    }

    $files = @($bundle.files)
    if ($files.Count -ne 7) {
        throw "repair audit bundle should contain exactly seven evidence file entries."
    }
    foreach ($expectedKind in @(
        "repair_operator_plan",
        "repair_approval_request",
        "repair_approval_decision",
        "approved_repair_invocation",
        "repair_batch_manifest",
        "repair_batch_validation",
        "repair_batch_invocation"
    )) {
        if (@($files.kind) -notcontains $expectedKind) {
            throw "repair audit bundle missing expected evidence kind: $expectedKind"
        }
    }
    foreach ($file in $files) {
        if ([string]::IsNullOrWhiteSpace([string]$file.path) -or [string]::IsNullOrWhiteSpace([string]$file.sha256)) {
            throw "repair audit bundle file entry is missing path or hash."
        }
    }

    $validationPath = Join-Path $tempRoot "audit-bundle-validation.json"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleValidatorPath `
        -BundlePath $bundlePath `
        -OutputPath $validationPath
    if ($LASTEXITCODE -ne 0) {
        throw "validate-repair-audit-bundle.ps1 failed"
    }
    $validationRaw = Get-Content -LiteralPath $validationPath -Raw
    $validation = $validationRaw | ConvertFrom-Json
    if ($validation.schema_version -ne 1 -or
        $validation.bundle_id -ne "repair-audit-bundle-test" -or
        $validation.file_count -ne 7 -or
        $validation.valid -ne $true) {
        throw "repair audit bundle validation has unexpected fields."
    }
    if ($validationRaw.Contains("do-not-copy-audit-bundle-value") -or $validationRaw.Contains("do-not-copy-audit-bundle-reason")) {
        throw "repair audit bundle validation leaked raw environment value or reason text."
    }

    $oldErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleWriterPath `
            -EvidencePath $summaryPath `
            -GeneratedBy "Bearer abc.def.ghi" 2>$null | Out-Null
        $badActorExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($badActorExitCode -eq 0) {
        throw "repair audit bundle should reject credential-like GeneratedBy values."
    }

    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleWriterPath `
            -EvidencePath $summaryPath `
            -GeneratedBy "operator-a" `
            -BundleID "repair-audit-bundle-token-secret" 2>$null | Out-Null
        $badBundleIDExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($badBundleIDExitCode -eq 0) {
        throw "repair audit bundle should reject credential-like bundle ids."
    }

    $sensitiveIDBundlePath = Join-Path $tempRoot "audit-bundle-sensitive-id.json"
    $sensitiveIDBundle = $bundleRaw | ConvertFrom-Json
    $sensitiveIDBundle.bundle_id = "repair-audit-bundle-token-secret"
    ($sensitiveIDBundle | ConvertTo-Json -Depth 10) | Set-Content -LiteralPath $sensitiveIDBundlePath -Encoding UTF8
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleValidatorPath `
            -BundlePath $sensitiveIDBundlePath 2>$null | Out-Null
        $sensitiveBundleIDExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($sensitiveBundleIDExitCode -eq 0) {
        throw "repair audit bundle validator should reject credential-like bundle ids."
    }

    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleWriterPath `
            -EvidencePath $summaryPath,$summaryPath `
            -GeneratedBy "operator-a" 2>$null | Out-Null
        $duplicateExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($duplicateExitCode -eq 0) {
        throw "repair audit bundle should reject duplicate evidence content."
    }

    $tamperedPlan = Get-Content -LiteralPath $planPath -Raw | ConvertFrom-Json
    $tamperedPlan.note = "tampered"
    ($tamperedPlan | ConvertTo-Json -Depth 8) | Set-Content -LiteralPath $planPath -Encoding UTF8
    $ErrorActionPreference = "Continue"
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $bundleValidatorPath `
            -BundlePath $bundlePath 2>$null | Out-Null
        $tamperedExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    if ($tamperedExitCode -eq 0) {
        throw "repair audit bundle validator should reject tampered evidence content."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   repair audit bundle self-test"
