param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,

    [Parameter(Mandatory = $true)]
    [string]$RequestPath,

    [Parameter(Mandatory = $true)]
    [string]$DecisionPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$InvocationSummaryPath = "",
    [string]$AuditBundlePath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

$validatorPath = Join-Path $PSScriptRoot "validate-repair-approval-chain.ps1"
if (-not (Test-Path -LiteralPath $validatorPath -PathType Leaf)) {
    throw "Missing repair approval chain validator: $validatorPath"
}

foreach ($path in @($PlanPath, $RequestPath, $DecisionPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing repair approval review input: $path"
    }
}

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "repair-review-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-RepairFileSha256 {
    param([string]$Path)

    return Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path)))
}

function Get-RepairStringSha256 {
    param([string]$Value)

    return Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value))
}

function ConvertTo-HtmlText {
    param([object]$Value)

    return [System.Net.WebUtility]::HtmlEncode([string]$Value)
}

function Add-TableRow {
    param(
        [System.Text.StringBuilder]$Builder,
        [string]$Key,
        [object]$Value
    )

    [void]$Builder.AppendLine("<tr><th>$(ConvertTo-HtmlText $Key)</th><td>$(ConvertTo-HtmlText $Value)</td></tr>")
}

function Add-SectionHeader {
    param(
        [System.Text.StringBuilder]$Builder,
        [string]$Title
    )

    [void]$Builder.AppendLine("<h2>$(ConvertTo-HtmlText $Title)</h2>")
}

function Add-SimpleTable {
    param(
        [System.Text.StringBuilder]$Builder,
        [hashtable]$Rows
    )

    [void]$Builder.AppendLine("<table>")
    foreach ($key in @($Rows.Keys | Sort-Object)) {
        Add-TableRow -Builder $Builder -Key $key -Value $Rows[$key]
    }
    [void]$Builder.AppendLine("</table>")
}

function Get-JsonDocument {
    param(
        [string]$Path,
        [string]$Label
    )

    try {
        return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
    } catch {
        throw "$Label must be valid JSON: $Path"
    }
}

$validationRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
    -PlanPath $PlanPath `
    -RequestPath $RequestPath `
    -DecisionPath $DecisionPath
if ($LASTEXITCODE -ne 0) {
    throw "Repair approval chain validation failed while building review page."
}
$validation = ($validationRaw -join "`n") | ConvertFrom-Json

$plan = Get-JsonDocument -Path $PlanPath -Label "Plan"
$request = Get-JsonDocument -Path $RequestPath -Label "Approval request"
$decision = Get-JsonDocument -Path $DecisionPath -Label "Approval decision"

$environmentKeys = @()
if ($plan.environment) {
    foreach ($property in @($plan.environment.PSObject.Properties | Sort-Object Name)) {
        Assert-LowSensitiveRepairAdHocEnv -Key ([string]$property.Name) -Value "low-sensitive-ref"
        $environmentKeys += [string]$property.Name
    }
}

$artifactRows = [ordered]@{
    plan_sha256 = Get-RepairFileSha256 -Path $PlanPath
    request_sha256 = Get-RepairFileSha256 -Path $RequestPath
    decision_sha256 = Get-RepairFileSha256 -Path $DecisionPath
    plan_path_sha256 = Get-RepairStringSha256 -Value ([string](Resolve-Path -LiteralPath $PlanPath))
    request_path_sha256 = Get-RepairStringSha256 -Value ([string](Resolve-Path -LiteralPath $RequestPath))
    decision_path_sha256 = Get-RepairStringSha256 -Value ([string](Resolve-Path -LiteralPath $DecisionPath))
}

$invocation = $null
if (-not [string]::IsNullOrWhiteSpace($InvocationSummaryPath)) {
    if (-not (Test-Path -LiteralPath $InvocationSummaryPath -PathType Leaf)) {
        throw "Missing approved invocation summary: $InvocationSummaryPath"
    }
    $invocation = Get-JsonDocument -Path $InvocationSummaryPath -Label "Approved invocation summary"
    if ($invocation.schema_version -ne 1) {
        throw "Unsupported approved invocation summary schema_version: $($invocation.schema_version)"
    }
    if ([string]$invocation.approval_id -ne [string]$validation.approval_id -or
        [string]$invocation.decision_id -ne [string]$validation.decision_id -or
        [string]$invocation.plan_sha256 -ne [string]$validation.plan_sha256 -or
        [string]$invocation.request_sha256 -ne [string]$validation.request_sha256 -or
        [string]$invocation.decision_sha256 -ne [string]$validation.decision_sha256) {
        throw "Approved invocation summary does not match the validated approval chain."
    }
    $artifactRows.invocation_summary_sha256 = Get-RepairFileSha256 -Path $InvocationSummaryPath
    $artifactRows.invocation_summary_path_sha256 = Get-RepairStringSha256 -Value ([string](Resolve-Path -LiteralPath $InvocationSummaryPath))
}

$auditBundle = $null
if (-not [string]::IsNullOrWhiteSpace($AuditBundlePath)) {
    if (-not (Test-Path -LiteralPath $AuditBundlePath -PathType Leaf)) {
        throw "Missing repair audit bundle: $AuditBundlePath"
    }
    $auditBundle = Get-JsonDocument -Path $AuditBundlePath -Label "Repair audit bundle"
    if ($auditBundle.schema_version -ne 1) {
        throw "Unsupported repair audit bundle schema_version: $($auditBundle.schema_version)"
    }
    $artifactRows.audit_bundle_sha256 = Get-RepairFileSha256 -Path $AuditBundlePath
    $artifactRows.audit_bundle_path_sha256 = Get-RepairStringSha256 -Value ([string](Resolve-Path -LiteralPath $AuditBundlePath))
}

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Repair Approval Review</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1080px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{width:260px;background:#f1f5f9;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}ul{margin:8px 0 8px 22px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Repair Approval Review</h1>")
[void]$html.AppendLine("<p class=`"note`">Static first-stage operator review page. It stores hashes and low-sensitive metadata only. It does not execute operators and intentionally omits environment values, reason text, business payloads, evidence bodies, and local file paths.</p>")

Add-SectionHeader -Builder $html -Title "Approval Summary"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    page_id = $PageID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    approval_id = [string]$validation.approval_id
    decision_id = [string]$validation.decision_id
    service = [string]$validation.service
    mode = [string]$validation.mode
    command = [string]$validation.command
    mode_env = [string]$validation.mode_env
    decision_status = [string]$decision.status
    request_status = [string]$request.status
    dry_run_requested = [bool]$plan.dry_run_requested
    executes = [bool]$validation.executes
})

Add-SectionHeader -Builder $html -Title "Artifact Hashes"
Add-SimpleTable -Builder $html -Rows $artifactRows

Add-SectionHeader -Builder $html -Title "Environment Keys"
if ($environmentKeys.Count -eq 0) {
    [void]$html.AppendLine("<p>No environment keys are present in the plan.</p>")
} else {
    [void]$html.AppendLine("<ul>")
    foreach ($key in $environmentKeys) {
        [void]$html.AppendLine("<li><code>$(ConvertTo-HtmlText $key)</code></li>")
    }
    [void]$html.AppendLine("</ul>")
}

Add-SectionHeader -Builder $html -Title "Reason References"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    request_reason_present = [bool]$request.reason_present
    request_reason_sha256 = [string]$request.reason_sha256
    decision_reason_present = [bool]$decision.reason_present
    decision_reason_sha256 = [string]$decision.reason_sha256
})

if ($null -ne $invocation) {
    Add-SectionHeader -Builder $html -Title "Invocation Preflight"
    Add-SimpleTable -Builder $html -Rows ([ordered]@{
        execute_requested = [bool]$invocation.execute_requested
        mutating_execution_allowed = [bool]$invocation.mutating_execution_allowed
        executed = [bool]$invocation.executed
        dry_run_requested = [bool]$invocation.dry_run_requested
        exit_code = [string]$invocation.exit_code
    })

    $preflightChecks = @($invocation.preflight_checks)
    if ($preflightChecks.Count -gt 0) {
        [void]$html.AppendLine("<table>")
        [void]$html.AppendLine("<tr><th>name</th><th>valid</th><th>instruction_count</th><th>manifest_sha256</th><th>manifest_path_sha256</th></tr>")
        foreach ($check in $preflightChecks) {
            [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $check.name)</td><td>$(ConvertTo-HtmlText $check.valid)</td><td>$(ConvertTo-HtmlText $check.instruction_count)</td><td>$(ConvertTo-HtmlText $check.manifest_sha256)</td><td>$(ConvertTo-HtmlText $check.manifest_path_sha256)</td></tr>")
        }
        [void]$html.AppendLine("</table>")
    } else {
        [void]$html.AppendLine("<p>No invocation preflight checks were recorded.</p>")
    }
}

if ($null -ne $auditBundle) {
    Add-SectionHeader -Builder $html -Title "Audit Bundle Summary"
    Add-SimpleTable -Builder $html -Rows ([ordered]@{
        bundle_id = [string]$auditBundle.bundle_id
        file_count = [int]$auditBundle.file_count
        reason_present = [bool]$auditBundle.reason_present
        reason_sha256 = [string]$auditBundle.reason_sha256
    })
    $kindSummary = @($auditBundle.kind_summary)
    if ($kindSummary.Count -gt 0) {
        [void]$html.AppendLine("<table>")
        [void]$html.AppendLine("<tr><th>kind</th><th>count</th></tr>")
        foreach ($entry in $kindSummary) {
            [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $entry.kind)</td><td>$(ConvertTo-HtmlText $entry.count)</td></tr>")
        }
        [void]$html.AppendLine("</table>")
    }
}

[void]$html.AppendLine("</main></body></html>")
$htmlText = $html.ToString()

if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8
} else {
    $htmlText
}
