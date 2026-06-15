param(
    [string]$ImagePrefix = "nexusim",
    [string]$ImageTag = "local",
    [string[]]$Services = @(),
    [string]$Platform = "linux/amd64",
    [string]$BinaryRoot = "bin\linux",
    [switch]$SkipGoBuild,
    [switch]$SkipDockerBuild,
    [switch]$ListServices
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$dockerRoot = Join-Path $repoRoot "deploy\docker"
$goEnvPath = Join-Path $PSScriptRoot "go-env.ps1"

function Get-ImplementedServices {
    Get-ChildItem -LiteralPath (Join-Path $repoRoot "services") -Directory |
        Sort-Object Name |
        Select-Object -ExpandProperty Name
}

function Get-GoArchFromPlatform {
    param([string]$TargetPlatform)

    switch ($TargetPlatform) {
        "linux/amd64" { return "amd64" }
        "linux/arm64" { return "arm64" }
        default {
            throw "Unsupported platform '$TargetPlatform'. Supported values: linux/amd64, linux/arm64."
        }
    }
}

function Get-ServiceCommand {
    param([string]$Service)

    $commandPath = Join-Path $repoRoot "services\$Service\cmd\$Service"
    if (-not (Test-Path -LiteralPath $commandPath)) {
        throw "Unknown service or missing service command: $Service"
    }
    return "./services/$Service/cmd/$Service"
}

if ($Services.Count -eq 0) {
    $Services = @(Get-ImplementedServices)
}

if ($ListServices) {
    $Services | ForEach-Object { Write-Output $_ }
    return
}

$goArch = Get-GoArchFromPlatform -TargetPlatform $Platform

foreach ($service in $Services) {
    [void](Get-ServiceCommand -Service $service)
    $dockerfile = Join-Path $dockerRoot "$service.runtime.Dockerfile"
    if (-not (Test-Path -LiteralPath $dockerfile)) {
        throw "Missing runtime Dockerfile for $service`: $dockerfile"
    }
}

Push-Location $repoRoot
try {
    if (Test-Path -LiteralPath $goEnvPath) {
        . $goEnvPath
    }

    $binaryRootPath = Join-Path $repoRoot $BinaryRoot
    New-Item -ItemType Directory -Force $binaryRootPath | Out-Null

    $previousGOOS = $env:GOOS
    $previousGOARCH = $env:GOARCH
    $previousCGO = $env:CGO_ENABLED

    try {
        if (-not $SkipGoBuild) {
            $env:GOOS = "linux"
            $env:GOARCH = $goArch
            $env:CGO_ENABLED = "0"
            foreach ($service in $Services) {
                $outputPath = Join-Path $binaryRootPath $service
                go build -o $outputPath (Get-ServiceCommand -Service $service)
            }
        }
    }
    finally {
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

    if (-not $SkipDockerBuild) {
        foreach ($service in $Services) {
            docker build --platform $Platform -f "deploy/docker/$service.runtime.Dockerfile" -t "$ImagePrefix/${service}:$ImageTag" .
        }
    }
}
finally {
    Pop-Location
}

Write-Host "OK   built $($Services.Count) service Docker images for $Platform."
