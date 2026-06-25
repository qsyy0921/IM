param(
    [Parameter(Mandatory = $true)]
    [string]$BatchInvocationPath,

    [Parameter(Mandatory = $true)]
    [string]$RedrivePlanRootPath,

    [Parameter(Mandatory = $true)]
    [string]$WorkflowServicePath,

    [string]$WorkflowServiceArgumentsJson = "[]",
    [string]$WorkflowServiceArgumentsJsonBase64 = "",

    [Parameter(Mandatory = $true)]
    [string]$ExecutionSummaryRootPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [Parameter(Mandatory = $true)]
    [string]$TenantID,

    [string]$ResultManifestPath = "",
    [string]$ResultManifestID = "",
    [switch]$AllowMutating
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

$resultWriterPath = Join-Path $PSScriptRoot "write-workflow-external-callback-batch-redrive-result-manifest.ps1"
if (-not (Test-Path -LiteralPath $resultWriterPath -PathType Leaf)) {
    throw "Missing workflow external callback batch redrive result writer: $resultWriterPath"
}

if (-not $AllowMutating) {
    throw "Refusing to run workflow external callback batch redrive without -AllowMutating."
}

foreach ($pathPair in @(
        @("BatchInvocationPath", $BatchInvocationPath, "Leaf"),
        @("RedrivePlanRootPath", $RedrivePlanRootPath, "Container"),
        @("WorkflowServicePath", $WorkflowServicePath, "Leaf")
    )) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    $type = [string]$pathPair[2]
    if (-not (Test-Path -LiteralPath $path -PathType $type)) {
        throw "Missing $name`: $path"
    }
}

Assert-ExternalRepairOutputPath -Value $BatchInvocationPath -FieldName "BatchInvocationPath"
Assert-ExternalRepairOutputPath -Value $RedrivePlanRootPath -FieldName "RedrivePlanRootPath"
Assert-ExternalRepairOutputPath -Value $ExecutionSummaryRootPath -FieldName "ExecutionSummaryRootPath"
if (-not [string]::IsNullOrWhiteSpace($ResultManifestPath)) {
    Assert-ExternalRepairOutputPath -Value $ResultManifestPath -FieldName "ResultManifestPath"
}
Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $TenantID -FieldName "TenantID"
if (-not [string]::IsNullOrWhiteSpace($ResultManifestID)) {
    Assert-LowSensitiveRepairIdentifier -Value $ResultManifestID -FieldName "ResultManifestID"
}

$workflowServiceArguments = @()
$workflowServiceArgumentsJsonValue = $WorkflowServiceArgumentsJson
if (-not [string]::IsNullOrWhiteSpace($WorkflowServiceArgumentsJsonBase64)) {
    try {
        $workflowServiceArgumentsJsonValue = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($WorkflowServiceArgumentsJsonBase64))
    } catch {
        throw "WorkflowServiceArgumentsJsonBase64 must be base64 encoded JSON string array."
    }
}
if (-not [string]::IsNullOrWhiteSpace($workflowServiceArgumentsJsonValue)) {
    try {
        $parsedWorkflowServiceArguments = $workflowServiceArgumentsJsonValue | ConvertFrom-Json
    } catch {
        throw "WorkflowServiceArgumentsJson must be a JSON string array."
    }
    foreach ($argument in @($parsedWorkflowServiceArguments)) {
        $workflowServiceArguments += [string]$argument
    }
}

if ([string]::IsNullOrWhiteSpace($env:NEXUSIM_PG_DSN)) {
    throw "NEXUSIM_PG_DSN is required so workflow-service can revalidate and mutate its delivery fact."
}

New-Item -ItemType Directory -Force -Path $ExecutionSummaryRootPath | Out-Null

function Get-CallbackRunnerFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-CallbackRunnerStringSha256Ref {
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

function Assert-NoRawCallbackRunnerText {
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
    Assert-NoRawCallbackRunnerText -Value $Value -FieldName $FieldName
}

function Get-SafeSummaryFileName {
    param([string]$PlanID)

    $safe = [regex]::Replace($PlanID, "[^A-Za-z0-9_.:-]", "_")
    $safe = $safe.Replace(":", "_")
    if ($safe.Length -eq 0) {
        throw "redrive_plan_id cannot be converted into a summary file name."
    }
    if ($safe.Length -gt 96) {
        $safe = $safe.Substring(0, 96)
    }
    return "$safe-summary.json"
}

function ConvertTo-ProcessArgumentString {
    param([string[]]$Arguments)

    $escaped = @()
    foreach ($argument in @($Arguments)) {
        $value = [string]$argument
        if ($value.Length -eq 0) {
            $escaped += '""'
            continue
        }
        $value = $value.Replace('\', '\\').Replace('"', '\"')
        if ($value -match "\s|`"") {
            $escaped += '"' + $value + '"'
        } else {
            $escaped += $value
        }
    }
    return ($escaped -join " ")
}

function Read-InvocationRedrive {
    param([object]$Redrive)

    foreach ($field in @(
            "redrive_plan_id",
            "workflow_id",
            "step_id",
            "source_delivery_status_sha256",
            "source_delivery_plan_sha256",
            "redrive_plan_sha256",
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
    }
}

function Invoke-WorkflowRedriveRuntime {
    param(
        [string]$PlanPath,
        [string]$SummaryPath,
        [string]$PlanID
    )

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo.FileName = (Resolve-Path -LiteralPath $WorkflowServicePath)
    $process.StartInfo.Arguments = ConvertTo-ProcessArgumentString -Arguments $workflowServiceArguments
    $process.StartInfo.UseShellExecute = $false
    $process.StartInfo.RedirectStandardOutput = $true
    $process.StartInfo.RedirectStandardError = $true
    $process.StartInfo.Environment["NEXUSIM_WORKFLOW_SERVICE_MODE"] = "external-callback-delivery-redrive"
    $process.StartInfo.Environment["NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID"] = $TenantID
    $process.StartInfo.Environment["NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE"] = (Resolve-Path -LiteralPath $PlanPath)
    $process.StartInfo.Environment["NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_SUMMARY_FILE"] = [System.IO.Path]::GetFullPath($SummaryPath)

    [void]$process.Start()
    [void]$process.StandardOutput.ReadToEnd()
    [void]$process.StandardError.ReadToEnd()
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) {
        throw "workflow-service redrive runtime failed for redrive_plan_id=$PlanID with exit_code=$($process.ExitCode)."
    }
}

$invocationRaw = Get-Content -LiteralPath $BatchInvocationPath -Raw
Assert-NoRawCallbackRunnerText -Value $invocationRaw -FieldName "BatchInvocationPath"
$invocation = Get-JsonDocument -Path $BatchInvocationPath -Label "Workflow external callback batch redrive invocation manifest"
Assert-True ((Get-JsonString -Object $invocation -Name "schema_version") -eq "nexusim.workflow.external_callback_batch_redrive_invocation.v1") "Unsupported batch invocation schema_version."

$runtime = $invocation.runtime_contract
Assert-True ($null -ne $runtime) "runtime_contract is required."
Assert-True ((Get-JsonString -Object $runtime -Name "service") -eq "workflow-service") "runtime_contract.service must be workflow-service."
Assert-True ((Get-JsonString -Object $runtime -Name "mode") -eq "external-callback-delivery-redrive") "runtime_contract.mode must be external-callback-delivery-redrive."
Assert-True ((Get-JsonString -Object $runtime -Name "plan_env") -eq "NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE") "runtime_contract.plan_env mismatch."
Assert-True (-not [bool]$runtime.batch_invocation_calls_service) "batch invocation must not call service."
Assert-True (-not [bool]$runtime.batch_invocation_records_decision) "batch invocation must not record decision."
Assert-True (-not [bool]$runtime.batch_invocation_calls_provider) "batch invocation must not call provider."
Assert-True (-not [bool]$runtime.batch_invocation_executes_target) "batch invocation must not execute target."
Assert-True ([bool]$runtime.requires_one_runtime_call_per_redrive_plan) "batch invocation must require one runtime call per redrive plan."

$redrives = @($invocation.redrives)
Assert-True ($redrives.Count -gt 0) "batch invocation must contain redrives."

$planFilesByHash = @{}
foreach ($file in @(Get-ChildItem -LiteralPath $RedrivePlanRootPath -Filter "*.json" -File | Sort-Object Name)) {
    $hash = Get-CallbackRunnerFileSha256Ref -Path $file.FullName
    if ($planFilesByHash.ContainsKey($hash)) {
        throw "Duplicate redrive plan file hash in RedrivePlanRootPath: $hash"
    }
    $planFilesByHash[$hash] = $file.FullName
}

$seenPlans = @{}
foreach ($item in $redrives) {
    $redrive = Read-InvocationRedrive -Redrive $item
    if ($seenPlans.ContainsKey($redrive.redrive_plan_id)) {
        throw "Duplicate redrive_plan_id in batch invocation: $($redrive.redrive_plan_id)"
    }
    $seenPlans[$redrive.redrive_plan_id] = $true

    if (-not $planFilesByHash.ContainsKey($redrive.redrive_plan_sha256)) {
        throw "Missing redrive plan file for redrive_plan_id=$($redrive.redrive_plan_id)."
    }
    $planPath = [string]$planFilesByHash[$redrive.redrive_plan_sha256]
    $plan = Get-JsonDocument -Path $planPath -Label "Workflow external callback redrive plan"
    Assert-True ((Get-JsonString -Object $plan -Name "schema_version") -eq "nexusim.workflow.external_callback_redrive_plan.v1") "Unsupported redrive plan schema_version."
    Assert-True ((Get-JsonString -Object $plan -Name "redrive_plan_id") -eq $redrive.redrive_plan_id) "redrive plan id does not match batch invocation."

    $summaryPath = Join-Path ([System.IO.Path]::GetFullPath($ExecutionSummaryRootPath)) (Get-SafeSummaryFileName -PlanID $redrive.redrive_plan_id)
    Remove-Item -LiteralPath $summaryPath -Force -ErrorAction SilentlyContinue
    Invoke-WorkflowRedriveRuntime -PlanPath $planPath -SummaryPath $summaryPath -PlanID $redrive.redrive_plan_id
    if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
        throw "workflow-service redrive runtime did not write execution summary for redrive_plan_id=$($redrive.redrive_plan_id)."
    }
}

$resultArgs = @(
    "-NoProfile", "-ExecutionPolicy", "Bypass",
    "-File", $resultWriterPath,
    "-BatchInvocationPath", $BatchInvocationPath,
    "-ExecutionSummaryRootPath", $ExecutionSummaryRootPath,
    "-GeneratedBy", $GeneratedBy
)
if (-not [string]::IsNullOrWhiteSpace($ResultManifestPath)) {
    $resultArgs += @("-OutputPath", $ResultManifestPath)
}
if (-not [string]::IsNullOrWhiteSpace($ResultManifestID)) {
    $resultArgs += @("-ResultManifestID", $ResultManifestID)
}

$output = & powershell @resultArgs 2>&1
if ($LASTEXITCODE -ne 0) {
    throw (($output | Out-String).Trim())
}

$summary = [ordered]@{
    schema_version = "nexusim.workflow.external_callback_batch_redrive_runner.v1"
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    batch_invocation_id = Get-JsonString -Object $invocation -Name "invocation_id"
    source_batch_invocation_sha256 = Get-CallbackRunnerFileSha256Ref -Path $BatchInvocationPath
    source_redrive_plan_root_sha256 = Get-CallbackRunnerStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $RedrivePlanRootPath))
    source_execution_summary_root_sha256 = Get-CallbackRunnerStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $ExecutionSummaryRootPath))
    workflow_service_path_sha256 = Get-CallbackRunnerFileSha256Ref -Path $WorkflowServicePath
    tenant_id = $TenantID
    redrive_count = $redrives.Count
    mode = "external-callback-delivery-redrive"
    called_workflow_service_runtime = $true
    records_decision = $false
    calls_provider = $false
    executes_target = $false
    mutates_delivery_fact = $true
    result_manifest_written = $true
}

$encoded = $summary | ConvertTo-Json -Depth 20 -Compress
Assert-NoRawCallbackRunnerText -Value $encoded -FieldName "workflow external callback batch redrive runner summary"
$encoded
