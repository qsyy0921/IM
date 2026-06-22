param(
    [switch]$StagedOnly
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$hiddenAlternateTerm = "fall" + "back"
$recoveryWord = "re" + "covery"
$recoveryUsedField = "recovery" + "_used"
$forbiddenPhrases = @(
    "\b$hiddenAlternateTerm\b",
    "hidden alternate",
    "silent alternate",
    ("static " + $recoveryWord),
    ("legacy staticpolicy " + $recoveryWord),
    ("local " + $recoveryWord + " key"),
    ("gateway-token / push-token " + $recoveryWord),
    ($recoveryWord + " chain"),
    ($recoveryWord + " flag"),
    $recoveryUsedField
)
$forbiddenPattern = "(?i)" + ($forbiddenPhrases -join "|")

$allowedPolicyPaths = @(
    "agent.md",
    "prompt.md",
    "docs/architecture/README.md",
    "docs/architecture/fail-closed-policy.md",
    "docs/architecture/target-architecture-complete.md",
    "docs/runbook/current-brief.md",
    "docs/runbook/current-goal.md",
    "docs/runbook/remaining-goals.md",
    "tools/check-local.ps1",
    "tools/check-fail-closed-policy.ps1"
)

function Normalize-PathForDiff {
    param([string]$Path)
    return $Path.Replace("\", "/")
}

function Test-AllowedPolicyPath {
    param([string]$Path)
    $normalized = Normalize-PathForDiff -Path $Path
    return $allowedPolicyPaths -contains $normalized
}

function Get-AddedForbiddenLines {
    param([string[]]$GitArgs)

    $currentPath = ""
    $results = New-Object System.Collections.Generic.List[object]
    $diffLines = & git -C $repoRoot @GitArgs
    if ($LASTEXITCODE -ne 0) {
        throw "git diff failed with exit code $LASTEXITCODE"
    }

    foreach ($line in $diffLines) {
        if ($line -match '^\+\+\+ b/(.+)$') {
            $currentPath = $Matches[1]
            continue
        }
        if ($line -match '^\+\+\+ /dev/null$') {
            $currentPath = ""
            continue
        }
        if ($line -notmatch '^\+' -or $line.StartsWith("+++")) {
            continue
        }
        if ($line -notmatch $forbiddenPattern) {
            continue
        }
        if ($currentPath -eq "" -or (Test-AllowedPolicyPath -Path $currentPath)) {
            continue
        }
        $results.Add([pscustomobject]@{
            Path = $currentPath
            Line = $line.Substring(1)
        })
    }
    return $results
}

$checks = @()
if (-not $StagedOnly) {
    $checks += @{
        Name = "working tree"
        Args = @("diff", "--unified=0", "--no-ext-diff", "--", ".")
    }
}
$checks += @{
    Name = "staged"
    Args = @("diff", "--cached", "--unified=0", "--no-ext-diff", "--", ".")
}

$failed = $false
foreach ($check in $checks) {
    $violations = @(Get-AddedForbiddenLines -GitArgs $check.Args)
    if ($violations.Count -eq 0) {
        Write-Host "OK   no new hidden alternate-path terms in $($check.Name) diff"
        continue
    }

    $failed = $true
    Write-Host "FAIL new hidden alternate-path terms in $($check.Name) diff:" -ForegroundColor Red
    foreach ($violation in $violations) {
        Write-Host ("  {0}: {1}" -f $violation.Path, $violation.Line) -ForegroundColor Red
    }
}

if ($failed) {
    Write-Host ""
    Write-Host "Use fail-closed, explicit retry, repair/redrive, local-test adapter, or compat-window terminology instead." -ForegroundColor Yellow
    Write-Host "If this is a deliberate policy change, update docs/architecture/fail-closed-policy.md first." -ForegroundColor Yellow
    exit 1
}
