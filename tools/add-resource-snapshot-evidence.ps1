param(
    [string]$ManifestPath = "docs/runbook/resource-snapshot-evidence.json",
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
    [Parameter(Mandatory = $true)]
    [string]$MarkdownPath,
    [switch]$RequireCleanGit,
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
$validator = Join-Path $PSScriptRoot "validate-resource-snapshot-evidence.ps1"

Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing resource snapshot evidence validator: $validator"
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($Name.Trim().Length -gt 0) "Name is required."
Assert-Condition ($SummaryPath.Trim().Length -gt 0) "SummaryPath is required."
Assert-Condition ($MarkdownPath.Trim().Length -gt 0) "MarkdownPath is required."
Assert-Condition ($Note.Trim().Length -gt 0) "Note is required."
Assert-LowSensitiveEvidenceText -Value $Name -FieldName "Name" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $SummaryPath -FieldName "SummaryPath"
Assert-LowSensitiveEvidenceText -Value $MarkdownPath -FieldName "MarkdownPath"
Assert-LowSensitiveEvidenceText -Value $Note -FieldName "Note"

$originalJson = Get-Content -LiteralPath $resolvedManifestPath -Raw
$manifest = $originalJson | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "resource snapshot evidence schema_version must be 1."

$newEntry = [ordered]@{
    name = $Name.Trim()
    summary_path = $SummaryPath.Trim()
    markdown_path = $MarkdownPath.Trim()
    require_clean_git = [bool]$RequireCleanGit
    note = $Note.Trim()
}

$entries = New-Object System.Collections.Generic.List[object]
$found = $false
foreach ($entry in @($manifest.entries)) {
    $entryName = Get-JsonPropertyString -Object $entry -Name "name"
    if ($entryName -eq $Name.Trim()) {
        Assert-Condition ([bool]$Replace) "resource snapshot evidence entry already exists: $($Name.Trim()). Use -Replace to update it."
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
    & $validator -ManifestPath $resolvedManifestPath | Out-Null
}
catch {
    $originalJson | Set-Content -LiteralPath $resolvedManifestPath -Encoding UTF8
    throw
}

if ($found) {
    Write-Host "OK   resource snapshot evidence entry updated: $($Name.Trim())"
}
else {
    Write-Host "OK   resource snapshot evidence entry added: $($Name.Trim())"
}
