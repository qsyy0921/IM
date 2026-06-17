param(
    [string]$ManifestPath = "docs/runbook/observability-evidence.json",
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [ValidateSet("service-debug-smoke", "prometheus-grafana-smoke")]
    [string]$Kind,
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
    [string]$ReportPath = "",
    [string]$Service = "",
    [switch]$RequireCleanGit,
    [switch]$RequireAlertmanager,
    [int]$ExpectedDashboardCount = 0,
    [Parameter(Mandatory = $true)]
    [string]$Note
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
$validator = Join-Path $PSScriptRoot "validate-observability-evidence.ps1"

Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing observability evidence validator: $validator"
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($Name.Trim().Length -gt 0) "Name is required."
Assert-Condition ($SummaryPath.Trim().Length -gt 0) "SummaryPath is required."
Assert-Condition ($Note.Trim().Length -gt 0) "Note is required."
if ($Kind -eq "service-debug-smoke") {
    Assert-Condition ($Service.Trim().Length -gt 0) "Service is required for service-debug-smoke evidence."
}
if ($Kind -eq "prometheus-grafana-smoke") {
    Assert-Condition ($ExpectedDashboardCount -ge 0) "ExpectedDashboardCount must be >= 0."
}

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "observability evidence schema_version must be 1."

foreach ($entry in @($manifest.entries)) {
    Assert-Condition ((Get-JsonPropertyString -Object $entry -Name "name") -ne $Name.Trim()) "observability evidence entry already exists: $($Name.Trim())"
}

$newEntry = [ordered]@{
    name = $Name.Trim()
    kind = $Kind
}
if ($Service.Trim().Length -gt 0) {
    $newEntry.service = $Service.Trim()
}
$newEntry.summary_path = $SummaryPath.Trim()
if ($ReportPath.Trim().Length -gt 0) {
    $newEntry.report_path = $ReportPath.Trim()
}
$newEntry.require_clean_git = [bool]$RequireCleanGit
if ($Kind -eq "prometheus-grafana-smoke") {
    if ($RequireAlertmanager) {
        $newEntry.require_alertmanager = $true
    }
    if ($ExpectedDashboardCount -gt 0) {
        $newEntry.expected_dashboard_count = $ExpectedDashboardCount
    }
}
$newEntry.note = $Note.Trim()

$entries = @($manifest.entries)
$entries += [pscustomobject]$newEntry

$updated = [ordered]@{
    schema_version = [int]$manifest.schema_version
    updated_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd")
    scope = Get-JsonPropertyString -Object $manifest -Name "scope"
    entries = $entries
}

$updated | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $resolvedManifestPath -Encoding UTF8

& $validator -ManifestPath $resolvedManifestPath | Out-Null
Write-Host "OK   observability evidence entry added: $($Name.Trim())"
