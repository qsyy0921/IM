param(
    [string]$SummaryPath = "",
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = "",
    [string]$TenantID = "nexusim-local",
    [string]$UserID = "ai-eval-smoke",
    [string]$DeviceID = "ai-eval-smoke-device",
    [string]$RequestTimeout = "15s",
    [switch]$NoApplyMigration
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

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "ai-eval-record-run-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}

if ([string]::IsNullOrWhiteSpace($SummaryPath)) {
    $SummaryPath = Join-Path $resultDir "profile-agent-safety-eval-summary.json"
    & (Join-Path $PSScriptRoot "run-ai-eval-profile-agent-safety.ps1") `
        -CasePath $CasePath `
        -ResultRoot $ResultRoot `
        -RunName ($RunName + "-profile-agent-safety") `
        -OutputPath $SummaryPath
}

$resolvedSummaryPath = Resolve-RepoPath $SummaryPath
Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "SummaryPath does not exist: $resolvedSummaryPath"

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "ai-eval-record-run-smoke-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}

$applyMigrationValue = "true"
if ($NoApplyMigration) {
    $applyMigrationValue = "false"
}

$goArgs = @(
    "run", "./services/ai-eval-service/cmd/ai-eval-record-smoke",
    "-summary", $resolvedSummaryPath,
    "-pg-dsn", $PGDSN,
    "-tenant-id", $TenantID,
    "-user-id", $UserID,
    "-device-id", $DeviceID,
    "-output", $resolvedOutputPath,
    "-timeout", $RequestTimeout,
    "-apply-migration=$applyMigrationValue"
)

Push-Location $repoRoot
try {
    & go @goArgs
    if ($LASTEXITCODE -ne 0) {
        throw "ai-eval RecordEvalRun smoke failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Write-Host "OK   ai-eval RecordEvalRun smoke completed: $resolvedOutputPath"
