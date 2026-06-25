param(
    [Parameter(Mandatory = $true)]
    [string]$ResultManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [string]$SummaryPath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $ResultManifestPath -PathType Leaf)) {
    throw "Missing workflow approval queue decision result manifest: $ResultManifestPath"
}

Assert-ExternalRepairOutputPath -Value $ResultManifestPath -FieldName "ResultManifestPath"
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"
Assert-ExternalRepairOutputPath -Value $SummaryPath -FieldName "SummaryPath" -AllowEmpty
Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "workflow-approval-queue-decision-result-review-" + [System.Guid]::NewGuid().ToString("N")
}
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-DecisionResultReviewFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
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

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Assert-NoRawDecisionResultReviewText {
    param(
        [string]$Value,
        [string]$FieldName
    )

    $match = [regex]::Match($Value, "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|EvidencePack|prompt)")
    if ($match.Success) {
        throw "$FieldName contains raw, secret, provider artifact, model input, URL, or credential-like content."
    }
}

function Assert-LowValue {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawDecisionResultReviewText -Value $Value -FieldName $FieldName
}

function Add-SectionHeader {
    param(
        [System.Text.StringBuilder]$Builder,
        [string]$Title
    )

    [void]$Builder.AppendLine("<h2>$(ConvertTo-HtmlText $Title)</h2>")
}

function Add-TableRow {
    param(
        [System.Text.StringBuilder]$Builder,
        [string]$Key,
        [object]$Value
    )

    [void]$Builder.AppendLine("<tr><th>$(ConvertTo-HtmlText $Key)</th><td>$(ConvertTo-HtmlText $Value)</td></tr>")
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

function Read-ResultItem {
    param(
        [object]$Item,
        [int]$Index
    )

    $queueID = Get-JsonString -Object $Item -Name "queue_id"
    $workflowID = Get-JsonString -Object $Item -Name "workflow_id"
    $stepID = Get-JsonString -Object $Item -Name "step_id"
    $decision = (Get-JsonString -Object $Item -Name "decision").ToUpperInvariant()
    $workflowStatus = (Get-JsonString -Object $Item -Name "workflow_status").ToUpperInvariant()
    $decisionID = Get-JsonString -Object $Item -Name "decision_id"
    $decisionType = (Get-JsonString -Object $Item -Name "decision_type").ToUpperInvariant()
    $sourceDecisionManifestSha256 = Get-JsonString -Object $Item -Name "source_decision_manifest_sha256"
    $executionSummarySha256 = Get-JsonString -Object $Item -Name "execution_summary_sha256"
    $executionSummaryPathSha256 = Get-JsonString -Object $Item -Name "execution_summary_path_sha256"
    $replayed = [bool]$Item.replayed

    foreach ($entry in @(
            @{ name = "queue_id"; value = $queueID },
            @{ name = "workflow_id"; value = $workflowID },
            @{ name = "step_id"; value = $stepID },
            @{ name = "decision"; value = $decision },
            @{ name = "workflow_status"; value = $workflowStatus },
            @{ name = "decision_id"; value = $decisionID },
            @{ name = "decision_type"; value = $decisionType },
            @{ name = "source_decision_manifest_sha256"; value = $sourceDecisionManifestSha256 },
            @{ name = "execution_summary_sha256"; value = $executionSummarySha256 },
            @{ name = "execution_summary_path_sha256"; value = $executionSummaryPathSha256 }
        )) {
        Assert-LowValue -Value ([string]$entry.value) -FieldName "items[$Index].$($entry.name)"
    }

    Assert-True (@("APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL") -contains $decision) "items[$Index].decision must be APPROVE, REJECT, REQUEST_CHANGES, or CANCEL."
    Assert-True ($decision -eq $decisionType) "items[$Index].decision must match decision_type."

    return [pscustomobject][ordered]@{
        queue_id = $queueID
        workflow_id = $workflowID
        step_id = $stepID
        decision = $decision
        workflow_status = $workflowStatus
        decision_id = $decisionID
        decision_type = $decisionType
        replayed = $replayed
        source_decision_manifest_sha256 = $sourceDecisionManifestSha256
        execution_summary_sha256 = $executionSummarySha256
        execution_summary_path_sha256 = $executionSummaryPathSha256
    }
}

$resultRawText = Get-Content -LiteralPath $ResultManifestPath -Raw
Assert-NoRawDecisionResultReviewText -Value $resultRawText -FieldName "workflow approval queue decision result manifest"
$result = Get-JsonDocument -Path $ResultManifestPath -Label "Workflow approval queue batch decision result manifest"
Assert-True ((Get-JsonString -Object $result -Name "schema_version") -eq "nexusim.workflow.approval_queue_batch_decision_result.v1") "Unsupported workflow approval queue decision result schema_version."
Assert-True ([bool]$result.called_workflow_service_runtime) "Decision result must prove it called workflow-service runtime."
Assert-True ([bool]$result.records_decision) "Decision result must prove workflow decisions were recorded."
Assert-True (-not [bool]$result.calls_action_executor) "Decision result must not call action-executor."
Assert-True (-not [bool]$result.executes_target) "Decision result must not execute target actions."
Assert-True ([bool]$result.mutates_workflow_fact) "Decision result must prove only workflow facts were mutated."

$rawItems = @($result.items)
Assert-True ($rawItems.Count -gt 0) "Decision result contains no items."
Assert-True ([int]$result.decision_count -eq $rawItems.Count) "decision_count must match item count."

$resultManifestID = Get-JsonString -Object $result -Name "result_manifest_id"
$batchDecisionID = Get-JsonString -Object $result -Name "batch_decision_id"
$tenantID = Get-JsonString -Object $result -Name "tenant_id"
$resultGeneratedBy = Get-JsonString -Object $result -Name "generated_by"
$resultGeneratedAt = Get-JsonString -Object $result -Name "generated_at"
$sourceBatchDecisionSha256 = Get-JsonString -Object $result -Name "source_batch_decision_sha256"
$sourceDecisionManifestRootSha256 = Get-JsonString -Object $result -Name "source_decision_manifest_root_sha256"
$executionSummaryRootSha256 = Get-JsonString -Object $result -Name "execution_summary_root_sha256"
$workflowOperatorPathSha256 = Get-JsonString -Object $result -Name "workflow_operator_path_sha256"
$note = Get-JsonString -Object $result -Name "note" -AllowEmpty

foreach ($entry in @(
        @{ name = "result_manifest_id"; value = $resultManifestID },
        @{ name = "batch_decision_id"; value = $batchDecisionID },
        @{ name = "tenant_id"; value = $tenantID },
        @{ name = "generated_by"; value = $resultGeneratedBy },
        @{ name = "source_batch_decision_sha256"; value = $sourceBatchDecisionSha256 },
        @{ name = "source_decision_manifest_root_sha256"; value = $sourceDecisionManifestRootSha256 },
        @{ name = "execution_summary_root_sha256"; value = $executionSummaryRootSha256 },
        @{ name = "workflow_operator_path_sha256"; value = $workflowOperatorPathSha256 }
    )) {
    Assert-LowValue -Value ([string]$entry.value) -FieldName $entry.name
}
Assert-NoRawDecisionResultReviewText -Value $resultGeneratedAt -FieldName "generated_at"
Assert-NoRawDecisionResultReviewText -Value $note -FieldName "note"

$items = @()
for ($i = 0; $i -lt $rawItems.Count; $i++) {
    $items += Read-ResultItem -Item $rawItems[$i] -Index $i
}

$resultManifestSha256 = Get-DecisionResultReviewFileSha256Ref -Path $ResultManifestPath

$summary = [ordered]@{
    schema_version = "nexusim.workflow.approval_queue_decision_result_review.v1"
    page_id = $PageID.Trim()
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy.Trim()
    source_result_manifest_sha256 = $resultManifestSha256
    result_manifest_id = $resultManifestID
    batch_decision_id = $batchDecisionID
    tenant_id = $tenantID
    decision_count = $items.Count
    decision_owner = "workflow-service.RecordWorkflowDecision"
    source_records_decision = $true
    source_called_workflow_service_runtime = $true
    source_calls_action_executor = $false
    source_executes_target = $false
    source_mutates_workflow_fact = $true
    review_page_calls_workflow_service = $false
    review_page_records_decision = $false
    review_page_calls_action_executor = $false
    review_page_executes_target = $false
    review_page_mutates_workflow_fact = $false
    items = $items
}

$summaryEncoded = $summary | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawDecisionResultReviewText -Value $summaryEncoded -FieldName "workflow approval queue decision result review summary"

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=""en"">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=""utf-8"">")
[void]$html.AppendLine("<meta name=""viewport"" content=""width=device-width, initial-scale=1"">")
[void]$html.AppendLine("<title>Workflow Approval Queue Decision Result Review</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:32px;color:#17202a;background:#f7f9fb;}main{max-width:1180px;margin:0 auto;}h1{font-size:24px;margin-bottom:4px;}h2{font-size:18px;margin-top:28px;}p{line-height:1.55;}table{border-collapse:collapse;width:100%;background:#fff;border:1px solid #d8e0e8;margin:12px 0 24px;}th,td{border:1px solid #d8e0e8;text-align:left;padding:8px 10px;vertical-align:top;font-size:13px;}th{background:#eef4f8;width:240px;}thead th{width:auto;}code{font-family:Consolas,monospace;font-size:12px;word-break:break-all;}.ok{color:#0b6b43;font-weight:600;}.stop{color:#9a3412;font-weight:600;}.note{background:#fff7ed;border:1px solid #fed7aa;padding:12px 14px;border-radius:6px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head>")
[void]$html.AppendLine("<body><main>")
[void]$html.AppendLine("<h1>Workflow Approval Queue Decision Result Review</h1>")
[void]$html.AppendLine("<p>This page is a low-sensitive, read-only operator review artifact. It does not call workflow-service, record decisions, call action-executor, run compensation, redrive provider work, or execute target actions.</p>")

Add-SectionHeader -Builder $html -Title "Summary"
Add-SimpleTable -Builder $html -Rows @{
    "page_id" = $PageID
    "page_generated_at" = $summary.generated_at
    "page_generated_by" = $GeneratedBy
    "source_result_manifest_sha256" = $resultManifestSha256
    "result_manifest_id" = $resultManifestID
    "batch_decision_id" = $batchDecisionID
    "tenant_id" = $tenantID
    "decision_count" = $items.Count
    "source_generated_by" = $resultGeneratedBy
    "source_generated_at" = $resultGeneratedAt
    "source_batch_decision_sha256" = $sourceBatchDecisionSha256
    "source_decision_manifest_root_sha256" = $sourceDecisionManifestRootSha256
    "execution_summary_root_sha256" = $executionSummaryRootSha256
    "workflow_operator_path_sha256" = $workflowOperatorPathSha256
}

Add-SectionHeader -Builder $html -Title "Boundary Checks"
Add-SimpleTable -Builder $html -Rows @{
    "decision_owner" = "workflow-service.RecordWorkflowDecision"
    "source_called_workflow_service_runtime" = "true"
    "source_records_decision" = "true"
    "source_calls_action_executor" = "false"
    "source_executes_target" = "false"
    "source_mutates_workflow_fact" = "true"
    "review_page_calls_workflow_service" = "false"
    "review_page_records_decision" = "false"
    "review_page_calls_action_executor" = "false"
    "review_page_executes_target" = "false"
}

Add-SectionHeader -Builder $html -Title "Decision Items"
[void]$html.AppendLine("<table><thead><tr><th>queue_id</th><th>workflow_id</th><th>step_id</th><th>decision</th><th>workflow_status</th><th>decision_id</th><th>replayed</th><th>source_decision_manifest_sha256</th><th>execution_summary_sha256</th></tr></thead><tbody>")
foreach ($item in $items) {
    [void]$html.AppendLine("<tr><td><code>$(ConvertTo-HtmlText $item.queue_id)</code></td><td><code>$(ConvertTo-HtmlText $item.workflow_id)</code></td><td><code>$(ConvertTo-HtmlText $item.step_id)</code></td><td class=""ok"">$(ConvertTo-HtmlText $item.decision)</td><td>$(ConvertTo-HtmlText $item.workflow_status)</td><td><code>$(ConvertTo-HtmlText $item.decision_id)</code></td><td>$(ConvertTo-HtmlText $item.replayed)</td><td><code>$(ConvertTo-HtmlText $item.source_decision_manifest_sha256)</code></td><td><code>$(ConvertTo-HtmlText $item.execution_summary_sha256)</code></td></tr>")
}
[void]$html.AppendLine("</tbody></table>")

if (-not [string]::IsNullOrWhiteSpace($note)) {
    [void]$html.AppendLine("<p class=""note"">$(ConvertTo-HtmlText $note)</p>")
}

[void]$html.AppendLine("</main></body></html>")

$htmlText = $html.ToString()
Assert-NoRawDecisionResultReviewText -Value $htmlText -FieldName "workflow approval queue decision result review page"

$outputDirectory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
Set-Content -LiteralPath $OutputPath -Value $htmlText -Encoding UTF8

if (-not [string]::IsNullOrWhiteSpace($SummaryPath)) {
    $summaryDirectory = Split-Path -Parent ([System.IO.Path]::GetFullPath($SummaryPath))
    New-Item -ItemType Directory -Force -Path $summaryDirectory | Out-Null
    $summary | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $SummaryPath -Encoding UTF8
}

$summaryEncoded
