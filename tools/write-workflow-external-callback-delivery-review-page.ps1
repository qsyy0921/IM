param(
    [Parameter(Mandatory = $true)]
    [string]$DeliveryPlanPath,

    [Parameter(Mandatory = $true)]
    [string]$DeliveryStatusPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$RedrivePlanPath = "",
    [string]$OutputPath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($entry in @(
        @{ path = $DeliveryPlanPath; name = "DeliveryPlanPath" },
        @{ path = $DeliveryStatusPath; name = "DeliveryStatusPath" }
    )) {
    if (-not (Test-Path -LiteralPath $entry.path -PathType Leaf)) {
        throw "Missing workflow external callback delivery review page input: $($entry.path)"
    }
    Assert-ExternalRepairOutputPath -Value $entry.path -FieldName $entry.name
}

if (-not [string]::IsNullOrWhiteSpace($RedrivePlanPath)) {
    if (-not (Test-Path -LiteralPath $RedrivePlanPath -PathType Leaf)) {
        throw "Missing workflow external callback redrive plan: $RedrivePlanPath"
    }
    Assert-ExternalRepairOutputPath -Value $RedrivePlanPath -FieldName "RedrivePlanPath"
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($DeliveryStatusPath))) "workflow-external-callback-delivery-review.html"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "workflow-external-callback-delivery-review-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-CallbackReviewFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackReviewStringSha256Ref {
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

function Assert-LowValue {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawCallbackReviewText -Value $Value -FieldName $FieldName
}

function Assert-NoRawCallbackReviewText {
    param(
        [string]$Value,
        [string]$FieldName
    )

    $match = [regex]::Match($Value, "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|EvidencePack|prompt)")
    if ($match.Success) {
        throw "$FieldName contains raw, secret, provider artifact, prompt, URL, or credential-like content."
    }
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

function Get-WorkflowBindingRows {
    param([object]$Binding)

    $rows = [ordered]@{}
    foreach ($field in @(
            "workflow_id",
            "step_id",
            "expected_workflow_type",
            "expected_status",
            "expected_target_service",
            "expected_target_operation",
            "expected_target_ref_hash",
            "expected_payload_schema_version",
            "expected_payload_ref_hash",
            "expected_approval_policy_ref",
            "decision_policy_ref"
        )) {
        $value = Get-ObjectString -Object $Binding -Name $field
        Assert-LowValue -Value $value -FieldName "workflow_binding.$field"
        $rows[$field] = $value
    }
    return $rows
}

$plan = Get-JsonDocument -Path $DeliveryPlanPath -Label "Workflow external callback delivery plan"
$status = Get-JsonDocument -Path $DeliveryStatusPath -Label "Workflow external callback delivery status"

Assert-True ((Get-ObjectString -Object $plan -Name "schema_version") -eq "nexusim.workflow.external_callback_delivery_plan.v1") "Unsupported delivery plan schema_version."
Assert-True ((Get-ObjectString -Object $status -Name "schema_version") -eq "nexusim.workflow.external_callback_delivery_status.v1") "Unsupported delivery status schema_version."

foreach ($doc in @($plan, $status)) {
    Assert-True ([bool]$doc.no_direct_execution) "external callback artifact must set no_direct_execution=true."
    Assert-True ([bool]$doc.no_decision_recorded) "external callback artifact must set no_decision_recorded=true."
    Assert-True ([bool]$doc.does_not_call_provider) "external callback artifact must set does_not_call_provider=true."
    Assert-True ([bool]$doc.does_not_execute_target) "external callback artifact must set does_not_execute_target=true."
}

$planFileSha256 = Get-CallbackReviewFileSha256Ref -Path $DeliveryPlanPath
$planPathSha256 = Get-CallbackReviewStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $DeliveryPlanPath))
$statusFileSha256 = Get-CallbackReviewFileSha256Ref -Path $DeliveryStatusPath
$statusPathSha256 = Get-CallbackReviewStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $DeliveryStatusPath))

Assert-True ((Get-ObjectString -Object $status -Name "source_delivery_plan_sha256") -eq $planFileSha256) "Delivery status does not match the source delivery plan hash."
Assert-True ((Get-ObjectString -Object $status -Name "source_delivery_plan_path_sha256") -eq $planPathSha256) "Delivery status does not match the source delivery plan path hash."

$planBinding = Get-WorkflowBindingRows -Binding $plan.workflow_binding
$statusBinding = Get-WorkflowBindingRows -Binding $status.workflow_binding
foreach ($key in @($planBinding.Keys)) {
    Assert-True ($planBinding[$key] -eq $statusBinding[$key]) "Delivery status workflow_binding.$key does not match delivery plan."
}
Assert-True ($statusBinding.expected_status -eq "WAITING_DECISION") "workflow_binding.expected_status must be WAITING_DECISION."

$deliveryStatus = (Get-ObjectString -Object $status -Name "delivery_status").ToUpperInvariant()
Assert-True (@("DELIVERED", "RETRY_PENDING", "DLQ") -contains $deliveryStatus) "Unsupported delivery_status: $deliveryStatus"

$attemptNumber = [int]$status.attempt_number
$maxAttempts = [int]$status.max_attempts
Assert-True ($attemptNumber -ge 1 -and $attemptNumber -le $maxAttempts) "attempt_number must be within max_attempts."

$redrive = $null
if ($deliveryStatus -eq "DELIVERED") {
    Assert-True ([string]::IsNullOrWhiteSpace($RedrivePlanPath)) "DELIVERED status must not include a redrive plan."
} else {
    Assert-True (-not [string]::IsNullOrWhiteSpace($RedrivePlanPath)) "RETRY_PENDING or DLQ status requires a redrive plan for operator review."
    $redrive = Get-JsonDocument -Path $RedrivePlanPath -Label "Workflow external callback redrive plan"
    Assert-True ((Get-ObjectString -Object $redrive -Name "schema_version") -eq "nexusim.workflow.external_callback_redrive_plan.v1") "Unsupported redrive plan schema_version."
    Assert-True ([bool]$redrive.no_direct_execution) "Redrive plan must set no_direct_execution=true."
    Assert-True ([bool]$redrive.no_decision_recorded) "Redrive plan must set no_decision_recorded=true."
    Assert-True ([bool]$redrive.does_not_call_provider) "Redrive plan must set does_not_call_provider=true."
    Assert-True ([bool]$redrive.does_not_execute_target) "Redrive plan must set does_not_execute_target=true."
    Assert-True ((Get-ObjectString -Object $redrive -Name "source_delivery_status_sha256") -eq $statusFileSha256) "Redrive plan does not match the source delivery status hash."
    Assert-True ((Get-ObjectString -Object $redrive -Name "source_delivery_status_path_sha256") -eq $statusPathSha256) "Redrive plan does not match the source delivery status path hash."
    Assert-True ((Get-ObjectString -Object $redrive -Name "source_delivery_plan_sha256") -eq $planFileSha256) "Redrive plan does not match the source delivery plan hash."

    $redriveBinding = Get-WorkflowBindingRows -Binding $redrive.workflow_binding
    foreach ($key in @($planBinding.Keys)) {
        Assert-True ($planBinding[$key] -eq $redriveBinding[$key]) "Redrive plan workflow_binding.$key does not match delivery plan."
    }
    Assert-True ((Get-ObjectString -Object $redrive.redrive_source -Name "delivery_status").ToUpperInvariant() -eq $deliveryStatus) "Redrive source delivery_status does not match delivery status."
}

$summaryRows = [ordered]@{
    page_id = $PageID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    delivery_status = $deliveryStatus
    workflow_id = $statusBinding.workflow_id
    step_id = $statusBinding.step_id
    expected_target_service = $statusBinding.expected_target_service
    expected_target_operation = $statusBinding.expected_target_operation
    attempt_number = $attemptNumber
    max_attempts = $maxAttempts
    final_decision_owner = "workflow-service.RecordWorkflowDecision"
    final_execution_owner = "approved downstream owner"
    review_records_decision = $false
    review_calls_provider = $false
    review_executes_target = $false
}

$artifactRows = [ordered]@{
    delivery_plan_sha256 = $planFileSha256
    delivery_plan_path_sha256 = $planPathSha256
    delivery_status_sha256 = $statusFileSha256
    delivery_status_path_sha256 = $statusPathSha256
}

$statusRows = [ordered]@{
    delivery_attempt_ref = Get-ObjectString -Object $status -Name "delivery_attempt_ref"
    delivery_result_ref = Get-ObjectString -Object $status -Name "delivery_result_ref" -AllowEmpty
    failure_class_ref = Get-ObjectString -Object $status -Name "failure_class_ref" -AllowEmpty
    next_retry_ref = Get-ObjectString -Object $status -Name "next_retry_ref" -AllowEmpty
    redrive_policy_ref = Get-ObjectString -Object $status -Name "redrive_policy_ref"
}
foreach ($key in @($statusRows.Keys)) {
    Assert-LowValue -Value ([string]$statusRows[$key]) -FieldName "status.$key" -AllowEmpty:([string]::IsNullOrWhiteSpace([string]$statusRows[$key]))
}

$redriveRows = [ordered]@{}
if ($null -ne $redrive) {
    $artifactRows.redrive_plan_sha256 = Get-CallbackReviewFileSha256Ref -Path $RedrivePlanPath
    $artifactRows.redrive_plan_path_sha256 = Get-CallbackReviewStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $RedrivePlanPath))
    $redriveRows.redrive_plan_id = Get-ObjectString -Object $redrive -Name "redrive_plan_id"
    $redriveRows.redrive_queue_ref = Get-ObjectString -Object $redrive.redrive_contract -Name "redrive_queue_ref"
    $redriveRows.redrive_reason_ref = Get-ObjectString -Object $redrive.redrive_contract -Name "redrive_reason_ref"
    $redriveRows.operator_review_ref = Get-ObjectString -Object $redrive.redrive_contract -Name "operator_review_ref" -AllowEmpty
    $redriveRows.requires_new_delivery_attempt_ref = [bool]$redrive.redrive_contract.requires_new_delivery_attempt_ref
    $redriveRows.requires_existing_waiting_workflow = [bool]$redrive.redrive_contract.requires_existing_waiting_workflow
    foreach ($key in @($redriveRows.Keys)) {
        Assert-LowValue -Value ([string]$redriveRows[$key]) -FieldName "redrive.$key" -AllowEmpty:($key -eq "operator_review_ref")
    }
}

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Workflow External Callback Delivery Review</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1180px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{background:#f1f5f9;width:280px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Workflow External Callback Delivery Review</h1>")
[void]$html.AppendLine("<p class=`"note`">Static operator page for external callback delivery status and redrive review. It validates delivery plan/status/redrive bindings and renders low-sensitive refs and hashes only. It does not call a provider, record a workflow decision, redrive delivery, execute a target action, embed local paths, or expose raw payload text, provider body text, auth material, model input text, evidence text, or callback material.</p>")

Add-SectionHeader -Builder $html -Title "Review Summary"
Add-SimpleTable -Builder $html -Rows $summaryRows

Add-SectionHeader -Builder $html -Title "Workflow Binding"
Add-SimpleTable -Builder $html -Rows $statusBinding

Add-SectionHeader -Builder $html -Title "Delivery Status"
Add-SimpleTable -Builder $html -Rows $statusRows

if ($null -ne $redrive) {
    Add-SectionHeader -Builder $html -Title "Redrive Plan"
    Add-SimpleTable -Builder $html -Rows $redriveRows
}

Add-SectionHeader -Builder $html -Title "Artifact Hashes"
Add-SimpleTable -Builder $html -Rows $artifactRows

Add-SectionHeader -Builder $html -Title "Boundary Contract"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    no_direct_execution = $true
    no_decision_recorded = $true
    does_not_call_provider = $true
    does_not_execute_target = $true
    delivered_status_is_not_decision = [bool]$status.status_contract.delivered_status_is_not_decision
    redrive_plan_is_not_provider_call = $true
    record_decision_requires_external_decision_manifest = $true
    final_decision_owner = "workflow-service.RecordWorkflowDecision"
})

[void]$html.AppendLine("</main></body></html>")

$htmlText = $html.ToString()
Assert-NoRawCallbackReviewText -Value $htmlText -FieldName "external callback delivery review page"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback delivery review page written: $OutputPath"
