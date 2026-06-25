param(
    [Parameter(Mandatory = $true)]
    [string]$DeliveryStatusRootPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$RedrivePlanRootPath = "",
    [string]$OutputPath = "",
    [string]$DashboardID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $DeliveryStatusRootPath -PathType Container)) {
    throw "Missing workflow external callback delivery status root: $DeliveryStatusRootPath"
}
Assert-ExternalRepairOutputPath -Value $DeliveryStatusRootPath -FieldName "DeliveryStatusRootPath"

if (-not [string]::IsNullOrWhiteSpace($RedrivePlanRootPath)) {
    if (-not (Test-Path -LiteralPath $RedrivePlanRootPath -PathType Container)) {
        throw "Missing workflow external callback redrive plan root: $RedrivePlanRootPath"
    }
    Assert-ExternalRepairOutputPath -Value $RedrivePlanRootPath -FieldName "RedrivePlanRootPath"
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path ([System.IO.Path]::GetFullPath($DeliveryStatusRootPath)) "workflow-external-callback-delivery-dashboard.html"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($DashboardID)) {
    $DashboardID = "workflow-external-callback-delivery-dashboard-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $DashboardID -FieldName "DashboardID"

function Get-CallbackDashboardFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackDashboardStringSha256Ref {
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

function Assert-NoRawCallbackDashboardText {
    param(
        [string]$Value,
        [string]$FieldName
    )

    $match = [regex]::Match($Value, "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|EvidencePack|prompt)")
    if ($match.Success) {
        throw "$FieldName contains raw, secret, provider artifact, prompt, URL, or credential-like content."
    }
}

function Assert-LowValue {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawCallbackDashboardText -Value $Value -FieldName $FieldName
}

function Read-WorkflowBinding {
    param([object]$Binding)

    if ($null -eq $Binding) {
        throw "workflow_binding is required."
    }

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
    Assert-True ($rows.expected_status -eq "WAITING_DECISION") "workflow_binding.expected_status must be WAITING_DECISION."
    return [pscustomobject]$rows
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

$statusFiles = @(Get-ChildItem -LiteralPath $DeliveryStatusRootPath -Filter "*.json" -File | Sort-Object Name)
if ($statusFiles.Count -eq 0) {
    throw "DeliveryStatusRootPath must contain at least one delivery status JSON file."
}

$redriveByStatusHash = @{}
if (-not [string]::IsNullOrWhiteSpace($RedrivePlanRootPath)) {
    foreach ($file in @(Get-ChildItem -LiteralPath $RedrivePlanRootPath -Filter "*.json" -File | Sort-Object Name)) {
        $redrive = Get-JsonDocument -Path $file.FullName -Label "Workflow external callback redrive plan"
        Assert-True ((Get-ObjectString -Object $redrive -Name "schema_version") -eq "nexusim.workflow.external_callback_redrive_plan.v1") "Unsupported redrive plan schema_version."
        Assert-True ([bool]$redrive.no_direct_execution) "Redrive plan must set no_direct_execution=true."
        Assert-True ([bool]$redrive.no_decision_recorded) "Redrive plan must set no_decision_recorded=true."
        Assert-True ([bool]$redrive.does_not_call_provider) "Redrive plan must set does_not_call_provider=true."
        Assert-True ([bool]$redrive.does_not_execute_target) "Redrive plan must set does_not_execute_target=true."

        $sourceStatusHash = Get-ObjectString -Object $redrive -Name "source_delivery_status_sha256"
        Assert-LowValue -Value $sourceStatusHash -FieldName "redrive.source_delivery_status_sha256"
        if ($redriveByStatusHash.ContainsKey($sourceStatusHash)) {
            throw "Duplicate redrive plan for delivery status hash: $sourceStatusHash"
        }

        $redriveByStatusHash[$sourceStatusHash] = [pscustomobject]@{
            plan = $redrive
            file_sha256 = Get-CallbackDashboardFileSha256Ref -Path $file.FullName
            path_sha256 = Get-CallbackDashboardStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $file.FullName))
        }
    }
}

$rows = @()
$counts = [ordered]@{
    DELIVERED = 0
    RETRY_PENDING = 0
    DLQ = 0
}

foreach ($file in $statusFiles) {
    $status = Get-JsonDocument -Path $file.FullName -Label "Workflow external callback delivery status"
    Assert-True ((Get-ObjectString -Object $status -Name "schema_version") -eq "nexusim.workflow.external_callback_delivery_status.v1") "Unsupported delivery status schema_version."
    Assert-True ([bool]$status.no_direct_execution) "Delivery status must set no_direct_execution=true."
    Assert-True ([bool]$status.no_decision_recorded) "Delivery status must set no_decision_recorded=true."
    Assert-True ([bool]$status.does_not_call_provider) "Delivery status must set does_not_call_provider=true."
    Assert-True ([bool]$status.does_not_execute_target) "Delivery status must set does_not_execute_target=true."

    $binding = Read-WorkflowBinding -Binding $status.workflow_binding
    $deliveryStatus = (Get-ObjectString -Object $status -Name "delivery_status").ToUpperInvariant()
    Assert-True (@("DELIVERED", "RETRY_PENDING", "DLQ") -contains $deliveryStatus) "Unsupported delivery_status: $deliveryStatus"

    $attemptNumber = [int]$status.attempt_number
    $maxAttempts = [int]$status.max_attempts
    Assert-True ($attemptNumber -ge 1 -and $attemptNumber -le $maxAttempts) "attempt_number must be within max_attempts."

    $sourcePlanHash = Get-ObjectString -Object $status -Name "source_delivery_plan_sha256"
    $attemptRef = Get-ObjectString -Object $status -Name "delivery_attempt_ref"
    $resultRef = Get-ObjectString -Object $status -Name "delivery_result_ref" -AllowEmpty
    $failureRef = Get-ObjectString -Object $status -Name "failure_class_ref" -AllowEmpty
    $nextRetryRef = Get-ObjectString -Object $status -Name "next_retry_ref" -AllowEmpty
    $redrivePolicyRef = Get-ObjectString -Object $status -Name "redrive_policy_ref"

    foreach ($entry in @(
            @{ name = "source_delivery_plan_sha256"; value = $sourcePlanHash },
            @{ name = "delivery_attempt_ref"; value = $attemptRef },
            @{ name = "delivery_result_ref"; value = $resultRef; allow_empty = $true },
            @{ name = "failure_class_ref"; value = $failureRef; allow_empty = $true },
            @{ name = "next_retry_ref"; value = $nextRetryRef; allow_empty = $true },
            @{ name = "redrive_policy_ref"; value = $redrivePolicyRef }
        )) {
        Assert-LowValue -Value ([string]$entry.value) -FieldName $entry.name -AllowEmpty:([bool]$entry.allow_empty)
    }

    $statusHash = Get-CallbackDashboardFileSha256Ref -Path $file.FullName
    $redriveEntry = $null
    $redrivePlanID = ""
    $redriveQueueRef = ""
    if ($redriveByStatusHash.ContainsKey($statusHash)) {
        $redriveEntry = $redriveByStatusHash[$statusHash]
        $redrive = $redriveEntry.plan
        Assert-True ($deliveryStatus -ne "DELIVERED") "DELIVERED status must not have a redrive plan."
        Assert-True ((Get-ObjectString -Object $redrive -Name "source_delivery_plan_sha256") -eq $sourcePlanHash) "Redrive plan source_delivery_plan_sha256 does not match status."
        $redriveBinding = Read-WorkflowBinding -Binding $redrive.workflow_binding
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
            Assert-True ($binding.$field -eq $redriveBinding.$field) "Redrive plan workflow_binding.$field does not match delivery status."
        }
        $redrivePlanID = Get-ObjectString -Object $redrive -Name "redrive_plan_id"
        $redriveQueueRef = Get-ObjectString -Object $redrive.redrive_contract -Name "redrive_queue_ref"
        Assert-LowValue -Value $redrivePlanID -FieldName "redrive_plan_id"
        Assert-LowValue -Value $redriveQueueRef -FieldName "redrive_queue_ref"
    }

    $counts[$deliveryStatus] = [int]$counts[$deliveryStatus] + 1
    $rows += [pscustomobject]@{
        workflow_id = $binding.workflow_id
        step_id = $binding.step_id
        target_service = $binding.expected_target_service
        target_operation = $binding.expected_target_operation
        payload_ref_hash = $binding.expected_payload_ref_hash
        approval_policy_ref = $binding.expected_approval_policy_ref
        delivery_status = $deliveryStatus
        attempt_number = $attemptNumber
        max_attempts = $maxAttempts
        delivery_attempt_ref = $attemptRef
        failure_class_ref = $failureRef
        next_retry_ref = $nextRetryRef
        redrive_policy_ref = $redrivePolicyRef
        redrive_plan_id = $redrivePlanID
        redrive_queue_ref = $redriveQueueRef
        source_status_sha256 = $statusHash
        source_status_path_sha256 = Get-CallbackDashboardStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $file.FullName))
        source_plan_sha256 = $sourcePlanHash
        redrive_plan_sha256 = if ($null -ne $redriveEntry) { $redriveEntry.file_sha256 } else { "" }
    }
}

$orphanRedrives = @()
foreach ($hash in @($redriveByStatusHash.Keys)) {
    if (-not ($rows | Where-Object { $_.source_status_sha256 -eq $hash })) {
        $orphanRedrives += $hash
    }
}
if ($orphanRedrives.Count -gt 0) {
    throw "RedrivePlanRootPath contains redrive plans without matching delivery status: $($orphanRedrives[0])"
}

$redriveCandidateCount = @($rows | Where-Object { $_.delivery_status -in @("RETRY_PENDING", "DLQ") }).Count
$redrivePlanCount = @($rows | Where-Object { -not [string]::IsNullOrWhiteSpace($_.redrive_plan_id) }).Count

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Workflow External Callback Delivery Dashboard</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1360px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{background:#f1f5f9;}table.summary th{width:300px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Workflow External Callback Delivery Dashboard</h1>")
[void]$html.AppendLine("<p class=`"note`">Static first-stage operator dashboard for external callback delivery triage. It aggregates low-sensitive delivery status and optional redrive plan artifacts. It does not call providers, record workflow decisions, redrive delivery jobs, execute target actions, or expose unsafe callback material, provider material, payload material, model input, evidence text, local path material, or auth material.</p>")

Add-SectionHeader -Builder $html -Title "Dashboard Summary"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    dashboard_id = $DashboardID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    delivery_status_root_sha256 = Get-CallbackDashboardStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $DeliveryStatusRootPath))
    redrive_plan_root_sha256 = if ([string]::IsNullOrWhiteSpace($RedrivePlanRootPath)) { "" } else { Get-CallbackDashboardStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $RedrivePlanRootPath)) }
    delivery_status_count = $rows.Count
    delivered_count = $counts.DELIVERED
    retry_pending_count = $counts.RETRY_PENDING
    dlq_count = $counts.DLQ
    redrive_candidate_count = $redriveCandidateCount
    redrive_plan_count = $redrivePlanCount
    final_decision_owner = "workflow-service.RecordWorkflowDecision"
    dashboard_records_decision = $false
    dashboard_calls_provider = $false
    dashboard_executes_target = $false
})

Add-SectionHeader -Builder $html -Title "Delivery Rows"
[void]$html.AppendLine("<table>")
[void]$html.AppendLine("<tr><th>workflow_id</th><th>step_id</th><th>target</th><th>payload_ref_hash</th><th>status</th><th>attempt</th><th>failure_class_ref</th><th>next_retry_ref</th><th>redrive_policy_ref</th><th>redrive_plan_id</th><th>redrive_queue_ref</th></tr>")
foreach ($row in $rows) {
    $attemptText = "$($row.attempt_number)/$($row.max_attempts)"
    $targetText = "$($row.target_service).$($row.target_operation)"
    [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $row.workflow_id)</td><td>$(ConvertTo-HtmlText $row.step_id)</td><td>$(ConvertTo-HtmlText $targetText)</td><td>$(ConvertTo-HtmlText $row.payload_ref_hash)</td><td>$(ConvertTo-HtmlText $row.delivery_status)</td><td>$(ConvertTo-HtmlText $attemptText)</td><td>$(ConvertTo-HtmlText $row.failure_class_ref)</td><td>$(ConvertTo-HtmlText $row.next_retry_ref)</td><td>$(ConvertTo-HtmlText $row.redrive_policy_ref)</td><td>$(ConvertTo-HtmlText $row.redrive_plan_id)</td><td>$(ConvertTo-HtmlText $row.redrive_queue_ref)</td></tr>")
}
[void]$html.AppendLine("</table>")

Add-SectionHeader -Builder $html -Title "Boundary Contract"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    dashboard_owner = "workflow-service.external-callback-delivery"
    dashboard_records_decision = $false
    dashboard_redrives_delivery = $false
    dashboard_calls_provider = $false
    dashboard_executes_target = $false
    redrive_requires_explicit_redrive_plan = $true
    final_decision_owner = "workflow-service.RecordWorkflowDecision"
    final_delivery_owner = "workflow-service external-callback-delivery worker"
})

[void]$html.AppendLine("</main></body></html>")

$htmlText = $html.ToString()
Assert-NoRawCallbackDashboardText -Value $htmlText -FieldName "external callback delivery dashboard"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback delivery dashboard written: $OutputPath"
