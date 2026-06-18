param(
    [string]$ManifestPath = "docs/runbook/api-gateway-legacy-evidence.json",
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results",
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [ValidateSet("legacy-observation-window", "legacy-removal-plan")]
    [string]$Kind,
    [string]$SummaryPath = "",
    [string]$PlanPath = "",
    [string]$ValidationSummaryPath = "",
    [string]$ReportPath = "",
    [ValidateSet("", "PASS", "FAIL", "READY", "BLOCKED")]
    [string]$ExpectedStatus = "",
    [switch]$RequireReadyRemoval,
    [Parameter(Mandatory = $true)]
    [string]$Note
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "evidence-metadata-safety.ps1")

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
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

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$resolvedManifestPath = Resolve-RepoPath $ManifestPath
$validator = Join-Path $PSScriptRoot "validate-api-gateway-legacy-evidence.ps1"

Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing api-gateway legacy evidence validator: $validator"
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($Name.Trim().Length -gt 0) "Name is required."
Assert-Condition ($Note.Trim().Length -gt 0) "Note is required."
Assert-LowSensitiveEvidenceText -Value $Name -FieldName "Name" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $SummaryPath -FieldName "SummaryPath" -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $PlanPath -FieldName "PlanPath" -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $ValidationSummaryPath -FieldName "ValidationSummaryPath" -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $ReportPath -FieldName "ReportPath" -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $ExpectedStatus -FieldName "ExpectedStatus" -MaxLength 32 -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $Note -FieldName "Note"

if ($Kind -eq "legacy-observation-window") {
    Assert-Condition ($SummaryPath.Trim().Length -gt 0) "SummaryPath is required for legacy-observation-window evidence."
    Assert-Condition ($PlanPath.Trim().Length -eq 0) "PlanPath must be empty for legacy-observation-window evidence."
}
if ($Kind -eq "legacy-removal-plan") {
    Assert-Condition ($PlanPath.Trim().Length -gt 0) "PlanPath is required for legacy-removal-plan evidence."
}

$originalJson = Get-Content -LiteralPath $resolvedManifestPath -Raw
$manifest = $originalJson | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "api-gateway legacy evidence schema_version must be 1."
foreach ($entry in @($manifest.entries)) {
    Assert-Condition ((Get-JsonPropertyString -Object $entry -Name "name") -ne $Name.Trim()) "api-gateway legacy evidence entry already exists: $($Name.Trim())"
}

$newEntry = [ordered]@{
    name = $Name.Trim()
    kind = $Kind
}
if ($SummaryPath.Trim().Length -gt 0) {
    $newEntry.summary_path = $SummaryPath.Trim()
}
if ($PlanPath.Trim().Length -gt 0) {
    $newEntry.plan_path = $PlanPath.Trim()
}
if ($ValidationSummaryPath.Trim().Length -gt 0) {
    $newEntry.validation_summary_path = $ValidationSummaryPath.Trim()
}
if ($ReportPath.Trim().Length -gt 0) {
    $newEntry.report_path = $ReportPath.Trim()
}
if ($ExpectedStatus.Trim().Length -gt 0) {
    $newEntry.expected_status = $ExpectedStatus.Trim()
}
$newEntry.require_ready_removal = [bool]$RequireReadyRemoval
$newEntry.note = $Note.Trim()

$entries = @($manifest.entries)
$entries += [pscustomobject]$newEntry
$updated = [ordered]@{
    schema_version = [int]$manifest.schema_version
    updated_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd")
    scope = Get-JsonPropertyString -Object $manifest -Name "scope"
    entries = $entries
}

try {
    $updated | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $resolvedManifestPath -Encoding UTF8
    & $validator -ManifestPath $resolvedManifestPath -ExpectedResultRoot $ExpectedResultRoot | Out-Null
}
catch {
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($resolvedManifestPath, $originalJson, $utf8NoBom)
    throw
}

Write-Host "OK   api-gateway legacy evidence entry added: $($Name.Trim())"
