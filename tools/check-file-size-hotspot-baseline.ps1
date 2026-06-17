$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$checkerPath = Join-Path $PSScriptRoot "check-file-size-budget.ps1"
$summaryPath = Join-Path $repoRoot "docs\runbook\file-size-hotspot-baseline.json"
$markdownPath = Join-Path $repoRoot "docs\runbook\file-size-hotspots.md"

if (-not (Test-Path -LiteralPath $checkerPath -PathType Leaf)) {
    throw "Missing file size budget checker: $checkerPath"
}
if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
    throw "Missing file size hotspot baseline JSON: $summaryPath"
}
if (-not (Test-Path -LiteralPath $markdownPath -PathType Leaf)) {
    throw "Missing file size hotspot markdown: $markdownPath"
}

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return ""
    }
    return ([string]$Object.$Name).Trim()
}

function Assert-FileSizeRowEqual {
    param(
        $Expected,
        $Actual,
        [string]$Context
    )

    Assert-Condition ((Get-JsonPropertyString -Object $Actual -Name "Path") -eq (Get-JsonPropertyString -Object $Expected -Name "Path")) "$Context path drifted; refresh file-size hotspot baseline."
    Assert-Condition ((Get-JsonPropertyString -Object $Actual -Name "Kind") -eq (Get-JsonPropertyString -Object $Expected -Name "Kind")) "$Context kind drifted; refresh file-size hotspot baseline."
    Assert-Condition ([int]$Actual.Lines -eq [int]$Expected.Lines) "$Context line count drifted; refresh file-size hotspot baseline."
    Assert-Condition ([int]$Actual.Warn -eq [int]$Expected.Warn) "$Context warn threshold drifted; refresh file-size hotspot baseline."
    Assert-Condition ([int]$Actual.Max -eq [int]$Expected.Max) "$Context max threshold drifted; refresh file-size hotspot baseline."
    Assert-Condition ([double]$Actual.WarnRatio -eq [double]$Expected.WarnRatio) "$Context warn ratio drifted; refresh file-size hotspot baseline."
    Assert-Condition ([double]$Actual.MaxRatio -eq [double]$Expected.MaxRatio) "$Context max ratio drifted; refresh file-size hotspot baseline."
}

$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json

Assert-Condition ((Get-JsonPropertyString -Object $summary -Name "scope") -match "not a code-quality score") "file size hotspot baseline scope must state it is not a code-quality score."
Assert-Condition ([int]$summary.file_count -gt 0) "file size hotspot baseline must include file_count."
Assert-Condition ($null -ne $summary.thresholds) "file size hotspot baseline thresholds are required."
Assert-Condition ($null -ne $summary.totals) "file size hotspot baseline totals are required."
Assert-Condition ([int]$summary.totals.failures -eq 0) "file size hotspot baseline must not record max-line failures."
Assert-Condition (@($summary.top_files).Count -gt 0) "file size hotspot baseline top_files are required."
Assert-Condition (@($summary.hotspots).Count -eq [int]$summary.totals.hotspots) "file size hotspot baseline hotspots count must match totals."
Assert-Condition ([int]$summary.thresholds.production_warn_lines -eq 2500) "file size hotspot production warning threshold drifted."
Assert-Condition ([int]$summary.thresholds.test_runner_max_lines -eq 3000) "file size hotspot test/runner max threshold drifted."
Assert-Condition ([int]$summary.thresholds.script_max_lines -eq 1500) "file size hotspot script max threshold drifted."
Assert-Condition ([double]$summary.thresholds.hotspot_warn_ratio -gt 0 -and [double]$summary.thresholds.hotspot_warn_ratio -le 1) "file size hotspot ratio must be in (0,1]."

$topFiles = @($summary.top_files)
$hotspots = @($summary.hotspots)
foreach ($row in $topFiles) {
    Assert-Condition ((Get-JsonPropertyString -Object $row -Name "Path").Length -gt 0) "file size hotspot top file path is required."
    Assert-Condition ((Get-JsonPropertyString -Object $row -Name "Kind").Length -gt 0) "file size hotspot top file kind is required."
    Assert-Condition ([int]$row.Lines -gt 0) "file size hotspot top file lines must be > 0."
    Assert-Condition ([int]$row.Warn -gt 0) "file size hotspot top file warn threshold must be > 0."
    Assert-Condition ([int]$row.Max -gt 0) "file size hotspot top file max threshold must be > 0."
    Assert-Condition ([double]$row.WarnRatio -ge 0) "file size hotspot top file warn ratio must be >= 0."
    Assert-Condition ([double]$row.MaxRatio -ge 0) "file size hotspot top file max ratio must be >= 0."
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-file-size-hotspot-baseline-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $currentSummaryPath = Join-Path $tempRoot "current-file-size-summary.json"
    $currentMarkdownPath = Join-Path $tempRoot "current-file-size-hotspots.md"

    & $checkerPath `
        -SummaryPath $currentSummaryPath `
        -MarkdownPath $currentMarkdownPath `
        -TopCount $topFiles.Count `
        -HotspotWarnRatio ([double]$summary.thresholds.hotspot_warn_ratio)
    Assert-Condition (Test-Path -LiteralPath $currentSummaryPath -PathType Leaf) "file size budget checker did not write current summary."
    Assert-Condition (Test-Path -LiteralPath $currentMarkdownPath -PathType Leaf) "file size budget checker did not write current markdown."

    $currentSummary = Get-Content -LiteralPath $currentSummaryPath -Raw | ConvertFrom-Json
    Assert-Condition ([int]$currentSummary.file_count -eq [int]$summary.file_count) "file size hotspot baseline file_count drifted; refresh baseline JSON and markdown."
    Assert-Condition ([int]$currentSummary.totals.warnings -eq [int]$summary.totals.warnings) "file size hotspot baseline warning count drifted; refresh baseline JSON and markdown."
    Assert-Condition ([int]$currentSummary.totals.failures -eq [int]$summary.totals.failures) "file size hotspot baseline failure count drifted; refresh baseline JSON and markdown."
    Assert-Condition ([int]$currentSummary.totals.hotspots -eq [int]$summary.totals.hotspots) "file size hotspot baseline hotspot count drifted; refresh baseline JSON and markdown."

    $currentTopFiles = @($currentSummary.top_files)
    Assert-Condition ($currentTopFiles.Count -eq $topFiles.Count) "file size hotspot baseline top file count drifted; refresh baseline JSON and markdown."
    for ($i = 0; $i -lt $topFiles.Count; $i++) {
        Assert-FileSizeRowEqual -Expected $topFiles[$i] -Actual $currentTopFiles[$i] -Context "file size hotspot top_files[$i]"
    }

    $currentHotspots = @($currentSummary.hotspots)
    Assert-Condition ($currentHotspots.Count -eq $hotspots.Count) "file size hotspot baseline hotspot row count drifted; refresh baseline JSON and markdown."
    for ($i = 0; $i -lt $hotspots.Count; $i++) {
        Assert-FileSizeRowEqual -Expected $hotspots[$i] -Actual $currentHotspots[$i] -Context "file size hotspot hotspots[$i]"
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

$markdown = Get-Content -LiteralPath $markdownPath -Raw
Assert-Condition ($markdown.Contains("# File Size Budget Hotspots")) "file size hotspot markdown missing title."
Assert-Condition ($markdown.Contains("Large files are review priorities")) "file size hotspot markdown missing boundary text."
Assert-Condition ($markdown.Contains("not automatic design failures")) "file size hotspot markdown must avoid over-claiming."

Write-Host "OK   file size hotspot baseline"
