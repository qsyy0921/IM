param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$MemoryTarget = "127.0.0.1:10580",
    [string]$KafkaBrokers = "localhost:9092",
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

function Test-MemoryAssertion {
    param(
        $Summary,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "must_preserve_memory_source_refs" {
            return [bool]$Summary.checks.source_ref_projected -and [bool]$Summary.checks.runtime_source_ref
        }
        "must_preserve_memory_validity_window" {
            return [bool]$Summary.checks.validity_window_current
        }
        "must_link_superseded_memory" {
            return [bool]$Summary.checks.supersession_link
        }
        "must_exclude_superseded_current_memory" {
            return [bool]$Summary.checks.superseded_hidden
        }
        "must_preserve_task_decision_dependency_edges" {
            return [bool]$Summary.checks.graph_edge_preserved
        }
        "must_allow_reviewed_multi_source_profile" {
            return [bool]$Summary.checks.reviewed_multi_source_profile_active
        }
        "must_recompute_profile_via_public_api" {
            return [bool]$Summary.checks.profile_recomputed_via_public_api
        }
        "must_preserve_profile_supporting_evidence" {
            return [bool]$Summary.checks.profile_supporting_evidence_preserved
        }
        "must_expire_profile_when_supporting_memory_deleted" {
            return [bool]$Summary.checks.deleted_support_profile_excluded
        }
        default {
            throw "unsupported memory-service eval assertion type: $type"
        }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "memory-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/memory",
    "-pg-dsn", $PGDSN,
    "-memory-target", $MemoryTarget,
    "-kafka-brokers", $KafkaBrokers,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-request-timeout", $RequestTimeout
)

Push-Location $repoRoot
try {
    & go @runArgs
    if ($LASTEXITCODE -ne 0) {
        throw "loadtest/memory failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$resultDir = Join-Path $ResultRoot $RunName
$smokeSummaryPath = Join-Path $resultDir "memory-projection-summary.json"
Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "Memory smoke summary missing: $smokeSummaryPath"
$summary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -ne "memory-service" -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-MemoryAssertion -Summary $summary -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "Memory eval assertion failed for case $($case.id): $type"
    }

    $caseResults.Add([pscustomobject]@{
        id = $case.id
        family = $case.family
        stage = $stage
        status = $status
        passed = $true
        smoke_run_name = $RunName
        assertions = $assertionResults
    })
}

Assert-Condition ($caseResults.Count -gt 0) "No active memory-service eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "memory-service"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage Memory eval adapter; executes loadtest/memory against local services; not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "memory-eval-adapter-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   Memory eval adapter summary written: $resolvedOutputPath"
