param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$SummaryTarget = "127.0.0.1:10620",
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
        default { throw "unsupported summary eval source_type: $SourceType" }
    }
}

function Test-CitationSourceRefMatch {
    param($Summary)

    $seed = $Summary.seed
    if ($null -eq $seed -or $null -eq $Summary.citation_refs) {
        return $false
    }
    foreach ($ref in @($Summary.citation_refs)) {
        $evidenceID = Get-JsonPropertyString -Object $ref -Name "evidence_id"
        if ($evidenceID.Length -eq 0) {
            continue
        }
        if (
            (Get-JsonPropertyString -Object $ref -Name "source_id") -eq (Get-JsonPropertyString -Object $seed -Name "message_id") -and
            (Get-JsonPropertyString -Object $ref -Name "source_event_id") -eq (Get-JsonPropertyString -Object $seed -Name "source_event_id") -and
            (Get-JsonPropertyString -Object $ref -Name "conversation_id") -eq (Get-JsonPropertyString -Object $seed -Name "conversation_id") -and
            [int64]$ref.conversation_seq -eq [int64]$seed.conversation_seq
        ) {
            return $true
        }
    }
    return $false
}

function Test-SummaryAssertion {
    param(
        $Summary,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "summary_status" {
            $expected = Get-JsonPropertyString -Object $Assertion -Name "expected_status"
            Assert-Condition ($expected.Length -gt 0) "summary_status assertion requires expected_status"
            return $Summary.summary_status -eq $expected
        }
        "must_include_citation" {
            return [int]$Summary.citation_count -gt 0
        }
        "must_match_citation_source_ref" {
            return Test-CitationSourceRefMatch -Summary $Summary
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
        "must_exclude_expired_superseded_memory_items" {
            return `
                [int64]$Summary.current_memory_at_seq -gt 0 `
                -and [bool]$Summary.expired_memory_excluded `
                -and [bool]$Summary.superseded_memory_excluded `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "expired_memory_event_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "superseded_memory_event_id").Length -gt 0
        }
        "must_not_return_future_memory_as_current" {
            return `
                [int64]$Summary.current_memory_at_seq -gt 0 `
                -and [bool]$Summary.future_memory_excluded `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "future_memory_event_id").Length -gt 0
        }
        "must_preserve_cross_group_source_refs" {
            return `
                [bool]$Summary.cross_group_source_refs_preserved `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "cross_group_conversation_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "cross_group_message_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "cross_group_source_event_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "cross_group_source_ref_id").Length -gt 0
        }
        "must_preserve_speaker_attribution" {
            return `
                [bool]$Summary.cross_group_speaker_attribution_preserved `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "sender_user_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "cross_group_actor_user_id").Length -gt 0
        }
        "must_preserve_multi_hop_actor_chain" {
            return `
                [bool]$Summary.cross_group_source_refs_preserved `
                -and [bool]$Summary.cross_group_speaker_attribution_preserved `
                -and [int]$Summary.source_counts.search_message -gt 0 `
                -and [int]$Summary.source_counts.memory_event -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "sender_user_id").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary.seed -Name "cross_group_actor_user_id").Length -gt 0
        }
        "must_require_complete_chain_before_answer" {
            return `
                [bool]$Summary.cross_group_source_refs_preserved `
                -and [bool]$Summary.cross_group_speaker_attribution_preserved `
                -and [int]$Summary.citation_count -gt 0 `
                -and [int]$Summary.memory_item_count -gt 0
        }
        "must_preserve_projection_versions" {
            return `
                [int64]$Summary.search_projection_version -gt 0 `
                -and [int64]$Summary.memory_projection_version -gt 0 `
                -and [int64]$Summary.search_projection_version -eq [int64]$Summary.seed.visibility_version `
                -and [int64]$Summary.memory_projection_version -eq [int64]$Summary.seed.memory_projection_version
        }
        "must_preserve_summary_retrieval_versions" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "summary_version").Length -gt 0 `
                -and (Get-JsonPropertyString -Object $Summary -Name "retrieval_version").Length -gt 0
        }
        "must_abstain" {
            return $Summary.summary_status -eq "INSUFFICIENT_EVIDENCE"
        }
        "must_return_empty_evidencepack" {
            return `
                (Get-JsonPropertyString -Object $Summary -Name "pack_id").Length -gt 0 `
                -and [int]$Summary.evidence_item_count -eq 0 `
                -and [int]$Summary.search_item_count -eq 0 `
                -and [int]$Summary.memory_item_count -eq 0 `
                -and [int]$Summary.source_counts.search_message -eq 0 `
                -and [int]$Summary.source_counts.memory_event -eq 0
        }
        "must_not_include_citation" {
            return [int]$Summary.citation_count -eq 0
        }
        default {
            throw "unsupported summary eval assertion type: $type"
        }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "summary-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

function Invoke-SummarySmokeRun {
    param(
        [string]$SmokeRunName,
        [string[]]$ExtraArgs = @()
    )

    $runArgs = @(
        "run", "./loadtest/summary",
        "-pg-dsn", $PGDSN,
        "-summary-target", $SummaryTarget,
        "-result-root", $ResultRoot,
        "-run-name", $SmokeRunName,
        "-focus", "phoenix launch decision",
        "-request-timeout", $RequestTimeout
    )
    $runArgs += $ExtraArgs

    Push-Location $repoRoot
    try {
        & go @runArgs
        if ($LASTEXITCODE -ne 0) {
            throw "loadtest/summary failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    $resultDir = Join-Path $ResultRoot $SmokeRunName
    $summaryPath = Join-Path $resultDir "summary-service-summary.json"
    Assert-Condition (Test-Path -LiteralPath $summaryPath -PathType Leaf) "Summary smoke result missing: $summaryPath"
    return [pscustomobject]@{
        run_name = $SmokeRunName
        result_dir = $resultDir
        summary_path = $summaryPath
        summary = (Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json)
    }
}

$defaultRun = Invoke-SummarySmokeRun -SmokeRunName $RunName
$summary = $defaultRun.summary
$resultDir = $defaultRun.result_dir
$smokeSummaryPath = $defaultRun.summary_path
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json
$noEvidenceRun = $null

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -ne "summary-service" -or $status -ne "active") {
        continue
    }

    $caseSummary = $summary
    $needsNoEvidenceRun = $false
    foreach ($assertion in @($case.required_assertions)) {
        $assertionType = Get-JsonPropertyString -Object $assertion -Name "type"
        if ($assertionType -in @("must_abstain", "must_return_empty_evidencepack", "must_not_include_citation")) {
            $needsNoEvidenceRun = $true
        }
    }
    if ($needsNoEvidenceRun) {
        if ($null -eq $noEvidenceRun) {
            $noEvidenceRun = Invoke-SummarySmokeRun -SmokeRunName ($RunName + "-no-evidence") -ExtraArgs @(
                "-scenario", "no-evidence",
                "-focus", "unseeded private recap"
            )
        }
        $caseSummary = $noEvidenceRun.summary
    }
    $caseSmokeRunName = $defaultRun.run_name
    if ($needsNoEvidenceRun) {
        $caseSmokeRunName = $noEvidenceRun.run_name
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-SummaryAssertion -Summary $caseSummary -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "Summary eval assertion failed for case $($case.id): $type"
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

Assert-Condition ($caseResults.Count -gt 0) "No active summary-service eval cases found in $resolvedCasePath"
$noEvidenceRunName = ""
$noEvidenceSummaryPath = ""
if ($null -ne $noEvidenceRun) {
    $noEvidenceRunName = $noEvidenceRun.run_name
    $noEvidenceSummaryPath = $noEvidenceRun.summary_path
}

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "summary-service"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage Summary eval adapter; executes loadtest/summary against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    no_evidence_run_name = $noEvidenceRunName
    no_evidence_summary_path = $noEvidenceSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "summary-eval-adapter-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   Summary eval adapter summary written: $resolvedOutputPath"
