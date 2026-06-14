$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$serviceBriefRoot = Join-Path $repoRoot "docs\runbook\service-briefs"
$sddRoot = Join-Path $repoRoot "docs\sdd"

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

$serviceNames = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
Get-ChildItem -LiteralPath $servicesRoot -Directory | ForEach-Object {
    [void]$serviceNames.Add($_.Name)
}

$briefNames = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
$briefFiles = Get-ChildItem -LiteralPath $serviceBriefRoot -Filter "*.md" -File |
    Where-Object { $_.Name -ne "README.md" }
foreach ($brief in $briefFiles) {
    [void]$briefNames.Add($brief.BaseName)
}

$violations = [System.Collections.Generic.List[string]]::new()
foreach ($serviceName in ($serviceNames | Sort-Object)) {
    if (-not $briefNames.Contains($serviceName)) {
        $violations.Add("services\$serviceName is missing docs\runbook\service-briefs\$serviceName.md")
        continue
    }

    $briefPath = Join-Path $serviceBriefRoot "$serviceName.md"
    $briefContent = Get-Content -LiteralPath $briefPath -Raw
    if ($briefContent.Contains("服务代码尚未实现") -or $briefContent.Contains("尚未实现")) {
        $violations.Add("$(Convert-ToRepoRelativePath -Path $briefPath): implemented service brief still says service is not implemented")
    }
}

foreach ($brief in ($briefFiles | Sort-Object Name)) {
    $briefName = $brief.BaseName
    if ($serviceNames.Contains($briefName)) {
        continue
    }

    $briefContent = Get-Content -LiteralPath $brief.FullName -Raw
    $isExplicitFutureBrief = $briefContent.Contains("服务代码尚未实现") -or
        $briefContent.Contains("尚未实现") -or
        $briefContent.Contains("SDD v0.1 draft")
    if (-not $isExplicitFutureBrief) {
        $violations.Add("$(Convert-ToRepoRelativePath -Path $brief.FullName): brief has no matching services\$briefName directory and is not explicitly marked as future/draft")
    }

    $sddPath = Join-Path $sddRoot "$briefName.md"
    if (-not (Test-Path -LiteralPath $sddPath)) {
        $violations.Add("$(Convert-ToRepoRelativePath -Path $brief.FullName): future/draft service brief is missing docs\sdd\$briefName.md")
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   service brief sync guardrails"
