param(
    [string]$Python = "python"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-python-worker-smoke-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot | Out-Null

try {
    $requestPath = Join-Path $tempRoot "request.json"
    $candidatePath = Join-Path $tempRoot "candidate.json"

    @{
        task_id = "python-worker-smoke-task"
        candidate_id = "python-worker-smoke-candidate"
        worker_kind = "MEMORY_EXTRACTION"
        output_type = "MEMORY_EVENT_CANDIDATE"
        candidate_text = "decision: keep Python as candidate-only AI worker"
        source_refs = @("message:tenant:conversation:seq1")
        citations = @("message:tenant:conversation:seq1")
        confidence = 0.9
    } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $requestPath -Encoding UTF8

    Push-Location $repoRoot
    try {
        & $Python "ai/python/scripts/run_candidate_worker.py" $requestPath | Set-Content -LiteralPath $candidatePath -Encoding UTF8
        if ($LASTEXITCODE -ne 0) {
            throw "Python worker smoke returned exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }

    $candidate = Get-Content -LiteralPath $candidatePath -Raw | ConvertFrom-Json
    if ($candidate.status -ne "CANDIDATE") {
        throw "Expected CANDIDATE status, got $($candidate.status)"
    }
    if ($candidate.worker_kind -ne "MEMORY_EXTRACTION") {
        throw "Expected MEMORY_EXTRACTION worker_kind, got $($candidate.worker_kind)"
    }
    if ($candidate.PSObject.Properties.Name -contains "candidate_text") {
        throw "Candidate output must not include raw candidate_text"
    }
    if (-not ($candidate.output_sha256 -match "^[0-9a-f]{64}$")) {
        throw "Candidate output_sha256 must be lowercase sha256 hex"
    }

    [pscustomobject]@{
        schema_version = 1
        smoke = "python-ai-worker-candidate"
        status = "passed"
        worker_kind = $candidate.worker_kind
        output_type = $candidate.output_type
        output_sha256_present = $true
        raw_text_returned = $false
        scope = "local candidate-only smoke; no external provider, no database, no business write"
    } | ConvertTo-Json -Depth 6
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
