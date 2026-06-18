$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "service-registry.ps1")

$servicesRoot = Join-Path $repoRoot "services"

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\", "/")
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if ($fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($prefix.Length)
    }
    return $fullPath
}

$requiredEndpoints = @(
    "/healthz",
    "/readyz",
    "/debug/metrics",
    "/metrics"
)

$violations = [System.Collections.Generic.List[string]]::new()

$activeServices = Get-NexusIMRegistryServiceNames -RepoRoot $repoRoot -Active
$actualServiceDirs = @(Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name | Select-Object -ExpandProperty Name)
$serviceDirDiff = Compare-Object -ReferenceObject $activeServices -DifferenceObject $actualServiceDirs
if ($serviceDirDiff) {
    $diffText = ($serviceDirDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Service registry active services mismatch with services directory: $diffText"
}

foreach ($serviceName in $activeServices) {
    $servicePath = Join-Path $servicesRoot $serviceName
    $productionFiles = @(Get-ChildItem -LiteralPath $servicePath -Recurse -Filter "*.go" -File |
        Where-Object { $_.Name -notlike "*_test.go" })
    if ($productionFiles.Count -eq 0) {
        $violations.Add("services\$serviceName has no production Go files")
        continue
    }

    $content = ($productionFiles | ForEach-Object { Get-Content -LiteralPath $_.FullName -Raw }) -join [Environment]::NewLine
    foreach ($endpoint in $requiredEndpoints) {
        if (-not $content.Contains("`"$endpoint`"")) {
            $mainPath = Join-Path $servicePath "cmd\$serviceName\main.go"
            if (Test-Path -LiteralPath $mainPath) {
                $violations.Add("$(Convert-ToRepoRelativePath -Path $mainPath): service is missing runtime endpoint $endpoint")
            }
            else {
                $violations.Add("services\${serviceName}: service is missing runtime endpoint $endpoint")
            }
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   service runtime endpoint guardrails"
