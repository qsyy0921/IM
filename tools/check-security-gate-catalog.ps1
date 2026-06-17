$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-security-gate-catalog.ps1"
if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing security gate catalog validator: $validator"
}

function Write-JsonFile {
    param(
        [string]$Path,
        $Value
    )

    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $Value | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Validator {
    param(
        [string]$CatalogPath
    )

    try {
        $output = & $validator -CatalogPath $CatalogPath 2>&1
        return [pscustomobject]@{
            ExitCode = 0
            Output = (($output | Out-String).Trim())
        }
    }
    catch {
        return [pscustomobject]@{
            ExitCode = 1
            Output = [string]$_.Exception.Message
        }
    }
}

$repoCatalog = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\security-gate-catalog.json"
$repoResult = Invoke-Validator -CatalogPath $repoCatalog
if ($repoResult.ExitCode -ne 0) {
    Write-Host "FAIL repo security gate catalog should pass." -ForegroundColor Red
    Write-Host $repoResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-security-gate-catalog-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $badCatalog = Join-Path $tempRoot "bad-security-gate-catalog.json"
    Write-JsonFile -Path $badCatalog -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "missing check-local entry"
                category = "listener-boundary"
                script = "tools/check-public-listener-auth-guards.ps1"
                check_local_label = "missing label"
                note = "fixture"
            }
        )
    })

    $badResult = Invoke-Validator -CatalogPath $badCatalog
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL catalog with missing check-local label should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("label")) {
        Write-Host "FAIL bad security gate catalog returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   security gate catalog self-test"
