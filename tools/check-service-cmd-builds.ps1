$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$goEnvPath = Join-Path $PSScriptRoot "go-env.ps1"

if (Test-Path -LiteralPath $goEnvPath) {
    . $goEnvPath
}

$packages = @()
foreach ($service in (Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name)) {
    $cmdDir = Join-Path $service.FullName "cmd\$($service.Name)"
    $mainPath = Join-Path $cmdDir "main.go"
    if (-not (Test-Path -LiteralPath $mainPath)) {
        throw "services\$($service.Name) is missing canonical cmd\$($service.Name)\main.go"
    }
    $packages += "./services/$($service.Name)/cmd/$($service.Name)"
}

if ($packages.Count -eq 0) {
    throw "No service command packages found under services\."
}

Push-Location $repoRoot
try {
    & go build @packages
    if ($LASTEXITCODE -ne 0) {
        throw "go build for service commands failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Write-Host "OK   service command builds checked ($($packages.Count) services)."
