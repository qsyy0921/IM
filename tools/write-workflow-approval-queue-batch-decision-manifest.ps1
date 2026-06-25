param(
    [Parameter(Mandatory = $true)]
    [string]$QueueSummaryPath,

    [Parameter(Mandatory = $true)]
    [string]$OutputRootPath,

    [Parameter(Mandatory = $true)]
    [ValidateSet("APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL")]
    [string]$Decision,

    [Parameter(Mandatory = $true)]
    [string]$DeciderRef,

    [string]$DecisionPolicyRef = "workflow.external-approval.v1",
    [string]$ReasonRef = "",
    [string]$ReasonFile = "",
    [string[]]$EvidenceRef = @(),
    [string]$BatchManifestPath = "",
    [string]$BatchDecisionID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")
. (Join-Path $PSScriptRoot "output-root-safety.ps1")

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$singleWriterPath = Join-Path $PSScriptRoot "write-workflow-decision-manifest.ps1"
if (-not (Test-Path -LiteralPath $singleWriterPath -PathType Leaf)) {
    throw "Missing workflow decision manifest writer: $singleWriterPath"
}
if (-not (Test-Path -LiteralPath $QueueSummaryPath -PathType Leaf)) {
    throw "Missing workflow approval queue summary: $QueueSummaryPath"
}

Assert-ExternalRepairOutputPath -Value $QueueSummaryPath -FieldName "QueueSummaryPath"
Assert-ExternalRepairOutputPath -Value $OutputRootPath -FieldName "OutputRootPath"
Assert-ExternalOutputRoot -Value $OutputRootPath -RepositoryRoot $repoRoot -Name "OutputRootPath"
if ([string]::IsNullOrWhiteSpace($BatchManifestPath)) {
    $BatchManifestPath = Join-Path ([System.IO.Path]::GetFullPath($OutputRootPath)) "workflow-approval-queue-batch-decision-manifest.json"
}
Assert-ExternalRepairOutputPath -Value $BatchManifestPath -FieldName "BatchManifestPath"
Assert-LowSensitiveRepairActor -Value $DeciderRef -FieldName "DeciderRef"
Assert-LowSensitiveRepairIdentifier -Value $DecisionPolicyRef -FieldName "DecisionPolicyRef"

if ([string]::IsNullOrWhiteSpace($BatchDecisionID)) {
    $BatchDecisionID = "workflow-approval-queue-batch-decision-" + [System.Guid]::NewGuid().ToString("N")
}
Assert-LowSensitiveRepairIdentifier -Value $BatchDecisionID -FieldName "BatchDecisionID"

if (-not [string]::IsNullOrWhiteSpace($ReasonRef) -and -not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    throw "Use either ReasonRef or ReasonFile, not both."
}
Assert-LowSensitiveRepairIdentifier -Value $ReasonRef -FieldName "ReasonRef" -AllowEmpty
if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
    if (-not (Test-Path -LiteralPath $ReasonFile -PathType Leaf)) {
        throw "Missing workflow batch decision reason file: $ReasonFile"
    }
}

function Get-BatchDecisionFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-BatchDecisionStringSha256Ref {
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

function Assert-NoRawBatchDecisionText {
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
    Assert-NoRawBatchDecisionText -Value $Value -FieldName $FieldName
}

function Get-SafeDecisionFileName {
    param(
        [string]$WorkflowID,
        [string]$StepID
    )

    $safe = [regex]::Replace("$WorkflowID-$StepID", "[^A-Za-z0-9_.:-]", "_").Replace(":", "_")
    if ($safe.Length -eq 0) {
        throw "workflow id and step id cannot be converted into a decision manifest file name."
    }
    if ($safe.Length -gt 120) {
        $safe = $safe.Substring(0, 120)
    }
    return "$safe-decision.json"
}

function Read-LowWorkflow {
    param(
        [object]$Workflow,
        [object]$Queue,
        [int]$Index
    )

    $workflowID = Get-JsonString -Object $Workflow -Name "workflow_id"
    $workflowType = Get-JsonString -Object $Workflow -Name "workflow_type"
    $status = (Get-JsonString -Object $Workflow -Name "status").ToUpperInvariant()
    $targetService = Get-JsonString -Object $Workflow -Name "target_service" -AllowEmpty
    $targetOperation = Get-JsonString -Object $Workflow -Name "target_operation" -AllowEmpty
    $targetRefHash = Get-JsonString -Object $Workflow -Name "target_ref_hash" -AllowEmpty
    $payloadSchemaVersion = Get-JsonString -Object $Workflow -Name "payload_schema_version" -AllowEmpty
    $payloadRefHash = Get-JsonString -Object $Workflow -Name "payload_ref_hash" -AllowEmpty
    $approvalPolicyRef = Get-JsonString -Object $Workflow -Name "approval_policy_ref" -AllowEmpty
    $currentStepID = Get-JsonString -Object $Workflow -Name "current_step_id" -AllowEmpty
    $reasonRef = Get-JsonString -Object $Workflow -Name "reason_ref" -AllowEmpty

    foreach ($entry in @(
            @{ name = "workflow_id"; value = $workflowID; allow = $false },
            @{ name = "workflow_type"; value = $workflowType; allow = $false },
            @{ name = "status"; value = $status; allow = $false },
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

    Assert-True ($status -eq "WAITING_DECISION") "workflow[$Index] status must be WAITING_DECISION for batch decision manifest."
    Assert-True ($currentStepID.Length -gt 0) "workflow[$Index].current_step_id is required for decision manifest binding."
    Assert-True ($targetService.Length -gt 0) "workflow[$Index].target_service is required for decision manifest binding."
    Assert-True ($targetOperation.Length -gt 0) "workflow[$Index].target_operation is required for decision manifest binding."
    Assert-True ($targetRefHash.Length -gt 0) "workflow[$Index].target_ref_hash is required for decision manifest binding."
    Assert-True ($payloadSchemaVersion.Length -gt 0) "workflow[$Index].payload_schema_version is required for decision manifest binding."
    Assert-True ($payloadRefHash.Length -gt 0) "workflow[$Index].payload_ref_hash is required for decision manifest binding."
    Assert-True ($approvalPolicyRef.Length -gt 0) "workflow[$Index].approval_policy_ref is required for decision manifest binding."

    if ($null -ne $Queue) {
        $queueID = Get-JsonString -Object $Queue -Name "queue_id"
        $queueType = Get-JsonString -Object $Queue -Name "workflow_type"
        $queueTargetService = Get-JsonString -Object $Queue -Name "target_service" -AllowEmpty
        $queueTargetOperation = Get-JsonString -Object $Queue -Name "target_operation" -AllowEmpty
        $queueApprovalPolicyRef = Get-JsonString -Object $Queue -Name "approval_policy_ref" -AllowEmpty
        Assert-True ($workflowType -eq $queueType) "workflow[$Index] workflow_type does not match queue workflow_type."
        if ($queueTargetService.Length -gt 0) {
            Assert-True ($targetService -eq $queueTargetService) "workflow[$Index] target_service does not match queue target_service."
        }
        if ($queueTargetOperation.Length -gt 0) {
            Assert-True ($targetOperation -eq $queueTargetOperation) "workflow[$Index] target_operation does not match queue target_operation."
        }
        if ($queueApprovalPolicyRef.Length -gt 0) {
            Assert-True ($approvalPolicyRef -eq $queueApprovalPolicyRef) "workflow[$Index] approval_policy_ref does not match queue approval_policy_ref."
        }
    } else {
        $queueID = "provider-replay"
    }

    return [pscustomobject][ordered]@{
        queue_id = $queueID
        workflow_id = $workflowID
        step_id = $currentStepID
        workflow_type = $workflowType
        status = $status
        target_service = $targetService
        target_operation = $targetOperation
        target_ref_hash = $targetRefHash
        payload_schema_version = $payloadSchemaVersion
        payload_ref_hash = $payloadRefHash
        approval_policy_ref = $approvalPolicyRef
        reason_ref = $reasonRef
    }
}

$summaryRaw = Get-Content -LiteralPath $QueueSummaryPath -Raw
Assert-NoRawBatchDecisionText -Value $summaryRaw -FieldName "QueueSummaryPath"
$summary = Get-JsonDocument -Path $QueueSummaryPath -Label "Workflow approval queue summary"
if ($summary.PSObject.Properties["decision"] -or $summary.PSObject.Properties["decisions"]) {
    throw "Workflow batch decision manifest must not accept a queue summary with recorded decision material."
}

$mode = Get-JsonString -Object $summary -Name "mode"
Assert-True (@("operator-queues", "provider-replay-queue") -contains $mode) "Workflow batch decision manifest only accepts operator-queues or provider-replay-queue summaries."
$tenantID = Get-JsonString -Object $summary -Name "tenant_id"
Assert-LowValue -Value $tenantID -FieldName "tenant_id"

$workflows = @()
if ($mode -eq "operator-queues") {
    $queues = @($summary.operator_queues)
    Assert-True ($queues.Count -gt 0) "operator-queues summary must include operator_queues."
    $workflowIndex = 0
    foreach ($queue in $queues) {
        $queueID = Get-JsonString -Object $queue -Name "queue_id"
        $queueStatus = (Get-JsonString -Object $queue -Name "status").ToUpperInvariant()
        $workflowCount = [int]$queue.workflow_count
        $queueWorkflows = @($queue.workflows)
        Assert-LowValue -Value $queueID -FieldName "queue.queue_id"
        Assert-True ($queueStatus -eq "WAITING_DECISION") "queue $queueID status must be WAITING_DECISION."
        Assert-True ($workflowCount -eq $queueWorkflows.Count) "queue $queueID workflow_count does not match workflows length."
        foreach ($workflow in $queueWorkflows) {
            $workflows += (Read-LowWorkflow -Workflow $workflow -Queue $queue -Index $workflowIndex)
            $workflowIndex++
        }
    }
} else {
    Assert-True ((Get-JsonString -Object $summary -Name "workflow_type") -eq "REPAIR_APPROVAL") "provider-replay-queue workflow_type must be REPAIR_APPROVAL."
    Assert-True ((Get-JsonString -Object $summary -Name "target_service") -eq "action-executor") "provider-replay-queue target_service must be action-executor."
    Assert-True ((Get-JsonString -Object $summary -Name "target_operation") -eq "PROVIDER_REPLAY_REQUEST") "provider-replay-queue target_operation must be PROVIDER_REPLAY_REQUEST."
    Assert-True ((Get-JsonString -Object $summary -Name "approval_policy_ref") -eq "admin.workflow.provider_replay.v1") "provider-replay-queue approval_policy_ref must be admin.workflow.provider_replay.v1."
    $workflowIndex = 0
    foreach ($workflow in @($summary.workflows)) {
        $queue = [pscustomobject][ordered]@{
            queue_id = "provider-replay"
            workflow_type = Get-JsonString -Object $summary -Name "workflow_type"
            target_service = Get-JsonString -Object $summary -Name "target_service"
            target_operation = Get-JsonString -Object $summary -Name "target_operation"
            approval_policy_ref = Get-JsonString -Object $summary -Name "approval_policy_ref"
        }
        $workflows += (Read-LowWorkflow -Workflow $workflow -Queue $queue -Index $workflowIndex)
        $workflowIndex++
    }
}
Assert-True ($workflows.Count -gt 0) "Workflow approval queue summary contains no workflows to decide."

New-Item -ItemType Directory -Force -Path $OutputRootPath | Out-Null
$decisionValue = $Decision.Trim().ToUpperInvariant()
$items = @()
$seenWorkflowStep = @{}
foreach ($workflow in $workflows) {
    $dedupe = "$($workflow.workflow_id):$($workflow.step_id)"
    if ($seenWorkflowStep.ContainsKey($dedupe)) {
        throw "Duplicate workflow/step in batch decision manifest: $dedupe"
    }
    $seenWorkflowStep[$dedupe] = $true

    $decisionPath = Join-Path ([System.IO.Path]::GetFullPath($OutputRootPath)) (Get-SafeDecisionFileName -WorkflowID $workflow.workflow_id -StepID $workflow.step_id)
    $writerArgs = @(
        "-NoProfile", "-ExecutionPolicy", "Bypass",
        "-File", $singleWriterPath,
        "-OutputPath", $decisionPath,
        "-WorkflowID", $workflow.workflow_id,
        "-StepID", $workflow.step_id,
        "-ExpectedWorkflowType", $workflow.workflow_type,
        "-ExpectedTargetService", $workflow.target_service,
        "-ExpectedTargetOperation", $workflow.target_operation,
        "-ExpectedTargetRefHash", $workflow.target_ref_hash,
        "-ExpectedPayloadSchemaVersion", $workflow.payload_schema_version,
        "-ExpectedPayloadRefHash", $workflow.payload_ref_hash,
        "-ExpectedApprovalPolicyRef", $workflow.approval_policy_ref,
        "-ExpectedStatus", "WAITING_DECISION",
        "-Decision", $decisionValue,
        "-DeciderRef", $DeciderRef,
        "-DecisionPolicyRef", $DecisionPolicyRef,
        "-IdempotencyKey", "workflow-batch-decision:${BatchDecisionID}:$($workflow.workflow_id):$($workflow.step_id):${decisionValue}:${DeciderRef}",
        "-CorrelationID", "workflow-batch-decision:$BatchDecisionID",
        "-CausationID", "workflow:$($workflow.workflow_id)",
        "-TraceID", "workflow-batch-decision:$BatchDecisionID"
    )
    if (-not [string]::IsNullOrWhiteSpace($ReasonRef)) {
        $writerArgs += @("-ReasonRef", $ReasonRef)
    }
    if (-not [string]::IsNullOrWhiteSpace($ReasonFile)) {
        $writerArgs += @("-ReasonFile", $ReasonFile)
    }
    foreach ($ref in @($EvidenceRef)) {
        if (-not [string]::IsNullOrWhiteSpace($ref)) {
            Assert-LowValue -Value ([string]$ref -replace ",", "") -FieldName "EvidenceRef"
            $writerArgs += @("-EvidenceRef", $ref)
        }
    }
    $writerOutput = & powershell @writerArgs 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($writerOutput | Out-String).Trim())
    }

    $decisionRaw = Get-Content -LiteralPath $decisionPath -Raw
    Assert-NoRawBatchDecisionText -Value $decisionRaw -FieldName "workflow decision manifest"
    $items += [ordered]@{
        queue_id = $workflow.queue_id
        workflow_id = $workflow.workflow_id
        step_id = $workflow.step_id
        expected_workflow_type = $workflow.workflow_type
        expected_status = "WAITING_DECISION"
        expected_target_service = $workflow.target_service
        expected_target_operation = $workflow.target_operation
        expected_target_ref_hash = $workflow.target_ref_hash
        expected_payload_schema_version = $workflow.payload_schema_version
        expected_payload_ref_hash = $workflow.payload_ref_hash
        expected_approval_policy_ref = $workflow.approval_policy_ref
        decision = $decisionValue
        decision_manifest_sha256 = Get-BatchDecisionFileSha256Ref -Path $decisionPath
        decision_manifest_path_sha256 = Get-BatchDecisionStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $decisionPath))
    }
}

$batch = [ordered]@{
    schema_version = "nexusim.workflow.approval_queue_batch_decision.v1"
    batch_decision_id = $BatchDecisionID
    generated_at = [DateTime]::UtcNow.ToString("o")
    decider_ref = $DeciderRef.Trim()
    decision = $decisionValue
    decision_policy_ref = $DecisionPolicyRef.Trim()
    tenant_id = $tenantID
    source_queue_summary_sha256 = Get-BatchDecisionFileSha256Ref -Path $QueueSummaryPath
    source_queue_summary_path_sha256 = Get-BatchDecisionStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $QueueSummaryPath))
    output_root_sha256 = Get-BatchDecisionStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $OutputRootPath))
    decision_count = $items.Count
    records_decision = $false
    calls_workflow_service = $false
    calls_action_executor = $false
    executes_target = $false
    requires_record_workflow_decision = $true
    decision_owner = "workflow-service.RecordWorkflowDecision"
    items = $items
    forbidden_contents = @(
        "payload_material",
        "provider_material",
        "reason_material",
        "local_path",
        "auth_material",
        "workflow_decision_response"
    )
    note = "Low-sensitive batch decision manifest for workflow approval queues. It creates per-workflow decision manifests only; it does not call workflow-service, record decisions, call action-executor, run compensation, redrive provider work, or execute target actions."
}

$encoded = $batch | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawBatchDecisionText -Value $encoded -FieldName "workflow approval queue batch decision manifest"

$batchDirectory = Split-Path -Parent ([System.IO.Path]::GetFullPath($BatchManifestPath))
New-Item -ItemType Directory -Force -Path $batchDirectory | Out-Null
$batch | ConvertTo-Json -Depth 40 | Set-Content -LiteralPath $BatchManifestPath -Encoding UTF8

Write-Host "OK   workflow approval queue batch decision manifest written: $BatchManifestPath"
