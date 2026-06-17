$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")

function Assert-Passes {
    param(
        [string]$Path,
        [string]$Message
    )

    try {
        Assert-ExternalOutputRoot -Value $Path -RepositoryRoot $repoRoot -Name "ResultRoot"
    }
    catch {
        Write-Host "FAIL $Message" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }
}

function Assert-Fails {
    param(
        [string]$Path,
        [string]$ExpectedPattern,
        [string]$Message
    )

    try {
        Assert-ExternalOutputRoot -Value $Path -RepositoryRoot $repoRoot -Name "ResultRoot"
        Write-Host "FAIL $Message" -ForegroundColor Red
        exit 1
    }
    catch {
        if ($_.Exception.Message -notmatch $ExpectedPattern) {
            Write-Host "FAIL $Message returned unexpected error." -ForegroundColor Red
            Write-Host $_.Exception.Message -ForegroundColor Red
            exit 1
        }
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-output-root-" + [System.Guid]::NewGuid().ToString("N"))
$repoSibling = [System.IO.Path]::GetFullPath($repoRoot + "-scratch")

Assert-Passes -Path $tempRoot -Message "Temp output root should be accepted."
Assert-Passes -Path $repoSibling -Message "Path sharing only a textual prefix with repo root should be accepted."
Assert-Fails -Path $repoRoot -ExpectedPattern "must not be inside the repository" -Message "Repository root should be rejected."
Assert-Fails -Path (Join-Path $repoRoot "loadtest\results\bad") -ExpectedPattern "must not be inside the repository" -Message "Repository child output root should be rejected."
Assert-Fails -Path "   " -ExpectedPattern "must not be empty" -Message "Blank output root should be rejected."

try {
    Assert-ExternalOutputRoot -Value (Join-Path $repoRoot "tmp-observation") -RepositoryRoot $repoRoot -Name "OutputRoot"
    Write-Host "FAIL OutputRoot name should be included in repository child rejection." -ForegroundColor Red
    exit 1
}
catch {
    if ($_.Exception.Message -notmatch "OutputRoot must not be inside the repository") {
        Write-Host "FAIL OutputRoot rejection returned unexpected error." -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }
}

Write-Host "OK   output root safety helper self-test"
