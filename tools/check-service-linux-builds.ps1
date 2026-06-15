param(
    [string[]]$Architectures = @("amd64", "arm64")
)

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

$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED

Push-Location $repoRoot
try {
    $env:GOOS = "linux"
    $env:CGO_ENABLED = "0"

    foreach ($architecture in $Architectures) {
        $env:GOARCH = $architecture
        & go build @packages
        if ($LASTEXITCODE -ne 0) {
            throw "go build for linux/$architecture service commands failed with exit code $LASTEXITCODE"
        }
    }
}
finally {
    Pop-Location

    if ($null -eq $previousGOOS) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    }
    else {
        $env:GOOS = $previousGOOS
    }

    if ($null -eq $previousGOARCH) {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    }
    else {
        $env:GOARCH = $previousGOARCH
    }

    if ($null -eq $previousCGO) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    else {
        $env:CGO_ENABLED = $previousCGO
    }
}

Write-Host "OK   linux service command builds checked ($($packages.Count) services; $($Architectures -join ', '))."
