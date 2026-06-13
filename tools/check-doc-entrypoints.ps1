param(
    [int]$DocsIndexMaxLines = 50,
    [int]$SDDIndexMaxLines = 50
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$checks = @(
    @{
        Path = Join-Path $repoRoot "docs\README.md"
        MaxLines = $DocsIndexMaxLines
        Purpose = "docs index"
    },
    @{
        Path = Join-Path $repoRoot "docs\sdd\README.md"
        MaxLines = $SDDIndexMaxLines
        Purpose = "sdd index"
    }
)

$failed = $false
foreach ($check in $checks) {
    if (-not (Test-Path -LiteralPath $check.Path)) {
        Write-Error "Missing $($check.Purpose): $($check.Path)"
    }

    $lineCount = (Get-Content -LiteralPath $check.Path).Count
    $relativePath = Resolve-Path -LiteralPath $check.Path -Relative
    if ($lineCount -gt $check.MaxLines) {
        Write-Host "FAIL $relativePath has $lineCount lines, max is $($check.MaxLines). Move details into service SDDs, runbook reports, or archive." -ForegroundColor Red
        $failed = $true
        continue
    }
    Write-Host "OK   $relativePath has $lineCount/$($check.MaxLines) lines ($($check.Purpose))."
}

if ($failed) {
    exit 1
}
