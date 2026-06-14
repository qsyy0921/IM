$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$forbiddenTerms = @(
    ("aka" + "shic"),
    ("ska" + "shic")
)
$excludedDirectories = @(
    ".git",
    "deploy\local\data",
    "loadtest\results"
)
$textExtensions = @(
    ".bat",
    ".css",
    ".csv",
    ".dockerfile",
    ".env",
    ".example",
    ".go",
    ".html",
    ".json",
    ".md",
    ".mod",
    ".proto",
    ".ps1",
    ".sql",
    ".sum",
    ".toml",
    ".txt",
    ".yaml",
    ".yml"
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

function Test-IsExcluded {
    param([string]$RelativePath)

    foreach ($directory in $excludedDirectories) {
        if ($RelativePath -eq $directory -or $RelativePath.StartsWith("$directory\")) {
            return $true
        }
    }
    return $false
}

function Test-IsTextFile {
    param([System.IO.FileInfo]$File)

    if ($File.Name -eq "Dockerfile") {
        return $true
    }
    return $textExtensions -contains $File.Extension.ToLowerInvariant()
}

$violations = @()
$files = Get-ChildItem -LiteralPath $repoRoot -Recurse -File |
    Where-Object {
        $relativePath = Convert-ToRepoRelativePath -Path $_.FullName
        if (Test-IsExcluded -RelativePath $relativePath) {
            return $false
        }
        return Test-IsTextFile -File $_
    }

foreach ($file in $files) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
    foreach ($term in $forbiddenTerms) {
        if ($content.IndexOf($term, [System.StringComparison]::OrdinalIgnoreCase) -ge 0) {
            $relative = Convert-ToRepoRelativePath -Path $file.FullName
            $violations += "${relative}: forbidden legacy project name '$term'"
            break
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   project naming guardrails"
