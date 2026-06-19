param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return ""
    }
    return ([string]$Object.$Name).Trim()
}

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Escape-MarkdownCell {
    param([string]$Value)

    return $Value.Replace("|", "\|").Replace("`r", " ").Replace("`n", " ").Trim()
}

function Assert-LowSensitiveText {
    param(
        [string]$Text,
        [string]$Context
    )

    $patterns = @(
        "sk-[A-Za-z0-9_-]{12,}",
        "Bearer\s+[A-Za-z0-9._-]{12,}",
        "-----BEGIN\s+(RSA\s+)?PRIVATE KEY-----",
        "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}",
        "1[3-9]\d{9}"
    )
    foreach ($pattern in $patterns) {
        Assert-Condition (-not [regex]::IsMatch($Text, $pattern)) "ai eval case contains sensitive-looking text in $Context"
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

$raw = Get-Content -LiteralPath $resolvedCasePath -Raw
Assert-LowSensitiveText -Text $raw -Context $CasePath

$document = $raw | ConvertFrom-Json
Assert-Condition ([int]$document.schema_version -eq 1) "ai eval cases schema_version must be 1."
$scope = Get-JsonPropertyString -Object $document -Name "scope"
Assert-Condition ($scope.Length -gt 0) "ai eval cases scope is required."
Assert-Condition ($scope -match "low-sensitive") "ai eval cases scope must state low-sensitive boundary."
Assert-Condition ($scope -match "not a production") "ai eval cases scope must state non-production boundary."

$knownFamilies = @(
    "retrieval_miss",
    "temporal_version",
    "attribution",
    "permission_leak",
    "profile_overgeneralization",
    "llm_output_safety",
    "python_worker_output_safety",
    "tool_policy_violation",
    "action_execution_safety"
)
$knownStatuses = @("active", "draft")
$knownAssertionTypes = @(
    "must_include_source_ref",
    "must_exclude_source_ref",
    "must_return_source_type",
    "must_not_return_source_type",
    "source_coverage_status",
    "dedupe_reason",
    "rerank_order",
    "must_abstain",
    "must_require_approval",
    "must_deny",
    "answer_status",
    "must_include_citation",
    "must_not_claim_llm_generation",
    "must_not_send_sensitive_prompt",
    "must_fallback_to_extractive",
    "must_reject_unsafe_llm_output",
    "must_reject_malformed_llm_output",
    "must_return_failed_candidate",
    "must_not_return_raw_output",
    "must_record_execution_audit",
    "must_record_tool_result_projection",
    "must_execute_safe_local_tool",
    "must_preserve_evidencepack_source_coverage",
    "must_preserve_projection_versions",
    "must_record_prepare_audit",
    "must_preserve_tool_policy_metadata",
    "must_execute_external_http_tool",
    "must_not_send_raw_tool_input",
    "must_classify_provider_failure",
    "must_reject_unsafe_tool_output",
    "must_not_store_raw_provider_output",
    "must_not_execute_external_tool",
    "must_verify_approved_proposal",
    "must_not_promote_group_fact_to_profile",
    "must_require_multiple_profile_sources",
    "must_mark_profile_candidate_pending",
    "must_preserve_group_scope",
    "must_reject_sensitive_agent_output",
    "must_not_emit_unapproved_action",
    "must_keep_output_low_sensitive",
    "must_require_evidencepack_citations"
)

$seenCaseIDs = @{}
$caseResults = @()
foreach ($case in @($document.cases)) {
    $id = Get-JsonPropertyString -Object $case -Name "id"
    $family = Get-JsonPropertyString -Object $case -Name "family"
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    $query = Get-JsonPropertyString -Object $case -Name "query"
    $risk = Get-JsonPropertyString -Object $case -Name "risk"

    Assert-Condition ($id.Length -gt 0) "ai eval case id is required."
    Assert-Condition (-not $seenCaseIDs.ContainsKey($id)) "duplicate ai eval case id: $id"
    $seenCaseIDs[$id] = $true
    Assert-Condition ($family -in $knownFamilies) "ai eval case $id has unknown family: $family"
    Assert-Condition ($stage.Length -gt 0) "ai eval case $id stage is required."
    Assert-Condition ($status -in $knownStatuses) "ai eval case $id has unknown status: $status"
    Assert-Condition ($query.Length -gt 0 -and $query.Length -le 512) "ai eval case $id query must be 1..512 chars."
    Assert-Condition ($risk.Length -gt 0) "ai eval case $id risk is required."
    Assert-LowSensitiveText -Text $query -Context "case $id query"

    $assertions = @($case.required_assertions)
    Assert-Condition ($assertions.Count -gt 0) "ai eval case $id must include required_assertions."
    $assertionTypes = @()
    foreach ($assertion in $assertions) {
        $assertionType = Get-JsonPropertyString -Object $assertion -Name "type"
        Assert-Condition ($assertionType -in $knownAssertionTypes) "ai eval case $id has unknown assertion type: $assertionType"
        $assertionTypes += $assertionType
        $assertionJson = $assertion | ConvertTo-Json -Depth 8 -Compress
        Assert-LowSensitiveText -Text $assertionJson -Context "case $id assertion $assertionType"
    }

    $caseResults += [pscustomobject]@{
        id = $id
        family = $family
        stage = $stage
        status = $status
        risk = $risk
        assertion_count = $assertions.Count
        assertions = ($assertionTypes -join ",")
    }
}

Assert-Condition ($caseResults.Count -gt 0) "ai eval cases must contain at least one case."

$families = @($caseResults | Group-Object family | Sort-Object Name | ForEach-Object {
    [pscustomobject]@{
        family = $_.Name
        count = $_.Count
    }
})

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    case_path = $resolvedCasePath
    case_count = $caseResults.Count
    family_count = $families.Count
    valid = $true
    scope = "first-stage AI eval case schema validation; low-sensitive synthetic cases; not a production benchmark"
    families = $families
}

if ($MarkdownPath.Trim().Length -gt 0) {
    $resolvedMarkdownPath = Resolve-RepoPath $MarkdownPath
    $markdownDir = Split-Path -Parent $resolvedMarkdownPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM AI Eval Cases")
    $lines.Add("")
    $lines.Add("- Case file: $resolvedCasePath")
    $lines.Add("- Cases: $($caseResults.Count)")
    $lines.Add("- Scope: low-sensitive first-stage eval case validation; not a production benchmark.")
    $lines.Add("")
    $lines.Add("| Case | Family | Stage | Status | Assertions | Risk |")
    $lines.Add("| --- | --- | --- | --- | --- | --- |")
    foreach ($result in $caseResults) {
        $lines.Add("| $(Escape-MarkdownCell $result.id) | $(Escape-MarkdownCell $result.family) | $(Escape-MarkdownCell $result.stage) | $(Escape-MarkdownCell $result.status) | $($result.assertion_count) | $(Escape-MarkdownCell $result.risk) |")
    }
    $lines.Add("")
    $lines.Add("These cases verify schema and expected assertions only. They do not execute model calls or production benchmarks.")
    $lines | Set-Content -LiteralPath $resolvedMarkdownPath -Encoding UTF8
    Write-Host "OK   ai eval cases markdown written: $resolvedMarkdownPath"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   ai eval case validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 6
}
