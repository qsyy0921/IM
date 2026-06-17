param(
    [string]$ManifestPath = "docs/runbook/resource-snapshot-evidence.json",
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results",
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

function Escape-MarkdownCell {
    param([string]$Value)

    return $Value.Replace("|", "\|").Replace("`r", " ").Replace("`n", " ").Trim()
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

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($ExpectedResultRoot.Trim().Length -gt 0) "ExpectedResultRoot is required."

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "resource snapshot evidence schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $manifest -Name "scope").Length -gt 0) "resource snapshot evidence scope is required."
Assert-Condition (@($manifest.entries).Count -gt 0) "resource snapshot evidence entries are required."

$seenNames = @{}
$entryResults = @()
$validatedFiles = 0

foreach ($entry in @($manifest.entries)) {
    $name = Get-JsonPropertyString -Object $entry -Name "name"
    $summaryPath = Get-JsonPropertyString -Object $entry -Name "summary_path"
    $markdownSummaryPath = Get-JsonPropertyString -Object $entry -Name "markdown_path"
    $note = Get-JsonPropertyString -Object $entry -Name "note"

    Assert-Condition ($name.Length -gt 0) "resource snapshot evidence entry name is required."
    Assert-Condition (-not $seenNames.ContainsKey($name)) "duplicate resource snapshot evidence name: $name"
    $seenNames[$name] = $true
    Assert-Condition ($summaryPath.Length -gt 0) "resource snapshot evidence entry $name summary_path is required."
    Assert-Condition ($markdownSummaryPath.Length -gt 0) "resource snapshot evidence entry $name markdown_path is required."
    Assert-Condition ($note.Length -gt 0) "resource snapshot evidence entry $name note is required."

    $resolvedSummaryPath = Resolve-RepoPath $summaryPath
    Assert-Condition (Test-PathInsideDirectory -Path $resolvedSummaryPath -Directory $ExpectedResultRoot) "resource snapshot summary_path for $name must point under $ExpectedResultRoot`: $summaryPath"
    $resolvedMarkdownSummaryPath = Resolve-RepoPath $markdownSummaryPath
    Assert-Condition (Test-PathInsideDirectory -Path $resolvedMarkdownSummaryPath -Directory $ExpectedResultRoot) "resource snapshot markdown_path for $name must point under $ExpectedResultRoot`: $markdownSummaryPath"

    $filesChecked = $false
    $serviceCount = $null
    $healthyEndpoints = $null
    $serviceContainers = $null
    if ($RequireFiles) {
        Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "resource snapshot summary does not exist for $name`: $summaryPath"
        Assert-Condition (Test-Path -LiteralPath $resolvedMarkdownSummaryPath -PathType Leaf) "resource snapshot markdown summary does not exist for $name`: $markdownSummaryPath"

        $summary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json
        $scope = Get-JsonPropertyString -Object $summary -Name "scope"
        $serviceCount = [int](Get-JsonProperty -Object $summary -Name "service_count")
        $endpoints = Get-JsonProperty -Object $summary -Name "endpoints"
        $totals = Get-JsonProperty -Object $summary -Name "totals"
        Assert-Condition ($scope.ToLowerInvariant().Contains("not a capacity benchmark")) "resource snapshot summary $name must state it is not a capacity benchmark."
        Assert-Condition ($serviceCount -eq 9) "resource snapshot summary $name must cover 9 services."
        Assert-Condition ($null -ne $endpoints) "resource snapshot summary $name endpoints are required."
        Assert-Condition ($null -ne $totals) "resource snapshot summary $name totals are required."
        $healthyEndpoints = [int]$endpoints.healthy
        $serviceContainers = [int]$totals.service_containers
        Assert-Condition ([int]$endpoints.total -eq 9) "resource snapshot summary $name must include 9 endpoint checks."
        Assert-Condition ($healthyEndpoints -eq 9) "resource snapshot summary $name must have 9 healthy endpoints."
        Assert-Condition ($serviceContainers -eq 9) "resource snapshot summary $name must include 9 service containers."

        $markdown = Get-Content -LiteralPath $resolvedMarkdownSummaryPath -Raw
        $markdownLower = $markdown.ToLowerInvariant()
        Assert-Condition ($markdownLower.Contains("not a capacity benchmark")) "resource snapshot markdown $name must state non-capacity boundary."
        Assert-Condition ($markdownLower.Contains("9/9 healthy")) "resource snapshot markdown $name must mention 9/9 healthy endpoints."
        $validatedFiles++
        $filesChecked = $true
    }

    $entryResults += [pscustomobject]@{
        name = $name
        summary_path = $summaryPath
        markdown_path = $markdownSummaryPath
        files_checked = $filesChecked
        service_count = $serviceCount
        healthy_endpoints = $healthyEndpoints
        service_containers = $serviceContainers
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
    scope = "local Docker health-state resource snapshot evidence manifest validation; not a capacity benchmark, production SLO, or sizing claim"
}

if ($MarkdownPath.Trim().Length -gt 0) {
    $resolvedMarkdownPath = Resolve-RepoPath $MarkdownPath
    $markdownDir = Split-Path -Parent $resolvedMarkdownPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM Resource Snapshot Evidence")
    $lines.Add("")
    $lines.Add("- Manifest: $resolvedManifestPath")
    $lines.Add("- Entries: $(@($manifest.entries).Count)")
    $lines.Add("- Files checked: $validatedFiles")
    $lines.Add("- Require files: $([bool]$RequireFiles)")
    $lines.Add("- Scope: local Docker health-state snapshot evidence; not a capacity benchmark, production SLO, or production sizing claim.")
    $lines.Add("")
    $lines.Add("| Name | Files checked | Summary path | Markdown path | Note |")
    $lines.Add("| --- | --- | --- | --- | --- |")
    foreach ($result in $entryResults) {
        $lines.Add("| $(Escape-MarkdownCell $result.name) | $($result.files_checked) | $(Escape-MarkdownCell $result.summary_path) | $(Escape-MarkdownCell $result.markdown_path) | $(Escape-MarkdownCell $result.note) |")
    }
    $lines.Add("")
    $lines.Add("This report indexes local one-shot Docker stats snapshots only. It does not prove capacity, SLO, HA, or production sizing readiness.")
    $lines | Set-Content -LiteralPath $resolvedMarkdownPath -Encoding UTF8
    Write-Host "OK   resource snapshot evidence markdown written: $resolvedMarkdownPath"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   resource snapshot evidence validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
