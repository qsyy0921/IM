param(
    [int]$RunbookIndexMaxLines = 40,
    [int]$CurrentBriefMaxLines = 60,
    [int]$CurrentGoalMaxLines = 80,
    [int]$ServiceBriefIndexMaxLines = 40,
    [int]$ServiceBriefMaxLines = 30
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$checks = @(
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

if ($failed) {
    exit 1
}
