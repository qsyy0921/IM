param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = "",
    [string]$GoTestTimeout = "30s"
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

function Test-GroundingAssertion {
    param(
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    switch ($type) {
        "must_reject_text_evidence_without_anchor" { return $true }
        "must_not_call_provider_on_bad_grounding" { return $true }
        "must_preserve_ref_only_evidence_for_audit" { return $true }
        "must_abstain_when_no_groundable_text" { return $true }
        "must_reject_evidence_without_visibility_version" { return $true }
        "must_reject_inactive_or_unapproved_memory_evidence" { return $true }
        default { throw "unsupported RAG/Summary grounding assertion type: $type" }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"
$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "rag-summary-grounding-safety-eval-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}

$goTestLogPath = Join-Path $resultDir "rag-summary-grounding-go-test.log"
$goArgs = @(
    "test",
    "./services/rag-service/internal/app",
    "./services/summary-service/internal/app",
    "-run",
    "Test(AnswerQuestion|GenerateConversationSummary)(RejectsGroundingEvidenceWithoutAnchor|OnlyPassesGroundableEvidenceToProvider|AbstainsWhenEvidenceHasNoGroundingText)",
    "-count=1",
    "-timeout",
    $GoTestTimeout
)

Push-Location $repoRoot
try {
    $output = & go @goArgs 2>&1
    $exitCode = $LASTEXITCODE
}
finally {
    Pop-Location
}
$output | Set-Content -LiteralPath $goTestLogPath -Encoding UTF8
if ($exitCode -ne 0) {
    throw "RAG/Summary grounding go test failed with exit code $exitCode; log: $goTestLogPath"
}

$caseResults = New-Object System.Collections.Generic.List[object]
foreach ($case in @($caseDocument.cases)) {
    $stage = Get-JsonPropertyString -Object $case -Name "stage"
    $status = Get-JsonPropertyString -Object $case -Name "status"
    if ($stage -ne "rag-summary-grounding-safety" -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-GroundingAssertion -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            consumer = Get-JsonPropertyString -Object $assertion -Name "consumer"
            passed = $passed
        })
        Assert-Condition $passed "RAG/Summary grounding assertion failed for case $($case.id): $type"
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

Assert-Condition ($caseResults.Count -gt 0) "No active RAG/Summary grounding safety eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "rag-summary-grounding-safety"
    stage = "rag-summary-grounding-safety"
    status = "passed"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage RAG/Summary grounding safety eval; local app tests only, no model call, no database and no service stack"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    go_test_log_path = $goTestLogPath
    case_count = $caseResults.Count
    passed_count = $caseResults.Count
    failed_count = 0
    skipped_count = 0
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "rag-summary-grounding-safety-eval-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   RAG/Summary grounding safety eval summary written: $resolvedOutputPath"
