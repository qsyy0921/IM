param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
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
    throw "action preflight safety smoke case missing: $CaseID"
}

function Test-PreflightAssertion {
    param(
        $SmokeSummary,
        $EvalCase,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    $smokeCase = Get-SmokeCase -Summary $SmokeSummary -CaseID (Get-JsonPropertyString -Object $EvalCase -Name "id")

    switch ($type) {
        "must_classify_action_preflight" {
            $expectedClassification = Get-JsonPropertyString -Object $Assertion -Name "expected_classification"
            $expectedErrorClass = Get-JsonPropertyString -Object $Assertion -Name "expected_error_class"
            if ($expectedClassification.Length -gt 0) {
                return (Get-JsonPropertyString -Object $smokeCase -Name "classification") -eq $expectedClassification
            }
            Assert-Condition ($expectedErrorClass.Length -gt 0) "must_classify_action_preflight requires expected_classification or expected_error_class"
            return (Get-JsonPropertyString -Object $smokeCase -Name "error_class") -eq $expectedErrorClass
        }
        "must_block_policy_denied_agent_action" {
            return `
                (Get-JsonPropertyString -Object $smokeCase -Name "execution_status") -eq "BLOCKED" `
                -and (Get-JsonPropertyString -Object $smokeCase -Name "result_status") -eq "BLOCKED" `
                -and (Get-JsonPropertyString -Object $smokeCase -Name "classification") -eq "TOOL_DENIED" `
                -and (-not [bool]$smokeCase.allowed) `
                -and (-not [bool]$smokeCase.executed)
        }
        "must_record_execution_audit" {
            return [bool]$smokeCase.audit_recorded
        }
        "must_record_tool_result_projection" {
            if (-not [bool]$smokeCase.projection_recorded) {
                return $false
            }
            $expectedStatus = Get-JsonPropertyString -Object $Assertion -Name "expected_status"
            if ($expectedStatus.Length -eq 0) {
                return (Get-JsonPropertyString -Object $smokeCase -Name "result_status").Length -gt 0
            }
            return (Get-JsonPropertyString -Object $smokeCase -Name "result_status") -eq $expectedStatus
        }
        "must_not_execute_external_tool" {
            return (-not [bool]$smokeCase.executed) -and (-not [bool]$smokeCase.output_sha256_present)
        }
        "must_not_create_approval_or_execution" {
            return `
                (-not [bool]$smokeCase.audit_recorded) `
                -and (-not [bool]$smokeCase.projection_recorded) `
                -and (-not [bool]$smokeCase.executed) `
                -and (-not [bool]$smokeCase.output_sha256_present)
        }
        default {
            throw "unsupported action preflight safety eval assertion type: $type"
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
    $RunName = "action-preflight-safety-eval-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}
$smokeSummaryPath = Join-Path $resultDir "action-preflight-safety-smoke.json"

Push-Location $repoRoot
try {
    & go run ./services/action-executor/cmd/action-preflight-safety-smoke -output $smokeSummaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "action preflight safety smoke failed with exit code $LASTEXITCODE"
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
    if ($stage -ne "action-executor-preflight-safety" -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-PreflightAssertion -SmokeSummary $smokeSummary -EvalCase $case -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "Action preflight safety eval assertion failed for case $($case.id): $type"
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

Assert-Condition ($caseResults.Count -gt 0) "No active action-executor preflight safety eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "action-preflight-safety"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage action-executor preflight safety eval; in-memory ports only, no external network and not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "action-preflight-safety-eval-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   action preflight safety eval summary written: $resolvedOutputPath"
