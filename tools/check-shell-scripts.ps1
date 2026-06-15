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

function Convert-ToBashRelativePath {
    param([string]$RelativePath)

    return $RelativePath -replace "\\", "/"
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
        Get-ChildItem -LiteralPath $rootPath -Recurse -File -Filter "*.sh"
    }
}

$scripts = @($scripts | Sort-Object FullName)
if ($scripts.Count -eq 0) {
    Write-Host "OK   shell parser checked 0 scripts under $($ScanRoots -join ', ')."
    exit 0
}

$bash = Get-Command bash -ErrorAction SilentlyContinue
if (-not $bash) {
    Write-Host "FAIL bash is required to syntax-check .sh scripts, but it was not found on PATH." -ForegroundColor Red
    exit 1
}

$failures = @()

Push-Location $repoRoot
try {
    foreach ($script in $scripts) {
        $relativePath = Convert-ToRepoRelativePath -Path $script.FullName
        if (Test-IsExcluded -RelativePath $relativePath) {
            continue
        }

        $bashPath = Convert-ToBashRelativePath -RelativePath $relativePath
        $previousErrorActionPreference = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        try {
            $output = & $bash.Source -n $bashPath 2>&1
            $exitCode = $LASTEXITCODE
        }
        finally {
            $ErrorActionPreference = $previousErrorActionPreference
        }
        if ($exitCode -ne 0) {
            $failures += [pscustomobject]@{
                Path = $relativePath
                ExitCode = $exitCode
                Output = (($output | Out-String).Trim())
            }
        }
    }
}
finally {
    Pop-Location
}

foreach ($failure in $failures) {
    Write-Host "FAIL $($failure.Path): bash -n exited $($failure.ExitCode)" -ForegroundColor Red
    if ($failure.Output) {
        Write-Host $failure.Output -ForegroundColor Red
    }
}

if ($failures.Count -gt 0) {
    exit 1
}

Write-Host "OK   shell parser checked $($scripts.Count) scripts under $($ScanRoots -join ', ')."
