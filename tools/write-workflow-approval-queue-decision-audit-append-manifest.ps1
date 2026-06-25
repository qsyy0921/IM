param(
    [Parameter(Mandatory = $true)]
    [string]$ReviewSummaryPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$AuditManifestID = "",
    [string]$AuditRecordID = "",
    [string]$OccurredAt = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $ReviewSummaryPath -PathType Leaf)) {
    throw "Missing ReviewSummaryPath: $ReviewSummaryPath"
}
Assert-ExternalRepairOutputPath -Value $ReviewSummaryPath -FieldName "ReviewSummaryPath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($ReviewSummaryPath))) "workflow-approval-queue-decision-audit-append.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($AuditManifestID)) {
    $AuditManifestID = "workflow-approval-decision-audit-append-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $AuditManifestID -FieldName "AuditManifestID"

function Get-AuditFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-AuditStringSha256Ref {
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
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-False {
    param([bool]$Condition, [string]$Message)
    if ($Condition) {
        throw $Message
    }
}

function Assert-NoRawAuditDecisionText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|https?://|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|callback_body|decision_body|reason_text|EvidencePack|prompt|local_path|filesystem_path|input_json)") {
        throw "$FieldName contains raw, secret, prompt, local path, provider artifact, or credential-like content."
    }
}

function Assert-NoRawAuditDecisionDocumentText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match '(?i)(bearer\s+\S+|password\s*[:=]|secret\s*[:=]|api[_-]?key\s*[:=]|access[_-]?key\s*[:=]|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.|https?://|postgres://|mysql://|mongodb://|provider_body|provider_error_body|message_body|raw_payload|payload_body|input_json|reason_text)') {
        throw "$FieldName contains raw, secret, prompt, provider artifact, URL, or credential-like content."
    }
}

function Assert-NoRawAuditDecisionOutputText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match '(?i)(bearer\s+\S+|password\s*[:=]|secret\s*[:=]|api[_-]?key\s*[:=]|access[_-]?key\s*[:=]|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.|https?://|postgres://|mysql://|mongodb://|"provider_body"\s*:|"provider_error_body"\s*:|"message_body"\s*:|"raw_payload"\s*:|"payload_body"\s*:|"input_json"\s*:|"reason_text"\s*:)') {
        throw "$FieldName contains raw, secret, prompt, provider artifact, URL, or credential-like content."
    }
}

function Assert-LowString {
    param([string]$Value, [string]$FieldName, [switch]$AllowEmpty)
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawAuditDecisionText -Value $Value -FieldName $FieldName
}

function ConvertTo-CanonicalFlatJson {
    param([hashtable]$Values)

    $parts = @()
    foreach ($key in @($Values.Keys | Sort-Object)) {
        $encodedKey = ConvertTo-Json ([string]$key) -Compress
        $encodedValue = ConvertTo-Json ([string]$Values[$key]) -Compress
        $parts += ("{0}:{1}" -f $encodedKey, $encodedValue)
    }
    return "{" + ($parts -join ",") + "}"
}

$summaryRaw = Get-Content -LiteralPath $ReviewSummaryPath -Raw
Assert-NoRawAuditDecisionDocumentText -Value $summaryRaw -FieldName "ReviewSummaryPath"
$summary = Get-JsonDocument -Path $ReviewSummaryPath -Label "Workflow approval queue decision result review summary"

Assert-True ((Get-JsonString -Object $summary -Name "schema_version") -eq "nexusim.workflow.approval_queue_decision_result_review.v1") "Unsupported workflow approval queue decision result review schema_version."
Assert-True ((Get-JsonString -Object $summary -Name "decision_owner") -eq "workflow-service.RecordWorkflowDecision") "decision_owner must be workflow-service.RecordWorkflowDecision."
Assert-True ([bool]$summary.source_records_decision) "Review summary must prove source records decisions."
Assert-True ([bool]$summary.source_called_workflow_service_runtime) "Review summary must prove source called workflow-service runtime."
Assert-False ([bool]$summary.source_calls_action_executor) "Review summary source must not call action-executor."
Assert-False ([bool]$summary.source_executes_target) "Review summary source must not execute target."
Assert-True ([bool]$summary.source_mutates_workflow_fact) "Review summary must prove source mutates workflow fact."
Assert-False ([bool]$summary.review_page_calls_workflow_service) "Review page must not call workflow-service."
Assert-False ([bool]$summary.review_page_records_decision) "Review page must not record decisions."
Assert-False ([bool]$summary.review_page_calls_action_executor) "Review page must not call action-executor."
Assert-False ([bool]$summary.review_page_executes_target) "Review page must not execute target."
Assert-False ([bool]$summary.review_page_mutates_workflow_fact) "Review page must not mutate workflow fact."

$items = @($summary.items)
Assert-True ($items.Count -gt 0) "Review summary contains no decision items."
Assert-True ([int]$summary.decision_count -eq $items.Count) "decision_count must match item count."

$pageID = Get-JsonString -Object $summary -Name "page_id"
$resultManifestID = Get-JsonString -Object $summary -Name "result_manifest_id"
$batchDecisionID = Get-JsonString -Object $summary -Name "batch_decision_id"
$tenantID = Get-JsonString -Object $summary -Name "tenant_id"
$sourceResultManifestSha256 = Get-JsonString -Object $summary -Name "source_result_manifest_sha256"

foreach ($entry in @(
        @{ name = "page_id"; value = $pageID },
        @{ name = "result_manifest_id"; value = $resultManifestID },
        @{ name = "batch_decision_id"; value = $batchDecisionID },
        @{ name = "tenant_id"; value = $tenantID },
        @{ name = "source_result_manifest_sha256"; value = $sourceResultManifestSha256 }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName ([string]$entry.name)
}

$decisionRefs = @()
$decisionHistogram = @{}
for ($i = 0; $i -lt $items.Count; $i++) {
    $item = $items[$i]
    $queueID = Get-JsonString -Object $item -Name "queue_id"
    $workflowID = Get-JsonString -Object $item -Name "workflow_id"
    $stepID = Get-JsonString -Object $item -Name "step_id"
    $decision = (Get-JsonString -Object $item -Name "decision").ToUpperInvariant()
    $workflowStatus = (Get-JsonString -Object $item -Name "workflow_status").ToUpperInvariant()
    $decisionID = Get-JsonString -Object $item -Name "decision_id"
    foreach ($entry in @(
            @{ name = "items[$i].queue_id"; value = $queueID },
            @{ name = "items[$i].workflow_id"; value = $workflowID },
            @{ name = "items[$i].step_id"; value = $stepID },
            @{ name = "items[$i].decision"; value = $decision },
            @{ name = "items[$i].workflow_status"; value = $workflowStatus },
            @{ name = "items[$i].decision_id"; value = $decisionID }
        )) {
        Assert-LowString -Value ([string]$entry.value) -FieldName ([string]$entry.name)
    }
    Assert-True (@("APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL") -contains $decision) "items[$i].decision must be valid."
    $decisionRefs += "$queueID|$workflowID|$stepID|$decision|$workflowStatus|$decisionID"
    if (-not $decisionHistogram.ContainsKey($decision)) {
        $decisionHistogram[$decision] = 0
    }
    $decisionHistogram[$decision] = [int]$decisionHistogram[$decision] + 1
}

$decisionRefsSHA256 = Get-AuditStringSha256Ref -Value (($decisionRefs | Sort-Object) -join "`n")
$decisionHistogramCompact = ConvertTo-CanonicalFlatJson -Values $decisionHistogram
$decisionHistogramSHA256 = Get-AuditStringSha256Ref -Value $decisionHistogramCompact
$reviewSummarySHA256 = Get-AuditFileSha256Ref -Path $ReviewSummaryPath

$occurredAtUnixMs = 0
if ([string]::IsNullOrWhiteSpace($OccurredAt)) {
    $occurredAtUnixMs = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
} else {
    try {
        $occurredAtUnixMs = ([DateTimeOffset]::Parse($OccurredAt).ToUniversalTime()).ToUnixTimeMilliseconds()
    } catch {
        throw "OccurredAt must be an ISO-8601 timestamp."
    }
}
if ($occurredAtUnixMs -le 0) {
    throw "OccurredAt must resolve to a positive Unix milliseconds timestamp."
}

if ([string]::IsNullOrWhiteSpace($AuditRecordID)) {
    $AuditRecordID = "workflow-service:audit:$batchDecisionID"
}
Assert-LowSensitiveRepairIdentifier -Value $AuditRecordID -FieldName "AuditRecordID"

$attributeValues = [ordered]@{
    batch_decision_id = $batchDecisionID
    decision_count = [string]$items.Count
    decision_histogram_sha256 = $decisionHistogramSHA256
    decision_owner = "workflow-service.RecordWorkflowDecision"
    decision_refs_sha256 = $decisionRefsSHA256
    event_type = "WORKFLOW_APPROVAL_QUEUE_BATCH_DECISION"
    result_manifest_id = $resultManifestID
    review_page_id = $pageID
    review_summary_sha256 = $reviewSummarySHA256
    source_result_manifest_sha256 = $sourceResultManifestSha256
}
$attributesCompact = ConvertTo-CanonicalFlatJson -Values $attributeValues
$attributesSHA256 = Get-AuditStringSha256Ref -Value $attributesCompact
$attributesObject = $attributesCompact | ConvertFrom-Json

$manifest = [ordered]@{
    schema_version = "nexusim.audit.external_append.v1"
    manifest_id = $AuditManifestID
    source_manifest_id = $pageID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    executes_append = $false
    mutates_audit_service = $false
    direct_append_allowed = $false
    requires_operator_execution = $true
    audit_stream = "security"
    source_service = "workflow-service"
    source_event_id = $batchDecisionID
    record_type = "WORKFLOW_APPROVAL_BATCH_DECISION"
    actor_ref = "service:workflow-service"
    subject_ref = "tenant:$tenantID"
    resource_ref = "workflow-approval-batch:$batchDecisionID"
    action = "RECORD_WORKFLOW_DECISIONS"
    outcome = "RECORDED"
    reason_code = "WORKFLOW_APPROVAL_QUEUE_BATCH_DECISION_RECORDED"
    risk_level = "HIGH"
    occurred_at_unix_ms = $occurredAtUnixMs
    attributes_json = $attributesObject
    attributes_sha256 = $attributesSHA256
    idempotency_key = $AuditRecordID
    correlation_id = $batchDecisionID
    causation_id = $resultManifestID
    trace_id = ""
    auth_context_contract = [ordered]@{
        tenant_id = $tenantID
        trace_id = ""
    }
    source_review_summary_sha256 = $reviewSummarySHA256
    source_review_summary_path_sha256 = Get-AuditStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $ReviewSummaryPath))
    execution_boundary = @(
        "audit_manifest_is_not_audit_append_execution",
        "does_not_call_audit_service",
        "does_not_record_workflow_decision",
        "does_not_call_action_executor",
        "does_not_execute_target_action",
        "does_not_mutate_workflow_fact"
    )
    required_checks = @(
        "source_decision_result_review_summary_verified",
        "workflow_service_recorded_decisions",
        "review_page_was_read_only",
        "action_executor_not_called",
        "target_action_not_executed",
        "audit_service_append_only",
        "idempotency_key_present"
    )
    forbidden_contents = @(
        "raw_payload",
        "payload_body",
        "provider_body",
        "provider_error_body",
        "decision_body",
        "reason_text",
        "EvidencePack",
        "input_json",
        "filesystem_path_material",
        "auth_material"
    )
    note = "Low-sensitive audit append manifest derived from a workflow approval queue decision result review summary. It does not append audit, record decisions, call action-executor, execute target actions, mutate workflow facts, or embed raw payloads."
}

$encoded = $manifest | ConvertTo-Json -Depth 40 -Compress
Assert-NoRawAuditDecisionOutputText -Value $encoded -FieldName "workflow approval queue decision audit append manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($OutputPath, ($manifest | ConvertTo-Json -Depth 40), $utf8NoBom)

Write-Host "OK   workflow approval queue decision audit append manifest written: $OutputPath"
