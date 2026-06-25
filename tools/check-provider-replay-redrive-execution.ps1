$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    & go test ./loadtest/actionexecutor -count=1
    if ($LASTEXITCODE -ne 0) {
        throw "go test ./loadtest/actionexecutor failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

Write-Host "OK   provider replay controlled redrive execution operator self-test"
