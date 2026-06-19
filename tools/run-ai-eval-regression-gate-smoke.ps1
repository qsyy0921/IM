param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = "",
    [string]$TenantID = "nexusim-local",
    [string]$UserID = "ai-eval-smoke",
    [string]$DeviceID = "ai-eval-smoke-device",
    [string]$RequestTimeout = "30s",
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

function Invoke-EvalRecorder {
    param(
        [string]$SummaryPath,
        [string]$RecordOutputPath
    )

    $applyMigrationValue = "true"
    if ($NoApplyMigration) {
        $applyMigrationValue = "false"
    }

    $goArgs = @(
        "run", "./services/ai-eval-service/cmd/ai-eval-record-smoke",
        "-summary", $SummaryPath,
        "-pg-dsn", $PGDSN,
        "-tenant-id", $TenantID,
        "-user-id", $UserID,
        "-device-id", $DeviceID,
        "-output", $RecordOutputPath,
        "-timeout", $RequestTimeout,
        "-apply-migration=$applyMigrationValue"
    )

    Push-Location $repoRoot
    try {
        & go @goArgs
        if ($LASTEXITCODE -ne 0) {
            throw "ai-eval RecordEvalRun recorder failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "ai-eval-regression-gate-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
if (-not (Test-Path -LiteralPath $resultDir)) {
    New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
}

$resolvedCasePath = Resolve-RepoPath $CasePath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"

$adapters = @(
    [pscustomobject]@{
        name = "profile-agent-safety"
        script = "run-ai-eval-profile-agent-safety.ps1"
        run_suffix = "profile-agent-safety"
        summary_file = "profile-agent-safety-eval-summary.json"
    },
    [pscustomobject]@{
        name = "action-external-http-provider"
        script = "run-ai-eval-action-external-adapter.ps1"
        run_suffix = "action-external-http"
        summary_file = "action-external-http-eval-summary.json"
    }
)

$adapterResults = New-Object System.Collections.Generic.List[object]
$totalCases = 0
$totalPassed = 0
$totalFailed = 0
$totalSkipped = 0

foreach ($adapter in $adapters) {
    $adapterRunName = "$RunName-$($adapter.run_suffix)"
    $summaryPath = Join-Path $resultDir $adapter.summary_file
    $recordPath = Join-Path $resultDir "$($adapter.name)-record-summary.json"
    $scriptPath = Join-Path $PSScriptRoot $adapter.script

    & $scriptPath `
        -CasePath $resolvedCasePath `
        -ResultRoot $ResultRoot `
        -RunName $adapterRunName `
        -OutputPath $summaryPath

    Assert-Condition (Test-Path -LiteralPath $summaryPath -PathType Leaf) "adapter summary missing: $summaryPath"
    Invoke-EvalRecorder -SummaryPath $summaryPath -RecordOutputPath $recordPath
    Assert-Condition (Test-Path -LiteralPath $recordPath -PathType Leaf) "record summary missing: $recordPath"

    $record = Get-Content -LiteralPath $recordPath -Raw | ConvertFrom-Json
    Assert-Condition ($record.status -eq "passed") "record smoke did not pass for $($adapter.name)"
    Assert-Condition ($record.eval_run_status -eq "PASSED") "eval run status was not PASSED for $($adapter.name)"
    Assert-Condition ([bool]$record.get_run_matched) "GetEvalRun did not match for $($adapter.name)"
    Assert-Condition ([bool]$record.list_contains_run) "ListEvalRuns did not contain run for $($adapter.name)"

    $totalCases += [int]$record.case_count
    $totalPassed += [int]$record.passed_count
    $totalFailed += [int]$record.failed_count
    $totalSkipped += [int]$record.skipped_count

    $adapterResults.Add([pscustomobject]@{
        name = $adapter.name
        run_id = $record.run_id
        suite_id = $record.suite_id
        stage = $record.stage
        eval_run_status = $record.eval_run_status
        case_count = [int]$record.case_count
        passed_count = [int]$record.passed_count
        failed_count = [int]$record.failed_count
        skipped_count = [int]$record.skipped_count
        summary_ref = $record.summary_ref
        record_summary_path = $recordPath
    })
}

$gateStatus = "passed"
if ($totalFailed -gt 0 -or $totalCases -eq 0) {
    $gateStatus = "failed"
}

$gateSummary = [pscustomobject]@{
    schema_version = 1
    status = $gateStatus
    scope = "first-stage AI eval multi-adapter regression gate; low-sensitive local adapters only"
    run_name = $RunName
    result_dir = $resultDir
    adapter_count = $adapterResults.Count
    case_count = $totalCases
    passed_count = $totalPassed
    failed_count = $totalFailed
    skipped_count = $totalSkipped
    adapters = $adapterResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $resultDir "ai-eval-regression-gate-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$gateSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8

if ($gateStatus -ne "passed") {
    throw "AI eval regression gate failed; summary: $resolvedOutputPath"
}

Write-Host "OK   ai-eval regression gate smoke completed: $resolvedOutputPath"
