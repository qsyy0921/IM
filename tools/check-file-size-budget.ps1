param(
    [int]$ProductionWarnLines = 2500,
    [int]$ProductionMaxLines = 3500,
    [int]$TestRunnerWarnLines = 2500,
    [int]$TestRunnerMaxLines = 3000,
    [int]$ScriptWarnLines = 1000,
    [int]$ScriptMaxLines = 1500,
    [int]$DocsWarnLines = 1200,
    [int]$DocsMaxLines = 1500,
    [string]$SummaryPath = "",
    [string]$MarkdownPath = "",
    [int]$TopCount = 15,
    [double]$HotspotWarnRatio = 0.8
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
$records = @()

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
        WarnRatio = if ($warnLines -gt 0) { [math]::Round(($lineCount / $warnLines), 4) } else { 0.0 }
        MaxRatio = if ($maxLines -gt 0) { [math]::Round(($lineCount / $maxLines), 4) } else { 0.0 }
    }

    $records += $record

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

if ($SummaryPath.Trim().Length -gt 0 -or $MarkdownPath.Trim().Length -gt 0) {
    if ($TopCount -lt 1) {
        throw "TopCount must be greater than zero."
    }
    if ($HotspotWarnRatio -le 0 -or $HotspotWarnRatio -gt 1) {
        throw "HotspotWarnRatio must be greater than 0 and less than or equal to 1."
    }

    $sortedByMaxRatio = @(
        $records |
            Sort-Object @{ Expression = { $_.MaxRatio }; Descending = $true }, @{ Expression = { $_.Lines }; Descending = $true } |
            Select-Object -First $TopCount
    )
    $hotspots = @(
        $records |
            Where-Object { $_.WarnRatio -ge $HotspotWarnRatio } |
            Sort-Object @{ Expression = { $_.WarnRatio }; Descending = $true }, @{ Expression = { $_.Lines }; Descending = $true }
    )

    $summary = [pscustomobject]@{
        created_at = (Get-Date).ToUniversalTime().ToString("o")
        scope = "handwritten Go/Markdown/PowerShell/Bash file-size budget snapshot; not a code-quality score"
        file_count = $files.Count
        thresholds = [pscustomobject]@{
            production_warn_lines = $ProductionWarnLines
            production_max_lines = $ProductionMaxLines
            test_runner_warn_lines = $TestRunnerWarnLines
            test_runner_max_lines = $TestRunnerMaxLines
            script_warn_lines = $ScriptWarnLines
            script_max_lines = $ScriptMaxLines
            docs_warn_lines = $DocsWarnLines
            docs_max_lines = $DocsMaxLines
            hotspot_warn_ratio = $HotspotWarnRatio
        }
        totals = [pscustomobject]@{
            warnings = $warnings.Count
            failures = $failures.Count
            hotspots = $hotspots.Count
        }
        top_files = $sortedByMaxRatio
        hotspots = $hotspots
    }

    if ($SummaryPath.Trim().Length -gt 0) {
        $summaryFullPath = [System.IO.Path]::GetFullPath($SummaryPath)
        $summaryDir = Split-Path -Parent $summaryFullPath
        if ($summaryDir -and -not (Test-Path -LiteralPath $summaryDir)) {
            New-Item -ItemType Directory -Force -Path $summaryDir | Out-Null
        }
        $summary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $summaryFullPath -Encoding UTF8
        Write-Host "OK   file size budget summary written: $summaryFullPath"
    }

    if ($MarkdownPath.Trim().Length -gt 0) {
        $markdownFullPath = [System.IO.Path]::GetFullPath($MarkdownPath)
        $markdownDir = Split-Path -Parent $markdownFullPath
        if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
            New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
        }

        $markdown = @()
        $markdown += "# File Size Budget Hotspots"
        $markdown += ""
        $markdown += "- Created at: $($summary.created_at)"
        $markdown += "- Scope: $($summary.scope)"
        $markdown += "- Files checked: $($summary.file_count)"
        $markdown += "- Warnings: $($summary.totals.warnings)"
        $markdown += "- Failures: $($summary.totals.failures)"
        $markdown += "- Hotspots at >= $([math]::Round($HotspotWarnRatio * 100, 0))% of warning threshold: $($summary.totals.hotspots)"
        $markdown += ""
        $markdown += "| File | Kind | Lines | Warn | Max | Warn % | Max % |"
        $markdown += "| --- | --- | ---: | ---: | ---: | ---: | ---: |"
        foreach ($row in $summary.top_files) {
            $warnPercent = [math]::Round(([double]$row.WarnRatio * 100), 1)
            $maxPercent = [math]::Round(([double]$row.MaxRatio * 100), 1)
            $markdown += "| $($row.Path) | $($row.Kind) | $($row.Lines) | $($row.Warn) | $($row.Max) | $warnPercent | $maxPercent |"
        }
        $markdown += ""
        $markdown += "This is a complexity governance snapshot only. Large files are review priorities, not automatic design failures."

        $markdown | Set-Content -LiteralPath $markdownFullPath -Encoding UTF8
        Write-Host "OK   file size budget markdown written: $markdownFullPath"
    }
}

if ($failures.Count -gt 0) {
    exit 1
}

Write-Host "OK   file size budgets checked ($($files.Count) handwritten Go/Markdown/script files)."
