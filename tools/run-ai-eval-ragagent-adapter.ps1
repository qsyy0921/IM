param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$MemoryTarget = "127.0.0.1:10580",
    [string]$RAGTarget = "127.0.0.1:10610",
    [string]$AgentTarget = "127.0.0.1:10630",
    [string]$ActionExecutorTarget = "127.0.0.1:10660",
    [string]$WorkflowTarget = "127.0.0.1:10750",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = "",
    [string]$RequestTimeout = "10s"
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

function Get-JsonPropertyBool {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $false
    }
    return [System.Convert]::ToBoolean($Object.$Name)
}

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Test-NoRawTextFields {
    param($Summary)

    foreach ($name in @("answer_text", "proposal_text", "raw_answer", "raw_proposal", "raw_model_output")) {
        if ($null -ne $Summary.PSObject.Properties[$name]) {
            return $false
        }
    }
    return $true
}

function Add-Assertion {
    param(
        [System.Collections.Generic.List[object]]$Assertions,
        [string]$Type,
        [bool]$Passed
    )

    $Assertions.Add([pscustomobject]@{
        type = $Type
        passed = $Passed
    })
    Assert-Condition $Passed "RAG-Agent demo eval assertion failed: $Type"
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "rag-agent-demo-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/ragagent",
    "-pg-dsn", $PGDSN,
    "-memory-target", $MemoryTarget,
    "-rag-target", $RAGTarget,
    "-agent-target", $AgentTarget,
    "-action-executor-target", $ActionExecutorTarget,
    "-workflow-target", $WorkflowTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-question", "phoenix launch decision",
    "-objective", "phoenix launch decision",
    "-request-timeout", $RequestTimeout
)

Push-Location $repoRoot
try {
    & go @runArgs
    if ($LASTEXITCODE -ne 0) {
        throw "loadtest/ragagent failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$resultDir = Join-Path $ResultRoot $RunName
$smokeSummaryPath = Join-Path $resultDir "rag-agent-demo-summary.json"
Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "RAG-Agent demo summary missing: $smokeSummaryPath"
$summary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json

$assertions = New-Object System.Collections.Generic.List[object]
Add-Assertion -Assertions $assertions -Type "rag_must_answer_grounded" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "rag_answered") -and
    (Get-JsonPropertyString -Object $summary -Name "rag_answer_status") -eq "GROUNDED" -and
    [int]$summary.rag_citation_count -gt 0 -and
    [int]$summary.rag_evidence_item_count -gt 0
)
Add-Assertion -Assertions $assertions -Type "agent_must_record_approved_proposal" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "agent_proposal_created") -and
    (Get-JsonPropertyString -Object $summary -Name "agent_proposal_status") -eq "APPROVED" -and
    (Get-JsonPropertyBool -Object $summary -Name "agent_requires_approval") -and
    (Get-JsonPropertyBool -Object $summary -Name "agent_approval_recorded")
)
Add-Assertion -Assertions $assertions -Type "action_executor_must_record_audit" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "action_execution_recorded") -and
    (Get-JsonPropertyString -Object $summary -Name "action_execution_status") -eq "RECORDED"
)
Add-Assertion -Assertions $assertions -Type "rag_and_agent_must_share_scope" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "shared_tenant_and_conversation") -and
    (Get-JsonPropertyString -Object $summary -Name "tenant_id").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "conversation_id").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "viewer_user_id").Length -gt 0
)
Add-Assertion -Assertions $assertions -Type "evidencepack_must_preserve_collaborative_memory" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "cross_group_source_refs_preserved") -and
    (Get-JsonPropertyBool -Object $summary -Name "cross_group_speaker_attribution_preserved") -and
    (Get-JsonPropertyBool -Object $summary -Name "memory_graph_edges_preserved") -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_aggregate_preserved")
)
Add-Assertion -Assertions $assertions -Type "public_candidate_review_must_enter_rag_agent_evidence_chain" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "public_candidate_review_approved") -and
    (Get-JsonPropertyBool -Object $summary -Name "public_candidate_evidence_in_rag") -and
    (Get-JsonPropertyBool -Object $summary -Name "public_candidate_evidence_in_agent") -and
    (Get-JsonPropertyBool -Object $summary -Name "public_candidate_temporal_update_preserved") -and
    (Get-JsonPropertyString -Object $summary -Name "public_candidate_memory_event_id").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "public_candidate_superseded_memory_event_id").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "public_candidate_fact_sha256").Length -gt 0
)
Add-Assertion -Assertions $assertions -Type "profile_repair_must_require_workflow_approval_and_enter_evidence_chain" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_approval_requested") -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_workflow_approved") -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_approval_verified") -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_executed") -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_profile_active") -and
    [int]$summary.profile_repair_support_count -ge 2 -and
    [int]$summary.profile_repair_supporting_memory_count -ge 2 -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_evidence_in_rag") -and
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_evidence_in_agent") -and
    (Get-JsonPropertyString -Object $summary -Name "profile_repair_workflow_id").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "profile_repair_payload_ref_hash").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "profile_repair_target_ref_hash").Length -gt 0
)
Add-Assertion -Assertions $assertions -Type "profile_repair_negative_gate_must_fail_closed" -Passed (
    (Get-JsonPropertyBool -Object $summary -Name "profile_repair_negative_cases_verified") -and
    [int]$summary.profile_repair_negative_case_count -ge 2 -and
    $null -ne $summary.profile_repair_negative_cases -and
    @($summary.profile_repair_negative_cases | Where-Object { -not [bool]$_.passed }).Count -eq 0
)
Add-Assertion -Assertions $assertions -Type "summary_must_be_low_sensitive" -Passed (
    (Get-JsonPropertyString -Object $summary -Name "rag_answer_text_sha256").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "agent_proposal_text_sha256").Length -gt 0 -and
    (Test-NoRawTextFields -Summary $summary)
)
Add-Assertion -Assertions $assertions -Type "versions_must_be_present" -Passed (
    (Get-JsonPropertyString -Object $summary -Name "rag_version").Length -gt 0 -and
    (Get-JsonPropertyString -Object $summary -Name "agent_version").Length -gt 0 -and
    $null -ne $summary.retrieval_versions -and
    @($summary.retrieval_versions).Count -gt 0
)

$case = [pscustomobject]@{
    id = "rag_agent_demo_end_to_end"
    family = "collaborative_memory_agent_demo"
    stage = "rag-agent-demo"
    status = "active"
    passed = $true
    smoke_run_name = $RunName
    assertions = $assertions
}

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "rag-agent-demo"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage RAG-Agent demo eval adapter; executes loadtest/ragagent against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = 1
    cases = @($case)
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "rag-agent-demo-eval-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   RAG-Agent demo eval adapter summary written: $resolvedOutputPath"
