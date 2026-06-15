$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$serviceLayers = @("api", "app", "domain", "trigger", "types")

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
$importPattern = '"[^"]*internal/infrastructure[^"]*"'

foreach ($service in (Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name)) {
    $internalRoot = Join-Path $service.FullName "internal"
    if (-not (Test-Path -LiteralPath $internalRoot)) {
        continue
    }

    foreach ($layer in $serviceLayers) {
        $layerRoot = Join-Path $internalRoot $layer
        if (-not (Test-Path -LiteralPath $layerRoot)) {
            continue
        }

        $goFiles = Get-ChildItem -LiteralPath $layerRoot -Recurse -Filter "*.go" -File |
            Where-Object { $_.Name -notlike "*_test.go" }
        foreach ($file in $goFiles) {
            $content = Get-Content -LiteralPath $file.FullName -Raw
            foreach ($match in [regex]::Matches($content, $importPattern)) {
                $violations.Add("$(Convert-ToRepoRelativePath -Path $file.FullName): forbidden infrastructure import $($match.Value)")
            }
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   DDD boundary import guardrails"
