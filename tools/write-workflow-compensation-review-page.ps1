param(
    [Parameter(Mandatory = $true)]
    [string]$BundlePath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $BundlePath -PathType Leaf)) {
    throw "Missing workflow compensation review bundle: $BundlePath"
}
Assert-ExternalRepairOutputPath -Value $BundlePath -FieldName "BundlePath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($BundlePath))) "workflow-compensation-review.html"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "workflow-compensation-review-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-CompReviewFileSha256 {
    param([string]$Path)

    return Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path)))
}

function Get-CompReviewStringSha256 {
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

function Get-JsonString {
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

function Get-JsonArray {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name] -or $null -eq $Object.$Name) {
        return @()
    }
    return @($Object.$Name)
}

function Assert-LowSensitiveString {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
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

function Add-List {
    param(
        [System.Text.StringBuilder]$Builder,
        [object[]]$Values,
        [string]$EmptyText
    )

    if ($Values.Count -eq 0) {
        [void]$Builder.AppendLine("<p>$(ConvertTo-HtmlText $EmptyText)</p>")
        return
    }
    [void]$Builder.AppendLine("<ul>")
    foreach ($value in $Values) {
        [void]$Builder.AppendLine("<li><code>$(ConvertTo-HtmlText $value)</code></li>")
    }
    [void]$Builder.AppendLine("</ul>")
}

$document = Get-JsonDocument -Path $BundlePath -Label "Workflow compensation review bundle"
$bundle = $document
if ($null -ne $document.PSObject.Properties["compensation_review"]) {
    $bundle = $document.compensation_review
}

$schemaVersion = Get-JsonString -Object $bundle -Name "schema_version"
if ($schemaVersion -ne "nexusim.workflow.compensation_review_bundle.v1") {
    throw "Unsupported workflow compensation review bundle schema_version: $schemaVersion"
}

if (-not [bool]$bundle.no_direct_execution) {
    throw "Workflow compensation review bundle must set no_direct_execution=true."
}
if (-not [bool]$bundle.no_decision_recorded) {
    throw "Workflow compensation review bundle must set no_decision_recorded=true."
}

$workflow = $bundle.workflow
if ($null -eq $workflow) {
    throw "workflow is required."
}

$workflowID = Get-JsonString -Object $workflow -Name "workflow_id"
$workflowType = Get-JsonString -Object $workflow -Name "workflow_type"
$workflowStatus = Get-JsonString -Object $workflow -Name "status"
$payloadRefHash = Get-JsonString -Object $workflow -Name "payload_ref_hash"
$targetService = Get-JsonString -Object $workflow -Name "target_service" -AllowEmpty
$targetOperation = Get-JsonString -Object $workflow -Name "target_operation" -AllowEmpty

Assert-LowSensitiveString -Value $workflowID -FieldName "workflow.workflow_id"
Assert-LowSensitiveString -Value $workflowType -FieldName "workflow.workflow_type"
Assert-LowSensitiveString -Value $workflowStatus -FieldName "workflow.status"
Assert-LowSensitiveString -Value $payloadRefHash -FieldName "workflow.payload_ref_hash"
Assert-LowSensitiveString -Value $targetService -FieldName "workflow.target_service" -AllowEmpty
Assert-LowSensitiveString -Value $targetOperation -FieldName "workflow.target_operation" -AllowEmpty

if ($workflowType -ne "COMPENSATION_REQUEST") {
    throw "workflow.workflow_type must be COMPENSATION_REQUEST."
}
if ($workflowStatus -ne "COMPENSATION_PENDING") {
    throw "workflow.status must be COMPENSATION_PENDING."
}

foreach ($field in @(
        "risk_level",
        "requester_ref",
        "requester_service",
        "target_ref_hash",
        "payload_schema_version",
        "approval_policy_ref",
        "timeout_policy_ref",
        "compensation_policy_ref",
        "reason_ref",
        "current_step_id",
        "correlation_id",
        "causation_id",
        "trace_id"
    )) {
    Assert-LowSensitiveString -Value (Get-JsonString -Object $workflow -Name $field -AllowEmpty) -FieldName "workflow.$field" -AllowEmpty
}
foreach ($evidenceRef in (Get-JsonArray -Object $workflow -Name "evidence_refs")) {
    Assert-LowSensitiveString -Value ([string]$evidenceRef) -FieldName "workflow.evidence_refs"
}

$instructionStatus = Get-JsonString -Object $bundle -Name "instruction_status"
if ($instructionStatus -ne "ACTIVE") {
    throw "instruction_status must be ACTIVE."
}
$instructionCount = [int]$bundle.instruction_count
$instructions = @(Get-JsonArray -Object $bundle -Name "instructions")
if ($instructionCount -ne $instructions.Count) {
    throw "instruction_count does not match instructions count."
}
if ($instructions.Count -eq 0) {
    throw "At least one compensation instruction is required."
}

foreach ($instruction in $instructions) {
    $instructionID = Get-JsonString -Object $instruction -Name "instruction_id"
    $instructionWorkflowID = Get-JsonString -Object $instruction -Name "workflow_id"
    $instructionPayloadRefHash = Get-JsonString -Object $instruction -Name "payload_ref_hash"
    $instructionTargetService = Get-JsonString -Object $instruction -Name "target_service"
    $instructionTargetOperation = Get-JsonString -Object $instruction -Name "target_operation"
    $instructionType = Get-JsonString -Object $instruction -Name "instruction_type"
    $status = Get-JsonString -Object $instruction -Name "status"

    foreach ($entry in @(
            @{ name = "instruction_id"; value = $instructionID },
            @{ name = "workflow_id"; value = $instructionWorkflowID },
            @{ name = "payload_ref_hash"; value = $instructionPayloadRefHash },
            @{ name = "target_service"; value = $instructionTargetService },
            @{ name = "target_operation"; value = $instructionTargetOperation },
            @{ name = "instruction_type"; value = $instructionType },
            @{ name = "environment"; value = Get-JsonString -Object $instruction -Name "environment" -AllowEmpty },
            @{ name = "config_kind"; value = Get-JsonString -Object $instruction -Name "config_kind" -AllowEmpty },
            @{ name = "bundle_key"; value = Get-JsonString -Object $instruction -Name "bundle_key" -AllowEmpty },
            @{ name = "target_version"; value = Get-JsonString -Object $instruction -Name "target_version" -AllowEmpty },
            @{ name = "operator_ref"; value = Get-JsonString -Object $instruction -Name "operator_ref" -AllowEmpty },
            @{ name = "reason_ref"; value = Get-JsonString -Object $instruction -Name "reason_ref" -AllowEmpty },
            @{ name = "status"; value = $status }
        )) {
        Assert-LowSensitiveString -Value ([string]$entry.value) -FieldName "instruction.$($entry.name)" -AllowEmpty:($entry.name -in @("environment", "config_kind", "bundle_key", "target_version", "operator_ref", "reason_ref"))
    }

    if ($instructionWorkflowID -ne $workflowID) {
        throw "Instruction $instructionID workflow_id does not match workflow.workflow_id."
    }
    if ($instructionPayloadRefHash -ne $payloadRefHash) {
        throw "Instruction $instructionID payload_ref_hash does not match workflow.payload_ref_hash."
    }
    if ($targetService.Length -gt 0 -and $instructionTargetService -ne $targetService) {
        throw "Instruction $instructionID target_service does not match workflow.target_service."
    }
    if ($targetOperation.Length -gt 0 -and $instructionTargetOperation -ne $targetOperation) {
        throw "Instruction $instructionID target_operation does not match workflow.target_operation."
    }
    if ($status -ne "ACTIVE") {
        throw "Instruction $instructionID status must be ACTIVE."
    }
}

$reviewChecks = @(Get-JsonArray -Object $bundle -Name "review_checks")
$approvalBoundary = @(Get-JsonArray -Object $bundle -Name "approval_boundary")
$executionBoundary = @(Get-JsonArray -Object $bundle -Name "execution_boundary")
foreach ($value in @($reviewChecks + $approvalBoundary + $executionBoundary)) {
    Assert-LowSensitiveString -Value ([string]$value) -FieldName "review boundary"
}

$artifactRows = [ordered]@{
    bundle_sha256 = Get-CompReviewFileSha256 -Path $BundlePath
    bundle_path_sha256 = Get-CompReviewStringSha256 -Value ([string](Resolve-Path -LiteralPath $BundlePath))
    page_id = $PageID
}

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Workflow Compensation Review</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1180px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{background:#f1f5f9;}table.summary th{width:290px;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}ul{margin:8px 0 8px 22px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Workflow Compensation Review</h1>")
[void]$html.AppendLine("<p class=`"note`">Static first-stage operator review page. It renders low-sensitive refs and hashes from a verified compensation review bundle only. It does not approve, reject, record a decision, execute compensation, call downstream services, or embed raw payloads, reason text, provider bodies, local paths, credentials, or EvidencePack text.</p>")

Add-SectionHeader -Builder $html -Title "Review Summary"
[void]$html.AppendLine("<table class=`"summary`">")
foreach ($entry in ([ordered]@{
        page_id = $PageID
        generated_at = (Get-Date).ToUniversalTime().ToString("o")
        generated_by = $GeneratedBy
        schema_version = $schemaVersion
        workflow_id = $workflowID
        workflow_type = $workflowType
        status = $workflowStatus
        risk_level = Get-JsonString -Object $workflow -Name "risk_level" -AllowEmpty
        target_service = $targetService
        target_operation = $targetOperation
        payload_schema_version = Get-JsonString -Object $workflow -Name "payload_schema_version" -AllowEmpty
        payload_ref_hash = $payloadRefHash
        approval_policy_ref = Get-JsonString -Object $workflow -Name "approval_policy_ref" -AllowEmpty
        compensation_policy_ref = Get-JsonString -Object $workflow -Name "compensation_policy_ref" -AllowEmpty
        instruction_count = $instructionCount
        no_direct_execution = [bool]$bundle.no_direct_execution
        no_decision_recorded = [bool]$bundle.no_decision_recorded
    }).GetEnumerator()) {
    Add-TableRow -Builder $html -Key $entry.Key -Value $entry.Value
}
[void]$html.AppendLine("</table>")

Add-SectionHeader -Builder $html -Title "Workflow Binding"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    requester_ref = Get-JsonString -Object $workflow -Name "requester_ref" -AllowEmpty
    requester_service = Get-JsonString -Object $workflow -Name "requester_service" -AllowEmpty
    target_ref_hash = Get-JsonString -Object $workflow -Name "target_ref_hash" -AllowEmpty
    timeout_policy_ref = Get-JsonString -Object $workflow -Name "timeout_policy_ref" -AllowEmpty
    reason_ref = Get-JsonString -Object $workflow -Name "reason_ref" -AllowEmpty
    current_step_id = Get-JsonString -Object $workflow -Name "current_step_id" -AllowEmpty
    correlation_id = Get-JsonString -Object $workflow -Name "correlation_id" -AllowEmpty
    causation_id = Get-JsonString -Object $workflow -Name "causation_id" -AllowEmpty
    trace_id = Get-JsonString -Object $workflow -Name "trace_id" -AllowEmpty
})

Add-SectionHeader -Builder $html -Title "Instruction References"
[void]$html.AppendLine("<table>")
[void]$html.AppendLine("<tr><th>instruction_id</th><th>type</th><th>status</th><th>target_service</th><th>target_operation</th><th>payload_ref_hash</th><th>environment</th><th>config_kind</th><th>bundle_key</th><th>target_version</th><th>operator_ref</th><th>reason_ref</th></tr>")
foreach ($instruction in $instructions) {
    [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $instruction.instruction_id)</td><td>$(ConvertTo-HtmlText $instruction.instruction_type)</td><td>$(ConvertTo-HtmlText $instruction.status)</td><td>$(ConvertTo-HtmlText $instruction.target_service)</td><td>$(ConvertTo-HtmlText $instruction.target_operation)</td><td>$(ConvertTo-HtmlText $instruction.payload_ref_hash)</td><td>$(ConvertTo-HtmlText $instruction.environment)</td><td>$(ConvertTo-HtmlText $instruction.config_kind)</td><td>$(ConvertTo-HtmlText $instruction.bundle_key)</td><td>$(ConvertTo-HtmlText $instruction.target_version)</td><td>$(ConvertTo-HtmlText $instruction.operator_ref)</td><td>$(ConvertTo-HtmlText $instruction.reason_ref)</td></tr>")
}
[void]$html.AppendLine("</table>")

Add-SectionHeader -Builder $html -Title "Review Checks"
Add-List -Builder $html -Values $reviewChecks -EmptyText "No review checks were recorded."

Add-SectionHeader -Builder $html -Title "Approval Boundary"
Add-List -Builder $html -Values $approvalBoundary -EmptyText "No approval boundary entries were recorded."

Add-SectionHeader -Builder $html -Title "Execution Boundary"
Add-List -Builder $html -Values $executionBoundary -EmptyText "No execution boundary entries were recorded."

Add-SectionHeader -Builder $html -Title "Artifact Hashes"
Add-SimpleTable -Builder $html -Rows $artifactRows

[void]$html.AppendLine("</main></body></html>")
$htmlText = $html.ToString()

$parent = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
}
$htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow compensation review page written: $OutputPath"
