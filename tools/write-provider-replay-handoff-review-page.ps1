param(
    [Parameter(Mandatory = $true)]
    [string]$HandoffPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$PageID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

if (-not (Test-Path -LiteralPath $HandoffPath -PathType Leaf)) {
    throw "Missing provider replay handoff artifact: $HandoffPath"
}
Assert-ExternalRepairOutputPath -Value $HandoffPath -FieldName "HandoffPath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($HandoffPath))) "provider-replay-handoff-review.html"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($PageID)) {
    $PageID = "provider-replay-handoff-review-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $PageID -FieldName "PageID"

function Get-ProviderReplayFileSha256 {
    param([string]$Path)

    return Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path)))
}

function Get-ProviderReplayStringSha256 {
    param([string]$Value)

    return Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value))
}

function ConvertTo-HtmlText {
    param([object]$Value)

    return [System.Net.WebUtility]::HtmlEncode([string]$Value)
}

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
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

function Assert-LowSensitiveProviderReplayValue {
    param(
        [string]$Value,
        [string]$FieldName,
        [switch]$AllowEmpty
    )

    $text = ([string]$Value).Trim()
    if ($text.Length -eq 0) {
        if ($AllowEmpty) {
            return
        }
        throw "$FieldName is required."
    }
    if ($text.Length -gt 512) {
        throw "$FieldName is too long for a low-sensitive provider replay ref."
    }
    if ($text -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|private://|raw:|dsn=|postgres://|http://|https://|message_body|provider_body|prompt|input_json|output_json)" -or
        $text -match "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}") {
        throw "$FieldName must be low-sensitive and must not contain credential-like, personal, URL, raw body, prompt, or provider payload content."
    }
}

function ConvertTo-CanonicalJson {
    param([object]$Object)

    if ($null -eq $Object) {
        return "null"
    }
    if ($Object -is [bool]) {
        if ($Object) { return "true" }
        return "false"
    }
    if ($Object -is [int] -or $Object -is [long] -or $Object -is [double] -or $Object -is [decimal]) {
        return [string]$Object
    }
    if ($Object -is [string]) {
        return ($Object | ConvertTo-Json -Compress)
    }
    if ($Object -is [System.Collections.IEnumerable] -and -not ($Object -is [string]) -and $null -eq $Object.PSObject.Properties["Keys"]) {
        $items = @()
        foreach ($item in @($Object)) {
            $items += ConvertTo-CanonicalJson -Object $item
        }
        return "[" + ($items -join ",") + "]"
    }

    $properties = @($Object.PSObject.Properties | Sort-Object Name)
    $pairs = @()
    foreach ($property in $properties) {
        $encodedName = ([string]$property.Name | ConvertTo-Json -Compress)
        $encodedValue = ConvertTo-CanonicalJson -Object $property.Value
        $pairs += "$encodedName`:$encodedValue"
    }
    return "{" + ($pairs -join ",") + "}"
}

function Get-Sha256Text {
    param([string]$Text)

    return Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Text))
}

function Add-TableRow {
    param(
        [System.Text.StringBuilder]$Builder,
        [string]$Key,
        [object]$Value
    )

    [void]$Builder.AppendLine("<tr><th>$(ConvertTo-HtmlText $Key)</th><td>$(ConvertTo-HtmlText $Value)</td></tr>")
}

function Add-SectionHeader {
    param(
        [System.Text.StringBuilder]$Builder,
        [string]$Title
    )

    [void]$Builder.AppendLine("<h2>$(ConvertTo-HtmlText $Title)</h2>")
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

function Add-List {
    param(
        [System.Text.StringBuilder]$Builder,
        [object[]]$Values,
        [string]$EmptyText
    )

    if ($Values.Count -eq 0) {
        [void]$Builder.AppendLine("<p>$(ConvertTo-HtmlText $EmptyText)</p>")
        return
    }
    [void]$Builder.AppendLine("<ul>")
    foreach ($value in $Values) {
        [void]$Builder.AppendLine("<li><code>$(ConvertTo-HtmlText $value)</code></li>")
    }
    [void]$Builder.AppendLine("</ul>")
}

try {
    $document = Get-Content -LiteralPath $HandoffPath -Raw | ConvertFrom-Json
} catch {
    throw "Provider replay handoff artifact must be valid JSON: $HandoffPath"
}

$kind = Get-JsonString -Object $document -Name "kind"
Assert-Condition ($kind -eq "action-executor.provider-failure.replay-admin-workflow-handoff") "provider replay handoff kind is not supported."

$contract = $document.handoff_contract
Assert-Condition ($null -ne $contract) "handoff_contract is required."
Assert-Condition ((Get-JsonString -Object $contract -Name "admin_operation_type") -eq "PROVIDER_REPLAY_REQUEST") "handoff contract admin_operation_type mismatch."
Assert-Condition ((Get-JsonString -Object $contract -Name "workflow_type") -eq "REPAIR_APPROVAL") "handoff contract workflow_type mismatch."
Assert-Condition ((Get-JsonString -Object $contract -Name "target_service") -eq "action-executor") "handoff contract target_service mismatch."
Assert-Condition ((Get-JsonString -Object $contract -Name "target_operation") -eq "PROVIDER_REPLAY_REQUEST") "handoff contract target_operation mismatch."
Assert-Condition ((Get-JsonString -Object $contract -Name "redrive_entrypoint") -eq "RedriveProviderFailure") "handoff contract redrive_entrypoint mismatch."
Assert-Condition ((Get-JsonString -Object $contract -Name "approval_policy_ref") -eq "admin.workflow.provider_replay.v1") "handoff contract approval_policy_ref mismatch."
Assert-Condition ((Get-JsonString -Object $contract -Name "payload_schema_version") -eq "admin.provider_replay_request.v1") "handoff contract payload_schema_version mismatch."
Assert-Condition (-not [bool]$contract.direct_execution_allowed) "handoff contract direct_execution_allowed must be false."
Assert-Condition ([bool]$contract.source_dlq_immutable) "handoff contract source_dlq_immutable must be true."

foreach ($property in @($contract.PSObject.Properties)) {
    if ($property.Value -is [string]) {
        Assert-LowSensitiveProviderReplayValue -Value ([string]$property.Value) -FieldName "handoff_contract.$($property.Name)"
    }
}
$allowedProviderReplayGates = @(
    "admin_operation_request",
    "workflow_repair_approval",
    "fresh_agent_proposal",
    "fresh_agent_approval",
    "fresh_prepared_audit",
    "matching_skill_tool_resource",
    "new_input_json",
    "reason_sha256",
    "action_executor_redrive_entrypoint"
)
foreach ($requiredGate in (Get-JsonArray -Object $contract -Name "requires")) {
    $gateValue = ([string]$requiredGate).Trim()
    Assert-Condition ($gateValue -in $allowedProviderReplayGates) "handoff_contract.requires contains unsupported gate: $gateValue"
}

$adminRequests = @(Get-JsonArray -Object $document -Name "admin_operation_requests")
$workflowRequests = @(Get-JsonArray -Object $document -Name "workflow_handoff_requests")
Assert-Condition ($adminRequests.Count -gt 0) "provider replay handoff contains no admin_operation_requests."
if ($workflowRequests.Count -gt 0) {
    Assert-Condition ($workflowRequests.Count -eq $adminRequests.Count) "workflow_handoff_requests count must match admin_operation_requests count when present."
}

foreach ($request in $adminRequests) {
    $operationType = Get-JsonString -Object $request -Name "operation_type"
    $payloadSchemaVersion = Get-JsonString -Object $request -Name "payload_schema_version"
    $targetRefHash = Get-JsonString -Object $request -Name "target_ref_hash"
    $payloadHash = Get-JsonString -Object $request -Name "operation_payload_hash"
    $workflowPolicy = Get-JsonString -Object $request -Name "expected_workflow_policy"
    $riskLevel = (Get-JsonString -Object $request -Name "risk_level").ToUpperInvariant()

    Assert-Condition ($operationType -eq "PROVIDER_REPLAY_REQUEST") "admin operation request operation_type must be PROVIDER_REPLAY_REQUEST."
    Assert-Condition ($payloadSchemaVersion -eq "admin.provider_replay_request.v1") "admin operation request payload_schema_version mismatch."
    Assert-Condition ($workflowPolicy -eq "admin.workflow.provider_replay.v1") "admin operation request expected_workflow_policy mismatch."
    Assert-Condition ($riskLevel -eq "HIGH" -or $riskLevel -eq "CRITICAL") "admin operation request risk_level must be HIGH or CRITICAL."

    foreach ($field in @(
            "auth_tenant_id",
            "operator_ref",
            "operator_role",
            "operation_type",
            "target_ref_hash",
            "risk_level",
            "payload_schema_version",
            "operation_payload_hash",
            "reason_ref",
            "idempotency_key",
            "correlation_id",
            "causation_id",
            "trace_id",
            "expected_workflow_policy"
        )) {
        Assert-LowSensitiveProviderReplayValue -Value (Get-JsonString -Object $request -Name $field -AllowEmpty) -FieldName "admin_operation_request.$field" -AllowEmpty
    }
    foreach ($ref in (Get-JsonArray -Object $request -Name "evidence_refs")) {
        Assert-LowSensitiveProviderReplayValue -Value ([string]$ref) -FieldName "admin_operation_request.evidence_refs"
    }
    Assert-Condition ((Get-JsonArray -Object $request -Name "evidence_refs").Count -gt 0) "admin operation request evidence_refs are required."

    $payload = $request.operation_payload
    Assert-Condition ($null -ne $payload) "admin operation request operation_payload is required."
    Assert-Condition ((Get-JsonString -Object $payload -Name "redrive_entrypoint") -eq "RedriveProviderFailure") "operation_payload redrive_entrypoint mismatch."
    Assert-Condition ([bool]$payload.requires_fresh_proposal) "operation_payload requires_fresh_proposal must be true."
    Assert-Condition ([bool]$payload.requires_fresh_approval) "operation_payload requires_fresh_approval must be true."
    Assert-Condition ([bool]$payload.requires_prepared_audit) "operation_payload requires_prepared_audit must be true."
    Assert-Condition ([bool]$payload.requires_new_input) "operation_payload requires_new_input must be true."
    Assert-Condition ([bool]$payload.requires_reason_sha256) "operation_payload requires_reason_sha256 must be true."
    Assert-Condition ([bool]$payload.source_dlq_immutable) "operation_payload source_dlq_immutable must be true."
    Assert-Condition (-not [bool]$payload.direct_execution_allowed) "operation_payload direct_execution_allowed must be false."
    Assert-Condition ((Get-JsonString -Object $payload -Name "provider_failure_ref_hash") -eq $targetRefHash) "operation_payload provider_failure_ref_hash must match target_ref_hash."
    foreach ($field in @(
            "provider_failure_ref_hash",
            "source_execution_ref_hash",
            "source_result_ref_hash",
            "replay_candidate_id",
            "redrive_entrypoint"
        )) {
        Assert-LowSensitiveProviderReplayValue -Value (Get-JsonString -Object $payload -Name $field) -FieldName "operation_payload.$field"
    }
    $canonicalPayload = ConvertTo-CanonicalJson -Object $payload
    Assert-Condition ($payloadHash -eq ("sha256:" + (Get-Sha256Text -Text $canonicalPayload))) "admin operation request payload hash mismatch."
}

foreach ($workflowRequest in $workflowRequests) {
    Assert-Condition ((Get-JsonString -Object $workflowRequest -Name "workflow_type") -eq "REPAIR_APPROVAL") "workflow handoff request workflow_type mismatch."
    Assert-Condition ((Get-JsonString -Object $workflowRequest -Name "requester_service") -eq "admin-service") "workflow handoff request requester_service mismatch."
    Assert-Condition ((Get-JsonString -Object $workflowRequest -Name "target_service") -eq "action-executor") "workflow handoff request target_service mismatch."
    Assert-Condition ((Get-JsonString -Object $workflowRequest -Name "target_operation") -eq "PROVIDER_REPLAY_REQUEST") "workflow handoff request target_operation mismatch."
    Assert-Condition ((Get-JsonString -Object $workflowRequest -Name "payload_schema_version") -eq "admin.provider_replay_request.v1") "workflow handoff request payload_schema_version mismatch."
    Assert-Condition ((Get-JsonString -Object $workflowRequest -Name "approval_policy_ref") -eq "admin.workflow.provider_replay.v1") "workflow handoff request approval_policy_ref mismatch."
    foreach ($field in @(
            "workflow_type",
            "requester_service",
            "target_service",
            "target_operation",
            "risk_level",
            "target_ref_hash",
            "payload_schema_version",
            "payload_ref_hash",
            "approval_policy_ref",
            "reason_ref",
            "idempotency_key",
            "correlation_id",
            "causation_id",
            "trace_id"
        )) {
        Assert-LowSensitiveProviderReplayValue -Value (Get-JsonString -Object $workflowRequest -Name $field -AllowEmpty) -FieldName "workflow_handoff_request.$field" -AllowEmpty
    }
    foreach ($ref in (Get-JsonArray -Object $workflowRequest -Name "evidence_refs")) {
        Assert-LowSensitiveProviderReplayValue -Value ([string]$ref) -FieldName "workflow_handoff_request.evidence_refs"
    }
}

$rows = @(Get-JsonArray -Object $document -Name "rows")
foreach ($row in $rows) {
    foreach ($field in @(
            "replay_candidate_id",
            "replay_state",
            "tenant_id",
            "provider_failure_id",
            "execution_id",
            "result_id",
            "proposal_id",
            "approval_id",
            "prepared_audit_id",
            "user_id_hash",
            "skill_id",
            "tool_name",
            "resource_type",
            "resource_id_hash",
            "classification",
            "status",
            "failure_ref_hash"
        )) {
        Assert-LowSensitiveProviderReplayValue -Value (Get-JsonString -Object $row -Name $field -AllowEmpty) -FieldName "row.$field" -AllowEmpty
    }
    Assert-Condition ((Get-JsonString -Object $row -Name "status") -eq "DLQ") "provider replay handoff rows must be DLQ candidates."
    Assert-Condition ((Get-JsonString -Object $row -Name "replay_state") -eq "AWAITING_ADMIN_WORKFLOW") "provider replay handoff rows must await admin workflow."
}

$html = [System.Text.StringBuilder]::new()
[void]$html.AppendLine("<!doctype html>")
[void]$html.AppendLine("<html lang=`"en`">")
[void]$html.AppendLine("<head>")
[void]$html.AppendLine("<meta charset=`"utf-8`">")
[void]$html.AppendLine("<title>NexusIM Provider Replay Handoff Review</title>")
[void]$html.AppendLine("<style>")
[void]$html.AppendLine("body{font-family:Segoe UI,Arial,sans-serif;margin:24px;color:#17202a;background:#f8fafc;}main{max-width:1240px;margin:auto;background:#fff;border:1px solid #d9e2ec;border-radius:8px;padding:24px;}h1{font-size:24px;margin:0 0 16px;}h2{font-size:18px;margin-top:28px;border-bottom:1px solid #e6edf5;padding-bottom:6px;}table{width:100%;border-collapse:collapse;margin:12px 0;}th,td{text-align:left;border:1px solid #e6edf5;padding:8px;vertical-align:top;}th{background:#f1f5f9;}table.summary th{width:290px;}code{font-family:Consolas,monospace;background:#eef2f7;padding:2px 4px;border-radius:4px;}ul{margin:8px 0 8px 22px;}p.note{background:#fff7ed;border:1px solid #fed7aa;padding:10px;border-radius:6px;}")
[void]$html.AppendLine("</style>")
[void]$html.AppendLine("</head><body><main>")
[void]$html.AppendLine("<h1>NexusIM Provider Replay Handoff Review</h1>")
[void]$html.AppendLine("<p class=`"note`">Static first-stage operator review page. It renders low-sensitive refs and hashes from an action-executor provider replay handoff artifact only. It does not submit admin operations, create workflows, record approvals, call RedriveProviderFailure, execute tools, mutate provider failure rows, or embed raw provider input, provider output, error body, new input, reason text, EvidencePack text, local paths, or credentials.</p>")

Add-SectionHeader -Builder $html -Title "Review Summary"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    page_id = $PageID
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    generated_by = $GeneratedBy
    kind = $kind
    admin_operation_requests = $adminRequests.Count
    workflow_handoff_requests = $workflowRequests.Count
    candidate_rows = $rows.Count
    handoff_sha256 = Get-ProviderReplayFileSha256 -Path $HandoffPath
    handoff_path_sha256 = Get-ProviderReplayStringSha256 -Value ([string](Resolve-Path -LiteralPath $HandoffPath))
})

Add-SectionHeader -Builder $html -Title "Handoff Contract"
Add-SimpleTable -Builder $html -Rows ([ordered]@{
    admin_operation_type = $contract.admin_operation_type
    workflow_type = $contract.workflow_type
    target_service = $contract.target_service
    target_operation = $contract.target_operation
    redrive_entrypoint = $contract.redrive_entrypoint
    approval_policy_ref = $contract.approval_policy_ref
    payload_schema_version = $contract.payload_schema_version
    direct_execution_allowed = [bool]$contract.direct_execution_allowed
    source_dlq_immutable = [bool]$contract.source_dlq_immutable
})
Add-List -Builder $html -Values @(Get-JsonArray -Object $contract -Name "requires") -EmptyText "No required gates were recorded."

Add-SectionHeader -Builder $html -Title "Admin Operation Requests"
[void]$html.AppendLine("<table>")
[void]$html.AppendLine("<tr><th>tenant</th><th>operator_ref</th><th>operation_type</th><th>target_ref_hash</th><th>risk</th><th>payload_hash</th><th>reason_ref</th><th>idempotency_key</th><th>expected_policy</th></tr>")
foreach ($request in $adminRequests) {
    [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $request.auth_tenant_id)</td><td>$(ConvertTo-HtmlText $request.operator_ref)</td><td>$(ConvertTo-HtmlText $request.operation_type)</td><td>$(ConvertTo-HtmlText $request.target_ref_hash)</td><td>$(ConvertTo-HtmlText $request.risk_level)</td><td>$(ConvertTo-HtmlText $request.operation_payload_hash)</td><td>$(ConvertTo-HtmlText $request.reason_ref)</td><td>$(ConvertTo-HtmlText $request.idempotency_key)</td><td>$(ConvertTo-HtmlText $request.expected_workflow_policy)</td></tr>")
}
[void]$html.AppendLine("</table>")

Add-SectionHeader -Builder $html -Title "Workflow Handoff Requests"
if ($workflowRequests.Count -eq 0) {
    [void]$html.AppendLine("<p>No workflow handoff requests were recorded.</p>")
} else {
    [void]$html.AppendLine("<table>")
    [void]$html.AppendLine("<tr><th>workflow_type</th><th>requester_service</th><th>target_service</th><th>target_operation</th><th>risk</th><th>target_ref_hash</th><th>payload_ref_hash</th><th>approval_policy_ref</th><th>reason_ref</th></tr>")
    foreach ($request in $workflowRequests) {
        [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $request.workflow_type)</td><td>$(ConvertTo-HtmlText $request.requester_service)</td><td>$(ConvertTo-HtmlText $request.target_service)</td><td>$(ConvertTo-HtmlText $request.target_operation)</td><td>$(ConvertTo-HtmlText $request.risk_level)</td><td>$(ConvertTo-HtmlText $request.target_ref_hash)</td><td>$(ConvertTo-HtmlText $request.payload_ref_hash)</td><td>$(ConvertTo-HtmlText $request.approval_policy_ref)</td><td>$(ConvertTo-HtmlText $request.reason_ref)</td></tr>")
    }
    [void]$html.AppendLine("</table>")
}

Add-SectionHeader -Builder $html -Title "Candidate Rows"
if ($rows.Count -eq 0) {
    [void]$html.AppendLine("<p>No candidate rows were recorded.</p>")
} else {
    [void]$html.AppendLine("<table>")
    [void]$html.AppendLine("<tr><th>candidate_id</th><th>state</th><th>tenant</th><th>failure_id</th><th>skill</th><th>tool</th><th>resource_type</th><th>classification</th><th>status</th><th>failure_ref_hash</th></tr>")
    foreach ($row in $rows) {
        [void]$html.AppendLine("<tr><td>$(ConvertTo-HtmlText $row.replay_candidate_id)</td><td>$(ConvertTo-HtmlText $row.replay_state)</td><td>$(ConvertTo-HtmlText $row.tenant_id)</td><td>$(ConvertTo-HtmlText $row.provider_failure_id)</td><td>$(ConvertTo-HtmlText $row.skill_id)</td><td>$(ConvertTo-HtmlText $row.tool_name)</td><td>$(ConvertTo-HtmlText $row.resource_type)</td><td>$(ConvertTo-HtmlText $row.classification)</td><td>$(ConvertTo-HtmlText $row.status)</td><td>$(ConvertTo-HtmlText $row.failure_ref_hash)</td></tr>")
    }
    [void]$html.AppendLine("</table>")
}

Add-SectionHeader -Builder $html -Title "Execution Boundary"
Add-List -Builder $html -Values @(
    "review_page_is_read_only",
    "does_not_submit_admin_operation",
    "does_not_create_workflow",
    "does_not_record_approval",
    "does_not_call_redrive_provider_failure",
    "does_not_mutate_provider_failure_row",
    "requires_fresh_agent_proposal_and_approval"
) -EmptyText "No execution boundary was recorded."

[void]$html.AppendLine("</main></body></html>")
$htmlText = $html.ToString()

$parent = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
}
$htmlText | Set-Content -LiteralPath $OutputPath -Encoding UTF8

Write-Host "OK   provider replay handoff review page written: $OutputPath"
