param(
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RepoRoot = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $repoRoot = Split-Path -Parent $PSScriptRoot
}
else {
    $repoRoot = [System.IO.Path]::GetFullPath($RepoRoot)
}
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

    if ($RelativePath.StartsWith("tools\check-") -and $RelativePath.EndsWith(".ps1")) {
        return $true
    }

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

function Test-UsesOutputRootHelper {
    param([string]$Text)

    return $Text -match '(?m)^\s*\.\s*\([^\r\n]*output-root-safety\.ps1'
}

function Test-CallsOutputRootGuard {
    param([string]$Text)

    return $Text -match '(?m)^\s*Assert-ExternalOutputRoot\b'
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
$guardFailures = @()
foreach ($file in ($files | Sort-Object FullName)) {
    $relativePath = Convert-ToRepoRelativePath -Path $file.FullName
    if (Test-IsAllowedInternalReference -RelativePath $relativePath) {
        continue
    }
    if (Test-IsExcluded -RelativePath $relativePath) {
        continue
    }

    $fileText = Get-Content -LiteralPath $file.FullName -Raw
    if ($fileText -match "\[string\]\`$(ResultRoot|OutputRoot)") {
        if (-not (Test-UsesOutputRootHelper -Text $fileText)) {
            $guardFailures += [pscustomobject]@{
                Path = $relativePath
                Text = "ResultRoot/OutputRoot writer is missing output-root-safety.ps1 helper"
            }
        }
        if (-not (Test-CallsOutputRootGuard -Text $fileText)) {
            $guardFailures += [pscustomobject]@{
                Path = $relativePath
                Text = "ResultRoot/OutputRoot writer is missing an active Assert-ExternalOutputRoot call"
            }
        }
    }

    $lineNumber = 0
    foreach ($line in ($fileText -split "`r?`n")) {
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

foreach ($failure in $guardFailures) {
    Write-Host "FAIL $($failure.Path) is missing output-root guard: $($failure.Text)" -ForegroundColor Red
}
foreach ($failure in $failures) {
    Write-Host "FAIL $($failure.Path):$($failure.Line) uses repo-local loadtest results path: $($failure.Text)" -ForegroundColor Red
}

if ($guardFailures.Count -gt 0 -or $failures.Count -gt 0) {
    Write-Host "Raw loadtest output must default to $ExpectedResultRoot, and ResultRoot/OutputRoot writers must reject repository-local output roots." -ForegroundColor Red
    exit 1
}

Write-Host "OK   loadtest output defaults and ResultRoot/OutputRoot guards keep raw output outside the repository; expected raw root is $ExpectedResultRoot."
