param(
    [int]$PromptMaxLines = 60,
    [int]$AgentGuideMaxLines = 140,
    [int]$RunbookIndexMaxLines = 40,
    [int]$CurrentBriefMaxLines = 60,
    [int]$CurrentGoalMaxLines = 80,
    [int]$RemainingGoalsMaxLines = 100,
    [int]$ServiceBriefIndexMaxLines = 40,
    [int]$ServiceBriefMaxLines = 30
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$checks = @(
    @{
        Path = Join-Path $repoRoot "prompt.md"
        MaxLines = $PromptMaxLines
        Purpose = "codex goal-box prompt"
    },
    @{
        Path = Join-Path $repoRoot "agent.md"
        MaxLines = $AgentGuideMaxLines
        Purpose = "agent progress guide"
    },
    @{
        Path = Join-Path $repoRoot "docs\runbook\README.md"
        MaxLines = $RunbookIndexMaxLines
        Purpose = "runbook index"
    },
    @{
        Path = Join-Path $repoRoot "docs\runbook\current-brief.md"
        MaxLines = $CurrentBriefMaxLines
        Purpose = "per-turn entrypoint"
    },
    @{
        Path = Join-Path $repoRoot "docs\runbook\current-goal.md"
        MaxLines = $CurrentGoalMaxLines
        Purpose = "long-term goal summary"
    },
    @{
        Path = Join-Path $repoRoot "docs\runbook\remaining-goals.md"
        MaxLines = $RemainingGoalsMaxLines
        Purpose = "remaining goals backlog"
    },
    @{
        Path = Join-Path $repoRoot "docs\runbook\service-briefs\README.md"
        MaxLines = $ServiceBriefIndexMaxLines
        Purpose = "service status index"
    }
)

$serviceBriefDir = Join-Path $repoRoot "docs\runbook\service-briefs"
if (Test-Path -LiteralPath $serviceBriefDir) {
    $serviceBriefFiles = Get-ChildItem -LiteralPath $serviceBriefDir -Filter "*.md" -File |
        Where-Object { $_.Name -ne "README.md" } |
        Sort-Object Name
    foreach ($serviceBrief in $serviceBriefFiles) {
        $checks += @{
            Path = $serviceBrief.FullName
            MaxLines = $ServiceBriefMaxLines
            Purpose = "single-service brief"
        }
    }
}

$failed = $false
foreach ($check in $checks) {
    if (-not (Test-Path -LiteralPath $check.Path)) {
        Write-Error "Missing $($check.Purpose): $($check.Path)"
    }

    $lineCount = (Get-Content -LiteralPath $check.Path).Count
    $relativePath = Resolve-Path -LiteralPath $check.Path -Relative
    if ($lineCount -gt $check.MaxLines) {
        Write-Host "FAIL $relativePath has $lineCount lines, max is $($check.MaxLines). Split details into docs/runbook/service-briefs/, docs/runbook/loadtest/, or docs/runbook/archive/." -ForegroundColor Red
        $failed = $true
        continue
    }
    Write-Host "OK   $relativePath has $lineCount/$($check.MaxLines) lines ($($check.Purpose))."
}

function Assert-Contains {
    param(
        [string]$Text,
        [string]$Needle,
        [string]$Message
    )
    if (-not $Text.Contains($Needle)) {
        Write-Host "FAIL $Message" -ForegroundColor Red
        $script:failed = $true
    }
}

$promptPath = Join-Path $repoRoot "prompt.md"
$agentGuidePath = Join-Path $repoRoot "agent.md"
$prompt = Get-Content -LiteralPath $promptPath -Raw
$agentGuide = Get-Content -LiteralPath $agentGuidePath -Raw

Assert-Contains $prompt "agent.md" "prompt.md must route Codex to agent.md."
Assert-Contains $prompt "docs/runbook/current-goal.md" "prompt.md must route concrete goals to current-goal.md."
Assert-Contains $prompt "docs/runbook/remaining-goals.md" "prompt.md must route unfinished work to remaining-goals.md."
Assert-Contains $agentGuide 'Read `prompt.md`' "agent.md must tell Codex to read prompt.md."
Assert-Contains $agentGuide 'Read `agent.md`' "agent.md must explicitly include its own progress-management rules in the startup checklist."
Assert-Contains $agentGuide "Do not read long SDD" "agent.md must keep long-document reads opt-in."

if ($failed) {
    exit 1
}
