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

function Add-EmptyDefaultGuardFailures {
    param(
        [string]$Text,
        [string]$RelativePath,
        [System.Collections.ArrayList]$Failures
    )

    foreach ($match in [regex]::Matches($Text, '\[string\]\$(ResultRoot|OutputRoot)\s*=\s*""')) {
        $parameterName = $match.Groups[1].Value
        $recoveryPattern = 'if\s*\(-not\s*\$' + [regex]::Escape($parameterName) + '\)\s*\{[\s\S]{0,300}?H:\\NexusIM\\loadtest-results'
        $guardPattern = '(?m)^\s*Assert-ExternalOutputRoot\b[^\r\n]*-Value\s+\$' + [regex]::Escape($parameterName) + '\b'
        $recoveryMatch = [regex]::Match($Text, $recoveryPattern)
        $guardMatch = [regex]::Match($Text, $guardPattern)

        if (-not $recoveryMatch.Success) {
            [void]$Failures.Add([pscustomobject]@{
                Path = $RelativePath
                Text = "$parameterName has an empty default and must set an H:\NexusIM\loadtest-results recovery before validation"
            })
        }
        elseif ($guardMatch.Success -and $recoveryMatch.Index -gt $guardMatch.Index) {
            [void]$Failures.Add([pscustomobject]@{
                Path = $RelativePath
                Text = "$parameterName sets the H:\NexusIM\loadtest-results recovery after Assert-ExternalOutputRoot"
            })
        }
    }
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
$guardFailures = [System.Collections.ArrayList]@()
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
            [void]$guardFailures.Add([pscustomobject]@{
                Path = $relativePath
                Text = "ResultRoot/OutputRoot writer is missing output-root-safety.ps1 helper"
            })
        }
        if (-not (Test-CallsOutputRootGuard -Text $fileText)) {
            [void]$guardFailures.Add([pscustomobject]@{
                Path = $relativePath
                Text = "ResultRoot/OutputRoot writer is missing an active Assert-ExternalOutputRoot call"
            })
        }
        Add-EmptyDefaultGuardFailures -Text $fileText -RelativePath $relativePath -Failures $guardFailures
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
