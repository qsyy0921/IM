param(
    [string]$ManifestPath = "docs/runbook/capacity-longrun-campaign-evidence.json",
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results",
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [ValidateSet("planned", "completed")]
    [string]$Status,
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [string]$SummaryPath = "",
    [string]$ReportPath = "",
    [Parameter(Mandatory = $true)]
    [string]$Note,
    [switch]$Replace
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
$validator = Join-Path $PSScriptRoot "validate-capacity-longrun-campaign-evidence.ps1"

Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing capacity long-run campaign evidence validator: $validator"
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($Name.Trim().Length -gt 0) "Name is required."
Assert-Condition ($PlanPath.Trim().Length -gt 0) "PlanPath is required."
Assert-Condition ($Note.Trim().Length -gt 0) "Note is required."
Assert-LowSensitiveEvidenceText -Value $Name -FieldName "Name" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $PlanPath -FieldName "PlanPath"
Assert-LowSensitiveEvidenceText -Value $SummaryPath -FieldName "SummaryPath" -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $ReportPath -FieldName "ReportPath" -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $Note -FieldName "Note"

if ($Status -eq "planned") {
    Assert-Condition ($SummaryPath.Trim().Length -eq 0) "SummaryPath must be empty for planned long-run campaign evidence."
    Assert-Condition ($ReportPath.Trim().Length -eq 0) "ReportPath must be empty for planned long-run campaign evidence."
}
else {
    Assert-Condition ($SummaryPath.Trim().Length -gt 0) "SummaryPath is required for completed long-run campaign evidence."
    Assert-Condition ($ReportPath.Trim().Length -gt 0) "ReportPath is required for completed long-run campaign evidence."
}

$originalJson = Get-Content -LiteralPath $resolvedManifestPath -Raw
$manifest = $originalJson | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "capacity long-run campaign evidence schema_version must be 1."

$newEntry = [ordered]@{
    name = $Name.Trim()
    status = $Status
    plan_path = $PlanPath.Trim()
    note = $Note.Trim()
}
if ($SummaryPath.Trim().Length -gt 0) {
    $newEntry.summary_path = $SummaryPath.Trim()
}
if ($ReportPath.Trim().Length -gt 0) {
    $newEntry.report_path = $ReportPath.Trim()
}

$entries = New-Object System.Collections.Generic.List[object]
$found = $false
foreach ($entry in @($manifest.entries)) {
    $entryName = Get-JsonPropertyString -Object $entry -Name "name"
    if ($entryName -eq $Name.Trim()) {
        Assert-Condition ([bool]$Replace) "capacity long-run campaign evidence entry already exists: $($Name.Trim()). Use -Replace to update it."
        $entries.Add([pscustomobject]$newEntry)
        $found = $true
        continue
    }
    $entries.Add($entry)
}
if (-not $found) {
    $entries.Add([pscustomobject]$newEntry)
}

$updated = [ordered]@{
    schema_version = [int]$manifest.schema_version
    updated_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd")
    scope = Get-JsonPropertyString -Object $manifest -Name "scope"
    entries = @($entries.ToArray())
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

if ($found) {
    Write-Host "OK   capacity long-run campaign evidence entry updated: $($Name.Trim())"
}
else {
    Write-Host "OK   capacity long-run campaign evidence entry added: $($Name.Trim())"
}
