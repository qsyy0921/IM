param(
    [string]$ManifestPath = "docs/runbook/api-gateway-legacy-evidence.json",
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

function Resolve-EvidencePath {
    param(
        [string]$PathValue,
        [string]$Context,
        [switch]$AllowEmpty
    )

    if ($PathValue.Trim().Length -eq 0) {
        if ($AllowEmpty) {
            return ""
        }
        throw "$Context is required."
    }
    $resolved = Resolve-RepoPath $PathValue
    Assert-Condition (Test-PathInsideDirectory -Path $resolved -Directory $ExpectedResultRoot) "$Context must point under $ExpectedResultRoot`: $PathValue"
    return $resolved
}

function Escape-MarkdownCell {
    param([string]$Value)

    return $Value.Replace("|", "\|").Replace("`r", " ").Replace("`n", " ").Trim()
}

function Validate-ObservationWindowSummary {
    param(
        [string]$Path,
        [string]$ExpectedStatus
    )

    Assert-Condition (Test-Path -LiteralPath $Path -PathType Leaf) "api-gateway legacy observation-window summary does not exist: $Path"
    $summary = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    $status = Get-JsonPropertyString -Object $summary -Name "status"
    Assert-Condition ($status -in @("PASS", "FAIL")) "api-gateway legacy observation-window summary status must be PASS or FAIL: $Path"
    if ($ExpectedStatus.Trim().Length -gt 0) {
        Assert-Condition ($status -eq $ExpectedStatus.Trim()) "api-gateway legacy observation-window summary status mismatch: expected=$ExpectedStatus actual=$status"
    }
    Assert-Condition ([int64]$summary.observation_count -gt 0) "api-gateway legacy observation-window summary observation_count must be positive."
    Assert-Condition ([int64]$summary.total_facade_requests -ge 0) "api-gateway legacy observation-window summary total_facade_requests is required."
    Assert-Condition ([int64]$summary.total_legacy_descriptor_requests -ge 0) "api-gateway legacy observation-window summary total_legacy_descriptor_requests is required."
    Assert-Condition ([int64]$summary.total_other_requests -ge 0) "api-gateway legacy observation-window summary total_other_requests is required."
}

function Validate-LegacyRemovalPlan {
    param(
        [string]$Path,
        [string]$ExpectedStatus,
        [bool]$RequireReadyRemoval
    )

    Assert-Condition (Test-Path -LiteralPath $Path -PathType Leaf) "api-gateway legacy removal plan does not exist: $Path"
    $validator = Join-Path $PSScriptRoot "validate-api-gateway-legacy-removal-plan.ps1"
    Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing legacy removal plan validator: $validator"
    & $validator -PlanPath $Path | Out-Null

    $plan = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    $status = Get-JsonPropertyString -Object $plan -Name "status"
    Assert-Condition ($status -in @("READY", "BLOCKED")) "api-gateway legacy removal plan status must be READY or BLOCKED: $Path"
    if ($ExpectedStatus.Trim().Length -gt 0) {
        Assert-Condition ($status -eq $ExpectedStatus.Trim()) "api-gateway legacy removal plan status mismatch: expected=$ExpectedStatus actual=$status"
    }
    if ($RequireReadyRemoval) {
        Assert-Condition ($status -eq "READY" -and [bool]$plan.ready_for_removal) "api-gateway legacy removal plan must be READY and ready_for_removal=true."
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($ExpectedResultRoot.Trim().Length -gt 0) "ExpectedResultRoot is required."

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "api-gateway legacy evidence schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $manifest -Name "scope").Length -gt 0) "api-gateway legacy evidence scope is required."

$knownKinds = @("legacy-observation-window", "legacy-removal-plan")
$seenNames = @{}
$validatedFiles = 0
$entryResults = @()

foreach ($entry in @($manifest.entries)) {
    $name = Get-JsonPropertyString -Object $entry -Name "name"
    $kind = Get-JsonPropertyString -Object $entry -Name "kind"
    $summaryPath = Get-JsonPropertyString -Object $entry -Name "summary_path"
    $planPath = Get-JsonPropertyString -Object $entry -Name "plan_path"
    $validationSummaryPath = Get-JsonPropertyString -Object $entry -Name "validation_summary_path"
    $reportPath = Get-JsonPropertyString -Object $entry -Name "report_path"
    $expectedStatus = Get-JsonPropertyString -Object $entry -Name "expected_status"
    $note = Get-JsonPropertyString -Object $entry -Name "note"

    Assert-Condition ($name.Length -gt 0) "api-gateway legacy evidence entry name is required."
    Assert-Condition (-not $seenNames.ContainsKey($name)) "duplicate api-gateway legacy evidence entry name: $name"
    $seenNames[$name] = $true
    Assert-Condition ($kind -in $knownKinds) "api-gateway legacy evidence entry $name has unknown kind: $kind"
    Assert-Condition ($note.Length -gt 0) "api-gateway legacy evidence entry $name note is required."
    if ($expectedStatus.Length -gt 0) {
        Assert-Condition ($expectedStatus -in @("PASS", "FAIL", "READY", "BLOCKED")) "api-gateway legacy evidence entry $name has invalid expected_status: $expectedStatus"
    }

    $resolvedSummaryPath = Resolve-EvidencePath -PathValue $summaryPath -Context "api-gateway legacy evidence entry $name summary_path" -AllowEmpty
    $resolvedPlanPath = Resolve-EvidencePath -PathValue $planPath -Context "api-gateway legacy evidence entry $name plan_path" -AllowEmpty
    $resolvedValidationSummaryPath = Resolve-EvidencePath -PathValue $validationSummaryPath -Context "api-gateway legacy evidence entry $name validation_summary_path" -AllowEmpty
    $resolvedReportPath = Resolve-EvidencePath -PathValue $reportPath -Context "api-gateway legacy evidence entry $name report_path" -AllowEmpty

    if ($kind -eq "legacy-observation-window") {
        Assert-Condition ($resolvedSummaryPath.Length -gt 0) "api-gateway legacy observation-window evidence entry $name requires summary_path."
        Assert-Condition ($resolvedPlanPath.Length -eq 0) "api-gateway legacy observation-window evidence entry $name must not include plan_path."
    }
    if ($kind -eq "legacy-removal-plan") {
        Assert-Condition ($resolvedPlanPath.Length -gt 0) "api-gateway legacy removal-plan evidence entry $name requires plan_path."
    }

    $fileChecked = $false
    if ($RequireFiles) {
        if ($kind -eq "legacy-observation-window") {
            Validate-ObservationWindowSummary -Path $resolvedSummaryPath -ExpectedStatus $expectedStatus
        }
        elseif ($kind -eq "legacy-removal-plan") {
            Validate-LegacyRemovalPlan -Path $resolvedPlanPath -ExpectedStatus $expectedStatus -RequireReadyRemoval ([bool]$entry.require_ready_removal)
            if ($resolvedSummaryPath.Length -gt 0) {
                Validate-ObservationWindowSummary -Path $resolvedSummaryPath -ExpectedStatus ""
            }
        }
        if ($resolvedValidationSummaryPath.Length -gt 0) {
            Assert-Condition (Test-Path -LiteralPath $resolvedValidationSummaryPath -PathType Leaf) "api-gateway legacy evidence validation_summary_path does not exist for $name`: $validationSummaryPath"
            $validationSummary = Get-Content -LiteralPath $resolvedValidationSummaryPath -Raw | ConvertFrom-Json
            Assert-Condition ([bool]$validationSummary.valid) "api-gateway legacy evidence validation summary must be valid for $name."
        }
        if ($resolvedReportPath.Length -gt 0) {
            Assert-Condition (Test-Path -LiteralPath $resolvedReportPath -PathType Leaf) "api-gateway legacy evidence report_path does not exist for $name`: $reportPath"
            $reportText = (Get-Content -LiteralPath $resolvedReportPath -Raw).ToLowerInvariant()
            Assert-Condition ($reportText.Contains("legacy") -and $reportText.Contains("api-gateway")) "api-gateway legacy evidence report must mention legacy and api-gateway for $name."
            Assert-Condition ($reportText.Contains("not") -and ($reportText.Contains("production") -or $reportText.Contains("slo"))) "api-gateway legacy evidence report must state non-production boundary for $name."
        }
        $validatedFiles++
        $fileChecked = $true
    }

    $entryResults += [pscustomobject]@{
        name = $name
        kind = $kind
        summary_path = $summaryPath
        plan_path = $planPath
        validation_summary_path = $validationSummaryPath
        report_path = $reportPath
        expected_status = $expectedStatus
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
    scope = "local api-gateway legacy descriptor migration evidence validation; not a production migration, SLO, or removal approval claim"
}

if ($MarkdownPath.Trim().Length -gt 0) {
    $resolvedMarkdownPath = Resolve-RepoPath $MarkdownPath
    $markdownDir = Split-Path -Parent $resolvedMarkdownPath
    if ($markdownDir -and -not (Test-Path -LiteralPath $markdownDir)) {
        New-Item -ItemType Directory -Force -Path $markdownDir | Out-Null
    }

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add("# NexusIM api-gateway Legacy Evidence")
    $lines.Add("")
    $lines.Add("- Manifest: $resolvedManifestPath")
    $lines.Add("- Entries: $(@($manifest.entries).Count)")
    $lines.Add("- Files checked: $validatedFiles")
    $lines.Add("- Require files: $([bool]$RequireFiles)")
    $lines.Add("- Scope: local api-gateway legacy descriptor migration evidence validation; not a production migration, SLO, or removal approval claim.")
    $lines.Add("")
    $lines.Add("| Name | Kind | Files checked | Expected status | Summary path | Plan path | Validation summary | Report path | Note |")
    $lines.Add("| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
    foreach ($result in $entryResults) {
        $lines.Add("| $(Escape-MarkdownCell $result.name) | $(Escape-MarkdownCell $result.kind) | $($result.files_checked) | $(Escape-MarkdownCell $result.expected_status) | $(Escape-MarkdownCell $result.summary_path) | $(Escape-MarkdownCell $result.plan_path) | $(Escape-MarkdownCell $result.validation_summary_path) | $(Escape-MarkdownCell $result.report_path) | $(Escape-MarkdownCell $result.note) |")
    }
    $lines.Add("")
    $lines.Add("This report indexes local or target-environment api-gateway legacy descriptor migration evidence only. It does not approve removal, mutate descriptors, prove production SLOs, or replace operator review.")
    $lines | Set-Content -LiteralPath $resolvedMarkdownPath -Encoding UTF8
    Write-Host "OK   api-gateway legacy evidence markdown written: $resolvedMarkdownPath"
}

if ($OutputPath.Trim().Length -gt 0) {
    $resolvedOutputPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $resolvedOutputPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8
    Write-Host "OK   api-gateway legacy evidence validation written: $resolvedOutputPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
