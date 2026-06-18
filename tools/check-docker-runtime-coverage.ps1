$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "service-registry.ps1")

$servicesRoot = Join-Path $repoRoot "services"
$dockerRoot = Join-Path $repoRoot "deploy\docker"

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\") + "\"
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if ($fullPath.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($root.Length) -replace "/", "\"
    }
    return $fullPath -replace "/", "\"
}

$implementedServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Active
$actualServiceDirs = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name | Select-Object -ExpandProperty Name)
$serviceDirDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $actualServiceDirs
if ($serviceDirDiff) {
    $diffText = ($serviceDirDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Service registry active services mismatch with services directory: $diffText"
}

$runtimeDockerfiles = @(Get-ChildItem -LiteralPath $dockerRoot -File -Filter "*.runtime.Dockerfile" |
    Where-Object { $_.Name -notlike "*loadtest*" } |
    Sort-Object Name)
$dockerServices = @($runtimeDockerfiles | ForEach-Object { $_.Name -replace "\.runtime\.Dockerfile$", "" } | Sort-Object)

$serviceDiff = Compare-Object -ReferenceObject $implementedServices -DifferenceObject $dockerServices
if ($serviceDiff) {
    $diffText = ($serviceDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Docker runtime coverage mismatch with services directory: $diffText"
}

foreach ($service in $implementedServices) {
    $dockerfile = Join-Path $dockerRoot "$service.runtime.Dockerfile"
    if (-not (Test-Path -LiteralPath $dockerfile)) {
        throw "Missing Docker runtime file: $(Convert-ToRepoRelativePath -Path $dockerfile)"
    }

    $content = Get-Content -LiteralPath $dockerfile -Raw
    $copyPattern = "COPY\s+bin/linux/$([regex]::Escape($service))\s+/$([regex]::Escape($service))"
    $entrypointPattern = "ENTRYPOINT\s+\[\s*`"/$([regex]::Escape($service))`"\s*\]"
    if ($content -notmatch $copyPattern) {
        throw "$(Convert-ToRepoRelativePath -Path $dockerfile): runtime Dockerfile must copy bin/linux/$service to /$service"
    }
    if ($content -notmatch $entrypointPattern) {
        throw "$(Convert-ToRepoRelativePath -Path $dockerfile): runtime Dockerfile must enter /$service"
    }
}

Write-Host "OK   Docker runtime coverage for $($implementedServices.Count) services."
