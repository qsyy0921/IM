$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$summaryPath = Join-Path $repoRoot "docs\runbook\file-size-hotspot-baseline.json"
$markdownPath = Join-Path $repoRoot "docs\runbook\file-size-hotspots.md"

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

foreach ($row in @($summary.top_files)) {
    Assert-Condition ((Get-JsonPropertyString -Object $row -Name "Path").Length -gt 0) "file size hotspot top file path is required."
    Assert-Condition ((Get-JsonPropertyString -Object $row -Name "Kind").Length -gt 0) "file size hotspot top file kind is required."
    Assert-Condition ([int]$row.Lines -gt 0) "file size hotspot top file lines must be > 0."
    Assert-Condition ([int]$row.Warn -gt 0) "file size hotspot top file warn threshold must be > 0."
    Assert-Condition ([int]$row.Max -gt 0) "file size hotspot top file max threshold must be > 0."
    Assert-Condition ([double]$row.WarnRatio -ge 0) "file size hotspot top file warn ratio must be >= 0."
    Assert-Condition ([double]$row.MaxRatio -ge 0) "file size hotspot top file max ratio must be >= 0."
}

$markdown = Get-Content -LiteralPath $markdownPath -Raw
Assert-Condition ($markdown.Contains("# File Size Budget Hotspots")) "file size hotspot markdown missing title."
Assert-Condition ($markdown.Contains("Large files are review priorities")) "file size hotspot markdown missing boundary text."
Assert-Condition ($markdown.Contains("not automatic design failures")) "file size hotspot markdown must avoid over-claiming."

Write-Host "OK   file size hotspot baseline"
