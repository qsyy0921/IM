param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$Python = "python",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = ""
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

function Get-SmokeCase {
    param(
        $Summary,
        [string]$CaseID
    )

    foreach ($case in @($Summary.cases)) {
        if ((Get-JsonPropertyString -Object $case -Name "id") -eq $CaseID) {
            return $case
        }
    }
    throw "agent output smoke case missing: $CaseID"
}

function Test-AgentOutputAssertion {
    param(
        $SmokeSummary,
        $EvalCase,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    $smokeCase = Get-SmokeCase -Summary $SmokeSummary -CaseID (Get-JsonPropertyString -Object $EvalCase -Name "id")

    switch ($type) {
        "must_require_evidencepack_citations" {
            return [int]$smokeCase.citation_count -gt 0
        }
        "must_not_claim_llm_generation" {
            return -not [bool]$smokeCase.generated_by_llm
        }
        "must_keep_output_low_sensitive" {
            return `
                (-not [bool]$smokeCase.raw_output_returned) `
                -and (-not [bool]$smokeCase.business_write) `
                -and (-not [bool]$smokeCase.external_provider)
        }
        "must_reject_candidate_output" {
            $expected = Get-JsonPropertyString -Object $Assertion -Name "error_class"
            Assert-Condition ($expected.Length -gt 0) "must_reject_candidate_output requires error_class"
            return `
                (Get-JsonPropertyString -Object $smokeCase -Name "candidate_status") -eq "REJECTED" `
                -and (Get-JsonPropertyString -Object $smokeCase -Name "expected_error_class") -eq $expected `
                -and [bool]$smokeCase.failure_class_verified
        }
        "must_not_return_raw_output" {
            return -not [bool]$smokeCase.raw_output_returned
        }
        default {
            throw "unsupported agent output eval assertion type: $type"
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
    $RunName = "agent-output-regression-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}
$smokeSummaryPath = Join-Path $resultDir "agent-python-worker-provider-smoke.json"

Push-Location $repoRoot
try {
    & go run ./services/agent-service/cmd/agent-python-worker-provider-smoke `
        -python $Python `
        -output $smokeSummaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "agent python worker provider smoke failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "Smoke summary missing: $smokeSummaryPath"
$smokeSummary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -ne "agent-service-python-worker-provider" -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-AgentOutputAssertion -SmokeSummary $smokeSummary -EvalCase $case -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "Agent output eval assertion failed for case $($case.id): $type"
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

Assert-Condition ($caseResults.Count -gt 0) "No active agent-service Python provider eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "agent-python-worker-provider"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage Agent output regression; local agent-service provider boundary only, no database and no business write"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "agent-output-regression-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   Agent output regression summary written: $resolvedOutputPath"
