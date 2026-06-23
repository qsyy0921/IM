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

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
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

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

$caseDocument = Get-Content -LiteralPath $resolvedCasePath -Raw | ConvertFrom-Json
$expectedCaseIDs = @(
    "retrieval-source-coverage-empty-memory",
    "retrieval-temporal-superseded-filter",
    "retrieval-attribution-source-ref-required",
    "retrieval-cross-tenant-permission-deny"
)
foreach ($caseID in $expectedCaseIDs) {
    $match = @($caseDocument.cases | Where-Object {
        (Get-JsonPropertyString -Object $_ -Name "id") -eq $caseID `
            -and (Get-JsonPropertyString -Object $_ -Name "stage") -eq "retrieval-gateway" `
            -and (Get-JsonPropertyString -Object $_ -Name "status") -eq "active"
    })
    Assert-Condition ($match.Count -eq 1) "active retrieval negative case missing from catalog: $caseID"
}

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "retrieval-negative-eval-adapter-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/retrievalnegative",
    "-pg-dsn", $PGDSN,
    "-retrieval-target", $RetrievalTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-request-timeout", $RequestTimeout
)

Push-Location $repoRoot
try {
    & go @runArgs
    if ($LASTEXITCODE -ne 0) {
        throw "loadtest/retrievalnegative failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$resultDir = Join-Path $ResultRoot $RunName
$summaryPath = Join-Path $resultDir "retrieval-negative-eval-adapter-summary.json"
Assert-Condition (Test-Path -LiteralPath $summaryPath -PathType Leaf) "Retrieval negative summary missing: $summaryPath"
$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
Assert-Condition ((Get-JsonPropertyString -Object $summary -Name "adapter") -eq "retrieval-gateway-negative") "unexpected adapter name in summary"
Assert-Condition ([int]$summary.case_count -eq $expectedCaseIDs.Count) "unexpected retrieval negative case_count"
Assert-Condition ([int]$summary.passed_count -eq $expectedCaseIDs.Count) "retrieval negative adapter did not pass all cases"
Assert-Condition ([int]$summary.failed_count -eq 0) "retrieval negative adapter has failed cases"
Assert-Condition ([int]$summary.skipped_count -eq 0) "retrieval negative adapter must not skip cases"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = $summaryPath
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
if ([System.IO.Path]::GetFullPath($summaryPath) -ne $resolvedOutputPath) {
    Copy-Item -LiteralPath $summaryPath -Destination $resolvedOutputPath -Force
}
Write-Host "OK   Retrieval negative eval adapter summary written: $resolvedOutputPath"
