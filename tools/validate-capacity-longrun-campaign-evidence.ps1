param(
    [string]$ManifestPath = "docs/runbook/capacity-longrun-campaign-evidence.json",
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results",
    [string]$ReportRoot = "docs/runbook/loadtest",
    [switch]$RequireFiles,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

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

function Get-JsonProperty {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $null
    }
    return $Object.$Name
}

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Test-PathInsideDirectory {
    param(
        [string]$Path,
        [string]$Directory
    )

    $fullPath = [System.IO.Path]::GetFullPath($Path).TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )
    $fullDirectory = [System.IO.Path]::GetFullPath($Directory).TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )

    if ($fullPath.Equals($fullDirectory, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }

    $prefix = $fullDirectory + [System.IO.Path]::DirectorySeparatorChar
    return $fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)
}

function Escape-MarkdownCell {
    param([string]$Value)

    return $Value.Replace("|", "\|").Replace("`r", " ").Replace("`n", " ").Trim()
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$reportRoot = Resolve-RepoPath $ReportRoot
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($ExpectedResultRoot.Trim().Length -gt 0) "ExpectedResultRoot is required."

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "capacity long-run campaign evidence schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $manifest -Name "scope").Length -gt 0) "capacity long-run campaign evidence scope is required."

$knownStatuses = @("planned", "completed")
$seenNames = @{}
$validatedFiles = 0
$entryResults = @()

foreach ($entry in @($manifest.entries)) {
    $name = Get-JsonPropertyString -Object $entry -Name "name"
    $status = Get-JsonPropertyString -Object $entry -Name "status"
    $planPath = Get-JsonPropertyString -Object $entry -Name "plan_path"
    $summaryPath = Get-JsonPropertyString -Object $entry -Name "summary_path"
    $reportPath = Get-JsonPropertyString -Object $entry -Name "report_path"
    $note = Get-JsonPropertyString -Object $entry -Name "note"

    Assert-Condition ($name.Length -gt 0) "capacity long-run campaign evidence entry name is required."
    Assert-Condition (-not $seenNames.ContainsKey($name)) "duplicate capacity long-run campaign evidence entry name: $name"
    $seenNames[$name] = $true
    Assert-Condition ($status -in $knownStatuses) "capacity long-run campaign evidence entry $name has unknown status: $status"
    Assert-Condition ($planPath.Length -gt 0) "capacity long-run campaign evidence entry $name plan_path is required."
    Assert-Condition ($note.Length -gt 0) "capacity long-run campaign evidence entry $name note is required."

    $resolvedPlanPath = Resolve-RepoPath $planPath
    Assert-Condition (Test-PathInsideDirectory -Path $resolvedPlanPath -Directory $ExpectedResultRoot) "capacity long-run campaign plan_path for $name must point under $ExpectedResultRoot`: $planPath"

    $fileChecked = $false
    if ($RequireFiles) {
        Assert-Condition (Test-Path -LiteralPath $resolvedPlanPath -PathType Leaf) "capacity long-run campaign plan does not exist for $name`: $resolvedPlanPath"
        $plan = Get-Content -LiteralPath $resolvedPlanPath -Raw | ConvertFrom-Json
        Assert-Condition ([int]$plan.schema_version -eq 1) "capacity long-run campaign plan for $name schema_version must be 1."
        Assert-Condition ((Get-JsonPropertyString -Object $plan -Name "scope") -match "not a production SLO") "capacity long-run campaign plan for $name must state non-SLO boundary."
        Assert-Condition ([int]$plan.duration_seconds -ge 1800) "capacity long-run campaign plan for $name must be at least 30m."
        Assert-Condition ([int]$plan.service_count -gt 0) "capacity long-run campaign plan for $name must include services."
        $planOutputRoot = Get-JsonPropertyString -Object $plan -Name "output_root"
        Assert-Condition ($planOutputRoot.Length -gt 0) "capacity long-run campaign plan for $name output_root is required."
        Assert-Condition (Test-PathInsideDirectory -Path $planOutputRoot -Directory $ExpectedResultRoot) "capacity long-run campaign plan for $name output_root must stay under $ExpectedResultRoot."
        $validatedFiles++
        $fileChecked = $true
    }

    if ($status -eq "completed") {
        Assert-Condition ($summaryPath.Length -gt 0) "completed capacity long-run campaign evidence entry $name summary_path is required."
        Assert-Condition ($reportPath.Length -gt 0) "completed capacity long-run campaign evidence entry $name report_path is required."
        $resolvedSummaryPath = Resolve-RepoPath $summaryPath
        Assert-Condition (Test-PathInsideDirectory -Path $resolvedSummaryPath -Directory $ExpectedResultRoot) "capacity long-run campaign summary_path for $name must point under $ExpectedResultRoot`: $summaryPath"
        $resolvedReportPath = Resolve-RepoPath $reportPath
        Assert-Condition (Test-PathInsideDirectory -Path $resolvedReportPath -Directory $reportRoot) "capacity long-run campaign report_path for $name must stay under docs/runbook/loadtest: $reportPath"
        if ($RequireFiles) {
            Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "capacity long-run campaign summary does not exist for $name`: $resolvedSummaryPath"
            Assert-Condition (Test-Path -LiteralPath $resolvedReportPath -PathType Leaf) "capacity long-run campaign report does not exist for $name`: $reportPath"
            $summary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json
            Assert-Condition ([int]$summary.schema_version -eq 1) "capacity long-run campaign summary for $name schema_version must be 1."
            Assert-Condition ((Get-JsonPropertyString -Object $summary -Name "scope") -match "not a production SLO") "capacity long-run campaign summary for $name must state non-SLO boundary."
            Assert-Condition ((Get-JsonPropertyString -Object $summary -Name "status") -eq "completed") "capacity long-run campaign summary for $name must be completed."
            Assert-Condition ([int]$summary.service_count -gt 0) "capacity long-run campaign summary for $name service_count must be positive."
            Assert-Condition ([int]$summary.completed_service_count -eq [int]$summary.service_count) "capacity long-run campaign summary for $name must complete every service."
            Assert-Condition ([int]$summary.failed_service_count -eq 0) "capacity long-run campaign summary for $name must not have failed services."
            Assert-Condition ([double]$summary.minimum_duration_seconds -ge 1800) "capacity long-run campaign summary for $name minimum_duration_seconds must be at least 30m."
            $report = Get-Content -LiteralPath $resolvedReportPath -Raw
            $reportLower = $report.ToLowerInvariant()
            Assert-Condition ($reportLower.Contains("capacity") -and ($reportLower.Contains("production") -or $reportLower.Contains("slo") -or $reportLower.Contains("sizing"))) "capacity long-run campaign report must state capacity and non-production/SLO/sizing boundary for $name."
        }
    }
    else {
        Assert-Condition ($summaryPath.Length -eq 0) "planned capacity long-run campaign evidence entry $name must not include summary_path before execution."
        Assert-Condition ($reportPath.Length -eq 0) "planned capacity long-run campaign evidence entry $name must not include report_path before execution."
    }

    $entryResults += [pscustomobject]@{
        name = $name
        status = $status
        plan_path = $planPath
        summary_path = $summaryPath
        report_path = $reportPath
        files_checked = $fileChecked
        note = $note
    }
}

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    manifest_path = $resolvedManifestPath
    entry_count = @($manifest.entries).Count
    files_required = [bool]$RequireFiles
    validated_files = $validatedFiles
    valid = $true
    scope = "local capacity long-run campaign evidence validation; not a production SLO or sizing proof"
}

if ($MarkdownPath.Trim().Length -gt 0) {
    $resolvedMarkdownPath = Resolve-RepoPath $MarkdownPath
    $markdownDir = Split-Path -Parent $resolvedMarkdownPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM Capacity Long-Run Campaign Evidence")
    $lines.Add("")
    $lines.Add("- Manifest: $resolvedManifestPath")
    $lines.Add("- Entries: $(@($manifest.entries).Count)")
    $lines.Add("- Files checked: $validatedFiles")
    $lines.Add("- Require files: $([bool]$RequireFiles)")
    $lines.Add("- Scope: long-run capacity campaign planning/evidence index; not a production SLO or sizing proof.")
    $lines.Add("")
    $lines.Add("| Name | Status | Files checked | Plan path | Summary path | Report path | Note |")
    $lines.Add("| --- | --- | --- | --- | --- | --- | --- |")
    foreach ($result in $entryResults) {
        $lines.Add("| $(Escape-MarkdownCell $result.name) | $(Escape-MarkdownCell $result.status) | $($result.files_checked) | $(Escape-MarkdownCell $result.plan_path) | $(Escape-MarkdownCell $result.summary_path) | $(Escape-MarkdownCell $result.report_path) | $(Escape-MarkdownCell $result.note) |")
    }
    $lines.Add("")
    $lines.Add("Planned entries prove only that an execution campaign was prepared. Completed entries still require external raw summaries and repository reports.")
    $lines | Set-Content -LiteralPath $resolvedMarkdownPath -Encoding UTF8
    Write-Host "OK   capacity long-run campaign evidence markdown written: $resolvedMarkdownPath"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   capacity long-run campaign evidence validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
