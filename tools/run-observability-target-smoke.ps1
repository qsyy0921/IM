param(
    [string]$PrometheusBaseUrl = "",
    [string]$GrafanaBaseUrl = "",
    [string]$GrafanaUsername = "admin",
    [string]$GrafanaPassword = "",
    [switch]$IncludeAlertmanager,
    [string]$RunName = "",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$FixtureDir = "",
    [int]$TimeoutSeconds = 15,
    [string[]]$ExpectedDashboardUids = @(
        "nexusim-api-gateway",
        "nexusim-contacts-service",
        "nexusim-conversation-service",
        "nexusim-delivery-service",
        "nexusim-identity-service",
        "nexusim-message-service",
        "nexusim-policy-service",
        "nexusim-push-gateway",
        "nexusim-receipt-service"
    )
)

$ErrorActionPreference = "Stop"

$summaryWriter = Join-Path $PSScriptRoot "write-observability-smoke-summary.ps1"
$summaryValidator = Join-Path $PSScriptRoot "validate-observability-smoke-summary.ps1"

function New-DefaultRunName {
    return "target-observability-smoke-" + (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
}

function Convert-ToBaseUrl {
    param(
        [string]$Value,
        [string]$Name
    )

    $trimmed = $Value.Trim().TrimEnd("/")
    if ($trimmed.Length -eq 0) {
        throw "$Name is required when FixtureDir is not set."
    }
    if (-not ($trimmed -match "^https?://")) {
        throw "$Name must start with http:// or https://."
    }
    return $trimmed
}

function Get-BasicAuthHeader {
    param(
        [string]$Username,
        [string]$Password
    )

    if ($Username.Trim().Length -eq 0 -or $Password.Length -eq 0) {
        return @{}
    }

    $bytes = [System.Text.Encoding]::ASCII.GetBytes($Username + ":" + $Password)
    return @{ Authorization = "Basic " + [Convert]::ToBase64String($bytes) }
}

function Read-FixtureJson {
    param(
        [string]$FixtureName
    )

    $path = Join-Path $FixtureDir $FixtureName
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing observability target smoke fixture: $path"
    }
    return Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
}

function Invoke-JsonEndpoint {
    param(
        [string]$Url,
        [hashtable]$Headers = @{}
    )

    return Invoke-RestMethod -Uri $Url -Headers $Headers -TimeoutSec $TimeoutSeconds
}

function Get-PrometheusRules {
    if ($FixtureDir.Trim().Length -gt 0) {
        return Read-FixtureJson -FixtureName "prometheus-rules.json"
    }

    [void](Invoke-JsonEndpoint -Url "$PrometheusBaseUrl/-/ready")
    return Invoke-JsonEndpoint -Url "$PrometheusBaseUrl/api/v1/rules"
}

function Get-PrometheusAlertmanagers {
    if ($FixtureDir.Trim().Length -gt 0) {
        return Read-FixtureJson -FixtureName "prometheus-alertmanagers.json"
    }

    return Invoke-JsonEndpoint -Url "$PrometheusBaseUrl/api/v1/alertmanagers"
}

function Get-GrafanaSearch {
    param([hashtable]$Headers)

    if ($FixtureDir.Trim().Length -gt 0) {
        return @(Read-FixtureJson -FixtureName "grafana-search.json")
    }

    [void](Invoke-JsonEndpoint -Url "$GrafanaBaseUrl/api/health" -Headers $Headers)
    return @(Invoke-JsonEndpoint -Url "$GrafanaBaseUrl/api/search?type=dash-db" -Headers $Headers)
}

if (-not (Test-Path -LiteralPath $summaryWriter -PathType Leaf)) {
    throw "Missing observability smoke summary writer: $summaryWriter"
}
if (-not (Test-Path -LiteralPath $summaryValidator -PathType Leaf)) {
    throw "Missing observability smoke summary validator: $summaryValidator"
}
if ($FixtureDir.Trim().Length -gt 0) {
    $FixtureDir = [System.IO.Path]::GetFullPath($FixtureDir)
    if (-not (Test-Path -LiteralPath $FixtureDir -PathType Container)) {
        throw "FixtureDir does not exist: $FixtureDir"
    }
}
else {
    $PrometheusBaseUrl = Convert-ToBaseUrl -Value $PrometheusBaseUrl -Name "PrometheusBaseUrl"
    $GrafanaBaseUrl = Convert-ToBaseUrl -Value $GrafanaBaseUrl -Name "GrafanaBaseUrl"
}
if ($RunName.Trim().Length -eq 0) {
    $RunName = New-DefaultRunName
}

$rules = Get-PrometheusRules
if ($rules.status -ne "success" -or -not $rules.data.groups) {
    throw "Prometheus rules endpoint did not return success with rule groups."
}
$ruleGroupCount = [int]$rules.data.groups.Count

$grafanaHeaders = Get-BasicAuthHeader -Username $GrafanaUsername -Password $GrafanaPassword
$search = Get-GrafanaSearch -Headers $grafanaHeaders
$foundDashboardSet = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
foreach ($item in @($search)) {
    if ($item.uid) {
        [void]$foundDashboardSet.Add([string]$item.uid)
    }
}
$foundDashboardUids = @($ExpectedDashboardUids | Where-Object { $foundDashboardSet.Contains($_) })

$activeAlertmanagerUrls = @()
if ($IncludeAlertmanager) {
    $alertmanagers = Get-PrometheusAlertmanagers
    if ($alertmanagers.status -ne "success" -or -not $alertmanagers.data.activeAlertmanagers) {
        throw "Prometheus alertmanagers endpoint did not return active Alertmanager targets."
    }
    $activeAlertmanagerUrls = @($alertmanagers.data.activeAlertmanagers | ForEach-Object { [string]$_.url })
    if ($activeAlertmanagerUrls.Count -eq 0) {
        throw "Prometheus alertmanagers endpoint returned no active Alertmanager targets."
    }
}

$summaryDir = Join-Path $ResultRoot $RunName
& $summaryWriter `
    -OutputDir $summaryDir `
    -RunName $RunName `
    -RuleGroupCount $ruleGroupCount `
    -ExpectedDashboardUids $ExpectedDashboardUids `
    -FoundDashboardUids $foundDashboardUids `
    -AlertmanagerChecked ([bool]$IncludeAlertmanager) `
    -ActiveAlertmanagerUrls $activeAlertmanagerUrls `
    -Scope "target environment Prometheus/Grafana smoke evidence; not a production SLO or Alertmanager validation"

$summaryPath = Join-Path $summaryDir "observability-smoke-summary.json"
$validationPath = Join-Path $summaryDir "observability-smoke-validation.json"
& $summaryValidator `
    -SummaryPath $summaryPath `
    -ExpectedDashboardUids $ExpectedDashboardUids `
    -OutputPath $validationPath `
    -RequireAlertmanager:$IncludeAlertmanager

Write-Host "OK   target observability smoke summary validated: $summaryPath"
Write-Host "observability_summary_dir=$summaryDir"
