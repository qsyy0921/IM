param(
    [string]$Python = "python",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
$writeSummaryFile = -not [string]::IsNullOrWhiteSpace($OutputPath)
if ($writeSummaryFile) {
    Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"
    if ([string]::IsNullOrWhiteSpace($RunName)) {
        $RunName = "python-worker-eval-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
    }
    $resultDir = Join-Path $ResultRoot $RunName
    Assert-ExternalOutputRoot -Value $resultDir -RepositoryRoot $repoRoot -Name "ResultDir"
    if (-not (Test-Path -LiteralPath $resultDir)) {
        New-Item -ItemType Directory -Force -Path $resultDir | Out-Null
    }
} else {
    $resultDir = ""
}
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-python-worker-eval-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot | Out-Null

function Invoke-WorkerCase {
    param(
        [string]$CaseID,
        [hashtable]$Payload,
        [string]$ExpectedErrorClass
    )

    $requestPath = Join-Path $tempRoot "$CaseID-request.json"
    $outputPath = Join-Path $tempRoot "$CaseID-output.json"
    $Payload | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $requestPath -Encoding UTF8

    Push-Location $repoRoot
    try {
        & $Python "ai/python/scripts/run_candidate_worker.py" $requestPath | Set-Content -LiteralPath $outputPath -Encoding UTF8
        $exitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }

    $candidate = Get-Content -LiteralPath $outputPath -Raw | ConvertFrom-Json
    if ($exitCode -eq 0) {
        throw "Case $CaseID expected failed candidate, got exit code 0"
    }
    if ($candidate.status -ne "FAILED") {
        throw "Case $CaseID expected FAILED status, got $($candidate.status)"
    }
    if ($candidate.error_class -ne $ExpectedErrorClass) {
        throw "Case $CaseID expected $ExpectedErrorClass, got $($candidate.error_class)"
    }
    if ($candidate.PSObject.Properties.Name -contains "candidate_text") {
        throw "Case $CaseID returned raw candidate_text"
    }
    if ($candidate.output_sha256 -ne "") {
        throw "Case $CaseID failed candidate must not include output hash"
    }

    [pscustomobject]@{
        case_id = $CaseID
        status = "passed"
        expected_error_class = $ExpectedErrorClass
        candidate_status = $candidate.status
        raw_output_returned = $false
    }
}

try {
    $basePayload = @{
        task_id = "python-worker-eval-task"
        candidate_id = "python-worker-eval-candidate"
        worker_kind = "MEMORY_EXTRACTION"
        output_type = "MEMORY_EVENT_CANDIDATE"
        source_refs = @("message:tenant:conversation:seq1")
        citations = @("message:tenant:conversation:seq1")
        confidence = 0.7
    }

    $malformedPayload = $basePayload.Clone()
    $unsafePayload = $basePayload.Clone()
    $unsafePayload["candidate_text"] = "Bearer secret-token-value"

    $cases = @(
        Invoke-WorkerCase `
            -CaseID "python-worker-malformed-output-fails-closed" `
            -Payload $malformedPayload `
            -ExpectedErrorClass "MALFORMED_INPUT"
        Invoke-WorkerCase `
            -CaseID "python-worker-unsafe-output-fails-closed" `
            -Payload $unsafePayload `
            -ExpectedErrorClass "UNSAFE_INPUT"
    )

    $summary = [pscustomobject]@{
        schema_version = 1
        adapter = "python-ai-worker"
        generated_at = (Get-Date).ToUniversalTime().ToString("o")
        status = "passed"
        run_name = $RunName
        result_dir = $resultDir
        case_count = $cases.Count
        scope = "local low-sensitive Python worker eval adapter; no external provider, no database, no business write"
        cases = $cases
    }
    $summaryJson = $summary | ConvertTo-Json -Depth 8
    if ($writeSummaryFile) {
        if ([System.IO.Path]::IsPathRooted($OutputPath)) {
            $resolvedOutputPath = [System.IO.Path]::GetFullPath($OutputPath)
        } else {
            $resolvedOutputPath = [System.IO.Path]::GetFullPath((Join-Path $repoRoot $OutputPath))
        }
        $outputDir = Split-Path -Parent $resolvedOutputPath
        Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
        if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
            New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
        }
        $summaryJson | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
        Write-Host "OK   Python worker eval adapter summary written: $resolvedOutputPath"
    } else {
        $summaryJson
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
