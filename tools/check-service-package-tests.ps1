$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$goEnvPath = Join-Path $PSScriptRoot "go-env.ps1"

if (Test-Path -LiteralPath $goEnvPath) {
    . $goEnvPath
}

Push-Location $repoRoot
try {
    & go test ./services/...
    if ($LASTEXITCODE -ne 0) {
        throw "go test ./services/... failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Write-Host "OK   service package tests passed."
