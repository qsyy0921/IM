param(
    [Parameter(Mandatory = $true)]
    [string]$ManifestPath,

    [string]$OutputPath = "",
    [switch]$Execute,
    [switch]$AllowMutating
)

$ErrorActionPreference = "Stop"

$batchValidatorPath = Join-Path $PSScriptRoot "validate-repair-batch-manifest.ps1"
$singleInvokerPath = Join-Path $PSScriptRoot "invoke-approved-repair-operator.ps1"
foreach ($path in @($batchValidatorPath, $singleInvokerPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair batch invocation dependency: $path"
    }
}
if (-not (Test-Path -LiteralPath $ManifestPath -PathType Leaf)) {
    throw "Missing repair batch manifest file: $ManifestPath"
}

$validationJson = & powershell -NoProfile -ExecutionPolicy Bypass -File $batchValidatorPath `
    -ManifestPath $ManifestPath
if ($LASTEXITCODE -ne 0) {
    throw "Repair batch manifest validation failed."
}
$validation = $validationJson | ConvertFrom-Json

$manifest = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
$items = @($manifest.items)
if ($items.Count -eq 0) {
    throw "Repair batch manifest contains no items."
}

$nonDryRunItems = @($items | Where-Object { $_.dry_run_requested -ne $true })
if ($Execute -and $nonDryRunItems.Count -gt 0 -and -not $AllowMutating) {
    throw "Refusing to execute a repair batch with $($nonDryRunItems.Count) non-dry-run item(s). Pass -AllowMutating only after explicit operator approval."
}

$itemSummaries = @()
$allDryRun = $true
$executedCount = 0
$failedCount = 0

foreach ($item in $items) {
    $planPath = [string]$item.plan_path
    $requestPath = [string]$item.request_path
    $decisionPath = [string]$item.decision_path

    foreach ($path in @($planPath, $requestPath, $decisionPath)) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Repair batch item references missing approval-chain file: $path"
        }
    }

    $invokeArgs = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $singleInvokerPath,
        "-PlanPath", $planPath,
        "-RequestPath", $requestPath,
        "-DecisionPath", $decisionPath
    )
    if ($Execute) {
        $invokeArgs += "-Execute"
    }
    if ($AllowMutating) {
        $invokeArgs += "-AllowMutating"
    }

    $singleJson = & powershell @invokeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Approved repair invocation failed for batch item $($item.index)."
    }
    $single = $singleJson | ConvertFrom-Json
    if ($single.plan_sha256 -ne $item.plan_sha256 -or
        $single.request_sha256 -ne $item.request_sha256 -or
        $single.decision_sha256 -ne $item.decision_sha256 -or
        $single.approval_id -ne $item.approval_id -or
        $single.decision_id -ne $item.decision_id) {
        throw "Repair batch item invocation summary does not match manifest metadata: $($item.index)"
    }

    if ($single.dry_run_requested -ne $true) {
        $allDryRun = $false
    }
    if ($single.executed -eq $true) {
        $executedCount++
    }
    if ($single.exit_code -ne $null -and [int]$single.exit_code -ne 0) {
        $failedCount++
    }

    $itemSummaries += [ordered]@{
        index = [int]$item.index
        approval_id = [string]$single.approval_id
        decision_id = [string]$single.decision_id
        service = [string]$single.service
        mode = [string]$single.mode
        command = [string]$single.command
        plan_sha256 = [string]$single.plan_sha256
        request_sha256 = [string]$single.request_sha256
        decision_sha256 = [string]$single.decision_sha256
        dry_run_requested = [bool]$single.dry_run_requested
        execute_requested = [bool]$single.execute_requested
        mutating_execution_allowed = [bool]$single.mutating_execution_allowed
        executed = [bool]$single.executed
        exit_code = $single.exit_code
    }
}

$summary = [ordered]@{
    schema_version = 1
    prepared_at = (Get-Date).ToUniversalTime().ToString("o")
    batch_id = [string]$validation.batch_id
    item_count = $items.Count
    service_count = [int]$validation.service_count
    mode_count = [int]$validation.mode_count
    all_items_dry_run = [bool]$allDryRun
    execute_requested = [bool]$Execute
    mutating_execution_allowed = [bool]$AllowMutating
    executed_count = $executedCount
    failed_count = $failedCount
    items = $itemSummaries
    note = "Repair batch invocation summary. It redacts environment values, reasons, and business data."
}

$json = $summary | ConvertTo-Json -Depth 10
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
