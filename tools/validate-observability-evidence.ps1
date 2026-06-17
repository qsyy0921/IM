param(
    [string]$ManifestPath = "docs/runbook/observability-evidence.json",
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

function Validate-ServiceDebugSmoke {
    param(
        $Entry,
        [string]$SummaryPath
    )

    $service = Get-JsonPropertyString -Object $Entry -Name "service"
    Assert-Condition ($service.Length -gt 0) "service-debug-smoke entry service is required."

    $summary = Get-Content -LiteralPath $SummaryPath -Raw | ConvertFrom-Json
    Assert-Condition ([bool]$summary.success) "service debug smoke summary success must be true: $SummaryPath"
    if ([bool]$Entry.require_clean_git) {
        $allow = Get-JsonProperty -Object $summary -Name "allow"
        $deny = Get-JsonProperty -Object $summary -Name "deny"
        Assert-Condition (-not [bool]$allow.git_dirty) "service debug smoke allow summary must be clean: $SummaryPath"
        Assert-Condition (-not [bool]$deny.git_dirty) "service debug smoke deny summary must be clean: $SummaryPath"
    }

    foreach ($field in @("allow_debug_metrics", "deny_debug_metrics")) {
        $metrics = Get-JsonProperty -Object $summary -Name $field
        Assert-Condition ($null -ne $metrics) "service debug smoke summary missing $field`: $SummaryPath"
        Assert-Condition ((Get-JsonPropertyString -Object $metrics -Name "service") -eq $service) "service debug smoke $field service mismatch."
        Assert-Condition ([int64]$metrics.grpc.total_requests -gt 0) "service debug smoke $field grpc.total_requests must be positive."
        Assert-Condition ([int64]$metrics.decisions.total -gt 0) "service debug smoke $field decisions.total must be positive."
    }
}

function Validate-PrometheusGrafanaSmoke {
    param(
        $Entry,
        [string]$SummaryPath
    )

    $validator = Join-Path $PSScriptRoot "validate-observability-smoke-summary.ps1"
    Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing observability smoke summary validator: $validator"

    $args = @{ SummaryPath = $SummaryPath }
    if ([bool]$Entry.require_alertmanager) {
        $args.RequireAlertmanager = $true
    }
    & $validator @args | Out-Null

    $summary = Get-Content -LiteralPath $SummaryPath -Raw | ConvertFrom-Json
    $expectedDashboardCount = [int](Get-JsonPropertyString -Object $Entry -Name "expected_dashboard_count")
    if ($expectedDashboardCount -gt 0) {
        Assert-Condition ([int]$summary.dashboard_count.expected -eq $expectedDashboardCount) "observability dashboard expected count mismatch."
        Assert-Condition ([int]$summary.dashboard_count.found -ge $expectedDashboardCount) "observability dashboard found count is lower than expected."
    }
}

function Validate-ObservabilityImagePreparePlan {
    param(
        [string]$SummaryPath,
        [bool]$RequireReport
    )

    $validator = Join-Path $PSScriptRoot "validate-observability-image-prepare-plan.ps1"
    Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing observability image prepare plan validator: $validator"

    $args = @{
        PlanPath = $SummaryPath
    }
    if ($RequireReport) {
        $args.RequireReport = $true
    }
    & $validator @args | Out-Null
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedReportRoot = Resolve-RepoPath $ReportRoot
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($ExpectedResultRoot.Trim().Length -gt 0) "ExpectedResultRoot is required."
Assert-Condition ($ReportRoot.Trim().Length -gt 0) "ReportRoot is required."

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "observability evidence schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $manifest -Name "scope").Length -gt 0) "observability evidence scope is required."
Assert-Condition (@($manifest.entries).Count -gt 0) "observability evidence entries are required."

$knownKinds = @("service-debug-smoke", "prometheus-grafana-smoke", "observability-image-prepare-plan")
$seenNames = @{}
$validatedFiles = 0
$entryResults = @()

foreach ($entry in @($manifest.entries)) {
    $name = Get-JsonPropertyString -Object $entry -Name "name"
    $kind = Get-JsonPropertyString -Object $entry -Name "kind"
    $summaryPath = Get-JsonPropertyString -Object $entry -Name "summary_path"
    $reportPath = Get-JsonPropertyString -Object $entry -Name "report_path"
    $note = Get-JsonPropertyString -Object $entry -Name "note"

    Assert-Condition ($name.Length -gt 0) "observability evidence entry name is required."
    Assert-Condition (-not $seenNames.ContainsKey($name)) "duplicate observability evidence entry name: $name"
    $seenNames[$name] = $true
    Assert-Condition ($kind -in $knownKinds) "observability evidence entry $name has unknown kind: $kind"
    Assert-Condition ($summaryPath.Length -gt 0) "observability evidence entry $name summary_path is required."
    Assert-Condition ($note.Length -gt 0) "observability evidence entry $name note is required."

    $resolvedSummaryPath = Resolve-RepoPath $summaryPath
    Assert-Condition (Test-PathInsideDirectory -Path $resolvedSummaryPath -Directory $ExpectedResultRoot) "observability evidence summary_path for $name must point under $ExpectedResultRoot`: $summaryPath"
    if ($reportPath.Length -gt 0) {
        $resolvedReportPath = Resolve-RepoPath $reportPath
        if ($kind -eq "observability-image-prepare-plan") {
            Assert-Condition (Test-PathInsideDirectory -Path $resolvedReportPath -Directory $ExpectedResultRoot) "observability image prepare report_path for $name must point under $ExpectedResultRoot`: $reportPath"
        }
        else {
            Assert-Condition (Test-PathInsideDirectory -Path $resolvedReportPath -Directory $resolvedReportRoot) "observability evidence report_path for $name must stay under $ReportRoot`: $reportPath"
        }
        Assert-Condition (Test-Path -LiteralPath $resolvedReportPath -PathType Leaf) "observability evidence report does not exist for $name`: $reportPath"
        $reportText = (Get-Content -LiteralPath $resolvedReportPath -Raw).ToLowerInvariant()
        Assert-Condition ($reportText.Contains("observability") -or $reportText.Contains("metrics")) "observability evidence report must mention observability or metrics for $name."
        Assert-Condition ($reportText.Contains("not") -and ($reportText.Contains("production") -or $reportText.Contains("slo"))) "observability evidence report must state non-production boundary for $name."
    }

    $fileChecked = $false
    if ($RequireFiles) {
        Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "observability evidence summary does not exist for $name`: $resolvedSummaryPath"
        switch ($kind) {
            "service-debug-smoke" {
                Validate-ServiceDebugSmoke -Entry $entry -SummaryPath $resolvedSummaryPath
            }
            "prometheus-grafana-smoke" {
                Validate-PrometheusGrafanaSmoke -Entry $entry -SummaryPath $resolvedSummaryPath
            }
            "observability-image-prepare-plan" {
                Validate-ObservabilityImagePreparePlan -SummaryPath $resolvedSummaryPath -RequireReport ($reportPath.Length -gt 0)
            }
        }
        $validatedFiles++
        $fileChecked = $true
    }

    $entryResults += [pscustomobject]@{
        name = $name
        kind = $kind
        service = Get-JsonPropertyString -Object $entry -Name "service"
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
    scope = "local observability evidence manifest validation; not a production SLO, retention, Alertmanager route, or unified collector claim"
}

if ($MarkdownPath.Trim().Length -gt 0) {
    $resolvedMarkdownPath = Resolve-RepoPath $MarkdownPath
    $markdownDir = Split-Path -Parent $resolvedMarkdownPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM Observability Evidence")
    $lines.Add("")
    $lines.Add("- Manifest: $resolvedManifestPath")
    $lines.Add("- Entries: $(@($manifest.entries).Count)")
    $lines.Add("- Files checked: $validatedFiles")
    $lines.Add("- Require files: $([bool]$RequireFiles)")
    $lines.Add("- Scope: local observability evidence manifest validation; not a production SLO, retention, Alertmanager route, or unified collector claim.")
    $lines.Add("")
    $lines.Add("| Name | Kind | Service | Files checked | Summary path | Report path | Note |")
    $lines.Add("| --- | --- | --- | --- | --- | --- | --- |")
    foreach ($result in $entryResults) {
        $lines.Add("| $(Escape-MarkdownCell $result.name) | $(Escape-MarkdownCell $result.kind) | $(Escape-MarkdownCell $result.service) | $($result.files_checked) | $(Escape-MarkdownCell $result.summary_path) | $(Escape-MarkdownCell $result.report_path) | $(Escape-MarkdownCell $result.note) |")
    }
    $lines.Add("")
    $lines.Add("This report indexes local observability smoke evidence only. It does not prove production SLOs, retention, Alertmanager routing, unified collector deployment, or long-running observability readiness.")
    $lines | Set-Content -LiteralPath $resolvedMarkdownPath -Encoding UTF8
    Write-Host "OK   observability evidence markdown written: $resolvedMarkdownPath"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   observability evidence validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
