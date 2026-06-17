$ErrorActionPreference = "Stop"

$checkScript = Join-Path $PSScriptRoot "check-project-naming.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source
$legacyName = ("aka" + "shic")

function Invoke-NamingCheck {
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-project-naming-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $tempRoot | Out-Null

    $cleanScript = Join-Path $tempRoot "clean.sh"
    Set-Content -LiteralPath $cleanScript -Encoding ASCII -Value @(
        "#!/usr/bin/env bash",
        "echo NexusIM"
    )
    $cleanTypescript = Join-Path $tempRoot "clean.ts"
    Set-Content -LiteralPath $cleanTypescript -Encoding ASCII -Value @(
        "export const projectName = 'NexusIM';"
    )

    $cleanResult = Invoke-NamingCheck -RepoRoot $tempRoot
    if ($cleanResult.ExitCode -ne 0) {
        Write-Host "FAIL clean shell fixture should pass project naming guard." -ForegroundColor Red
        if ($cleanResult.Output) {
            Write-Host $cleanResult.Output -ForegroundColor Red
        }
        exit 1
    }

    $legacyScript = Join-Path $tempRoot "legacy.sh"
    Set-Content -LiteralPath $legacyScript -Encoding ASCII -Value @(
        "#!/usr/bin/env bash",
        "echo $legacyName"
    )

    $legacyResult = Invoke-NamingCheck -RepoRoot $tempRoot
    if ($legacyResult.ExitCode -eq 0) {
        Write-Host "FAIL shell fixture with legacy project name should fail project naming guard." -ForegroundColor Red
        exit 1
    }

    Remove-Item -LiteralPath $legacyScript -Force
    $legacyTypescript = Join-Path $tempRoot "legacy.tsx"
    Set-Content -LiteralPath $legacyTypescript -Encoding ASCII -Value @(
        "export const legacyName = '$legacyName';"
    )
    $legacyTypescriptResult = Invoke-NamingCheck -RepoRoot $tempRoot
    if ($legacyTypescriptResult.ExitCode -eq 0) {
        Write-Host "FAIL TSX fixture with legacy project name should fail project naming guard." -ForegroundColor Red
        exit 1
    }

    Remove-Item -LiteralPath $legacyTypescript -Force
    $legacyPython = Join-Path $tempRoot "legacy.py"
    Set-Content -LiteralPath $legacyPython -Encoding ASCII -Value @(
        "PROJECT_NAME = '$legacyName'"
    )
    $legacyPythonResult = Invoke-NamingCheck -RepoRoot $tempRoot
    if ($legacyPythonResult.ExitCode -eq 0) {
        Write-Host "FAIL Python fixture with legacy project name should fail project naming guard." -ForegroundColor Red
        exit 1
    }

    Remove-Item -LiteralPath $legacyPython -Force
    $legacyBib = Join-Path $tempRoot "legacy.bib"
    Set-Content -LiteralPath $legacyBib -Encoding ASCII -Value @(
        "@misc{legacy,",
        "  title = {$legacyName memory note}",
        "}"
    )
    $legacyBibResult = Invoke-NamingCheck -RepoRoot $tempRoot
    if ($legacyBibResult.ExitCode -eq 0) {
        Write-Host "FAIL BibTeX fixture with legacy project name should fail project naming guard." -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   project naming self-test covers shell, frontend, scripting, and bibliography files."
