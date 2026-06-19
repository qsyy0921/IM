param(
    [string]$CasePath = "docs/runbook/ai-eval/retrieval-eval-cases.json",
    [string]$GatePolicyPath = "docs/runbook/ai-eval/gate-policy.local.json",
    [string]$ResultRoot = "",
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

function Resolve-RepoPath {
    param([string]$PathValue)

    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $repoRoot $PathValue))
}

function Get-JsonPropertyValue {
    param(
        $Object,
        [string]$Name,
        $DefaultValue = $null
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $DefaultValue
    }
    return $Object.$Name
}

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name,
        [string]$DefaultValue = ""
    )

    $value = Get-JsonPropertyValue -Object $Object -Name $Name -DefaultValue $DefaultValue
    if ($null -eq $value) {
        return ""
    }
    return ([string]$value).Trim()
}

function Test-ForbiddenPersistedFields {
    param(
        [string]$Path,
        [string[]]$ForbiddenFields
    )

    $raw = Get-Content -LiteralPath $Path -Raw
    foreach ($field in $ForbiddenFields) {
        $name = ([string]$field).Trim()
        if ($name.Length -eq 0) {
            continue
        }
        Assert-Condition (-not $raw.Contains("`"$name`"")) "forbidden persisted field found in ${Path}: $name"
    }
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
. (Join-Path $PSScriptRoot "output-root-safety.ps1")

if ([string]::IsNullOrWhiteSpace($ResultRoot)) {
    $ResultRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-ai-eval-ci-gate-" + [System.Guid]::NewGuid().ToString("N"))
}
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"
New-Item -ItemType Directory -Force -Path $ResultRoot | Out-Null

$resolvedCasePath = Resolve-RepoPath $CasePath
$resolvedGatePolicyPath = Resolve-RepoPath $GatePolicyPath
Assert-Condition (Test-Path -LiteralPath $resolvedCasePath -PathType Leaf) "CasePath does not exist: $resolvedCasePath"
Assert-Condition (Test-Path -LiteralPath $resolvedGatePolicyPath -PathType Leaf) "GatePolicyPath does not exist: $resolvedGatePolicyPath"

& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "validate-ai-eval-cases.ps1") `
    -CasePath $resolvedCasePath `
    -OutputPath (Join-Path $ResultRoot "ai-eval-case-validation.json")
if ($LASTEXITCODE -ne 0) {
    throw "validate-ai-eval-cases.ps1 failed with exit code $LASTEXITCODE"
}

& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "validate-ai-eval-gate-policy.ps1") `
    -GatePolicyPath $resolvedGatePolicyPath
if ($LASTEXITCODE -ne 0) {
    throw "validate-ai-eval-gate-policy.ps1 failed with exit code $LASTEXITCODE"
}

$gatePolicy = Get-Content -LiteralPath $resolvedGatePolicyPath -Raw | ConvertFrom-Json
$requiredAdapters = @($gatePolicy.adapters | Where-Object { [bool]$_.required })
Assert-Condition ($requiredAdapters.Count -gt 0) "gate policy has no required CI-safe adapters"

$forbiddenFields = @($gatePolicy.forbidden_persisted_fields | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_.Length -gt 0 })
$adapterResults = [System.Collections.Generic.List[object]]::new()
$totalCases = 0
$totalPassed = 0
$totalFailed = 0

foreach ($adapter in $requiredAdapters) {
    $adapterName = Get-JsonPropertyString -Object $adapter -Name "name"
    $scriptName = Get-JsonPropertyString -Object $adapter -Name "script"
    $summaryFile = Get-JsonPropertyString -Object $adapter -Name "summary_file" -DefaultValue "$adapterName-summary.json"
    Assert-Condition ($adapterName.Length -gt 0) "required adapter name is empty"
    Assert-Condition ($scriptName.Length -gt 0) "required adapter script is empty for $adapterName"
    Assert-Condition (-not [bool](Get-JsonPropertyValue -Object $adapter -Name "requires_service_stack" -DefaultValue $false)) "required adapter must be CI-safe and must not require service stack: $adapterName"

    $scriptPath = Join-Path $PSScriptRoot $scriptName
    $adapterRunName = "ai-eval-ci-$adapterName"
    $summaryPath = Join-Path $ResultRoot $summaryFile
    Assert-Condition (Test-Path -LiteralPath $scriptPath -PathType Leaf) "adapter script missing for ${adapterName}: $scriptPath"

    & powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath `
        -CasePath $resolvedCasePath `
        -ResultRoot $ResultRoot `
        -RunName $adapterRunName `
        -OutputPath $summaryPath
    if ($LASTEXITCODE -ne 0) {
        throw "$scriptName failed with exit code $LASTEXITCODE"
    }

    Assert-Condition (Test-Path -LiteralPath $summaryPath -PathType Leaf) "adapter summary missing for ${adapterName}: $summaryPath"
    Test-ForbiddenPersistedFields -Path $summaryPath -ForbiddenFields $forbiddenFields

    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    $caseCount = [int](Get-JsonPropertyValue -Object $summary -Name "case_count" -DefaultValue 0)
    Assert-Condition ($caseCount -gt 0) "adapter case_count must be positive for $adapterName"

    $totalCases += $caseCount
    $totalPassed += $caseCount
    $adapterResults.Add([pscustomobject]@{
        name = $adapterName
        case_count = $caseCount
        summary_path = $summaryPath
    })
}

$minCaseCount = [int](Get-JsonPropertyValue -Object $gatePolicy.policy -Name "min_case_count" -DefaultValue 1)
$maxFailedCount = [int](Get-JsonPropertyValue -Object $gatePolicy.policy -Name "max_failed_count" -DefaultValue 0)
Assert-Condition ($totalCases -ge $minCaseCount) "AI eval CI gate case_count below policy: min=$minCaseCount actual=$totalCases"
Assert-Condition ($totalFailed -le $maxFailedCount) "AI eval CI gate failed_count above policy: max=$maxFailedCount actual=$totalFailed"

$gateSummary = [pscustomobject]@{
    schema_version = 1
    status = "passed"
    scope = "CI-safe AI eval regression gate skeleton; required local adapters only, no PostgreSQL, no Docker, no live RAG or Agent service stack"
    gate_id = Get-JsonPropertyString -Object $gatePolicy -Name "gate_id"
    gate_policy_ref = $resolvedGatePolicyPath
    result_root = $ResultRoot
    adapter_count = $adapterResults.Count
    case_count = $totalCases
    passed_count = $totalPassed
    failed_count = $totalFailed
    adapters = $adapterResults
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $ResultRoot "ai-eval-ci-gate-summary.json"
}
$resolvedOutputPath = Resolve-RepoPath $OutputPath
$outputDir = Split-Path -Parent $resolvedOutputPath
Assert-ExternalOutputRoot -Value $outputDir -RepositoryRoot $repoRoot -Name "OutputPath directory"
if ($outputDir -and -not (Test-Path -LiteralPath $outputDir)) {
    New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
}
$gateSummary | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $resolvedOutputPath -Encoding UTF8

Write-Host "OK   ai-eval CI-safe regression gate completed: $resolvedOutputPath"
