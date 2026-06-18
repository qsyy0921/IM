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
    $canonicalCmdDir = Join-Path $servicePath "cmd\$serviceName"
    $canonicalMain = Join-Path $canonicalCmdDir "main.go"
    $canonicalTest = Join-Path $canonicalCmdDir "main_test.go"

    if (-not (Test-Path -LiteralPath $canonicalMain)) {
        $violations.Add("services\$serviceName is missing canonical cmd\$serviceName\main.go")
        continue
    }
    if (-not (Test-Path -LiteralPath $canonicalTest)) {
        $violations.Add("$(Convert-ToRepoRelativePath -Path $canonicalMain): missing sibling main_test.go for startup/config guard coverage")
    }
}

$cmdMainFiles = Get-ChildItem -LiteralPath $servicesRoot -Recurse -Filter "main.go" -File |
    Where-Object { $_.FullName -match "\\cmd\\" }
foreach ($file in $cmdMainFiles) {
    $testFile = Join-Path $file.DirectoryName "main_test.go"
    if (-not (Test-Path -LiteralPath $testFile)) {
        $violations.Add("$(Convert-ToRepoRelativePath -Path $file.FullName): missing sibling main_test.go")
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   service cmd startup test guardrails"
