param(
    [Parameter(Mandatory = $true)]
    [string]$BatchInvocationPath,

    [Parameter(Mandatory = $true)]
    [string]$ExecutionSummaryRootPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$ResultManifestID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($pathPair in @(
        @("BatchInvocationPath", $BatchInvocationPath),
        @("ExecutionSummaryRootPath", $ExecutionSummaryRootPath)
    )) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    if ($name -eq "ExecutionSummaryRootPath") {
        if (-not (Test-Path -LiteralPath $path -PathType Container)) {
            throw "Missing $name`: $path"
        }
    } elseif (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing $name`: $path"
    }
    Assert-ExternalRepairOutputPath -Value $path -FieldName $name
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path ([System.IO.Path]::GetFullPath($ExecutionSummaryRootPath)) "workflow-external-callback-batch-redrive-result-manifest.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($ResultManifestID)) {
    $ResultManifestID = "workflow-external-callback-batch-redrive-result-" + [System.Guid]::NewGuid().ToString("N")
}
Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $ResultManifestID -FieldName "ResultManifestID"

function Get-BatchResultFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-BatchResultStringSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
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

function Assert-NoRawBatchResultText {
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
    Assert-NoRawBatchResultText -Value $Value -FieldName $FieldName
}

function Assert-Same {
    param(
        [string]$Actual,
        [string]$Expected,
        [string]$FieldName
    )
    if ($Actual -ne $Expected) {
        throw "$FieldName mismatch."
    }
}

function Read-InvocationRedrive {
    param([object]$Redrive)

    foreach ($field in @(
            "redrive_plan_id",
            "workflow_id",
            "step_id",
            "delivery_status",
            "source_delivery_status_sha256",
            "source_delivery_plan_sha256",
            "redrive_plan_sha256",
            "redrive_queue_ref",
            "redrive_reason_ref",
            "target_service",
            "target_operation",
            "target_ref_hash",
            "payload_schema_version",
            "payload_ref_hash",
            "approval_policy_ref",
            "decision_policy_ref"
        )) {
        Assert-LowValue -Value (Get-JsonString -Object $Redrive -Name $field) -FieldName "redrives.$field"
    }
    return [pscustomobject]@{
        redrive_plan_id = Get-JsonString -Object $Redrive -Name "redrive_plan_id"
        workflow_id = Get-JsonString -Object $Redrive -Name "workflow_id"
        step_id = Get-JsonString -Object $Redrive -Name "step_id"
        source_delivery_status_sha256 = Get-JsonString -Object $Redrive -Name "source_delivery_status_sha256"
        source_delivery_plan_sha256 = Get-JsonString -Object $Redrive -Name "source_delivery_plan_sha256"
        redrive_plan_sha256 = Get-JsonString -Object $Redrive -Name "redrive_plan_sha256"
        target_service = Get-JsonString -Object $Redrive -Name "target_service"
        target_operation = Get-JsonString -Object $Redrive -Name "target_operation"
        target_ref_hash = Get-JsonString -Object $Redrive -Name "target_ref_hash"
        payload_schema_version = Get-JsonString -Object $Redrive -Name "payload_schema_version"
        payload_ref_hash = Get-JsonString -Object $Redrive -Name "payload_ref_hash"
        approval_policy_ref = Get-JsonString -Object $Redrive -Name "approval_policy_ref"
        decision_policy_ref = Get-JsonString -Object $Redrive -Name "decision_policy_ref"
        redrive_queue_ref = Get-JsonString -Object $Redrive -Name "redrive_queue_ref"
        redrive_reason_ref = Get-JsonString -Object $Redrive -Name "redrive_reason_ref"
    }
}

$invocationRaw = Get-Content -LiteralPath $BatchInvocationPath -Raw
Assert-NoRawBatchResultText -Value $invocationRaw -FieldName "BatchInvocationPath"
$invocation = Get-JsonDocument -Path $BatchInvocationPath -Label "Workflow external callback batch redrive invocation manifest"
Assert-True ((Get-JsonString -Object $invocation -Name "schema_version") -eq "nexusim.workflow.external_callback_batch_redrive_invocation.v1") "Unsupported batch invocation schema_version."

$runtime = $invocation.runtime_contract
Assert-True ($null -ne $runtime) "runtime_contract is required."
Assert-True ((Get-JsonString -Object $runtime -Name "service") -eq "workflow-service") "runtime_contract.service must be workflow-service."
Assert-True ((Get-JsonString -Object $runtime -Name "mode") -eq "external-callback-delivery-redrive") "runtime_contract.mode must be external-callback-delivery-redrive."
Assert-True (-not [bool]$runtime.batch_invocation_calls_service) "batch invocation must not call service."
Assert-True (-not [bool]$runtime.batch_invocation_records_decision) "batch invocation must not record decision."
Assert-True (-not [bool]$runtime.batch_invocation_calls_provider) "batch invocation must not call provider."
Assert-True (-not [bool]$runtime.batch_invocation_executes_target) "batch invocation must not execute target."
Assert-True ([bool]$runtime.requires_one_runtime_call_per_redrive_plan) "batch invocation must require one runtime call per redrive plan."

$invocationID = Get-JsonString -Object $invocation -Name "invocation_id"
Assert-LowValue -Value $invocationID -FieldName "invocation_id"
$redrives = @($invocation.redrives)
Assert-True ($redrives.Count -gt 0) "batch invocation must contain redrives."

$expectedByPlanID = @{}
foreach ($redrive in $redrives) {
    $expected = Read-InvocationRedrive -Redrive $redrive
    if ($expectedByPlanID.ContainsKey($expected.redrive_plan_id)) {
        throw "Duplicate redrive_plan_id in batch invocation: $($expected.redrive_plan_id)"
    }
    $expectedByPlanID[$expected.redrive_plan_id] = $expected
}

$summaryFiles = @(Get-ChildItem -LiteralPath $ExecutionSummaryRootPath -Filter "*.json" -File | Sort-Object Name)
Assert-True ($summaryFiles.Count -gt 0) "ExecutionSummaryRootPath must contain at least one redrive execution summary JSON file."

$seenSummaryPlans = @{}
$results = @()
foreach ($file in $summaryFiles) {
    $summaryRaw = Get-Content -LiteralPath $file.FullName -Raw
    Assert-NoRawBatchResultText -Value $summaryRaw -FieldName "redrive execution summary"
    $summary = Get-JsonDocument -Path $file.FullName -Label "Workflow external callback redrive execution summary"
    Assert-True ((Get-JsonString -Object $summary -Name "schema_version") -eq "nexusim.workflow.external_callback_redrive_execution_summary.v1") "Unsupported redrive execution summary schema_version."
    Assert-True ((Get-JsonString -Object $summary -Name "mode") -eq "external-callback-delivery-redrive") "Redrive execution summary mode must be external-callback-delivery-redrive."
    Assert-True ([bool]$summary.executed_redrive) "Redrive execution summary must set executed_redrive=true."
    Assert-True (-not [bool]$summary.records_decision) "Redrive execution summary must not record decision."
    Assert-True (-not [bool]$summary.calls_provider) "Redrive execution summary must not call provider."
    Assert-True (-not [bool]$summary.executes_target) "Redrive execution summary must not execute target."
    Assert-True ([bool]$summary.mutates_delivery_fact) "Redrive execution summary must prove workflow-service mutated its delivery fact."
    Assert-True ((Get-JsonString -Object $summary -Name "delivery_status") -eq "PENDING") "Redrive execution summary delivery_status must be PENDING."
    Assert-True ((Get-JsonString -Object $summary -Name "outbox_event_type") -eq "workflow.external_callback.redriven.v1") "Redrive execution summary outbox_event_type mismatch."

    $planID = Get-JsonString -Object $summary -Name "redrive_plan_id"
    Assert-LowValue -Value $planID -FieldName "summary.redrive_plan_id"
    if (-not $expectedByPlanID.ContainsKey($planID)) {
        throw "Execution summary has no matching invocation redrive plan: $planID"
    }
    if ($seenSummaryPlans.ContainsKey($planID)) {
        throw "Duplicate redrive execution summary for plan: $planID"
    }
    $seenSummaryPlans[$planID] = $true
    $expected = $expectedByPlanID[$planID]

    foreach ($field in @(
            "workflow_id",
            "step_id",
            "target_service",
            "target_operation",
            "target_ref_hash",
            "payload_schema_version",
            "payload_ref_hash",
            "approval_policy_ref",
            "decision_policy_ref",
            "redrive_plan_sha256",
            "source_delivery_status_sha256",
            "source_delivery_plan_sha256"
        )) {
        Assert-Same -Actual (Get-JsonString -Object $summary -Name $field) -Expected $expected.$field -FieldName "summary.$field"
    }
    Assert-Same -Actual (Get-JsonString -Object $summary -Name "last_redrive_plan_sha256") -Expected $expected.redrive_plan_sha256 -FieldName "summary.last_redrive_plan_sha256"
    Assert-Same -Actual (Get-JsonString -Object $summary -Name "last_redrive_reason_ref") -Expected $expected.redrive_reason_ref -FieldName "summary.last_redrive_reason_ref"

    $deliveryID = Get-JsonString -Object $summary -Name "delivery_id"
    $tenantID = Get-JsonString -Object $summary -Name "tenant_id"
    Assert-LowValue -Value $deliveryID -FieldName "summary.delivery_id"
    Assert-LowValue -Value $tenantID -FieldName "summary.tenant_id"
    $redriveCount = [int]$summary.redrive_count
    Assert-True ($redriveCount -gt 0) "summary.redrive_count must be positive."

    $results += [ordered]@{
        redrive_plan_id = $planID
        workflow_id = Get-JsonString -Object $summary -Name "workflow_id"
        step_id = Get-JsonString -Object $summary -Name "step_id"
        delivery_id = $deliveryID
        tenant_id = $tenantID
        delivery_status = Get-JsonString -Object $summary -Name "delivery_status"
        redrive_count = $redriveCount
        redrive_plan_sha256 = Get-JsonString -Object $summary -Name "redrive_plan_sha256"
        source_delivery_status_sha256 = Get-JsonString -Object $summary -Name "source_delivery_status_sha256"
        source_delivery_plan_sha256 = Get-JsonString -Object $summary -Name "source_delivery_plan_sha256"
        execution_summary_sha256 = Get-BatchResultFileSha256Ref -Path $file.FullName
        execution_summary_path_sha256 = Get-BatchResultStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $file.FullName))
        target_service = Get-JsonString -Object $summary -Name "target_service"
        target_operation = Get-JsonString -Object $summary -Name "target_operation"
        payload_ref_hash = Get-JsonString -Object $summary -Name "payload_ref_hash"
        approval_policy_ref = Get-JsonString -Object $summary -Name "approval_policy_ref"
        last_redrive_reason_ref = Get-JsonString -Object $summary -Name "last_redrive_reason_ref"
        outbox_event_type = Get-JsonString -Object $summary -Name "outbox_event_type"
    }
}

foreach ($planID in @($expectedByPlanID.Keys)) {
    if (-not $seenSummaryPlans.ContainsKey($planID)) {
        throw "Missing redrive execution summary for plan: $planID"
    }
}

$manifest = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_batch_redrive_result.v1"
    result_manifest_id = $ResultManifestID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    source_batch_invocation_sha256 = Get-BatchResultFileSha256Ref -Path $BatchInvocationPath
    source_batch_invocation_path_sha256 = Get-BatchResultStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $BatchInvocationPath))
    source_execution_summary_root_sha256 = Get-BatchResultStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $ExecutionSummaryRootPath))
    batch_invocation_id = $invocationID
    expected_redrive_count = $redrives.Count
    execution_summary_count = $summaryFiles.Count
    result_count = $results.Count
    manifest_is_execution = $false
    records_decision = $false
    calls_provider = $false
    executes_target = $false
    mutates_delivery_fact = $false
    runtime_contract = [ordered]@{
        service = "workflow-service"
        mode = "external-callback-delivery-redrive"
        plan_env = "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE"
        summary_env = "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_SUMMARY_FILE"
        result_manifest_calls_service = $false
        result_manifest_records_decision = $false
        result_manifest_calls_provider = $false
        result_manifest_executes_target = $false
    }
    results = $results
    required_checks = @(
        "source_batch_invocation_manifest_verified",
        "one_execution_summary_per_redrive_plan",
        "execution_summary_matches_invocation_binding",
        "workflow_service_runtime_reported_executed_redrive",
        "delivery_fact_returned_to_pending",
        "redriven_outbox_event_declared",
        "result_manifest_contains_only_low_sensitive_refs"
    )
    execution_boundary = @(
        "result_manifest_is_not_redrive_execution",
        "does_not_call_workflow_service",
        "does_not_record_workflow_decision",
        "does_not_call_provider",
        "does_not_execute_target_action",
        "does_not_modify_delivery_rows"
    )
    forbidden_contents = @(
        "raw_callback_url",
        "callback_request_material",
        "provider_response_material",
        "decision_material",
        "payload_material",
        "evidence_text_material",
        "local_path_material",
        "auth_material"
    )
    note = "Low-sensitive batch redrive result manifest. It binds workflow-service redrive runtime summaries to a prior batch invocation; it does not execute redrive, record decisions, call providers, execute targets, or embed raw callback/provider/payload material."
}

$encoded = $manifest | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawBatchResultText -Value $encoded -FieldName "workflow external callback batch redrive result manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$manifest | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   workflow external callback batch redrive result manifest written: $OutputPath"
