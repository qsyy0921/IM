param(
    [int]$TopCount = 10
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$checkerPath = Join-Path $PSScriptRoot "check-file-size-budget.ps1"
$summaryPath = Join-Path $repoRoot "docs\runbook\file-size-hotspot-baseline.json"
$markdownPath = Join-Path $repoRoot "docs\runbook\file-size-hotspots.md"

if (-not (Test-Path -LiteralPath $checkerPath -PathType Leaf)) {
    throw "Missing file size budget checker: $checkerPath"
}

$hotspotWarnRatio = 0.8
if (Test-Path -LiteralPath $summaryPath -PathType Leaf) {
    $summary = Get-Content -LiteralPath $summaryPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($null -ne $summary.thresholds -and $null -ne $summary.thresholds.hotspot_warn_ratio) {
        $hotspotWarnRatio = [double]$summary.thresholds.hotspot_warn_ratio
    }
}

& $checkerPath `
    -SummaryPath $summaryPath `
    -MarkdownPath $markdownPath `
    -TopCount $TopCount `
    -HotspotWarnRatio $hotspotWarnRatio

& (Join-Path $PSScriptRoot "check-file-size-hotspot-baseline.ps1")
