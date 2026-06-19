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
    throw "external adapter smoke case missing: $CaseID"
}

function Test-ExternalAdapterAssertion {
    param(
        $SmokeSummary,
        $Assertion
    )

    $type = Get-JsonPropertyString -Object $Assertion -Name "type"
    $success = Get-SmokeCase -Summary $SmokeSummary -CaseID "external-http-success"
    $providerFailure = Get-SmokeCase -Summary $SmokeSummary -CaseID "external-http-provider-unavailable"
    $unsafeOutput = Get-SmokeCase -Summary $SmokeSummary -CaseID "external-http-unsafe-output"

    switch ($type) {
        "must_execute_external_http_tool" {
            return `
                [bool]$success.executed `
                -and (Get-JsonPropertyString -Object $success -Name "execution_status") -eq "RECORDED" `
                -and (Get-JsonPropertyString -Object $success -Name "result_status") -eq "SUCCEEDED" `
                -and [bool]$success.output_sha256_present
        }
        "must_not_send_raw_tool_input" {
            foreach ($case in @($SmokeSummary.cases)) {
                if ([bool]$case.raw_input_sent) {
                    return $false
                }
            }
            return $true
        }
        "must_classify_provider_failure" {
            $expected = Get-JsonPropertyString -Object $Assertion -Name "expected_classification"
            if ($expected.Length -eq 0) {
                $expected = "TOOL_PROVIDER_UNAVAILABLE"
            }
            return `
                (Get-JsonPropertyString -Object $providerFailure -Name "execution_status") -eq "FAILED" `
                -and (Get-JsonPropertyString -Object $providerFailure -Name "result_status") -eq "FAILED" `
                -and (Get-JsonPropertyString -Object $providerFailure -Name "classification") -eq $expected `
                -and (-not [bool]$providerFailure.executed) `
                -and (-not [bool]$providerFailure.output_sha256_present)
        }
        "must_reject_unsafe_tool_output" {
            return `
                (Get-JsonPropertyString -Object $unsafeOutput -Name "execution_status") -eq "FAILED" `
                -and (Get-JsonPropertyString -Object $unsafeOutput -Name "result_status") -eq "FAILED" `
                -and (Get-JsonPropertyString -Object $unsafeOutput -Name "classification") -eq "TOOL_OUTPUT_UNSAFE" `
                -and (-not [bool]$unsafeOutput.executed) `
                -and (-not [bool]$unsafeOutput.output_sha256_present)
        }
        "must_not_store_raw_provider_output" {
            foreach ($case in @($SmokeSummary.cases)) {
                if ([bool]$case.provider_body_persisted) {
                    return $false
                }
            }
            return $true
        }
        "must_record_tool_result_projection" {
            $expectedStatus = Get-JsonPropertyString -Object $Assertion -Name "expected_status"
            if ($expectedStatus.Length -eq 0) {
                return (Get-JsonPropertyString -Object $success -Name "result_status").Length -gt 0
            }
            return (Get-JsonPropertyString -Object $success -Name "result_status") -eq $expectedStatus
        }
        default {
            throw "unsupported external adapter eval assertion type: $type"
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
    $RunName = "action-external-http-eval-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}
$smokeSummaryPath = Join-Path $resultDir "action-external-http-adapter-smoke.json"

Push-Location $repoRoot
try {
    & go run ./services/action-executor/cmd/action-external-http-adapter-smoke -output $smokeSummaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "action external HTTP adapter smoke failed with exit code $LASTEXITCODE"
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
    if ($stage -ne "action-executor-external-adapter" -or $status -ne "active") {
        continue
    }

    $assertionResults = New-Object System.Collections.Generic.List[object]
    foreach ($assertion in @($case.required_assertions)) {
        $type = Get-JsonPropertyString -Object $assertion -Name "type"
        $passed = Test-ExternalAdapterAssertion -SmokeSummary $smokeSummary -Assertion $assertion
        $assertionResults.Add([pscustomobject]@{
            type = $type
            passed = $passed
        })
        Assert-Condition $passed "External adapter eval assertion failed for case $($case.id): $type"
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

Assert-Condition ($caseResults.Count -gt 0) "No active action-executor external adapter eval cases found in $resolvedCasePath"

$adapterSummary = [pscustomobject]@{
    schema_version = 1
    adapter = "action-external-http-provider"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    scope = "first-stage action-executor external HTTP adapter eval; local fixture only, no external network and not a production benchmark"
    case_path = $resolvedCasePath
    run_name = $RunName
    result_dir = $resultDir
    smoke_summary_path = $smokeSummaryPath
    case_count = $caseResults.Count
    cases = $caseResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "action-external-http-eval-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$adapterSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
Write-Host "OK   action external HTTP adapter eval summary written: $resolvedOutputPath"
