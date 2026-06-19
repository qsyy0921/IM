param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$RAGTarget = "127.0.0.1:10610",
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
        default { throw "unsupported rag eval source_type: $SourceType" }
    }
}

function Test-RAGAssertion {
    param(
        $Summary,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "answer_status" {
            $expected = Get-JsonPropertyString -Object $Assertion -Name "expected_status"
            Assert-Condition ($expected.Length -gt 0) "answer_status assertion requires expected_status"
            return $Summary.answer_status -eq $expected
        }
        "must_include_citation" {
            return [int]$Summary.citation_count -gt 0
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
        "must_abstain" {
            return $Summary.answer_status -eq "INSUFFICIENT_EVIDENCE"
        }
        default {
            throw "unsupported rag eval assertion type: $type"
        }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "rag-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/rag",
    "-pg-dsn", $PGDSN,
    "-rag-target", $RAGTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-question", "phoenix launch decision",
    "-request-timeout", $RequestTimeout
)

Push-Location $repoRoot
try {
    & go @runArgs
    if ($LASTEXITCODE -ne 0) {
        throw "loadtest/rag failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$resultDir = Join-Path $ResultRoot $RunName
$smokeSummaryPath = Join-Path $resultDir "rag-answer-summary.json"
Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "RAG smoke summary missing: $smokeSummaryPath"
$summary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -ne "rag-service" -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-RAGAssertion -Summary $summary -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "RAG eval assertion failed for case $($case.id): $type"
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

Assert-Condition ($caseResults.Count -gt 0) "No active rag-service eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "rag-service"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage RAG eval adapter; executes loadtest/rag against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "rag-eval-adapter-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   RAG eval adapter summary written: $resolvedOutputPath"
