param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$RetrievalTarget = "127.0.0.1:10590",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = "",
    [string]$RequestTimeout = "30s"
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
        default { throw "unsupported retrieval eval source_type: $SourceType" }
    }
}

function Test-RetrievalAssertion {
    param(
        $Summary,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "must_return_source_type" {
            $sourceType = Get-JsonPropertyString -Object $Assertion -Name "source_type"
            return (Get-ExpectedSourceCount -Summary $Summary -SourceType $sourceType) -gt 0
        }
        "must_preserve_evidencepack_source_coverage" {
            $searchCount = [int]$Summary.source_counts.search_message
            $memoryCount = [int]$Summary.source_counts.memory_event
            $profileCount = 0
            if ($null -ne $Summary.source_counts.PSObject.Properties["profile_aggregate"]) {
                $profileCount = [int]$Summary.source_counts.profile_aggregate
            }
            return `
                (Get-JsonPropertyString -Object $Summary -Name "pack_id").Length -gt 0 `
                -and [int]$Summary.item_count -eq ($searchCount + $memoryCount + $profileCount) `
                -and [int]$Summary.search_item_count -eq $searchCount `
                -and [int]$Summary.memory_item_count -eq $memoryCount `
                -and [int]$Summary.profile_item_count -eq $profileCount `
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
                -and [int]$Summary.source_counts.memory_event -gt 0
        }
        "must_require_complete_chain_before_answer" {
            return `
                [bool]$Summary.cross_group_source_refs_preserved `
                -and [bool]$Summary.cross_group_speaker_attribution_preserved `
                -and [int]$Summary.memory_item_count -gt 0
        }
        "must_preserve_projection_versions" {
            return `
                [int64]$Summary.search_projection_version -gt 0 `
                -and [int64]$Summary.memory_projection_version -gt 0 `
                -and [int64]$Summary.search_projection_version -eq [int64]$Summary.seed.visibility_version `
                -and [int64]$Summary.memory_projection_version -eq [int64]$Summary.seed.memory_projection_version
        }
        "must_preserve_source_chain_rerank" {
            return `
                [bool]$Summary.source_chain_rerank_preserved `
                -and [double]$Summary.memory_rerank_score -gt [double]$Summary.search_rerank_score `
                -and [double]$Summary.memory_rerank_score -gt 1.0
        }
        default {
            throw "unsupported retrieval eval assertion type: $type"
        }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "retrieval-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/retrieval",
    "-pg-dsn", $PGDSN,
    "-retrieval-target", $RetrievalTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-query", "phoenix launch decision",
    "-request-timeout", $RequestTimeout
)

Push-Location $repoRoot
try {
    & go @runArgs
    if ($LASTEXITCODE -ne 0) {
        throw "loadtest/retrieval failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$resultDir = Join-Path $ResultRoot $RunName
$smokeSummaryPath = Join-Path $resultDir "retrieval-evidence-summary.json"
Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "Retrieval smoke summary missing: $smokeSummaryPath"
$summary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -ne "retrieval-gateway" -or $status -ne "active") {
        continue
    }

    $caseID = Get-JsonPropertyString -Object $case -Name "id"
    if ($caseID -ne "retrieval-gateway-current-memory-live-preserves-chain") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-RetrievalAssertion -Summary $summary -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "Retrieval eval assertion failed for case $($case.id): $type"
    }

    $caseResults.Add([pscustomobject]@{
        id = $caseID
        family = $case.family
        stage = $stage
        status = $status
        passed = $true
        smoke_run_name = $RunName
        assertions = $assertionResults
    })
}

Assert-Condition ($caseResults.Count -eq 1) "Retrieval positive adapter must report exactly one positive live EvidencePack case"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "retrieval-gateway"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage Retrieval eval adapter; executes loadtest/retrieval against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "retrieval-eval-adapter-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   Retrieval eval adapter summary written: $resolvedOutputPath"
