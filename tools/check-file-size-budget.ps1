param(
    [int]$ProductionWarnLines = 2500,
    [int]$ProductionMaxLines = 3500,
    [int]$TestRunnerWarnLines = 2500,
    [int]$TestRunnerMaxLines = 3000,
    [int]$ScriptWarnLines = 1000,
    [int]$ScriptMaxLines = 1500,
    [int]$DocsWarnLines = 1200,
    [int]$DocsMaxLines = 1500
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$excludedDirectories = @(
    ".git",
    "bin",
    "deploy\local\data",
    "loadtest\results"
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

function Test-IsGeneratedGo {
    param([string]$RelativePath)

    return (
        $RelativePath -like "api\proto\*" -or
        $RelativePath -like "schemas\kafka\*" -or
        $RelativePath -like "*.pb.go" -or
        $RelativePath -like "*.pb.gw.go" -or
        $RelativePath -like "*_grpc.pb.go"
    )
}

$files = Get-ChildItem -LiteralPath $repoRoot -Recurse -File |
    Where-Object {
        $relativePath = Convert-ToRepoRelativePath -Path $_.FullName
        if (Test-IsExcluded -RelativePath $relativePath) {
            return $false
        }
        if ($_.Extension -eq ".go" -and (Test-IsGeneratedGo -RelativePath $relativePath)) {
            return $false
        }
        return $_.Extension -in @(".go", ".md", ".ps1", ".sh")
    } |
    Sort-Object FullName

$warnings = @()
$failures = @()

foreach ($file in $files) {
    $relativePath = Convert-ToRepoRelativePath -Path $file.FullName
    $lineCount = (Get-Content -LiteralPath $file.FullName).Count

    if ($file.Extension -eq ".md") {
        $kind = "docs"
        $warnLines = $DocsWarnLines
        $maxLines = $DocsMaxLines
    }
    elseif ($file.Extension -in @(".ps1", ".sh")) {
        $kind = "script/runner"
        $warnLines = $ScriptWarnLines
        $maxLines = $ScriptMaxLines
    }
    elseif ($relativePath -like "loadtest\*" -or $file.Name -like "*_test.go") {
        $kind = "test/runner"
        $warnLines = $TestRunnerWarnLines
        $maxLines = $TestRunnerMaxLines
    }
    else {
        $kind = "production"
        $warnLines = $ProductionWarnLines
        $maxLines = $ProductionMaxLines
    }

    $record = [pscustomobject]@{
        Path = $relativePath
        Lines = $lineCount
        Kind = $kind
        Warn = $warnLines
        Max = $maxLines
    }

    if ($lineCount -gt $maxLines) {
        $failures += $record
    }
    elseif ($lineCount -gt $warnLines) {
        $warnings += $record
    }
}

foreach ($warning in ($warnings | Sort-Object Lines -Descending)) {
    Write-Host "WARN $($warning.Path) has $($warning.Lines) lines ($($warning.Kind)); split before adding more code." -ForegroundColor Yellow
}

foreach ($failure in ($failures | Sort-Object Lines -Descending)) {
    Write-Host "FAIL $($failure.Path) has $($failure.Lines) lines ($($failure.Kind)); max is $($failure.Max). Split the file before continuing." -ForegroundColor Red
}

if ($failures.Count -gt 0) {
    exit 1
}

Write-Host "OK   file size budgets checked ($($files.Count) handwritten Go/Markdown/script files)."
