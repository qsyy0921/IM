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

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "agent-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/agent",
    "-pg-dsn", $PGDSN,
    "-agent-target", $AgentTarget,
    "-action-executor-target", $ActionExecutorTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-objective", "phoenix launch decision",
    "-request-timeout", $RequestTimeout
)

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

$resultDir = Join-Path $ResultRoot $RunName
$smokeSummaryPath = Join-Path $resultDir "agent-proposal-summary.json"
Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "Agent smoke summary missing: $smokeSummaryPath"
$summary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -notin @("agent-service", "action-executor") -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-AgentAssertion -Summary $summary -Assertion $assertion
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
        assertions = $assertionResults
    })
}

Assert-Condition ($caseResults.Count -gt 0) "No active agent/action-executor eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "agent-action-executor"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage Agent execution eval adapter; executes loadtest/agent against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
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
