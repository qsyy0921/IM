param(
    [string]$ManifestPath = "docs/runbook/capacity-baseline-evidence.json",
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

function Get-ServiceFromRunner {
    param([string]$Runner)

    switch ($Runner) {
        "demo" { return "api-gateway" }
        "identity" { return "identity-service" }
        "sendmessage" { return "message-service" }
        "memberchange" { return "conversation-service" }
        "delivery" { return "delivery-service" }
        "pushgateway" { return "push-gateway" }
        "receipt" { return "receipt-service" }
        "contacts" { return "contacts-service" }
        "policy" { return "policy-service" }
        default { return "" }
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "capacity baseline evidence schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $manifest -Name "scope").Length -gt 0) "capacity baseline evidence scope is required."
Assert-Condition (@($manifest.entries).Count -gt 0) "capacity baseline evidence entries are required."

$expectedServices = @(
    "api-gateway",
    "identity-service",
    "message-service",
    "conversation-service",
    "delivery-service",
    "push-gateway",
    "receipt-service",
    "contacts-service",
    "policy-service"
)
$knownBaselineTypes = @("direct", "seeded", "stack", "cluster")
$seenServices = @{}
$entryResults = @()
$validatedFiles = 0

foreach ($entry in @($manifest.entries)) {
    $service = Get-JsonPropertyString -Object $entry -Name "service"
    $runner = Get-JsonPropertyString -Object $entry -Name "runner"
    $baselineType = Get-JsonPropertyString -Object $entry -Name "baseline_type"
    $summaryPath = Get-JsonPropertyString -Object $entry -Name "summary_path"
    $reportPath = Get-JsonPropertyString -Object $entry -Name "report_path"
    $note = Get-JsonPropertyString -Object $entry -Name "note"

    Assert-Condition ($service -in $expectedServices) "capacity baseline evidence entry has unknown service: $service"
    Assert-Condition (-not $seenServices.ContainsKey($service)) "duplicate capacity baseline evidence service: $service"
    $seenServices[$service] = $true
    Assert-Condition ((Get-ServiceFromRunner -Runner $runner) -eq $service) "capacity baseline evidence runner $runner does not match service $service"
    Assert-Condition ($baselineType -in $knownBaselineTypes) "capacity baseline evidence entry $service has unknown baseline_type: $baselineType"
    Assert-Condition ($summaryPath.Length -gt 0) "capacity baseline evidence entry $service summary_path is required."
    Assert-Condition ($reportPath.Length -gt 0) "capacity baseline evidence entry $service report_path is required."
    Assert-Condition ($note.Length -gt 0) "capacity baseline evidence entry $service note is required."

    $resolvedReportPath = Resolve-RepoPath $reportPath
    Assert-Condition (Test-Path -LiteralPath $resolvedReportPath -PathType Leaf) "capacity baseline report does not exist for $service`: $reportPath"
    $report = Get-Content -LiteralPath $resolvedReportPath -Raw
    $reportLower = $report.ToLowerInvariant()
    Assert-Condition ($reportLower.Contains("capacity")) "capacity baseline report must mention capacity for $service`: $reportPath"
    Assert-Condition ($reportLower.Contains("production") -or $reportLower.Contains("slo") -or $reportLower.Contains("sizing")) "capacity baseline report must state non-production boundary for $service`: $reportPath"

    $fileChecked = $false
    if ($RequireFiles) {
        $resolvedSummaryPath = Resolve-RepoPath $summaryPath
        Assert-Condition (Test-Path -LiteralPath $resolvedSummaryPath -PathType Leaf) "capacity baseline summary does not exist for $service`: $resolvedSummaryPath"
        $summary = Get-Content -LiteralPath $resolvedSummaryPath -Raw | ConvertFrom-Json
        $capacity = Get-JsonProperty -Object $summary -Name "capacity_summary"
        Assert-Condition ($null -ne $capacity) "capacity baseline summary for $service must contain capacity_summary: $resolvedSummaryPath"

        $summaryService = Get-JsonPropertyString -Object $summary -Name "service"
        if ($summaryService.Length -gt 0) {
            Assert-Condition ($summaryService -eq $service) "capacity baseline summary service mismatch for $service`: $summaryService"
        }

        $capacityText = ($capacity | ConvertTo-Json -Depth 10)
        Assert-Condition (($capacityText -match "duration") -or ($capacityText -match "per_second") -or ($capacityText -match "_rps")) "capacity baseline summary for $service lacks duration/rate fields."
        $validatedFiles++
        $fileChecked = $true
    }

    $entryResults += [pscustomobject]@{
        service = $service
        runner = $runner
        baseline_type = $baselineType
        summary_path = $summaryPath
        report_path = $reportPath
        files_checked = $fileChecked
        note = $note
    }
}

$missingServices = @($expectedServices | Where-Object { -not $seenServices.ContainsKey($_) })
Assert-Condition ($missingServices.Count -eq 0) "capacity baseline evidence missing service(s): $($missingServices -join ', ')"

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    manifest_path = $resolvedManifestPath
    entry_count = @($manifest.entries).Count
    files_required = [bool]$RequireFiles
    validated_files = $validatedFiles
    valid = $true
    scope = "local capacity baseline evidence manifest validation; not a production SLO or sizing claim"
}

if ($MarkdownPath.Trim().Length -gt 0) {
    $resolvedMarkdownPath = Resolve-RepoPath $MarkdownPath
    $markdownDir = Split-Path -Parent $resolvedMarkdownPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM Capacity Baseline Evidence")
    $lines.Add("")
    $lines.Add("- Manifest: $resolvedManifestPath")
    $lines.Add("- Entries: $(@($manifest.entries).Count)")
    $lines.Add("- Files checked: $validatedFiles")
    $lines.Add("- Require files: $([bool]$RequireFiles)")
    $lines.Add("- Scope: local short capacity-baseline evidence manifest validation; not a production SLO, long-running capacity, or production sizing claim.")
    $lines.Add("")
    $lines.Add("| Service | Runner | Type | Files checked | Summary path | Report path | Note |")
    $lines.Add("| --- | --- | --- | --- | --- | --- | --- |")
    foreach ($result in $entryResults) {
        $lines.Add("| $(Escape-MarkdownCell $result.service) | $(Escape-MarkdownCell $result.runner) | $(Escape-MarkdownCell $result.baseline_type) | $($result.files_checked) | $(Escape-MarkdownCell $result.summary_path) | $(Escape-MarkdownCell $result.report_path) | $(Escape-MarkdownCell $result.note) |")
    }
    $lines.Add("")
    $lines.Add("This report indexes local short baselines only. It does not prove production capacity, SLO, HA, or sizing readiness.")
    $lines | Set-Content -LiteralPath $resolvedMarkdownPath -Encoding UTF8
    Write-Host "OK   capacity baseline evidence markdown written: $resolvedMarkdownPath"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   capacity baseline evidence validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
