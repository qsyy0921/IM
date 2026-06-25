param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,

    [Parameter(Mandatory = $true)]
    [string]$RequestPath,

    [Parameter(Mandatory = $true)]
    [string]$DecisionPath,

    [Parameter(Mandatory = $true)]
    [string]$InstructionManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$InvocationSummaryPath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

$chainValidatorPath = Join-Path $PSScriptRoot "validate-repair-approval-chain.ps1"
$manifestValidatorPath = Join-Path $PSScriptRoot "validate-workflow-compensation-instruction-manifest.ps1"
foreach ($path in @($chainValidatorPath, $manifestValidatorPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow compensation instruction approval page dependency: $path"
    }
}

foreach ($entry in @(
        @{ path = $PlanPath; name = "PlanPath" },
        @{ path = $RequestPath; name = "RequestPath" },
        @{ path = $DecisionPath; name = "DecisionPath" },
        @{ path = $InstructionManifestPath; name = "InstructionManifestPath" }
    )) {
    if (-not (Test-Path -LiteralPath $entry.path -PathType Leaf)) {
        throw "Missing workflow compensation instruction approval page input: $($entry.path)"
    }
    Assert-ExternalRepairOutputPath -Value $entry.path -FieldName $entry.name
}

if (-not [string]::IsNullOrWhiteSpace($InvocationSummaryPath)) {
    if (-not (Test-Path -LiteralPath $InvocationSummaryPath -PathType Leaf)) {
        throw "Missing workflow compensation instruction approval invocation summary: $InvocationSummaryPath"
    }
    Assert-ExternalRepairOutputPath -Value $InvocationSummaryPath -FieldName "InvocationSummaryPath"
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($PlanPath))) "workflow-compensation-instruction-approval.html"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "workflow-compensation-instruction-approval-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-ApprovalPageFileSha256 {
    param([string]$Path)

    return Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path)))
}

function Get-ApprovalPageStringSha256 {
    param([string]$Value)

    return Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value))
}

function ConvertTo-HtmlText {
    param([object]$Value)

    return [System.Net.WebUtility]::HtmlEncode([string]$Value)
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

function Get-ObjectString {
    param(
        [object]$Object,
        [string]$Name,
        [switch]$AllowEmpty
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        if ($AllowEmpty) {
            return ""
        }
        throw "$Name is required."
    }
    $value = ([string]$Object.$Name).Trim()
    if ($value.Length -eq 0 -and -not $AllowEmpty) {
        throw "$Name is required."
    }
    return $value
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

function Assert-LowSensitiveApprovalValue {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
}

$validationRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $chainValidatorPath `
    -PlanPath $PlanPath `
    -RequestPath $RequestPath `
    -DecisionPath $DecisionPath
if ($LASTEXITCODE -ne 0) {
    throw "Repair approval chain validation failed while building workflow compensation instruction approval page."
}
$validation = ($validationRaw -join "`n") | ConvertFrom-Json

if ([string]$validation.service -ne "workflow-service") {
    throw "Workflow compensation instruction approval page only accepts workflow-service plans."
}
if ([string]$validation.mode -ne "compensation-instruction-import") {
    throw "Workflow compensation instruction approval page only accepts compensation-instruction-import mode."
}

$plan = Get-JsonDocument -Path $PlanPath -Label "Repair operator plan"
$request = Get-JsonDocument -Path $RequestPath -Label "Repair approval request"
$decision = Get-JsonDocument -Path $DecisionPath -Label "Repair approval decision"
$manifest = Get-JsonDocument -Path $InstructionManifestPath -Label "Workflow compensation instruction manifest"

$manifestSummaryRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $manifestValidatorPath `
    -ManifestPath $InstructionManifestPath
if ($LASTEXITCODE -ne 0) {
    throw "Workflow compensation instruction manifest validation failed."
}
$manifestSummary = ($manifestSummaryRaw -join "`n") | ConvertFrom-Json

$tenantID = Get-ObjectString -Object $plan.environment -Name "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID"
$manifestPathFromPlan = Get-ObjectString -Object $plan.environment -Name "NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE"
Assert-LowSensitiveApprovalValue -Value $tenantID -FieldName "tenant_id"

$resolvedManifestFromArg = [System.IO.Path]::GetFullPath((Resolve-Path -LiteralPath $InstructionManifestPath))
$resolvedManifestFromPlan = [System.IO.Path]::GetFullPath($manifestPathFromPlan)
if (-not $resolvedManifestFromArg.Equals($resolvedManifestFromPlan, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "InstructionManifestPath does not match NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE in the approved plan."
}

$manifestPathSha256 = Get-ApprovalPageStringSha256 -Value $manifestPathFromPlan
$summaryResolvedManifest = [System.IO.Path]::GetFullPath([string]$manifestSummary.manifest_path)
if (-not $summaryResolvedManifest.Equals($resolvedManifestFromArg, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Workflow compensation instruction manifest summary path does not match input."
}

$instructions = @($manifest.instructions)
if ($instructions.Count -ne [int]$manifestSummary.instruction_count) {
    throw "Instruction manifest count does not match validator summary."
}

foreach ($instruction in $instructions) {
    foreach ($field in @(
            "instruction_id",
            "workflow_id",
            "payload_ref_hash",
            "environment",
            "config_kind",
            "bundle_key",
            "target_version",
            "operator_ref",
            "reason_ref"
        )) {
        Assert-LowSensitiveApprovalValue -Value (Get-ObjectString -Object $instruction -Name $field -AllowEmpty) -FieldName "instruction.$field" -AllowEmpty
    }
}

$invocation = $null
if (-not [string]::IsNullOrWhiteSpace($InvocationSummaryPath)) {
    $invocation = Get-JsonDocument -Path $InvocationSummaryPath -Label "Approved repair invocation summary"
    if ([int]$invocation.schema_version -ne 1) {
        throw "Unsupported approved repair invocation summary schema_version: $($invocation.schema_version)"
    }
    if ([string]$invocation.approval_id -ne [string]$validation.approval_id -or
        [string]$invocation.decision_id -ne [string]$validation.decision_id -or
        [string]$invocation.plan_sha256 -ne [string]$validation.plan_sha256 -or
        [string]$invocation.request_sha256 -ne [string]$validation.request_sha256 -or
        [string]$invocation.decision_sha256 -ne [string]$validation.decision_sha256) {
        throw "Approved repair invocation summary does not match the validated approval chain."
    }

    $instructionPreflight = @($invocation.preflight_checks) |
        Where-Object { [string]$_.name -eq "workflow_compensation_instruction_manifest" } |
        Select-Object -First 1
    if ($null -eq $instructionPreflight) {
        throw "Approved repair invocation summary is missing workflow compensation instruction manifest preflight."
    }
    if (-not [bool]$instructionPreflight.valid) {
        throw "Approved repair invocation workflow compensation instruction manifest preflight is not valid."
    }
    if ([string]$instructionPreflight.manifest_sha256 -ne [string]$manifestSummary.manifest_sha256) {
        throw "Approved repair invocation manifest_sha256 does not match instruction manifest."
    }
    if ([string]$instructionPreflight.manifest_path_sha256 -ne $manifestPathSha256) {
        throw "Approved repair invocation manifest_path_sha256 does not match approved plan."
    }
}

$artifactRows = [ordered]@{
    page_id = $PageID
    plan_sha256 = Get-ApprovalPageFileSha256 -Path $PlanPath
    request_sha256 = Get-ApprovalPageFileSha256 -Path $RequestPath
    decision_sha256 = Get-ApprovalPageFileSha256 -Path $DecisionPath
    instruction_manifest_sha256 = [string]$manifestSummary.manifest_sha256
    instruction_manifest_path_sha256 = $manifestPathSha256
}
if ($null -ne $invocation) {
    $artifactRows.invocation_summary_sha256 = Get-ApprovalPageFileSha256 -Path $InvocationSummaryPath
    $artifactRows.invocation_summary_path_sha256 = Get-ApprovalPageStringSha256 -Value ([string](Resolve-Path -LiteralPath $InvocationSummaryPath))
}

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Workflow Compensation Instruction Approval</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1180px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{background:#f1f5f9;}table.summary th{width:280px;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Workflow Compensation Instruction Approval</h1>")
[void]$html.AppendLine("<p class=`"note`">Static first-stage operator page for compensation instruction import approval. It verifies the approval chain and instruction manifest binding, then renders low-sensitive refs and hashes only. It does not create approvals, record decisions, import instructions, execute compensation, call workflow-service, call control-plane-service, or embed raw payloads, reason text, local paths, credentials, provider bodies, or evidence bodies.</p>")

Add-SectionHeader -Builder $html -Title "Approval Summary"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    page_id = $PageID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    approval_id = [string]$validation.approval_id
    decision_id = [string]$validation.decision_id
    service = [string]$validation.service
    mode = [string]$validation.mode
    mode_env = [string]$validation.mode_env
    command = [string]$validation.command
    tenant_id = $tenantID
    request_status = [string]$request.status
    decision_status = [string]$decision.status
    executes = [bool]$validation.executes
    final_import_owner = "workflow-service"
    final_compensation_execution_owner = "workflow-service compensation-executor"
})

Add-SectionHeader -Builder $html -Title "Instruction Manifest"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    schema_version = [string]$manifestSummary.schema_version
    instruction_count = [int]$manifestSummary.instruction_count
    manifest_sha256 = [string]$manifestSummary.manifest_sha256
    manifest_path_sha256 = $manifestPathSha256
})

Add-SectionHeader -Builder $html -Title "Instructions"
[void]$html.AppendLine("<table>")
[void]$html.AppendLine("<tr><th>instruction_id</th><th>workflow_id</th><th>payload_ref_hash</th><th>environment</th><th>config_kind</th><th>bundle_key</th><th>target_version</th><th>operator_ref</th><th>reason_ref</th></tr>")
foreach ($instruction in $instructions) {
    [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $instruction.instruction_id)</td><td>$(ConvertTo-HtmlText $instruction.workflow_id)</td><td>$(ConvertTo-HtmlText $instruction.payload_ref_hash)</td><td>$(ConvertTo-HtmlText $instruction.environment)</td><td>$(ConvertTo-HtmlText $instruction.config_kind)</td><td>$(ConvertTo-HtmlText $instruction.bundle_key)</td><td>$(ConvertTo-HtmlText $instruction.target_version)</td><td>$(ConvertTo-HtmlText $instruction.operator_ref)</td><td>$(ConvertTo-HtmlText $instruction.reason_ref)</td></tr>")
}
[void]$html.AppendLine("</table>")

Add-SectionHeader -Builder $html -Title "Approval Reason Hashes"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    request_reason_present = [bool]$request.reason_present
    request_reason_sha256 = [string]$request.reason_sha256
    decision_reason_present = [bool]$decision.reason_present
    decision_reason_sha256 = [string]$decision.reason_sha256
})

if ($null -ne $invocation) {
    Add-SectionHeader -Builder $html -Title "Approved Invocation Preflight"
    Add-SimpleTable -Builder $html -Rows ([ordered]@{
        execute_requested = [bool]$invocation.execute_requested
        mutating_execution_allowed = [bool]$invocation.mutating_execution_allowed
        executed = [bool]$invocation.executed
        dry_run_requested = [bool]$invocation.dry_run_requested
    })
}

Add-SectionHeader -Builder $html -Title "Artifact Hashes"
Add-SimpleTable -Builder $html -Rows $artifactRows

Add-SectionHeader -Builder $html -Title "Execution Boundary"
[void]$html.AppendLine("<table>")
foreach ($entry in ([ordered]@{
        approval_page_is_read_only = $true
        does_not_execute_operator = $true
        does_not_import_instruction = $true
        does_not_record_workflow_decision = $true
        does_not_execute_compensation = $true
        does_not_call_control_plane_service = $true
        final_execution_owner_preserved = "workflow-service compensation-executor"
    }).GetEnumerator()) {
    Add-TableRow -Builder $html -Key $entry.Key -Value $entry.Value
}
[void]$html.AppendLine("</table>")

[void]$html.AppendLine("</main></body></html>")
$htmlText = $html.ToString()

$parent = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
}
$htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow compensation instruction approval page written: $OutputPath"
