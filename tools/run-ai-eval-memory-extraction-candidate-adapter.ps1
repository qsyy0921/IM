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
        [string]$Name,
        [string]$DefaultValue = ""
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $DefaultValue
    }
    if ($null -eq $Object.$Name) {
        return $DefaultValue
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

function Find-SmokeCase {
    param(
        $SmokeSummary,
        [string]$CaseID
    )

    $match = @($SmokeSummary.cases | Where-Object { (Get-JsonPropertyString -Object $_ -Name "case_id") -eq $CaseID })
    Assert-Condition ($match.Count -eq 1) "memory extraction smoke case missing: $CaseID"
    return $match[0]
}

function Resolve-SmokeCaseForEvalCase {
    param(
        $SmokeSummary,
        [string]$CaseID
    )

    if ($CaseID -eq "python-memory-extraction-profile-signal-review-required") {
        return Find-SmokeCase -SmokeSummary $SmokeSummary -CaseID "python-memory-extraction-explicit-cues-hash-only"
    }
    return Find-SmokeCase -SmokeSummary $SmokeSummary -CaseID $CaseID
}

function Invoke-Assertion {
    param(
        [string]$CaseID,
        $Assertion,
        $SmokeCase
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "must_extract_memory_candidates_hash_only" {
            $expectedCount = [int]$Assertion.expected_candidate_count
            Assert-Condition ($SmokeCase.result_status -eq "COMPLETED") "$CaseID expected completed result"
            Assert-Condition ([int]$SmokeCase.candidate_count -eq $expectedCount) "$CaseID candidate count mismatch"
            Assert-Condition ([bool]$SmokeCase.requires_go_validation) "$CaseID must require Go validation"
            foreach ($expectedType in @($Assertion.expected_event_types)) {
                Assert-Condition (@($SmokeCase.memory_event_types) -contains ([string]$expectedType)) "$CaseID missing memory event type $expectedType"
            }
        }
        "must_keep_ordinary_chat_zero_candidate" {
            Assert-Condition ($SmokeCase.result_status -eq "COMPLETED") "$CaseID expected completed result"
            Assert-Condition ([int]$SmokeCase.candidate_count -eq 0) "$CaseID must not produce candidates"
            Assert-Condition ([int]$SmokeCase.ordinary_message_count -gt 0) "$CaseID must count ordinary messages"
        }
        "must_require_profile_signal_review" {
            Assert-Condition ([bool]$SmokeCase.profile_review_required) "$CaseID profile signal must require review"
            Assert-Condition (@($SmokeCase.memory_event_types) -contains "PROFILE_SIGNAL") "$CaseID must include PROFILE_SIGNAL"
        }
        "must_fail_closed_on_unsafe_memory_input" {
            $expectedErrorClass = Get-JsonPropertyString -Object $Assertion -Name "error_class"
            Assert-Condition ($SmokeCase.result_status -eq "REJECTED") "$CaseID expected rejected result"
            Assert-Condition ([int]$SmokeCase.candidate_count -eq 0) "$CaseID must not produce candidates"
            Assert-Condition ([bool]$SmokeCase.rejected_before_worker) "$CaseID must reject before worker persistence path"
            Assert-Condition ((Get-JsonPropertyString -Object $SmokeCase -Name "expected_error_class") -eq $expectedErrorClass) "$CaseID error class mismatch"
        }
        "must_not_return_raw_output" {
            Assert-Condition (-not [bool]$SmokeCase.raw_text_returned) "$CaseID returned raw output"
        }
        "must_not_persist_memory_fact" {
            Assert-Condition (-not [bool]$SmokeCase.final_memory_persisted) "$CaseID persisted final memory fact"
        }
        default {
            throw "unsupported memory extraction eval assertion type: $type"
        }
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

$writeSummaryFile = -not [string]::IsNullOrWhiteSpace($OutputPath)
if ($writeSummaryFile) {
    Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"
    if ([string]::IsNullOrWhiteSpace($RunName)) {
        $RunName = "memory-extraction-candidate-eval-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
    }
    $resultDir = Join-Path $ResultRoot $RunName
    Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
    if (-not (Test-Path -LiteralPath $resultDir)) {
        New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
    }
}
else {
    $resultDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-memory-extraction-eval-" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}

$smokeSummaryPath = Join-Path $resultDir "memory-extraction-go-adapter-smoke.json"
Push-Location $repoRoot
try {
    & go run ./tools/memory-extraction-go-adapter-smoke `
        -python $Python `
        -output $smokeSummaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "memory extraction Go adapter smoke failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
Assert-Condition (Test-Path -LiteralPath $smokeSummaryPath -PathType Leaf) "smoke summary missing: $smokeSummaryPath"
$smokeSummary = Get-Content -LiteralPath $smokeSummaryPath -Raw | ConvertFrom-Json
Assert-Condition ($smokeSummary.status -eq "passed") "memory extraction smoke did not pass"

$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json
$activeCases = @($caseDocument.cases | Where-Object {
        ([string]$_.stage).Trim() -eq "python-memory-extraction-candidate" -and
        ([string]$_.status).Trim() -eq "active"
    })
Assert-Condition ($activeCases.Count -gt 0) "no active python-memory-extraction-candidate cases"

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in $activeCases) {
    $caseID = Get-JsonPropertyString -Object $case -Name "id"
    $smokeCase = Resolve-SmokeCaseForEvalCase -SmokeSummary $smokeSummary -CaseID $caseID
    foreach ($assertion in @($case.required_assertions)) {
        Invoke-Assertion -CaseID $caseID -Assertion $assertion -SmokeCase $smokeCase
    }
    $caseResults.Add([pscustomobject]@{
        case_id = $caseID
        status = "passed"
        stage = "python-memory-extraction-candidate"
        source_smoke_case_id = Get-JsonPropertyString -Object $smokeCase -Name "case_id"
        candidate_count = [int]$smokeCase.candidate_count
        ordinary_message_count = [int]$smokeCase.ordinary_message_count
        raw_text_returned = [bool]$smokeCase.raw_text_returned
        final_memory_persisted = [bool]$smokeCase.final_memory_persisted
        requires_go_validation = [bool]$smokeCase.requires_go_validation
    })
}

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "python-memory-extraction-candidate"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    status = "passed"
    run_name = $RunName
    result_dir = $resultDir
    case_count = $caseResults.Count
    passed_count = $caseResults.Count
    failed_count = 0
    skipped_count = 0
    case_path = $resolvedCasePath
    smoke_summary_path = $smokeSummaryPath
    scope = "local low-sensitive Python memory extraction candidate eval adapter; no external provider, no database, no business write"
    cases = $caseResults
}

$summaryJson = $adapterSummary | ConvertTo-Json -Depth 8
if ($writeSummaryFile) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $summaryJson | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   Memory extraction candidate eval adapter summary written: $resolvedOutputPath"
}
else {
    $summaryJson
}
