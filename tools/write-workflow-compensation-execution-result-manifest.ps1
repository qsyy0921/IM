param(
    [Parameter(Mandatory = $true)]
    [string]$InvocationPath,

    [Parameter(Mandatory = $true)]
    [string]$CompensationSummaryPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$ResultManifestID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($pathPair in @(
    @("InvocationPath", $InvocationPath),
    @("CompensationSummaryPath", $CompensationSummaryPath)
)) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing $name`: $path"
    }
    Assert-ExternalRepairOutputPath -Value $path -FieldName $name
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($CompensationSummaryPath))) "workflow-compensation-execution-result-manifest.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($ResultManifestID)) {
    $ResultManifestID = "workflow-compensation-execution-result-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $ResultManifestID -FieldName "ResultManifestID"

function Get-CompResultFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
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

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-False {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if ($Condition) {
        throw $Message
    }
}

function Assert-LowString {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
}

function Assert-NoRawText {
    param(
        [string]$Value,
        [string]$FieldName
    )
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|reason_text|EvidencePack|prompt|local_path)") {
        throw "$FieldName contains raw, secret, prompt, local path, provider artifact, or credential-like content."
    }
}

$invocation = Get-JsonDocument -Path $InvocationPath -Label "Workflow compensation execution invocation manifest"
$summary = Get-JsonDocument -Path $CompensationSummaryPath -Label "Workflow compensation summary"

Assert-True ((Get-JsonString -Object $invocation -Name "schema_version") -eq "nexusim.workflow.compensation_execution_invocation.v1") "Unsupported workflow compensation execution invocation schema_version."
Assert-False ([bool]$invocation.manifest_is_execution) "invocation.manifest_is_execution must be false."
Assert-False ([bool]$invocation.executes_compensation) "invocation.executes_compensation must be false."
Assert-True ([bool]$invocation.requires_explicit_operator_execution) "invocation.requires_explicit_operator_execution must be true."

Assert-True ((Get-JsonString -Object $summary -Name "mode") -eq "list-compensations") "Compensation summary mode must be list-compensations."

$workflow = $invocation.workflow
if ($null -eq $workflow) {
    throw "invocation.workflow is required."
}
$workflowID = Get-JsonString -Object $workflow -Name "workflow_id"
$workflowType = Get-JsonString -Object $workflow -Name "workflow_type"
$targetService = Get-JsonString -Object $workflow -Name "target_service"
$targetOperation = Get-JsonString -Object $workflow -Name "target_operation"
$payloadRefHash = Get-JsonString -Object $workflow -Name "payload_ref_hash"

Assert-True ($workflowType -eq "COMPENSATION_REQUEST") "workflow.workflow_type must be COMPENSATION_REQUEST."
Assert-True ((Get-JsonString -Object $summary -Name "workflow_id") -eq $workflowID) "Compensation summary workflow_id does not match invocation."

$compensations = @(Get-JsonArray -Object $summary -Name "compensations")
Assert-True ($compensations.Count -gt 0) "Compensation summary must include at least one compensation."

$matched = $null
foreach ($candidate in $compensations) {
    if ((Get-JsonString -Object $candidate -Name "workflow_id") -eq $workflowID -and
        (Get-JsonString -Object $candidate -Name "payload_ref_hash") -eq $payloadRefHash -and
        (Get-JsonString -Object $candidate -Name "target_service") -eq $targetService -and
        (Get-JsonString -Object $candidate -Name "target_operation") -eq $targetOperation) {
        $matched = $candidate
        break
    }
}
if ($null -eq $matched) {
    throw "No compensation row matches invocation workflow, payload and target refs."
}

$status = Get-JsonString -Object $matched -Name "status"
Assert-True (($status -eq "SUCCEEDED" -or $status -eq "FAILED")) "Compensation result status must be SUCCEEDED or FAILED."

$compensationID = Get-JsonString -Object $matched -Name "compensation_id"
$downstreamService = Get-JsonString -Object $matched -Name "downstream_service" -AllowEmpty
$downstreamRequestRef = Get-JsonString -Object $matched -Name "downstream_request_ref" -AllowEmpty
$failureClass = Get-JsonString -Object $matched -Name "failure_class" -AllowEmpty
$publicError = Get-JsonString -Object $matched -Name "public_error" -AllowEmpty

foreach ($item in @(
    @{ name = "workflow_id"; value = $workflowID },
    @{ name = "target_service"; value = $targetService },
    @{ name = "target_operation"; value = $targetOperation },
    @{ name = "payload_ref_hash"; value = $payloadRefHash },
    @{ name = "compensation_id"; value = $compensationID },
    @{ name = "downstream_service"; value = $downstreamService; allow = $true },
    @{ name = "downstream_request_ref"; value = $downstreamRequestRef; allow = $true },
    @{ name = "failure_class"; value = $failureClass; allow = $true },
    @{ name = "public_error"; value = $publicError; allow = $true }
)) {
    Assert-LowString -Value $item.value -FieldName $item.name -AllowEmpty:([bool]$item.allow)
}

$manifest = [ordered]@{
    schema_version = "nexusim.workflow.compensation_execution_result.v1"
    result_manifest_id = $ResultManifestID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    manifest_is_execution = $false
    executes_compensation = $false
    records_decision = $false
    calls_downstream_service = $false
    source_invocation_sha256 = Get-CompResultFileSha256Ref -Path $InvocationPath
    source_compensation_summary_sha256 = Get-CompResultFileSha256Ref -Path $CompensationSummaryPath
    workflow = [ordered]@{
        workflow_id = $workflowID
        workflow_type = $workflowType
        target_service = $targetService
        target_operation = $targetOperation
        target_ref_hash = Get-JsonString -Object $workflow -Name "target_ref_hash" -AllowEmpty
        payload_schema_version = Get-JsonString -Object $workflow -Name "payload_schema_version" -AllowEmpty
        payload_ref_hash = $payloadRefHash
        approval_policy_ref = Get-JsonString -Object $workflow -Name "approval_policy_ref" -AllowEmpty
        compensation_policy_ref = Get-JsonString -Object $workflow -Name "compensation_policy_ref" -AllowEmpty
    }
    compensation_result = [ordered]@{
        compensation_id = $compensationID
        source_step_id = Get-JsonString -Object $matched -Name "source_step_id" -AllowEmpty
        status = $status
        downstream_service = $downstreamService
        downstream_request_ref = $downstreamRequestRef
        failure_class = $failureClass
        public_error = $publicError
        created_at_unix_ms = [int64]((Get-JsonString -Object $matched -Name "created_at_unix_ms" -AllowEmpty) -as [int64])
        updated_at_unix_ms = [int64]((Get-JsonString -Object $matched -Name "updated_at_unix_ms" -AllowEmpty) -as [int64])
        completed_at_unix_ms = [int64]((Get-JsonString -Object $matched -Name "completed_at_unix_ms" -AllowEmpty) -as [int64])
    }
    required_checks = @(
        "source_invocation_manifest_verified",
        "list_compensations_summary_from_workflow_service_public_api",
        "compensation_row_matches_invocation_workflow_payload_target",
        "compensation_status_is_terminal",
        "result_manifest_contains_only_low_sensitive_refs"
    )
    execution_boundary = @(
        "result_manifest_is_not_execution",
        "does_not_call_downstream_service",
        "does_not_record_workflow_decision",
        "does_not_modify_workflow_or_compensation_rows",
        "workflow_service_compensation_executor_remains_final_execution_owner"
    )
    note = "Low-sensitive workflow compensation execution result manifest. It binds a workflow-service compensation query result to a prior execution invocation; it does not execute compensation, record decisions, call downstream services, or embed raw payloads."
}

$encoded = $manifest | ConvertTo-Json -Depth 30
Assert-NoRawText -Value $encoded -FieldName "workflow compensation execution result manifest"
$encoded | Set-Content -LiteralPath $OutputPath -Encoding UTF8
Write-Host "OK   workflow compensation execution result manifest written: $OutputPath"
