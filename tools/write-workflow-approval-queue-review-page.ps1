param(
    [Parameter(Mandatory = $true)]
    [string]$QueueSummaryPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $QueueSummaryPath -PathType Leaf)) {
    throw "Missing workflow approval queue summary: $QueueSummaryPath"
}
Assert-ExternalRepairOutputPath -Value $QueueSummaryPath -FieldName "QueueSummaryPath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($QueueSummaryPath))) "workflow-approval-queue-review.html"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "workflow-approval-queue-review-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-QueueReviewFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-QueueReviewStringSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
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

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Assert-NoRawQueueReviewText {
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
    Assert-NoRawQueueReviewText -Value $Value -FieldName $FieldName
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

function Read-LowWorkflow {
    param(
        [object]$Workflow,
        [object]$Queue,
        [int]$Index
    )

    $workflowID = Get-ObjectString -Object $Workflow -Name "workflow_id"
    $workflowType = Get-ObjectString -Object $Workflow -Name "workflow_type"
    $status = (Get-ObjectString -Object $Workflow -Name "status").ToUpperInvariant()
    $riskLevel = Get-ObjectString -Object $Workflow -Name "risk_level" -AllowEmpty
    $targetService = Get-ObjectString -Object $Workflow -Name "target_service" -AllowEmpty
    $targetOperation = Get-ObjectString -Object $Workflow -Name "target_operation" -AllowEmpty
    $targetRefHash = Get-ObjectString -Object $Workflow -Name "target_ref_hash" -AllowEmpty
    $payloadSchemaVersion = Get-ObjectString -Object $Workflow -Name "payload_schema_version" -AllowEmpty
    $payloadRefHash = Get-ObjectString -Object $Workflow -Name "payload_ref_hash" -AllowEmpty
    $approvalPolicyRef = Get-ObjectString -Object $Workflow -Name "approval_policy_ref" -AllowEmpty
    $currentStepID = Get-ObjectString -Object $Workflow -Name "current_step_id" -AllowEmpty
    $reasonRef = Get-ObjectString -Object $Workflow -Name "reason_ref" -AllowEmpty

    foreach ($entry in @(
            @{ name = "workflow_id"; value = $workflowID; allow = $false },
            @{ name = "workflow_type"; value = $workflowType; allow = $false },
            @{ name = "status"; value = $status; allow = $false },
            @{ name = "risk_level"; value = $riskLevel; allow = $true },
            @{ name = "target_service"; value = $targetService; allow = $true },
            @{ name = "target_operation"; value = $targetOperation; allow = $true },
            @{ name = "target_ref_hash"; value = $targetRefHash; allow = $true },
            @{ name = "payload_schema_version"; value = $payloadSchemaVersion; allow = $true },
            @{ name = "payload_ref_hash"; value = $payloadRefHash; allow = $true },
            @{ name = "approval_policy_ref"; value = $approvalPolicyRef; allow = $true },
            @{ name = "current_step_id"; value = $currentStepID; allow = $true },
            @{ name = "reason_ref"; value = $reasonRef; allow = $true }
        )) {
        Assert-LowValue -Value ([string]$entry.value) -FieldName "workflow[$Index].$($entry.name)" -AllowEmpty:([bool]$entry.allow)
    }

    Assert-True ($status -eq "WAITING_DECISION") "workflow[$Index] status must be WAITING_DECISION for approval queue review."
    if ($null -ne $Queue) {
        $queueType = Get-ObjectString -Object $Queue -Name "workflow_type"
        Assert-True ($workflowType -eq $queueType) "workflow[$Index] workflow_type does not match queue workflow_type."
        $queueTargetService = Get-ObjectString -Object $Queue -Name "target_service" -AllowEmpty
        $queueTargetOperation = Get-ObjectString -Object $Queue -Name "target_operation" -AllowEmpty
        $queueApprovalPolicyRef = Get-ObjectString -Object $Queue -Name "approval_policy_ref" -AllowEmpty
        if (-not [string]::IsNullOrWhiteSpace($queueTargetService)) {
            Assert-True ($targetService -eq $queueTargetService) "workflow[$Index] target_service does not match queue target_service."
        }
        if (-not [string]::IsNullOrWhiteSpace($queueTargetOperation)) {
            Assert-True ($targetOperation -eq $queueTargetOperation) "workflow[$Index] target_operation does not match queue target_operation."
        }
        if (-not [string]::IsNullOrWhiteSpace($queueApprovalPolicyRef)) {
            Assert-True ($approvalPolicyRef -eq $queueApprovalPolicyRef) "workflow[$Index] approval_policy_ref does not match queue approval_policy_ref."
        }
    }

    return [pscustomobject][ordered]@{
        workflow_id = $workflowID
        workflow_type = $workflowType
        status = $status
        risk_level = $riskLevel
        target_service = $targetService
        target_operation = $targetOperation
        target_ref_hash = $targetRefHash
        payload_schema_version = $payloadSchemaVersion
        payload_ref_hash = $payloadRefHash
        approval_policy_ref = $approvalPolicyRef
        current_step_id = $currentStepID
        reason_ref = $reasonRef
    }
}

$summary = Get-JsonDocument -Path $QueueSummaryPath -Label "Workflow approval queue summary"
$mode = Get-ObjectString -Object $summary -Name "mode"
Assert-True (@("operator-queues", "provider-replay-queue") -contains $mode) "Workflow approval queue review only accepts operator-queues or provider-replay-queue summaries."
if ($summary.PSObject.Properties["decision"] -or $summary.PSObject.Properties["decisions"]) {
    throw "Workflow approval queue review must not include recorded decision material."
}

$tenantID = Get-ObjectString -Object $summary -Name "tenant_id"
Assert-LowValue -Value $tenantID -FieldName "tenant_id"

$queues = @()
if ($mode -eq "operator-queues") {
    $queues = @($summary.operator_queues)
    Assert-True ($queues.Count -gt 0) "operator-queues summary must include operator_queues."
} else {
    $queue = [pscustomobject][ordered]@{
        queue_id = "provider-replay"
        workflow_type = Get-ObjectString -Object $summary -Name "workflow_type"
        status = Get-ObjectString -Object $summary -Name "status"
        target_service = Get-ObjectString -Object $summary -Name "target_service"
        target_operation = Get-ObjectString -Object $summary -Name "target_operation"
        approval_policy_ref = Get-ObjectString -Object $summary -Name "approval_policy_ref"
        workflow_count = @($summary.workflows).Count
        workflows = @($summary.workflows)
    }
    Assert-True ($queue.workflow_type -eq "REPAIR_APPROVAL") "provider-replay-queue workflow_type must be REPAIR_APPROVAL."
    Assert-True ($queue.target_service -eq "action-executor") "provider-replay-queue target_service must be action-executor."
    Assert-True ($queue.target_operation -eq "PROVIDER_REPLAY_REQUEST") "provider-replay-queue target_operation must be PROVIDER_REPLAY_REQUEST."
    Assert-True ($queue.approval_policy_ref -eq "admin.workflow.provider_replay.v1") "provider-replay-queue approval_policy_ref must be admin.workflow.provider_replay.v1."
    $queues = @($queue)
}

$queueRows = @()
$workflowRows = @()
$workflowIndex = 0
foreach ($queue in $queues) {
    $queueID = Get-ObjectString -Object $queue -Name "queue_id"
    $workflowType = Get-ObjectString -Object $queue -Name "workflow_type"
    $status = (Get-ObjectString -Object $queue -Name "status").ToUpperInvariant()
    $targetService = Get-ObjectString -Object $queue -Name "target_service" -AllowEmpty
    $targetOperation = Get-ObjectString -Object $queue -Name "target_operation" -AllowEmpty
    $approvalPolicyRef = Get-ObjectString -Object $queue -Name "approval_policy_ref" -AllowEmpty
    $workflows = @($queue.workflows)
    $workflowCount = [int]$queue.workflow_count

    foreach ($entry in @(
            @{ name = "queue_id"; value = $queueID; allow = $false },
            @{ name = "workflow_type"; value = $workflowType; allow = $false },
            @{ name = "status"; value = $status; allow = $false },
            @{ name = "target_service"; value = $targetService; allow = $true },
            @{ name = "target_operation"; value = $targetOperation; allow = $true },
            @{ name = "approval_policy_ref"; value = $approvalPolicyRef; allow = $true }
        )) {
        Assert-LowValue -Value ([string]$entry.value) -FieldName "queue.$($entry.name)" -AllowEmpty:([bool]$entry.allow)
    }
    Assert-True ($status -eq "WAITING_DECISION") "queue $queueID status must be WAITING_DECISION."
    Assert-True ($workflowCount -eq $workflows.Count) "queue $queueID workflow_count does not match workflows length."

    $queueRows += [ordered]@{
        queue_id = $queueID
        workflow_type = $workflowType
        status = $status
        target_service = $targetService
        target_operation = $targetOperation
        approval_policy_ref = $approvalPolicyRef
        workflow_count = $workflowCount
    }

    foreach ($workflow in $workflows) {
        $row = Read-LowWorkflow -Workflow $workflow -Queue $queue -Index $workflowIndex
        $row | Add-Member -NotePropertyName "queue_id" -NotePropertyValue $queueID -Force
        $workflowRows += $row
        $workflowIndex++
    }
}

$summaryFileSha256 = Get-QueueReviewFileSha256Ref -Path $QueueSummaryPath
$summaryPathSha256 = Get-QueueReviewStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $QueueSummaryPath))
$totalWorkflows = $workflowRows.Count

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Workflow Approval Queue Review</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1240px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{background:#f1f5f9;}table.summary th{width:280px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Workflow Approval Queue Review</h1>")
[void]$html.AppendLine("<p class=`"note`">Static first-stage approval queue review page. It renders low-sensitive workflow refs from workflow-service ListWorkflows output. It does not create approvals, record workflow decisions, call action-executor, run compensation, redrive provider work, or execute target actions. Final decisions still require workflow-service RecordWorkflowDecision with binding checks.</p>")

Add-SectionHeader -Builder $html -Title "Review Summary"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    page_id = $PageID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    mode = $mode
    tenant_id = $tenantID
    queue_count = $queueRows.Count
    workflow_count = $totalWorkflows
    source_summary_sha256 = $summaryFileSha256
    source_summary_path_sha256 = $summaryPathSha256
    decision_owner = "workflow-service.RecordWorkflowDecision"
    review_records_decision = $false
    review_executes_target = $false
})

Add-SectionHeader -Builder $html -Title "Queues"
[void]$html.AppendLine("<table>")
[void]$html.AppendLine("<tr><th>queue_id</th><th>workflow_type</th><th>status</th><th>target_service</th><th>target_operation</th><th>approval_policy_ref</th><th>workflow_count</th></tr>")
foreach ($queue in $queueRows) {
    [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $queue.queue_id)</td><td>$(ConvertTo-HtmlText $queue.workflow_type)</td><td>$(ConvertTo-HtmlText $queue.status)</td><td>$(ConvertTo-HtmlText $queue.target_service)</td><td>$(ConvertTo-HtmlText $queue.target_operation)</td><td>$(ConvertTo-HtmlText $queue.approval_policy_ref)</td><td>$(ConvertTo-HtmlText $queue.workflow_count)</td></tr>")
}
[void]$html.AppendLine("</table>")

Add-SectionHeader -Builder $html -Title "Workflows Awaiting Decision"
if ($workflowRows.Count -eq 0) {
    [void]$html.AppendLine("<p>No workflows are currently waiting in the rendered queues.</p>")
} else {
    [void]$html.AppendLine("<table>")
    [void]$html.AppendLine("<tr><th>queue_id</th><th>workflow_id</th><th>workflow_type</th><th>risk_level</th><th>target_service</th><th>target_operation</th><th>target_ref_hash</th><th>payload_schema_version</th><th>payload_ref_hash</th><th>approval_policy_ref</th><th>current_step_id</th><th>reason_ref</th></tr>")
    foreach ($workflow in $workflowRows) {
        [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $workflow.queue_id)</td><td>$(ConvertTo-HtmlText $workflow.workflow_id)</td><td>$(ConvertTo-HtmlText $workflow.workflow_type)</td><td>$(ConvertTo-HtmlText $workflow.risk_level)</td><td>$(ConvertTo-HtmlText $workflow.target_service)</td><td>$(ConvertTo-HtmlText $workflow.target_operation)</td><td>$(ConvertTo-HtmlText $workflow.target_ref_hash)</td><td>$(ConvertTo-HtmlText $workflow.payload_schema_version)</td><td>$(ConvertTo-HtmlText $workflow.payload_ref_hash)</td><td>$(ConvertTo-HtmlText $workflow.approval_policy_ref)</td><td>$(ConvertTo-HtmlText $workflow.current_step_id)</td><td>$(ConvertTo-HtmlText $workflow.reason_ref)</td></tr>")
    }
    [void]$html.AppendLine("</table>")
}

Add-SectionHeader -Builder $html -Title "Boundary Contract"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    queue_view_owner = "workflow-service.ListWorkflows"
    decision_entrypoint = "workflow-service.RecordWorkflowDecision"
    page_records_decision = $false
    page_creates_approval = $false
    page_calls_action_executor = $false
    page_calls_compensation_executor = $false
    page_redrives_provider_work = $false
    raw_payload_allowed = $false
})

[void]$html.AppendLine("</main></body></html>")

$htmlText = $html.ToString()
Assert-NoRawQueueReviewText -Value $htmlText -FieldName "workflow approval queue review page"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow approval queue review page written: $OutputPath"
