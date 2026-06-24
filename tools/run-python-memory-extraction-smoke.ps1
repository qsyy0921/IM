param(
    [string]$Python = "python"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-memory-extract-smoke-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot | Out-Null

try {
    $requestPath = Join-Path $tempRoot "memory-extraction-request.json"
    $outputPath = Join-Path $tempRoot "memory-extraction-output.json"

    @{
        schema_version = 1
        task_id = "memory-extraction-smoke"
        tenant_id = "tenant-a"
        conversation_id = "conv-alpha"
        messages = @(
            @{
                message_id = "msg-1"
                conversation_seq = 7
                speaker_id = "user-a"
                text = "decision: keep memory candidates source-backed"
            }
            @{
                message_id = "msg-2"
                conversation_seq = 8
                speaker_id = "user-b"
                text = "profile_signal: user-a coordinates launch follow-up"
            }
            @{
                message_id = "msg-3"
                conversation_seq = 9
                speaker_id = "user-c"
                text = "ordinary chat should not become memory"
            }
        )
    } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $requestPath -Encoding UTF8

    Push-Location $repoRoot
    try {
        & $Python "ai/python/scripts/run_memory_extraction_candidate.py" $requestPath |
            Set-Content -LiteralPath $outputPath -Encoding UTF8
        if ($LASTEXITCODE -ne 0) {
            throw "memory extraction candidate smoke returned exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }

    $result = Get-Content -LiteralPath $outputPath -Raw | ConvertFrom-Json
    if ($result.status -ne "COMPLETED") {
        throw "Expected COMPLETED status, got $($result.status)"
    }
    if ($result.candidate_count -ne 2) {
        throw "Expected 2 memory candidates, got $($result.candidate_count)"
    }
    if ($result.ordinary_message_count -ne 1) {
        throw "Expected 1 ordinary message, got $($result.ordinary_message_count)"
    }
    if (-not $result.report.requires_go_validation) {
        throw "Memory extraction candidates must require Go validation"
    }
    if ($result.report.raw_text_returned) {
        throw "Memory extraction result must not return raw text"
    }
    $profileCandidate = @($result.candidates | Where-Object { $_.memory_event_type -eq "PROFILE_SIGNAL" })[0]
    if (-not $profileCandidate) {
        throw "Expected PROFILE_SIGNAL candidate"
    }
    if ($profileCandidate.review_state -ne "NEEDS_REVIEW") {
        throw "PROFILE_SIGNAL candidate must require review"
    }
    if (-not ($profileCandidate.output_sha256 -match "^[0-9a-f]{64}$")) {
        throw "Candidate output_sha256 must be lowercase sha256 hex"
    }

    [pscustomobject]@{
        schema_version = 1
        smoke = "python-memory-extraction-candidate"
        status = "passed"
        candidate_count = $result.candidate_count
        ordinary_message_count = $result.ordinary_message_count
        profile_review_required = $true
        raw_text_returned = $false
        final_memory_persisted = $false
        scope = "local hash-only memory extraction candidate smoke; no external provider, no database, no business write"
    } | ConvertTo-Json -Depth 6
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
