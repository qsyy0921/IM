param(
    [string]$CatalogPath = "docs/runbook/security-gate-catalog.json",
    [string]$OutputPath = ""
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
    param(
        [string]$PathValue
    )

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$catalogFullPath = Resolve-RepoPath $CatalogPath
Assert-Condition (Test-Path -LiteralPath $catalogFullPath -PathType Leaf) "CatalogPath does not exist: $catalogFullPath"

$catalog = Get-Content -LiteralPath $catalogFullPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$catalog.schema_version -eq 1) "security gate catalog schema_version must be 1."
Assert-Condition ((Get-JsonPropertyString -Object $catalog -Name "scope").Length -gt 0) "security gate catalog scope is required."
Assert-Condition (@($catalog.entries).Count -gt 0) "security gate catalog entries are required."

$checkLocalPath = Resolve-RepoPath "tools/check-local.ps1"
Assert-Condition (Test-Path -LiteralPath $checkLocalPath -PathType Leaf) "check-local.ps1 is required."
$checkLocal = Get-Content -LiteralPath $checkLocalPath -Raw

$seenNames = @{}
$seenScripts = @{}
$categories = @{}
foreach ($entry in @($catalog.entries)) {
    $name = Get-JsonPropertyString -Object $entry -Name "name"
    $category = Get-JsonPropertyString -Object $entry -Name "category"
    $script = Get-JsonPropertyString -Object $entry -Name "script"
    $label = Get-JsonPropertyString -Object $entry -Name "check_local_label"
    $coveredByScript = Get-JsonPropertyString -Object $entry -Name "covered_by_script"
    $note = Get-JsonPropertyString -Object $entry -Name "note"

    Assert-Condition ($name.Length -gt 0) "security gate entry name is required."
    Assert-Condition (-not $seenNames.ContainsKey($name)) "duplicate security gate entry name: $name"
    $seenNames[$name] = $true
    Assert-Condition ($category.Length -gt 0) "security gate entry $name category is required."
    $categories[$category] = $true
    Assert-Condition ($script.StartsWith("tools/") -or $script.StartsWith("tools\")) "security gate entry $name script must live under tools."
    Assert-Condition (-not $seenScripts.ContainsKey($script)) "duplicate security gate script: $script"
    $seenScripts[$script] = $true
    Assert-Condition ($label.Length -gt 0) "security gate entry $name check_local_label is required."
    Assert-Condition ($note.Length -gt 0) "security gate entry $name note is required."

    $scriptPath = Resolve-RepoPath $script
    Assert-Condition (Test-Path -LiteralPath $scriptPath -PathType Leaf) "security gate entry $name script does not exist: $script"
    $scriptLeaf = Split-Path -Leaf $scriptPath
    Assert-Condition ($checkLocal.Contains($label)) "security gate entry $name label is not present in check-local.ps1: $label"

    if ($coveredByScript.Length -gt 0) {
        Assert-Condition ($coveredByScript.StartsWith("tools/") -or $coveredByScript.StartsWith("tools\")) "security gate entry $name covered_by_script must live under tools."
        $coveredByPath = Resolve-RepoPath $coveredByScript
        Assert-Condition (Test-Path -LiteralPath $coveredByPath -PathType Leaf) "security gate entry $name covered_by_script does not exist: $coveredByScript"
        $coveredByLeaf = Split-Path -Leaf $coveredByPath
        Assert-Condition ($checkLocal.Contains($coveredByLeaf)) "security gate entry $name covered_by_script is not invoked by check-local.ps1: $coveredByLeaf"
        $coveredByText = Get-Content -LiteralPath $coveredByPath -Raw
        Assert-Condition ($coveredByText.Contains($scriptLeaf)) "security gate entry $name script is not covered by ${coveredByLeaf}: $scriptLeaf"
    }
    else {
        Assert-Condition ($checkLocal.Contains($scriptLeaf)) "security gate entry $name script is not invoked by check-local.ps1: $scriptLeaf"
    }
}

foreach ($requiredCategory in @("architecture-boundary", "listener-boundary", "transport-security", "operator-safety")) {
    Assert-Condition ($categories.ContainsKey($requiredCategory)) "security gate catalog missing required category: $requiredCategory"
}

$validation = [pscustomobject]@{
    schema_version = 1
    validated_at = (Get-Date).ToUniversalTime().ToString("o")
    catalog_path = $catalogFullPath
    entry_count = @($catalog.entries).Count
    category_count = $categories.Count
    valid = $true
    scope = "local NexusIM security gate catalog validation; not a production security compliance proof"
}

if ($OutputPath.Trim().Length -gt 0) {
    $outputFullPath = Resolve-RepoPath $OutputPath
    $outputDir = Split-Path -Parent $outputFullPath
    if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    $validation | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $outputFullPath -Encoding UTF8
    Write-Host "OK   security gate catalog validation written: $outputFullPath"
}
else {
    $validation | ConvertTo-Json -Depth 5
}
