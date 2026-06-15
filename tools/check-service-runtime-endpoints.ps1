$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
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

foreach ($service in (Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name)) {
    $productionFiles = @(Get-ChildItem -LiteralPath $service.FullName -Recurse -Filter "*.go" -File |
        Where-Object { $_.Name -notlike "*_test.go" })
    if ($productionFiles.Count -eq 0) {
        $violations.Add("services\$($service.Name) has no production Go files")
        continue
    }

    $content = ($productionFiles | ForEach-Object { Get-Content -LiteralPath $_.FullName -Raw }) -join [Environment]::NewLine
    foreach ($endpoint in $requiredEndpoints) {
        if (-not $content.Contains("`"$endpoint`"")) {
            $mainPath = Join-Path $service.FullName "cmd\$($service.Name)\main.go"
            if (Test-Path -LiteralPath $mainPath) {
                $violations.Add("$(Convert-ToRepoRelativePath -Path $mainPath): service is missing runtime endpoint $endpoint")
            }
            else {
                $violations.Add("services\$($service.Name): service is missing runtime endpoint $endpoint")
            }
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   service runtime endpoint guardrails"
