param(
    [string]$ManifestPath = "docs/runbook/distributed-smoke-evidence.json",
    [string]$ExpectedResultRoot = "H:\NexusIM\loadtest-results",
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [ValidateSet("pushgateway-full", "redis-smoke", "postgres-smoke", "kafka-failover", "kafka-isr-flapping", "kafka-producer-fault", "kafka-consumer-churn")]
    [string]$Kind,
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
    [string]$ExpectedScenario = "",
    [string]$ExpectedRedisMode = "",
    [switch]$RequireCleanGit,
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
$validator = Join-Path $PSScriptRoot "validate-distributed-smoke-evidence.ps1"

Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing distributed smoke evidence validator: $validator"
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ($Name.Trim().Length -gt 0) "Name is required."
Assert-Condition ($SummaryPath.Trim().Length -gt 0) "SummaryPath is required."
Assert-Condition ($Note.Trim().Length -gt 0) "Note is required."
Assert-LowSensitiveEvidenceText -Value $Name -FieldName "Name" -MaxLength 128
Assert-LowSensitiveEvidenceText -Value $SummaryPath -FieldName "SummaryPath"
Assert-LowSensitiveEvidenceText -Value $ExpectedScenario -FieldName "ExpectedScenario" -MaxLength 128 -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $ExpectedRedisMode -FieldName "ExpectedRedisMode" -MaxLength 64 -AllowEmpty
Assert-LowSensitiveEvidenceText -Value $Note -FieldName "Note"

if ($Kind -eq "redis-smoke") {
    Assert-Condition ($ExpectedRedisMode.Trim().Length -gt 0) "ExpectedRedisMode is required for redis-smoke evidence."
}

$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "distributed smoke evidence schema_version must be 1."

foreach ($entry in @($manifest.entries)) {
    Assert-Condition ((Get-JsonPropertyString -Object $entry -Name "name") -ne $Name.Trim()) "distributed smoke evidence entry already exists: $($Name.Trim())"
}

$newEntry = [ordered]@{
    name = $Name.Trim()
    kind = $Kind
    summary_path = $SummaryPath.Trim()
}
if ($ExpectedRedisMode.Trim().Length -gt 0) {
    $newEntry.expected_redis_mode = $ExpectedRedisMode.Trim()
}
if ($ExpectedScenario.Trim().Length -gt 0) {
    $newEntry.expected_scenario = $ExpectedScenario.Trim()
}
$newEntry.require_clean_git = [bool]$RequireCleanGit
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

& $validator -ManifestPath $resolvedManifestPath -ExpectedResultRoot $ExpectedResultRoot | Out-Null
Write-Host "OK   distributed smoke evidence entry added: $($Name.Trim())"
