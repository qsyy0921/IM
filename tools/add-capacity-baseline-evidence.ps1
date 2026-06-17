param(
    [string]$ManifestPath = "docs/runbook/capacity-baseline-evidence.json",
    [Parameter(Mandatory = $true)]
    [ValidateSet("api-gateway", "identity-service", "message-service", "conversation-service", "delivery-service", "push-gateway", "receipt-service", "contacts-service", "policy-service")]
    [string]$Service,
    [Parameter(Mandatory = $true)]
    [ValidateSet("demo", "identity", "sendmessage", "memberchange", "delivery", "pushgateway", "receipt", "contacts", "policy")]
    [string]$Runner,
    [Parameter(Mandatory = $true)]
    [ValidateSet("direct", "seeded", "stack", "cluster")]
    [string]$BaselineType,
    [Parameter(Mandatory = $true)]
    [string]$SummaryPath,
    [Parameter(Mandatory = $true)]
    [string]$ReportPath,
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
$validator = Join-Path $PSScriptRoot "validate-capacity-baseline-evidence.ps1"

Assert-Condition (Test-Path -LiteralPath $validator -PathType Leaf) "Missing capacity baseline evidence validator: $validator"
Assert-Condition (Test-Path -LiteralPath $resolvedManifestPath -PathType Leaf) "ManifestPath does not exist: $resolvedManifestPath"
Assert-Condition ((Get-ServiceFromRunner -Runner $Runner) -eq $Service) "capacity baseline runner $Runner does not match service $Service"
Assert-Condition ($SummaryPath.Trim().Length -gt 0) "SummaryPath is required."
Assert-Condition ($ReportPath.Trim().Length -gt 0) "ReportPath is required."
Assert-Condition ($Note.Trim().Length -gt 0) "Note is required."
Assert-LowSensitiveEvidenceText -Value $SummaryPath -FieldName "SummaryPath"
Assert-LowSensitiveEvidenceText -Value $ReportPath -FieldName "ReportPath"
Assert-LowSensitiveEvidenceText -Value $Note -FieldName "Note"

$originalJson = Get-Content -LiteralPath $resolvedManifestPath -Raw
$manifest = $originalJson | ConvertFrom-Json
Assert-Condition ([int]$manifest.schema_version -eq 1) "capacity baseline evidence schema_version must be 1."

$newEntry = [ordered]@{
    service = $Service
    runner = $Runner
    baseline_type = $BaselineType
    summary_path = $SummaryPath.Trim()
    report_path = $ReportPath.Trim()
    note = $Note.Trim()
}

$entries = New-Object System.Collections.Generic.List[object]
$found = $false
foreach ($entry in @($manifest.entries)) {
    $entryService = Get-JsonPropertyString -Object $entry -Name "service"
    if ($entryService -eq $Service) {
        Assert-Condition ([bool]$Replace) "capacity baseline evidence service already exists: $Service. Use -Replace to update it."
        $entries.Add([pscustomobject]$newEntry)
        $found = $true
        continue
    }
    $entries.Add($entry)
}

$scope = Get-JsonPropertyString -Object $manifest -Name "scope"

if (-not $found) {
    $entries.Add([pscustomobject]$newEntry)
}

$updated = [ordered]@{
    schema_version = [int]$manifest.schema_version
    updated_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd")
    scope = $scope
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
    Write-Host "OK   capacity baseline evidence entry updated: $Service"
}
else {
    Write-Host "OK   capacity baseline evidence entry added: $Service"
}
