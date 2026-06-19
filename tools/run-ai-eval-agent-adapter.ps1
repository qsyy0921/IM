param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$AgentTarget = "127.0.0.1:10630",
    [string]$ActionExecutorTarget = "127.0.0.1:10660",
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

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Get-ExpectedSourceCount {
    param(
        $Summary,
        [string]$SourceType
    )

    switch ($SourceType) {
        "SEARCH_MESSAGE" { return [int]$Summary.source_counts.search_message }
        "MEMORY_EVENT" { return [int]$Summary.source_counts.memory_event }
        "MESSAGE" { return [int]$Summary.citation_count }
        default { throw "unsupported agent eval source_type: $SourceType" }
    }
}

function Test-AgentAssertion {
    param(
        $Summary,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "must_require_approval" {
            return [bool]$Summary.requires_approval -and [bool]$Summary.policy_requires_approval
        }
        "must_include_citation" {
            $sourceType = Get-JsonPropertyString -Object $Assertion -Name "source_type"
            if ($sourceType.Length -eq 0) {
                return [int]$Summary.citation_count -gt 0
            }
            return (Get-ExpectedSourceCount -Summary $Summary -SourceType $sourceType) -gt 0
        }
        "must_return_source_type" {
            $sourceType = Get-JsonPropertyString -Object $Assertion -Name "source_type"
            return (Get-ExpectedSourceCount -Summary $Summary -SourceType $sourceType) -gt 0
        }
        "must_not_claim_llm_generation" {
            return -not [bool]$Summary.generated_by_llm
        }
        "must_preserve_evidencepack_source_coverage" {
            $searchCount = [int]$Summary.source_counts.search_message
            $memoryCount = [int]$Summary.source_counts.memory_event
            return `
                (Get-JsonPropertyString -Object $Summary -Name "pack_id").Length -gt 0 `
                -and [int]$Summary.evidence_item_count -eq ($searchCount + $memoryCount) `
                -and [int]$Summary.search_item_count -eq $searchCount `
                -and [int]$Summary.memory_item_count -eq $memoryCount `
                -and $searchCount -gt 0 `
                -and $memoryCount -gt 0
        }
        "must_preserve_projection_versions" {
            return `
                [int64]$Summary.search_projection_version -gt 0 `
                -and [int64]$Summary.memory_projection_version -gt 0 `
                -and [int64]$Summary.search_projection_version -eq [int64]$Summary.seed.visibility_version `
                -and [int64]$Summary.memory_projection_version -eq [int64]$Summary.seed.memory_projection_version
        }
        "must_preserve_agent_retrieval_versions" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "agent_version").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary -Name "retrieval_version").Length -gt 0
        }
        "must_keep_proposal_only_before_execution" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "proposal_status") -eq "PROPOSED" `
                -and [bool]$Summary.requires_approval `
                -and (-not [bool]$Summary.generated_by_llm) `
                -and (-not [bool]$Summary.execution_executed)
        }
        "must_record_prepare_audit" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "prepared_audit_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.mcp_audit -Name "audit_id") -eq (Get-JsonPropertyString -Object $Summary -Name "prepared_audit_id") `
                -and (Get-JsonPropertyString -Object $Summary.mcp_audit -Name "status") -eq "ALLOWED" `
                -and [bool]$Summary.mcp_audit.allowed `
                -and [bool]$Summary.mcp_audit.requires_approval `
                -and [bool]$Summary.mcp_audit.input_sha256_present `
                -and [bool]$Summary.mcp_audit.idempotency_key_matches `
                -and [bool]$Summary.mcp_audit.low_sensitive_audit_only
        }
        "must_preserve_tool_policy_metadata" {
            return `
                [bool]$Summary.policy_allowed `
                -and [bool]$Summary.policy_requires_approval `
                -and [int64]$Summary.policy_permission_version -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary -Name "policy_decision_source") -eq "TOOL_RULE" `
                -and (Get-JsonPropertyString -Object $Summary -Name "policy_classification") -eq "TOOL_APPROVAL_REQUIRED" `
                -and (Get-JsonPropertyString -Object $Summary.mcp_audit -Name "decision_source") -eq (Get-JsonPropertyString -Object $Summary -Name "policy_decision_source") `
                -and (Get-JsonPropertyString -Object $Summary.mcp_audit -Name "classification") -eq (Get-JsonPropertyString -Object $Summary -Name "policy_classification") `
                -and [int64]$Summary.mcp_audit.permission_version -eq [int64]$Summary.policy_permission_version `
                -and (Get-JsonPropertyString -Object $Summary.action_audit -Name "decision_source") -eq (Get-JsonPropertyString -Object $Summary -Name "execution_decision_source") `
                -and (Get-JsonPropertyString -Object $Summary.action_audit -Name "classification") -eq (Get-JsonPropertyString -Object $Summary -Name "execution_classification")
        }
        "must_record_tool_payload_hash_only" {
            return `
                [bool]$Summary.mcp_audit.input_sha256_present `
                -and [bool]$Summary.mcp_audit.raw_input_column_absent `
                -and [bool]$Summary.action_audit.input_sha256_present `
                -and [bool]$Summary.action_audit.raw_input_column_absent `
                -and [bool]$Summary.action_audit.raw_output_column_absent `
                -and [bool]$Summary.action_audit.result_raw_output_column_absent `
                -and [bool]$Summary.action_audit.output_sha256_present `
                -and [bool]$Summary.action_audit.low_sensitive_audit_only
        }
        "must_verify_approved_proposal" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "approval_id").Length -gt 0 `
                -and [bool]$Summary.action_audit.proposal_id_matches `
                -and [bool]$Summary.action_audit.approval_id_matches `
                -and [bool]$Summary.action_audit.prepared_audit_id_matches
        }
        "must_record_execution_audit" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "execution_status") -eq "RECORDED" `
                -and (Get-JsonPropertyString -Object $Summary.action_audit -Name "status") -eq "RECORDED" `
                -and [bool]$Summary.action_audit.low_sensitive_audit_only
        }
        "must_record_tool_result_projection" {
            $expectedStatus = Get-JsonPropertyString -Object $Assertion -Name "expected_status"
            if ($expectedStatus.Length -eq 0) {
                $expectedStatus = "NOT_EXECUTED"
            }
            return `
                (Get-JsonPropertyString -Object $Summary -Name "execution_result_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary -Name "execution_result_status") -eq $expectedStatus `
                -and (Get-JsonPropertyString -Object $Summary -Name "execution_result_ref").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.action_audit -Name "result_status") -eq $expectedStatus `
                -and [bool]$Summary.action_audit.result_ref_present `
                -and [bool]$Summary.action_audit.result_execution_matches
        }
        "must_execute_safe_local_tool" {
            return `
                [bool]$Summary.execution_executed `
                -and [bool]$Summary.action_audit.executed `
                -and [bool]$Summary.action_audit.output_sha256_present `
                -and (Get-JsonPropertyString -Object $Summary -Name "tool_name") -eq "nexusim.local.echo" `
                -and (Get-JsonPropertyString -Object $Summary -Name "execution_result_status") -eq "SUCCEEDED"
        }
        "must_not_execute_external_tool" {
            return (-not [bool]$Summary.execution_executed) -and (-not [bool]$Summary.action_audit.executed)
        }
        default {
            throw "unsupported agent eval assertion type: $type"
        }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "agent-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

function Invoke-AgentSmokeRun {
    param(
        [string]$SmokeRunName,
        [string[]]$ExtraArgs = @()
    )

    $runArgs = @(
        "run", "./loadtest/agent",
        "-pg-dsn", $PGDSN,
        "-agent-target", $AgentTarget,
        "-action-executor-target", $ActionExecutorTarget,
        "-result-root", $ResultRoot,
        "-run-name", $SmokeRunName,
        "-objective", "phoenix launch decision",
        "-request-timeout", $RequestTimeout
    )
    $runArgs += $ExtraArgs

    Push-Location $repoRoot
    try {
        & go @runArgs
        if ($LASTEXITCODE -ne 0) {
            throw "loadtest/agent failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    $resultDir = Join-Path $ResultRoot $SmokeRunName
    $summaryPath = Join-Path $resultDir "agent-proposal-summary.json"
    Assert-Condition (Test-Path -LiteralPath $summaryPath -PathType Leaf) "Agent smoke summary missing: $summaryPath"
    return [pscustomobject]@{
        run_name = $SmokeRunName
        result_dir = $resultDir
        summary_path = $summaryPath
        summary = (Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json)
    }
}

$defaultRun = Invoke-AgentSmokeRun -SmokeRunName $RunName
$summary = $defaultRun.summary
$resultDir = $defaultRun.result_dir
$smokeSummaryPath = $defaultRun.summary_path
$safeToolRun = $null

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -notin @("agent-service", "action-executor") -or $status -ne "active") {
        continue
    }

    $caseSummary = $summary
    $needsSafeToolRun = $false
    foreach ($assertion in @($case.required_assertions)) {
        if ((Get-JsonPropertyString -Object $assertion -Name "type") -eq "must_execute_safe_local_tool") {
            $needsSafeToolRun = $true
        }
    }
    if ($needsSafeToolRun) {
        if ($null -eq $safeToolRun) {
            $safeToolRun = Invoke-AgentSmokeRun -SmokeRunName ($RunName + "-safe-local-tool") -ExtraArgs @(
                "-tool-name", "nexusim.local.echo",
                "-skill-id", "nexusim.local.echo",
                "-resource-type", "diagnostic",
                "-risk-level", "LOW",
                "-expect-executed"
            )
        }
        $caseSummary = $safeToolRun.summary
    }
    $caseSmokeRunName = $defaultRun.run_name
    if ($needsSafeToolRun) {
        $caseSmokeRunName = $safeToolRun.run_name
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-AgentAssertion -Summary $caseSummary -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "Agent eval assertion failed for case $($case.id): $type"
    }

    $caseResults.Add([pscustomobject]@{
        id = $case.id
        family = $case.family
        stage = $stage
        status = $status
        passed = $true
        smoke_run_name = $caseSmokeRunName
        assertions = $assertionResults
    })
}

Assert-Condition ($caseResults.Count -gt 0) "No active agent/action-executor eval cases found in $resolvedCasePath"
$safeToolRunName = ""
$safeToolSummaryPath = ""
if ($null -ne $safeToolRun) {
    $safeToolRunName = $safeToolRun.run_name
    $safeToolSummaryPath = $safeToolRun.summary_path
}

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "agent-action-executor"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage Agent execution eval adapter; executes loadtest/agent against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    safe_tool_run_name = $safeToolRunName
    safe_tool_summary_path = $safeToolSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "agent-eval-adapter-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   Agent eval adapter summary written: $resolvedOutputPath"
