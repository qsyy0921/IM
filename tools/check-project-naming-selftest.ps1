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
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   project naming self-test covers shell scripts."
