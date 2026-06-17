param(
    [Parameter(Mandatory = $true)]
    [string]$ManifestPath,

    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $ManifestPath -PathType Leaf)) {
    throw "Missing repair batch manifest file: $ManifestPath"
}

function Assert-RequiredString {
    param(
        [object]$Value,
        [string]$Name,
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace([string]$Value)) {
        throw "Repair batch document is missing $Name`: $Path"
    }
}

$manifestRaw = Get-Content -LiteralPath $ManifestPath -Raw
$manifest = $manifestRaw | ConvertFrom-Json

if ($manifest.schema_version -ne 1) {
    throw "Unsupported repair batch manifest schema_version: $($manifest.schema_version)"
}
if ($manifest.executes -ne $false) {
    throw "Repair batch manifest must not execute operators."
}
Assert-RequiredString $manifest.batch_id "batch_id" $ManifestPath
Assert-RequiredString $manifest.requested_by "requested_by" $ManifestPath
Assert-LowSensitiveRepairIdentifier -Value ([string]$manifest.batch_id) -FieldName "batch_id"
Assert-LowSensitiveRepairActor -Value ([string]$manifest.requested_by) -FieldName "requested_by"

$items = @($manifest.items)
if ($items.Count -eq 0) {
    throw "Repair batch manifest must include at least one item."
}
if ([int]$manifest.item_count -ne $items.Count) {
    throw "Repair batch manifest item_count does not match items length."
}

$seenApprovalIDs = @{}
$seenSummaryHashes = @{}
$services = @{}
$modes = @{}

foreach ($item in $items) {
    Assert-RequiredString $item.summary_path "item.summary_path" $ManifestPath
    Assert-RequiredString $item.summary_sha256 "item.summary_sha256" $ManifestPath
    Assert-RequiredString $item.approval_id "item.approval_id" $ManifestPath
    Assert-RequiredString $item.decision_id "item.decision_id" $ManifestPath
    Assert-RequiredString $item.service "item.service" $ManifestPath
    Assert-RequiredString $item.mode "item.mode" $ManifestPath
    Assert-RequiredString $item.command "item.command" $ManifestPath
    Assert-RequiredString $item.plan_path "item.plan_path" $ManifestPath
    Assert-RequiredString $item.request_path "item.request_path" $ManifestPath
    Assert-RequiredString $item.decision_path "item.decision_path" $ManifestPath
    Assert-RequiredString $item.plan_sha256 "item.plan_sha256" $ManifestPath
    Assert-RequiredString $item.request_sha256 "item.request_sha256" $ManifestPath
    Assert-RequiredString $item.decision_sha256 "item.decision_sha256" $ManifestPath

    $approvalID = [string]$item.approval_id
    Assert-LowSensitiveRepairIdentifier -Value $approvalID -FieldName "item.approval_id"
    Assert-LowSensitiveRepairIdentifier -Value ([string]$item.decision_id) -FieldName "item.decision_id"
    if ($seenApprovalIDs.ContainsKey($approvalID)) {
        throw "Duplicate approval_id in repair batch manifest: $approvalID"
    }
    $seenApprovalIDs[$approvalID] = $true

    $summaryHash = [string]$item.summary_sha256
    if ($seenSummaryHashes.ContainsKey($summaryHash)) {
        throw "Duplicate summary_sha256 in repair batch manifest: $summaryHash"
    }
    $seenSummaryHashes[$summaryHash] = $true

    if (-not (Test-Path -LiteralPath ([string]$item.summary_path) -PathType Leaf)) {
        throw "Repair batch manifest references missing invocation summary: $($item.summary_path)"
    }

    $summaryRaw = Get-Content -LiteralPath ([string]$item.summary_path) -Raw
    $actualSummaryHash = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($summaryRaw))
    if ($actualSummaryHash -ne $summaryHash) {
        throw "Repair batch manifest summary_sha256 mismatch for $($item.summary_path)"
    }

    $summary = $summaryRaw | ConvertFrom-Json
    if ($summary.schema_version -ne 1) {
        throw "Unsupported repair invocation summary schema_version in $($item.summary_path): $($summary.schema_version)"
    }
    if ($summary.execute_requested -ne $false -or $summary.executed -ne $false) {
        throw "Repair batch manifest item references an executed invocation summary: $($item.summary_path)"
    }

    $fieldPairs = @(
        @("approval_id", $summary.approval_id, $item.approval_id),
        @("decision_id", $summary.decision_id, $item.decision_id),
        @("service", $summary.service, $item.service),
        @("mode", $summary.mode, $item.mode),
        @("command", $summary.command, $item.command),
        @("mode_env", $summary.mode_env, $item.mode_env),
        @("plan_path", $summary.plan_path, $item.plan_path),
        @("request_path", $summary.request_path, $item.request_path),
        @("decision_path", $summary.decision_path, $item.decision_path),
        @("plan_sha256", $summary.plan_sha256, $item.plan_sha256),
        @("request_sha256", $summary.request_sha256, $item.request_sha256),
        @("decision_sha256", $summary.decision_sha256, $item.decision_sha256)
    )
    foreach ($pair in $fieldPairs) {
        if ([string]$pair[1] -ne [string]$pair[2]) {
            throw "Repair batch manifest item $($pair[0]) does not match invocation summary: $($item.summary_path)"
        }
    }
    if ([bool]$summary.dry_run_requested -ne [bool]$item.dry_run_requested) {
        throw "Repair batch manifest item dry_run_requested does not match invocation summary: $($item.summary_path)"
    }

    $services[[string]$item.service] = $true
    $modes["$($item.service)/$($item.mode)"] = $true
}

$result = [ordered]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    batch_id = [string]$manifest.batch_id
    item_count = $items.Count
    service_count = $services.Count
    mode_count = $modes.Count
    executes = $false
    valid = $true
    note = "Repair batch manifest validation only. It does not execute operators and does not copy environment values, reasons, or business data."
}

$json = $result | ConvertTo-Json -Depth 8
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
