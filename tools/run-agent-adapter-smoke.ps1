param(
    [string]$PGDSN = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$AgentTarget = "127.0.0.1:10630",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$Objective = "phoenix launch decision",
    [string]$ToolName = "conversation.note.create",
    [string]$ResourceType = "conversation_note",
    [string]$RiskLevel = "LOW",
    [string]$RequestTimeout = "10s"
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if ([string]::IsNullOrWhiteSpace($RunName)) {
    $RunName = "agent-adapter-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

$runArgs = @(
    "run", "./loadtest/agent",
    "-pg-dsn", $PGDSN,
    "-agent-target", $AgentTarget,
    "-result-root", $ResultRoot,
    "-run-name", $RunName,
    "-objective", $Objective,
    "-tool-name", $ToolName,
    "-resource-type", $ResourceType,
    "-risk-level", $RiskLevel,
    "-request-timeout", $RequestTimeout
)

Push-Location $repoRoot
try {
    & go @runArgs
    if ($LASTEXITCODE -ne 0) {
        throw "loadtest/agent failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$resultDir = Join-Path $ResultRoot $RunName
$summaryPath = Join-Path $resultDir "agent-proposal-summary.json"
if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
    throw "agent smoke output missing: $summaryPath"
}

$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
if ($summary.proposal_status -ne "PROPOSED") {
    throw "unexpected proposal_status: $($summary.proposal_status)"
}
if (-not [bool]$summary.requires_approval) {
    throw "agent proposal did not require approval"
}
if ([bool]$summary.generated_by_llm) {
    throw "first-stage agent smoke must not claim LLM generation"
}
if (-not [bool]$summary.policy_allowed) {
    throw "agent smoke did not observe an allowed tool policy"
}
if ([int]$summary.citation_count -lt 1) {
    throw "agent smoke missing citations"
}
if ([int]$summary.evidence_item_count -lt 2) {
    throw "agent smoke missing expected evidence items"
}

Write-Host "OK   agent adapter smoke passed: $summaryPath"
