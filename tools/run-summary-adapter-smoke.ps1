param(
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$SummaryTarget = "127.0.0.1:10620",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$Focus = "phoenix launch decision",
    [string]$RequestTimeout = "10s"
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "summary-adapter-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/summary",
    "-pg-dsn", $PGDSN,
    "-summary-target", $SummaryTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-focus", $Focus,
    "-request-timeout", $RequestTimeout
)

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

$resultDir = Join-Path $ResultRoot $RunName
$summaryPath = Join-Path $resultDir "summary-service-summary.json"
if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
    throw "summary smoke output missing: $summaryPath"
}

$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
if ($summary.summary_status -ne "GROUNDED") {
    throw "unexpected summary_status: $($summary.summary_status)"
}
if ([bool]$summary.generated_by_llm) {
    throw "first-stage summary smoke must not claim LLM generation"
}
if ([int]$summary.citation_count -lt 1) {
    throw "summary smoke missing citations"
}
if ([int]$summary.evidence_item_count -lt 2) {
    throw "summary smoke missing expected evidence items"
}

Write-Host "OK   summary adapter smoke passed: $summaryPath"
