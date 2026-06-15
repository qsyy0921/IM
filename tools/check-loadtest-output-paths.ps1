param(
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$allowedInternalReferences = @(
    "tools\check-file-size-budget.ps1",
    "tools\check-project-naming.ps1",
    "tools\check-powershell-scripts.ps1",
    "tools\check-shell-scripts.ps1",
    "tools\check-loadtest-output-paths.ps1"
)
$excludedDirectories = @(
    "loadtest\results"
)
$scannedExtensions = @(
    ".go",
    ".ps1",
    ".sh"
)

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\") + "\"
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if ($fullPath.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($root.Length) -replace "/", "\"
    }
    return $fullPath -replace "/", "\"
}

function Test-IsAllowedInternalReference {
    param([string]$RelativePath)

    foreach ($allowed in $allowedInternalReferences) {
        if ($RelativePath -eq $allowed) {
            return $true
        }
    }
    return $false
}

function Test-IsExcluded {
    param([string]$RelativePath)

    foreach ($directory in $excludedDirectories) {
        if ($RelativePath -eq $directory -or $RelativePath.StartsWith("$directory\")) {
            return $true
        }
    }
    return $false
}

$scanRoots = @(
    (Join-Path $repoRoot "loadtest"),
    (Join-Path $repoRoot "tools")
)

$files = foreach ($root in $scanRoots) {
    if (Test-Path -LiteralPath $root) {
        Get-ChildItem -LiteralPath $root -Recurse -File |
            Where-Object { $_.Extension -in $scannedExtensions }
    }
}

$failures = @()
foreach ($file in ($files | Sort-Object FullName)) {
    $relativePath = Convert-ToRepoRelativePath -Path $file.FullName
    if (Test-IsAllowedInternalReference -RelativePath $relativePath) {
        continue
    }
    if (Test-IsExcluded -RelativePath $relativePath) {
        continue
    }

    $lineNumber = 0
    foreach ($line in Get-Content -LiteralPath $file.FullName) {
        $lineNumber++
        if ($line -match "loadtest[\\/]+results") {
            $failures += [pscustomobject]@{
                Path = $relativePath
                Line = $lineNumber
                Text = $line.Trim()
            }
        }
    }
}

foreach ($failure in $failures) {
    Write-Host "FAIL $($failure.Path):$($failure.Line) uses repo-local loadtest results path: $($failure.Text)" -ForegroundColor Red
}

if ($failures.Count -gt 0) {
    Write-Host "Raw loadtest output must default to $ExpectedResultRoot; keep only reports/docs in the repo." -ForegroundColor Red
    exit 1
}

Write-Host "OK   loadtest output defaults avoid repo-local loadtest/results; expected raw root is $ExpectedResultRoot."
