param(
    [Parameter(Mandatory = $true)]
    [string]$AuditManifestPath,

    [Parameter(Mandatory = $true)]
    [string]$AuditAppendResultPath,

    [Parameter(Mandatory = $true)]
    [string]$GeneratedBy,

    [string]$OutputPath = "",
    [string]$ResultManifestID = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "repair-operator-safety.ps1")

foreach ($pathPair in @(
        @("AuditManifestPath", $AuditManifestPath),
        @("AuditAppendResultPath", $AuditAppendResultPath)
    )) {
    $name = [string]$pathPair[0]
    $path = [string]$pathPair[1]
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing $name`: $path"
    }
    Assert-ExternalRepairOutputPath -Value $path -FieldName $name
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path (Split-Path -Parent ([System.IO.Path]::GetFullPath($AuditAppendResultPath))) "workflow-compensation-execution-audit-append-result-manifest.json"
}
Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ([string]::IsNullOrWhiteSpace($ResultManifestID)) {
    $ResultManifestID = "workflow-compensation-audit-append-result-" + [System.Guid]::NewGuid().ToString("N")
}

Assert-LowSensitiveRepairActor -Value $GeneratedBy -FieldName "GeneratedBy"
Assert-LowSensitiveRepairIdentifier -Value $ResultManifestID -FieldName "ResultManifestID"

function Get-AuditResultFileSha256Ref {
    param([string]$Path)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $Path))))
}

function Get-AuditResultStringSha256Ref {
    param([string]$Value)
    return "sha256:" + (Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($Value)))
}

function Get-JsonDocument {
    param([string]$Path, [string]$Label)
    try {
        return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
    } catch {
        throw "$Label must be valid JSON: $Path"
    }
}

function Get-JsonString {
    param([object]$Object, [string]$Name, [switch]$AllowEmpty)
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
    param([object]$Object, [string]$Name)
    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name] -or $null -eq $Object.$Name) {
        return @()
    }
    return @($Object.$Name)
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

function Assert-NoRawText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ|postgres://|mysql://|mongodb://|raw:|payload_body|message_body|provider_body|provider_error|reason_text|EvidencePack|prompt|local_path|filesystem_path|input_json)") {
        throw "$FieldName contains raw, secret, prompt, local path, provider artifact, or credential-like content."
    }
}

function Assert-NoRawDocumentText {
    param([string]$Value, [string]$FieldName)
    if ($Value -match '(?i)(bearer\s+\S+|password\s*[:=]|secret\s*[:=]|api[_-]?key\s*[:=]|access[_-]?key\s*[:=]|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.|postgres://|mysql://|mongodb://|"provider_body"\s*:|"provider_error_body"\s*:|"message_body"\s*:|"raw_payload"\s*:|"payload_body"\s*:|"input_json"\s*:|"reason_text"\s*:)') {
        throw "$FieldName contains raw, secret, prompt, provider artifact, or credential-like content."
    }
}

function Assert-LowString {
    param([string]$Value, [string]$FieldName, [switch]$AllowEmpty)
    Assert-LowSensitiveRepairIdentifier -Value $Value -FieldName $FieldName -AllowEmpty:$AllowEmpty
    Assert-NoRawText -Value $Value -FieldName $FieldName
}

function Assert-LowRef {
    param([string]$Value, [string]$FieldName, [switch]$AllowEmpty)
    $text = ([string]$Value).Trim()
    if ($text.Length -eq 0) {
        if ($AllowEmpty) {
            return
        }
        throw "$FieldName is required."
    }
    if ($text.Length -gt 256 -or $text -notmatch "^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,255}$") {
        throw "$FieldName must be a low-sensitive ref using letters, digits, dot, underscore, dash, colon, or slash."
    }
    Assert-NoRawText -Value $text -FieldName $FieldName
}

function Assert-Sha256Ref {
    param([string]$Value, [string]$FieldName)
    if ($Value -notmatch "^sha256:[a-f0-9]{64}$") {
        throw "$FieldName must be sha256:<hex>."
    }
}

function Assert-ArrayContains {
    param([object[]]$Values, [string]$Expected, [string]$FieldName)
    if (@($Values) -notcontains $Expected) {
        throw "$FieldName must contain $Expected."
    }
}

function Assert-Same {
    param([string]$Actual, [string]$Expected, [string]$FieldName)
    if ($Actual -ne $Expected) {
        throw "$FieldName mismatch."
    }
}

function ConvertTo-CanonicalFlatJson {
    param([object]$Object)
    $parts = @()
    foreach ($property in @($Object.PSObject.Properties | Sort-Object Name)) {
        $encodedKey = ConvertTo-Json ([string]$property.Name) -Compress
        $encodedValue = ConvertTo-Json ([string]$property.Value) -Compress
        $parts += ("{0}:{1}" -f $encodedKey, $encodedValue)
    }
    return "{" + ($parts -join ",") + "}"
}

$auditManifestRaw = Get-Content -LiteralPath $AuditManifestPath -Raw
$auditResultRaw = Get-Content -LiteralPath $AuditAppendResultPath -Raw
Assert-NoRawDocumentText -Value $auditManifestRaw -FieldName "AuditManifestPath"
Assert-NoRawDocumentText -Value $auditResultRaw -FieldName "AuditAppendResultPath"

$auditManifest = Get-JsonDocument -Path $AuditManifestPath -Label "Workflow compensation audit append manifest"
$auditResult = Get-JsonDocument -Path $AuditAppendResultPath -Label "Workflow compensation audit append execution summary"

Assert-True ((Get-JsonString -Object $auditManifest -Name "schema_version") -eq "nexusim.audit.external_append.v1") "Unsupported audit append manifest schema_version."
Assert-False ([bool]$auditManifest.executes_append) "Audit manifest must not claim it executes append."
Assert-False ([bool]$auditManifest.mutates_audit_service) "Audit manifest must not mutate audit-service."
Assert-False ([bool]$auditManifest.direct_append_allowed) "Audit manifest direct_append_allowed must be false."
Assert-True ([bool]$auditManifest.requires_operator_execution) "Audit manifest must require operator execution."

$manifestID = Get-JsonString -Object $auditManifest -Name "manifest_id"
$sourceManifestID = Get-JsonString -Object $auditManifest -Name "source_manifest_id"
$auditStream = Get-JsonString -Object $auditManifest -Name "audit_stream"
$sourceService = Get-JsonString -Object $auditManifest -Name "source_service"
$sourceEventID = Get-JsonString -Object $auditManifest -Name "source_event_id"
$recordType = Get-JsonString -Object $auditManifest -Name "record_type"
$resourceRef = Get-JsonString -Object $auditManifest -Name "resource_ref"
$action = Get-JsonString -Object $auditManifest -Name "action"
$outcome = Get-JsonString -Object $auditManifest -Name "outcome"
$reasonCode = Get-JsonString -Object $auditManifest -Name "reason_code"
$riskLevel = Get-JsonString -Object $auditManifest -Name "risk_level"
$attributesSHA256 = Get-JsonString -Object $auditManifest -Name "attributes_sha256"
$idempotencyKey = Get-JsonString -Object $auditManifest -Name "idempotency_key"
$correlationID = Get-JsonString -Object $auditManifest -Name "correlation_id" -AllowEmpty
$causationID = Get-JsonString -Object $auditManifest -Name "causation_id" -AllowEmpty
$traceID = Get-JsonString -Object $auditManifest -Name "trace_id" -AllowEmpty
$authContract = $auditManifest.auth_context_contract
if ($null -eq $authContract) {
    throw "audit_manifest.auth_context_contract is required."
}
$tenantID = Get-JsonString -Object $authContract -Name "tenant_id"

Assert-Same -Actual $sourceService -Expected "workflow-service" -FieldName "audit_manifest.source_service"
Assert-Same -Actual $recordType -Expected "WORKFLOW_COMPENSATION_EXECUTION" -FieldName "audit_manifest.record_type"
Assert-Same -Actual $action -Expected "EXECUTE_COMPENSATION" -FieldName "audit_manifest.action"
Assert-True (($outcome -eq "SUCCEEDED" -or $outcome -eq "FAILED")) "audit_manifest.outcome must be terminal."
Assert-Sha256Ref -Value $attributesSHA256 -FieldName "audit_manifest.attributes_sha256"

$attributesCompact = ConvertTo-CanonicalFlatJson -Object $auditManifest.attributes_json
$computedAttributesSHA256 = Get-AuditResultStringSha256Ref -Value $attributesCompact
Assert-Same -Actual $computedAttributesSHA256 -Expected $attributesSHA256 -FieldName "audit_manifest.attributes_json_sha256"

foreach ($expected in @(
        "source_compensation_result_manifest_verified",
        "workflow_compensation_result_low_sensitive",
        "no_raw_compensation_payload",
        "audit_service_append_only",
        "idempotency_key_present"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $auditManifest -Name "required_checks") -Expected $expected -FieldName "audit_manifest.required_checks"
}
foreach ($expected in @(
        "audit_manifest_is_not_audit_append_execution",
        "does_not_call_audit_service",
        "does_not_execute_compensation",
        "does_not_record_workflow_decision",
        "does_not_modify_workflow_or_compensation_rows",
        "does_not_call_downstream_service"
    )) {
    Assert-ArrayContains -Values @(Get-JsonArray -Object $auditManifest -Name "execution_boundary") -Expected $expected -FieldName "audit_manifest.execution_boundary"
}

Assert-True ((Get-JsonString -Object $auditResult -Name "mode") -eq "external-audit-append") "Audit append summary mode must be external-audit-append."
Assert-Same -Actual (Get-JsonString -Object $auditResult -Name "manifest_id") -Expected $manifestID -FieldName "audit_result.manifest_id"
Assert-Same -Actual (Get-JsonString -Object $auditResult -Name "source_manifest_id") -Expected $sourceManifestID -FieldName "audit_result.source_manifest_id"
Assert-True ([bool]$auditResult.executed_append) "Audit append summary must have executed_append=true."

$request = $auditResult.request
$response = $auditResult.response
if ($null -eq $request) {
    throw "audit_result.request is required."
}
if ($null -eq $response) {
    throw "audit_result.response is required."
}

Assert-Same -Actual (Get-JsonString -Object $request -Name "tenant_id") -Expected $tenantID -FieldName "request.tenant_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "audit_stream") -Expected $auditStream -FieldName "request.audit_stream"
Assert-Same -Actual (Get-JsonString -Object $request -Name "source_service") -Expected $sourceService -FieldName "request.source_service"
Assert-Same -Actual (Get-JsonString -Object $request -Name "source_event_id") -Expected $sourceEventID -FieldName "request.source_event_id"
Assert-Same -Actual (Get-JsonString -Object $request -Name "record_type") -Expected $recordType -FieldName "request.record_type"
Assert-Same -Actual (Get-JsonString -Object $request -Name "resource_ref") -Expected $resourceRef -FieldName "request.resource_ref"
Assert-Same -Actual (Get-JsonString -Object $request -Name "action") -Expected $action -FieldName "request.action"
Assert-Same -Actual (Get-JsonString -Object $request -Name "outcome") -Expected $outcome -FieldName "request.outcome"
Assert-Same -Actual (Get-JsonString -Object $request -Name "reason_code") -Expected $reasonCode -FieldName "request.reason_code"
Assert-Same -Actual (Get-JsonString -Object $request -Name "risk_level") -Expected $riskLevel -FieldName "request.risk_level"
Assert-Same -Actual (Get-JsonString -Object $request -Name "attributes_sha256") -Expected $attributesSHA256 -FieldName "request.attributes_sha256"
Assert-Same -Actual (Get-JsonString -Object $request -Name "idempotency_key") -Expected $idempotencyKey -FieldName "request.idempotency_key"

$operatorUserRef = Get-JsonString -Object $request -Name "user_id"
$operatorDeviceRef = Get-JsonString -Object $request -Name "device_id"
$auditID = Get-JsonString -Object $response -Name "audit_id"
$recordHash = Get-JsonString -Object $response -Name "record_hash"
$previousRecordHash = Get-JsonString -Object $response -Name "previous_record_hash" -AllowEmpty
$responseIdempotencyKey = Get-JsonString -Object $response -Name "idempotency_key"
Assert-Same -Actual $responseIdempotencyKey -Expected $idempotencyKey -FieldName "response.idempotency_key"

foreach ($entry in @(
        @{ name = "manifest_id"; value = $manifestID; allow = $false },
        @{ name = "source_manifest_id"; value = $sourceManifestID; allow = $false },
        @{ name = "tenant_id"; value = $tenantID; allow = $false },
        @{ name = "audit_stream"; value = $auditStream; allow = $false },
        @{ name = "source_service"; value = $sourceService; allow = $false },
        @{ name = "source_event_id"; value = $sourceEventID; allow = $false },
        @{ name = "record_type"; value = $recordType; allow = $false },
        @{ name = "action"; value = $action; allow = $false },
        @{ name = "outcome"; value = $outcome; allow = $false },
        @{ name = "reason_code"; value = $reasonCode; allow = $false },
        @{ name = "risk_level"; value = $riskLevel; allow = $false },
        @{ name = "idempotency_key"; value = $idempotencyKey; allow = $false },
        @{ name = "correlation_id"; value = $correlationID; allow = $true },
        @{ name = "causation_id"; value = $causationID; allow = $true },
        @{ name = "trace_id"; value = $traceID; allow = $true },
        @{ name = "operator_user_ref"; value = $operatorUserRef; allow = $false },
        @{ name = "operator_device_ref"; value = $operatorDeviceRef; allow = $false },
        @{ name = "audit_id"; value = $auditID; allow = $false },
        @{ name = "record_hash"; value = $recordHash; allow = $false },
        @{ name = "previous_record_hash"; value = $previousRecordHash; allow = $true }
    )) {
    Assert-LowString -Value ([string]$entry.value) -FieldName ([string]$entry.name) -AllowEmpty:([bool]$entry.allow)
}
Assert-LowRef -Value $resourceRef -FieldName "resource_ref"
Assert-Sha256Ref -Value $attributesSHA256 -FieldName "attributes_sha256"

$sourceAuditManifestSHA256 = Get-AuditResultFileSha256Ref -Path $AuditManifestPath
$sourceAuditAppendSummarySHA256 = Get-AuditResultFileSha256Ref -Path $AuditAppendResultPath

$manifest = [ordered]@{
    schema_version = "nexusim.workflow.compensation_audit_append_result.v1"
    result_manifest_id = $ResultManifestID
    generated_at = [DateTime]::UtcNow.ToString("o")
    generated_by = $GeneratedBy
    source_audit_manifest_sha256 = $sourceAuditManifestSHA256
    source_audit_append_summary_sha256 = $sourceAuditAppendSummarySHA256
    source_audit_manifest_path_sha256 = Get-AuditResultStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $AuditManifestPath))
    source_audit_append_summary_path_sha256 = Get-AuditResultStringSha256Ref -Value ([string](Resolve-Path -LiteralPath $AuditAppendResultPath))
    manifest_is_execution = $false
    executes_append = $false
    mutates_audit_service = $false
    executes_compensation = $false
    records_decision = $false
    calls_downstream_service = $false
    audit_append_manifest = [ordered]@{
        manifest_id = $manifestID
        source_manifest_id = $sourceManifestID
        tenant_id = $tenantID
        audit_stream = $auditStream
        source_service = $sourceService
        source_event_id = $sourceEventID
        record_type = $recordType
        resource_ref = $resourceRef
        action = $action
        outcome = $outcome
        reason_code = $reasonCode
        risk_level = $riskLevel
        attributes_sha256 = $attributesSHA256
        idempotency_key = $idempotencyKey
        correlation_id = $correlationID
        causation_id = $causationID
        trace_id = $traceID
    }
    audit_append_request = [ordered]@{
        operator_user_ref = $operatorUserRef
        operator_device_ref = $operatorDeviceRef
        tenant_id = $tenantID
        source_service = $sourceService
        source_event_id = $sourceEventID
        record_type = $recordType
        resource_ref = $resourceRef
        action = $action
        outcome = $outcome
        attributes_sha256 = $attributesSHA256
        idempotency_key = $idempotencyKey
    }
    audit_append_result = [ordered]@{
        audit_id = $auditID
        record_hash = $recordHash
        previous_record_hash = $previousRecordHash
        idempotency_key = $responseIdempotencyKey
        executed_append = $true
    }
    required_checks = @(
        "source_audit_append_manifest_verified",
        "audit_append_summary_matches_manifest",
        "audit_service_reported_audit_record",
        "workflow_compensation_result_manifest_bound_by_sha256",
        "workflow_compensation_audit_low_sensitive",
        "operator_keeps_result_manifest_external"
    )
    forbidden_contents = @(
        "raw_payload",
        "payload_body",
        "provider_body",
        "provider_error_body",
        "reason_text",
        "evidence_payload",
        "input_json",
        "attributes_json",
        "filesystem_path_material",
        "auth_material"
    )
    execution_boundary = @(
        "result_manifest_is_not_audit_append_execution",
        "does_not_call_audit_service",
        "does_not_execute_compensation",
        "does_not_record_workflow_decision",
        "does_not_modify_workflow_or_compensation_rows",
        "does_not_call_downstream_service"
    )
    note = "Low-sensitive workflow compensation audit append result manifest. It binds an executed external audit append summary to its manifest but does not append audit, execute compensation, record decisions, call downstream services, or embed raw payloads."
}

$encoded = $manifest | ConvertTo-Json -Depth 30 -Compress
Assert-NoRawDocumentText -Value $encoded -FieldName "workflow compensation audit append result manifest"

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($OutputPath, ($manifest | ConvertTo-Json -Depth 30), $utf8NoBom)

Write-Host "OK   workflow compensation audit append result manifest written: $OutputPath"
