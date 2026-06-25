param(
    [Parameter(Mandatory = $true)]
    [string]$BatchDecisionPath,

    [Parameter(Mandatory = $true)]
    [string]$DecisionManifestRootPath,

    [Parameter(Mandatory = $true)]
    [string]$WorkflowOperatorPath,

    [string]$WorkflowOperatorArgumentsJson = "[]",
    [string]$WorkflowOperatorArgumentsJsonBase64 = "",

    [Parameter(Mandatory = $true)]
    [string]$ExecutionSummaryRootPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$ResultManifestPath = "",
    [string]$ResultManifestID = "",
    [switch]$AllowMutating
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")
. (Join-Path $PSScriptRoot "output-root-safety.ps1")

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$decisionValidatorPath = Join-Path $PSScriptRoot "validate-workflow-decision-manifest.ps1"
if (-not (Test-Path -LiteralPath $decisionValidatorPath -PathType Leaf)) {
    throw "Missing workflow decision manifest validator: $decisionValidatorPath"
}

if (-not $AllowMutating) {
    throw "Refusing to execute workflow approval queue batch decisions without -AllowMutating."
}

foreach ($pathPair in @(
        @("BatchDecisionPath", $BatchDecisionPath, "Leaf"),
        @("DecisionManifestRootPath", $DecisionManifestRootPath, "Container"),
        @("WorkflowOperatorPath", $WorkflowOperatorPath, "Leaf")
    )) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    $type = [string]$pathPair[2]
    if (-not (Test-Path -LiteralPath $path -PathType $type)) {
        throw "Missing $name`: $path"
    }
}

Assert-ExternalRepairOutputPath -Value $BatchDecisionPath -FieldName "BatchDecisionPath"
Assert-ExternalRepairOutputPath -Value $DecisionManifestRootPath -FieldName "DecisionManifestRootPath"
Assert-ExternalRepairOutputPath -Value $ExecutionSummaryRootPath -FieldName "ExecutionSummaryRootPath"
Assert-ExternalOutputRoot -Value $ExecutionSummaryRootPath -RepositoryRoot $repoRoot -Name "ExecutionSummaryRootPath"
if (-not [string]::IsNullOrWhiteSpace($ResultManifestPath)) {
    Assert-ExternalRepairOutputPath -Value $ResultManifestPath -FieldName "ResultManifestPath"
    $resultDirectory = Split-Path -Parent ([System.IO.Path]::GetFullPath($ResultManifestPath))
    Assert-ExternalOutputRoot -Value $resultDirectory -RepositoryRoot $repoRoot -Name "ResultManifestPath directory"
}
Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
if (-not [string]::IsNullOrWhiteSpace($ResultManifestID)) {
    Assert-LowSensitiveRepairIdentifier -Value $ResultManifestID -FieldName "ResultManifestID"
}

$workflowOperatorArguments = @()
$workflowOperatorArgumentsJsonValue = $WorkflowOperatorArgumentsJson
if (-not [string]::IsNullOrWhiteSpace($WorkflowOperatorArgumentsJsonBase64)) {
    try {
        $workflowOperatorArgumentsJsonValue = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($WorkflowOperatorArgumentsJsonBase64))
    } catch {
        throw "WorkflowOperatorArgumentsJsonBase64 must be base64 encoded JSON string array."
    }
}
if (-not [string]::IsNullOrWhiteSpace($workflowOperatorArgumentsJsonValue)) {
    try {
        $parsedWorkflowOperatorArguments = $workflowOperatorArgumentsJsonValue | ConvertFrom-Json
    } catch {
        throw "WorkflowOperatorArgumentsJson must be a JSON string array."
    }
    foreach ($argument in @($parsedWorkflowOperatorArguments)) {
        $workflowOperatorArguments += [string]$argument
    }
}

New-Item -ItemType Directory -Force -Path $ExecutionSummaryRootPath | Out-Null

function Get-BatchDecisionRunnerFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-BatchDecisionRunnerStringSha256Ref {
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

function Assert-NoRawBatchDecisionRunnerText {
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
    Assert-NoRawBatchDecisionRunnerText -Value $Value -FieldName $FieldName
}

function Get-SafeExecutionSummaryFileName {
    param(
        [string]$WorkflowID,
        [string]$StepID
    )

    $safe = [regex]::Replace("$WorkflowID-$StepID", "[^A-Za-z0-9_.:-]", "_")
    $safe = $safe.Replace(":", "_")
    if ($safe.Length -eq 0) {
        throw "workflow id and step id cannot be converted into an execution summary file name."
    }
    if ($safe.Length -gt 96) {
        $safe = $safe.Substring(0, 96)
    }
    return "$safe-record-decision-summary.json"
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

function Read-BatchDecisionItem {
    param([object]$Item)

    foreach ($field in @(
            "queue_id",
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
            "decision",
            "decision_manifest_sha256",
            "decision_manifest_path_sha256"
        )) {
        Assert-LowValue -Value (Get-JsonString -Object $Item -Name $field) -FieldName "items.$field"
    }

    $decision = (Get-JsonString -Object $Item -Name "decision").ToUpperInvariant()
    Assert-True (@("APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL") -contains $decision) "items.decision must be APPROVE, REJECT, REQUEST_CHANGES, or CANCEL."
    Assert-True ((Get-JsonString -Object $Item -Name "expected_status") -eq "WAITING_DECISION") "items.expected_status must be WAITING_DECISION."

    return [pscustomobject]@{
        queue_id = Get-JsonString -Object $Item -Name "queue_id"
        workflow_id = Get-JsonString -Object $Item -Name "workflow_id"
        step_id = Get-JsonString -Object $Item -Name "step_id"
        expected_workflow_type = Get-JsonString -Object $Item -Name "expected_workflow_type"
        expected_status = Get-JsonString -Object $Item -Name "expected_status"
        expected_target_service = Get-JsonString -Object $Item -Name "expected_target_service"
        expected_target_operation = Get-JsonString -Object $Item -Name "expected_target_operation"
        expected_target_ref_hash = Get-JsonString -Object $Item -Name "expected_target_ref_hash"
        expected_payload_schema_version = Get-JsonString -Object $Item -Name "expected_payload_schema_version"
        expected_payload_ref_hash = Get-JsonString -Object $Item -Name "expected_payload_ref_hash"
        expected_approval_policy_ref = Get-JsonString -Object $Item -Name "expected_approval_policy_ref"
        decision = $decision
        decision_manifest_sha256 = Get-JsonString -Object $Item -Name "decision_manifest_sha256"
        decision_manifest_path_sha256 = Get-JsonString -Object $Item -Name "decision_manifest_path_sha256"
    }
}

function Invoke-WorkflowRecordDecisionRuntime {
    param(
        [string]$DecisionManifestPath,
        [object]$Item
    )

    $arguments = @($workflowOperatorArguments)
    $arguments += @(
        "-mode", "record-decision",
        "-decision-manifest", (Resolve-Path -LiteralPath $DecisionManifestPath)
    )

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo.FileName = (Resolve-Path -LiteralPath $WorkflowOperatorPath)
    $process.StartInfo.Arguments = ConvertTo-ProcessArgumentString -Arguments $arguments
    $process.StartInfo.UseShellExecute = $false
    $process.StartInfo.RedirectStandardOutput = $true
    $process.StartInfo.RedirectStandardError = $true

    [void]$process.Start()
    $stdout = $process.StandardOutput.ReadToEnd()
    [void]$process.StandardError.ReadToEnd()
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) {
        throw "workflow record-decision runtime failed for workflow_id=$($Item.workflow_id) step_id=$($Item.step_id) with exit_code=$($process.ExitCode)."
    }
    Assert-NoRawBatchDecisionRunnerText -Value $stdout -FieldName "workflow record-decision runtime stdout"

    try {
        $result = $stdout | ConvertFrom-Json
    } catch {
        throw "workflow record-decision runtime must write JSON output for workflow_id=$($Item.workflow_id)."
    }

    $mode = Get-JsonString -Object $result -Name "mode"
    Assert-True ($mode -eq "record-decision") "workflow record-decision runtime output mode must be record-decision."
    $workflowID = Get-JsonString -Object $result -Name "workflow_id" -AllowEmpty
    if ($workflowID.Length -gt 0) {
        Assert-True ($workflowID -eq $Item.workflow_id) "workflow record-decision runtime workflow_id mismatch."
    }
    $decision = $result.decision
    Assert-True ($null -ne $decision) "workflow record-decision runtime output missing decision."
    Assert-True ((Get-JsonString -Object $decision -Name "workflow_id") -eq $Item.workflow_id) "workflow decision workflow_id mismatch."
    Assert-True ((Get-JsonString -Object $decision -Name "step_id") -eq $Item.step_id) "workflow decision step_id mismatch."
    Assert-True ((Get-JsonString -Object $decision -Name "decision_type") -eq $Item.decision) "workflow decision type mismatch."

    $workflow = $result.workflow
    $workflowStatus = ""
    if ($null -ne $workflow) {
        $workflowStatus = Get-JsonString -Object $workflow -Name "status" -AllowEmpty
        if ($workflowStatus.Length -gt 0) {
            Assert-LowValue -Value $workflowStatus -FieldName "workflow.status"
        }
    }

    foreach ($valuePair in @(
            @("decision_id", (Get-JsonString -Object $decision -Name "decision_id")),
            @("decider_ref", (Get-JsonString -Object $decision -Name "decider_ref")),
            @("decision_policy_ref", (Get-JsonString -Object $decision -Name "decision_policy_ref" -AllowEmpty)),
            @("reason_ref", (Get-JsonString -Object $decision -Name "reason_ref" -AllowEmpty))
        )) {
        Assert-LowValue -Value ([string]$valuePair[1]) -FieldName ([string]$valuePair[0]) -AllowEmpty
    }
    foreach ($ref in @($decision.evidence_refs)) {
        Assert-LowValue -Value ([string]$ref) -FieldName "decision.evidence_refs"
    }

    return [ordered]@{
        schema_version = "nexusim.workflow.approval_queue_decision_execution_summary.v1"
        generated_at = [DateTime]::UtcNow.ToString("o")
        queue_id = $Item.queue_id
        workflow_id = $Item.workflow_id
        step_id = $Item.step_id
        decision = $Item.decision
        workflow_status = $workflowStatus
        decision_id = Get-JsonString -Object $decision -Name "decision_id"
        decision_type = Get-JsonString -Object $decision -Name "decision_type"
        decider_ref = Get-JsonString -Object $decision -Name "decider_ref"
        decision_policy_ref = Get-JsonString -Object $decision -Name "decision_policy_ref" -AllowEmpty
        reason_ref = Get-JsonString -Object $decision -Name "reason_ref" -AllowEmpty
        evidence_refs = @($decision.evidence_refs)
        replayed = [bool]$result.replayed
        source_decision_manifest_sha256 = $Item.decision_manifest_sha256
        records_decision = $true
        calls_workflow_service = $true
        calls_action_executor = $false
        executes_target = $false
        mutates_workflow_fact = $true
    }
}

$batchRaw = Get-Content -LiteralPath $BatchDecisionPath -Raw
Assert-NoRawBatchDecisionRunnerText -Value $batchRaw -FieldName "BatchDecisionPath"
$batch = Get-JsonDocument -Path $BatchDecisionPath -Label "Workflow approval queue batch decision manifest"
Assert-True ((Get-JsonString -Object $batch -Name "schema_version") -eq "nexusim.workflow.approval_queue_batch_decision.v1") "Unsupported batch decision schema_version."
Assert-True (-not [bool]$batch.records_decision) "Batch decision manifest must not record decisions."
Assert-True (-not [bool]$batch.calls_workflow_service) "Batch decision manifest must not call workflow-service."
Assert-True (-not [bool]$batch.calls_action_executor) "Batch decision manifest must not call action-executor."
Assert-True (-not [bool]$batch.executes_target) "Batch decision manifest must not execute target actions."
Assert-True ([bool]$batch.requires_record_workflow_decision) "Batch decision manifest must require RecordWorkflowDecision."
Assert-True ((Get-JsonString -Object $batch -Name "decision_owner") -eq "workflow-service.RecordWorkflowDecision") "Batch decision owner must be workflow-service.RecordWorkflowDecision."

$items = @($batch.items)
Assert-True ($items.Count -gt 0) "Batch decision manifest contains no items."

$manifestFilesByHash = @{}
foreach ($file in @(Get-ChildItem -LiteralPath $DecisionManifestRootPath -Filter "*.json" -File -Recurse | Sort-Object FullName)) {
    $hash = Get-BatchDecisionRunnerFileSha256Ref -Path $file.FullName
    if ($manifestFilesByHash.ContainsKey($hash)) {
        throw "Duplicate workflow decision manifest file hash in DecisionManifestRootPath: $hash"
    }
    $manifestFilesByHash[$hash] = $file.FullName
}

$resultItems = @()
$seenWorkflowStep = @{}
foreach ($rawItem in $items) {
    $item = Read-BatchDecisionItem -Item $rawItem
    $dedupe = "$($item.workflow_id):$($item.step_id)"
    if ($seenWorkflowStep.ContainsKey($dedupe)) {
        throw "Duplicate workflow/step in batch decision runner: $dedupe"
    }
    $seenWorkflowStep[$dedupe] = $true

    if (-not $manifestFilesByHash.ContainsKey($item.decision_manifest_sha256)) {
        throw "Missing decision manifest file for workflow_id=$($item.workflow_id) step_id=$($item.step_id)."
    }
    $decisionManifestPath = [string]$manifestFilesByHash[$item.decision_manifest_sha256]
    $decisionPathHash = Get-BatchDecisionRunnerStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $decisionManifestPath))
    Assert-True ($decisionPathHash -eq $item.decision_manifest_path_sha256) "decision manifest path hash mismatch for workflow_id=$($item.workflow_id)."

    $validatorOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $decisionValidatorPath `
        -ManifestPath $decisionManifestPath `
        -ExpectedWorkflowID $item.workflow_id `
        -ExpectedStepID $item.step_id `
        -ExpectedDecision $item.decision 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($validatorOutput | Out-String).Trim())
    }

    $summary = Invoke-WorkflowRecordDecisionRuntime -DecisionManifestPath $decisionManifestPath -Item $item
    $summaryFileName = Get-SafeExecutionSummaryFileName -WorkflowID $item.workflow_id -StepID $item.step_id
    $summaryPath = Join-Path ([System.IO.Path]::GetFullPath($ExecutionSummaryRootPath)) $summaryFileName
    $summary | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $summaryPath -Encoding UTF8
    $summaryEncoded = $summary | ConvertTo-Json -Depth 20 -Compress
    Assert-NoRawBatchDecisionRunnerText -Value $summaryEncoded -FieldName "workflow approval queue decision execution summary"

    $resultItems += [ordered]@{
        queue_id = $summary.queue_id
        workflow_id = $summary.workflow_id
        step_id = $summary.step_id
        decision = $summary.decision
        workflow_status = $summary.workflow_status
        decision_id = $summary.decision_id
        decision_type = $summary.decision_type
        replayed = $summary.replayed
        source_decision_manifest_sha256 = $item.decision_manifest_sha256
        execution_summary_sha256 = Get-BatchDecisionRunnerFileSha256Ref -Path $summaryPath
        execution_summary_path_sha256 = Get-BatchDecisionRunnerStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $summaryPath))
    }
}

if ([string]::IsNullOrWhiteSpace($ResultManifestID)) {
    $ResultManifestID = "workflow-approval-queue-batch-decision-result-" + [System.Guid]::NewGuid().ToString("N")
}

$resultManifest = [ordered]@{
    schema_version = "nexusim.workflow.approval_queue_batch_decision_result.v1"
    result_manifest_id = $ResultManifestID.Trim()
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy.Trim()
    source_batch_decision_sha256 = Get-BatchDecisionRunnerFileSha256Ref -Path $BatchDecisionPath
    source_decision_manifest_root_sha256 = Get-BatchDecisionRunnerStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $DecisionManifestRootPath))
    execution_summary_root_sha256 = Get-BatchDecisionRunnerStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $ExecutionSummaryRootPath))
    workflow_operator_path_sha256 = Get-BatchDecisionRunnerFileSha256Ref -Path $WorkflowOperatorPath
    batch_decision_id = Get-JsonString -Object $batch -Name "batch_decision_id"
    tenant_id = Get-JsonString -Object $batch -Name "tenant_id"
    decision_count = $resultItems.Count
    called_workflow_service_runtime = $true
    records_decision = $true
    calls_action_executor = $false
    executes_target = $false
    mutates_workflow_fact = $true
    result_manifest_written = (-not [string]::IsNullOrWhiteSpace($ResultManifestPath))
    items = $resultItems
    note = "Workflow approval queue batch decision runner. It invokes the existing record-decision decision-manifest entrypoint once per item; it records workflow decisions only and does not execute target actions, compensation, provider replay, or action-executor tools."
}

$encoded = $resultManifest | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawBatchDecisionRunnerText -Value $encoded -FieldName "workflow approval queue batch decision result manifest"

if (-not [string]::IsNullOrWhiteSpace($ResultManifestPath)) {
    $resultDirectory = Split-Path -Parent ([System.IO.Path]::GetFullPath($ResultManifestPath))
    New-Item -ItemType Directory -Force -Path $resultDirectory | Out-Null
    $resultManifest | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $ResultManifestPath -Encoding UTF8
}

$encoded
