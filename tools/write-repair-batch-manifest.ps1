param(
    [Parameter(Mandatory = $true)]
    [string[]]$InvocationSummaryPath,

    [Parameter(Mandatory = $true)]
    [string]$RequestedBy,

    [string]$OutputPath = "",
    [string]$BatchID = "",
    [string]$ReasonFile = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

$expandedInvocationSummaryPaths = @()
foreach ($pathEntry in $InvocationSummaryPath) {
    foreach ($pathPart in ([string]$pathEntry -split ",")) {
        if (-not [string]::IsNullOrWhiteSpace($pathPart)) {
            $expandedInvocationSummaryPaths += $pathPart.Trim()
        }
    }
}

if ($expandedInvocationSummaryPaths.Count -eq 0) {
    throw "At least one approved repair invocation summary is required."
}
if ([string]::IsNullOrWhiteSpace($BatchID)) {
    $BatchID = "repair-batch-" + [System.Guid]::NewGuid().ToString("N")
}

function Assert-RequiredString {
    param(
        [object]$Value,
        [string]$Name,
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace([string]$Value)) {
        throw "Repair invocation summary is missing $Name`: $Path"
    }
}

Assert-LowSensitiveRepairActor -Value $RequestedBy -FieldName "RequestedBy"

$reasonPresent = $false
$reasonHash = ""
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    $reasonSummary = Read-RepairReasonFileSummary -Path $ReasonFile -MissingMessage "Missing repair batch reason file"
    $reasonPresent = [bool]$reasonSummary.Present
    $reasonHash = [string]$reasonSummary.Sha256
}

$seenApprovalIDs = @{}
$seenSummaryHashes = @{}
$items = @()
$index = 0

foreach ($summaryPath in $expandedInvocationSummaryPaths) {
    if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
        throw "Missing approved repair invocation summary file: $summaryPath"
    }

    $resolvedSummaryPath = [string](Resolve-Path -LiteralPath $summaryPath)
    $summaryRaw = Get-Content -LiteralPath $summaryPath -Raw
    $summary = $summaryRaw | ConvertFrom-Json
    $summaryHash = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($summaryRaw))

    if ($seenSummaryHashes.ContainsKey($summaryHash)) {
        throw "Duplicate repair invocation summary content in batch: $resolvedSummaryPath"
    }
    $seenSummaryHashes[$summaryHash] = $true

    if ($summary.schema_version -ne 1) {
        throw "Unsupported repair invocation summary schema_version in $resolvedSummaryPath`: $($summary.schema_version)"
    }
    if ($summary.execute_requested -ne $false -or $summary.executed -ne $false) {
        throw "Repair batch manifests only accept non-executed preflight summaries: $resolvedSummaryPath"
    }

    Assert-RequiredString $summary.approval_id "approval_id" $resolvedSummaryPath
    Assert-RequiredString $summary.decision_id "decision_id" $resolvedSummaryPath
    Assert-RequiredString $summary.service "service" $resolvedSummaryPath
    Assert-RequiredString $summary.mode "mode" $resolvedSummaryPath
    Assert-RequiredString $summary.command "command" $resolvedSummaryPath
    Assert-RequiredString $summary.plan_path "plan_path" $resolvedSummaryPath
    Assert-RequiredString $summary.request_path "request_path" $resolvedSummaryPath
    Assert-RequiredString $summary.decision_path "decision_path" $resolvedSummaryPath
    Assert-RequiredString $summary.plan_sha256 "plan_sha256" $resolvedSummaryPath
    Assert-RequiredString $summary.request_sha256 "request_sha256" $resolvedSummaryPath
    Assert-RequiredString $summary.decision_sha256 "decision_sha256" $resolvedSummaryPath

    $approvalID = [string]$summary.approval_id
    if ($seenApprovalIDs.ContainsKey($approvalID)) {
        throw "Duplicate approval_id in repair batch: $approvalID"
    }
    $seenApprovalIDs[$approvalID] = $true

    $items += [ordered]@{
        index = $index
        summary_path = $resolvedSummaryPath
        summary_sha256 = $summaryHash
        approval_id = $approvalID
        decision_id = [string]$summary.decision_id
        service = [string]$summary.service
        mode = [string]$summary.mode
        command = [string]$summary.command
        mode_env = [string]$summary.mode_env
        plan_path = [string]$summary.plan_path
        request_path = [string]$summary.request_path
        decision_path = [string]$summary.decision_path
        plan_sha256 = [string]$summary.plan_sha256
        request_sha256 = [string]$summary.request_sha256
        decision_sha256 = [string]$summary.decision_sha256
        dry_run_requested = [bool]$summary.dry_run_requested
        environment_keys = @($summary.environment_keys | Sort-Object)
        dry_run_env_keys = @($summary.dry_run_env_keys | Sort-Object)
    }
    $index++
}

$manifest = [ordered]@{
    schema_version = 1
    batch_id = $BatchID
    requested_at = (Get-Date).ToUniversalTime().ToString("o")
    requested_by = $RequestedBy
    item_count = $items.Count
    reason_present = $reasonPresent
    reason_sha256 = $reasonHash
    executes = $false
    items = $items
    note = "Repair batch manifest only groups approved preflight summaries. It redacts environment values, reasons, and business data."
}

$json = $manifest | ConvertTo-Json -Depth 10
if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $json | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $json
}
