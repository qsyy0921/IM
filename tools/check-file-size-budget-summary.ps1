$ErrorActionPreference = "Stop"

$checker = Join-Path $PSScriptRoot "check-file-size-budget.ps1"
if (-not (Test-Path -LiteralPath $checker -PathType Leaf)) {
    throw "Missing file size budget checker: $checker"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-file-size-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $summaryPath = Join-Path $tempRoot "file-size-summary.json"
    $markdownPath = Join-Path $tempRoot "file-size-hotspots.md"

    & $checker `
        -SummaryPath $summaryPath `
        -MarkdownPath $markdownPath `
        -TopCount 5 `
        -HotspotWarnRatio 0.5

    if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
        throw "File size summary JSON was not written."
    }
    if (-not (Test-Path -LiteralPath $markdownPath -PathType Leaf)) {
        throw "File size hotspot markdown was not written."
    }

    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    if ($summary.file_count -lt 1) {
        throw "File size summary must include checked file count."
    }
    if ($summary.top_files.Count -ne 5) {
        throw "File size summary must include exactly 5 top files for the self-test."
    }
    if ($summary.scope -notmatch "not a code-quality score") {
        throw "File size summary scope must state that it is not a code-quality score."
    }
    if ($summary.thresholds.production_warn_lines -lt 1 -or $summary.thresholds.hotspot_warn_ratio -ne 0.5) {
        throw "File size summary thresholds are incomplete."
    }
    foreach ($row in $summary.top_files) {
        if (-not $row.Path -or $row.Lines -lt 1 -or $row.Warn -lt 1 -or $row.Max -lt 1) {
            throw "File size summary top file row is incomplete."
        }
    }

    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("# File Size Budget Hotspots") -or -not $markdown.Contains("Large files are review priorities")) {
        throw "File size hotspot markdown missing expected boundary text."
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   file size budget summary self-test"
