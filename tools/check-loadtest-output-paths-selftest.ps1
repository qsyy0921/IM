$ErrorActionPreference = "Stop"

$checkScript = Join-Path $PSScriptRoot "check-loadtest-output-paths.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

function Invoke-OutputPathCheck {
    param([string]$RepoRoot)

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $checkScript -RepoRoot $RepoRoot 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
            Output = (($output | Out-String).Trim())
        }
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
}

function Set-FixtureFile {
    param(
        [string]$Path,
        [string[]]$Lines
    )

    $directory = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $directory)) {
        New-Item -ItemType Directory -Path $directory | Out-Null
    }
    Set-Content -LiteralPath $Path -Encoding ASCII -Value $Lines
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-output-paths-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $tempRoot | Out-Null

    $cleanScript = Join-Path $tempRoot "loadtest\demo\run-local-smoke.ps1"
    Set-FixtureFile -Path $cleanScript -Lines @(
        "param(",
        "    [string]`$ResultRoot = 'H:\NexusIM\loadtest-results'",
        ")",
        ". (Join-Path `$PSScriptRoot '..\..\tools\output-root-safety.ps1')",
        "Assert-ExternalOutputRoot -Value `$ResultRoot -RepositoryRoot 'E:\development\IM'",
        "Write-Host 'ok'"
    )

    $cleanResult = Invoke-OutputPathCheck -RepoRoot $tempRoot
    if ($cleanResult.ExitCode -ne 0) {
        Write-Host "FAIL fixture with helper and active guard should pass loadtest output path guard." -ForegroundColor Red
        if ($cleanResult.Output) {
            Write-Host $cleanResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $cleanEmptyDefaultScript = Join-Path $tempRoot "loadtest\demo\run-local-gradient.ps1"
    Set-FixtureFile -Path $cleanEmptyDefaultScript -Lines @(
        "param(",
        "    [string]`$ResultRoot = """"",
        ")",
        "if (-not `$ResultRoot) {",
        "    `$ResultRoot = Join-Path 'H:\NexusIM\loadtest-results' ('gradient-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))",
        "}",
        ". (Join-Path `$PSScriptRoot '..\..\tools\output-root-safety.ps1')",
        "Assert-ExternalOutputRoot -Value `$ResultRoot -RepositoryRoot 'E:\development\IM'",
        "Write-Host 'ok'"
    )

    $cleanEmptyDefaultResult = Invoke-OutputPathCheck -RepoRoot $tempRoot
    if ($cleanEmptyDefaultResult.ExitCode -ne 0) {
        Write-Host "FAIL fixture with early H-drive fallback should pass loadtest output path guard." -ForegroundColor Red
        if ($cleanEmptyDefaultResult.Output) {
            Write-Host $cleanEmptyDefaultResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $missingHelper = Join-Path $tempRoot "tools\bad-missing-helper.ps1"
    Set-FixtureFile -Path $missingHelper -Lines @(
        "param(",
        "    [string]`$OutputRoot = 'H:\NexusIM\loadtest-results'",
        ")",
        "Assert-ExternalOutputRoot -Value `$OutputRoot -RepositoryRoot 'E:\development\IM'"
    )

    $missingHelperResult = Invoke-OutputPathCheck -RepoRoot $tempRoot
    if ($missingHelperResult.ExitCode -eq 0 -or $missingHelperResult.Output -notmatch "missing output-root-safety\.ps1 helper") {
        Write-Host "FAIL fixture without helper should fail loadtest output path guard." -ForegroundColor Red
        if ($missingHelperResult.Output) {
            Write-Host $missingHelperResult.Output -ForegroundColor Red
        }
        exit 1
    }

    Remove-Item -LiteralPath $missingHelper -Force
    $commentOnly = Join-Path $tempRoot "tools\bad-comment-only.ps1"
    Set-FixtureFile -Path $commentOnly -Lines @(
        "param(",
        "    [string]`$OutputRoot = 'H:\NexusIM\loadtest-results'",
        ")",
        ". (Join-Path `$PSScriptRoot 'output-root-safety.ps1')",
        "# Assert-ExternalOutputRoot -Value `$OutputRoot -RepositoryRoot 'E:\development\IM'",
        "Write-Host 'unguarded'"
    )

    $commentOnlyResult = Invoke-OutputPathCheck -RepoRoot $tempRoot
    if ($commentOnlyResult.ExitCode -eq 0 -or $commentOnlyResult.Output -notmatch "missing an active Assert-ExternalOutputRoot call") {
        Write-Host "FAIL fixture with comment-only guard should fail loadtest output path guard." -ForegroundColor Red
        if ($commentOnlyResult.Output) {
            Write-Host $commentOnlyResult.Output -ForegroundColor Red
        }
        exit 1
    }

    Remove-Item -LiteralPath $commentOnly -Force
    $lateFallback = Join-Path $tempRoot "loadtest\demo\bad-late-fallback.ps1"
    Set-FixtureFile -Path $lateFallback -Lines @(
        "param(",
        "    [string]`$ResultRoot = """"",
        ")",
        ". (Join-Path `$PSScriptRoot '..\..\tools\output-root-safety.ps1')",
        "Assert-ExternalOutputRoot -Value `$ResultRoot -RepositoryRoot 'E:\development\IM'",
        "if (-not `$ResultRoot) {",
        "    `$ResultRoot = Join-Path 'H:\NexusIM\loadtest-results' ('late-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))",
        "}"
    )

    $lateFallbackResult = Invoke-OutputPathCheck -RepoRoot $tempRoot
    if ($lateFallbackResult.ExitCode -eq 0 -or $lateFallbackResult.Output -notmatch "fallback after Assert-ExternalOutputRoot") {
        Write-Host "FAIL fixture with late H-drive fallback should fail loadtest output path guard." -ForegroundColor Red
        if ($lateFallbackResult.Output) {
            Write-Host $lateFallbackResult.Output -ForegroundColor Red
        }
        exit 1
    }

    Remove-Item -LiteralPath $lateFallback -Force
    $repoLocalReference = Join-Path $tempRoot "loadtest\demo\main.go"
    Set-FixtureFile -Path $repoLocalReference -Lines @(
        "package main",
        "",
        "const badDefaultResultRoot = ""loadtest/results/demo"""
    )

    $repoLocalReferenceResult = Invoke-OutputPathCheck -RepoRoot $tempRoot
    if ($repoLocalReferenceResult.ExitCode -eq 0 -or $repoLocalReferenceResult.Output -notmatch "repo-local loadtest results path") {
        Write-Host "FAIL fixture with repo-local loadtest results path should fail loadtest output path guard." -ForegroundColor Red
        if ($repoLocalReferenceResult.Output) {
            Write-Host $repoLocalReferenceResult.Output -ForegroundColor Red
        }
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   loadtest output path guard self-test covers helper, active guard, fallback order, and repo-local path requirements."
