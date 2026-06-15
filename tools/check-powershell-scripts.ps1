param(
    [string[]]$ScanRoots = @("tools", "loadtest"),
    [string[]]$ExcludedDirectories = @("loadtest\results")
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\") + "\"
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if ($fullPath.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($root.Length) -replace "/", "\"
    }
    return $fullPath -replace "/", "\"
}

function Test-IsExcluded {
    param([string]$RelativePath)

    foreach ($directory in $ExcludedDirectories) {
        if ($RelativePath -eq $directory -or $RelativePath.StartsWith("$directory\")) {
            return $true
        }
    }
    return $false
}

$scripts = foreach ($scanRoot in $ScanRoots) {
    $rootPath = Join-Path $repoRoot $scanRoot
    if (Test-Path -LiteralPath $rootPath) {
        Get-ChildItem -LiteralPath $rootPath -Recurse -File -Filter "*.ps1"
    }
}

$failures = @()
foreach ($script in ($scripts | Sort-Object FullName)) {
    $relativePath = Convert-ToRepoRelativePath -Path $script.FullName
    if (Test-IsExcluded -RelativePath $relativePath) {
        continue
    }

    $parseErrors = $null
    [System.Management.Automation.PSParser]::Tokenize((Get-Content -LiteralPath $script.FullName -Raw), [ref]$parseErrors) | Out-Null
    if ($parseErrors -and $parseErrors.Count -gt 0) {
        foreach ($parseError in $parseErrors) {
            $failures += [pscustomobject]@{
                Path = $relativePath
                Line = $parseError.Token.StartLine
                Message = $parseError.Message
            }
        }
    }
}

foreach ($failure in $failures) {
    Write-Host "FAIL $($failure.Path): line $($failure.Line): $($failure.Message)" -ForegroundColor Red
}

if ($failures.Count -gt 0) {
    exit 1
}

Write-Host "OK   PowerShell parser checked $($scripts.Count) scripts under $($ScanRoots -join ', ')."
