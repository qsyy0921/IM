$ErrorActionPreference = "Stop"

$toolsRoot = $PSScriptRoot
$preparePath = Join-Path $toolsRoot "prepare-local-observability-images.ps1"

if (-not (Test-Path -LiteralPath $preparePath -PathType Leaf)) {
    throw "Missing prepare-local-observability-images.ps1"
}

function Assert-Contains {
    param(
        [string]$Text,
        [string]$Pattern,
        [string]$Message
    )

    if ($Text -notmatch [regex]::Escape($Pattern)) {
        throw $Message
    }
}

function Assert-NotContains {
    param(
        [string]$Text,
        [string]$Pattern,
        [string]$Message
    )

    if ($Text -match [regex]::Escape($Pattern)) {
        throw $Message
    }
}

function Invoke-PrepareDryRun {
    param(
        [switch]$IncludeAlertmanager,
        [string]$Platform = "",
        [string]$OutputDir = ""
    )

    $args = @()
    if ($IncludeAlertmanager) {
        $args += "-IncludeAlertmanager"
    }
    if (-not [string]::IsNullOrWhiteSpace($Platform)) {
        $args += @("-Platform", $Platform)
    }
    if (-not [string]::IsNullOrWhiteSpace($OutputDir)) {
        $args += @("-OutputDir", $OutputDir)
    }

    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $preparePath @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "prepare-local-observability-images.ps1 dry-run failed with exit code $LASTEXITCODE`: $($output -join [Environment]::NewLine)"
    }
    return ($output -join [Environment]::NewLine)
}

$envNames = @(
    "NEXUSIM_PROMETHEUS_IMAGE",
    "NEXUSIM_GRAFANA_IMAGE",
    "NEXUSIM_ALERTMANAGER_IMAGE"
)

$previousEnv = @{}
foreach ($envName in $envNames) {
    $previousEnv[$envName] = [Environment]::GetEnvironmentVariable($envName)
}

try {
    $suffix = [Guid]::NewGuid().ToString("N")
    $prometheusImage = "nexusim-selftest/prometheus-$suffix`:missing"
    $grafanaImage = "nexusim-selftest/grafana-$suffix`:missing"
    $alertmanagerImage = "nexusim-selftest/alertmanager-$suffix`:missing"

    [Environment]::SetEnvironmentVariable("NEXUSIM_PROMETHEUS_IMAGE", $prometheusImage)
    [Environment]::SetEnvironmentVariable("NEXUSIM_GRAFANA_IMAGE", $grafanaImage)
    [Environment]::SetEnvironmentVariable("NEXUSIM_ALERTMANAGER_IMAGE", $alertmanagerImage)

    $defaultOutput = Invoke-PrepareDryRun
    Assert-Contains -Text $defaultOutput -Pattern "missing_count=2" -Message "Default observability image dry-run must only include Prometheus and Grafana."
    Assert-Contains -Text $defaultOutput -Pattern "docker pull $prometheusImage" -Message "Default dry-run must print Prometheus pull command."
    Assert-Contains -Text $defaultOutput -Pattern "docker pull $grafanaImage" -Message "Default dry-run must print Grafana pull command."
    Assert-NotContains -Text $defaultOutput -Pattern $alertmanagerImage -Message "Default dry-run must not include Alertmanager."
    Assert-NotContains -Text $defaultOutput -Pattern "pulling " -Message "Dry-run must not pull images."

    $alertmanagerOutput = Invoke-PrepareDryRun -IncludeAlertmanager -Platform "linux/arm64"
    Assert-Contains -Text $alertmanagerOutput -Pattern "missing_count=3" -Message "Alertmanager dry-run must include all three observability images."
    Assert-Contains -Text $alertmanagerOutput -Pattern "docker pull --platform linux/arm64 $prometheusImage" -Message "Platform dry-run must include Prometheus platform pull command."
    Assert-Contains -Text $alertmanagerOutput -Pattern "docker pull --platform linux/arm64 $grafanaImage" -Message "Platform dry-run must include Grafana platform pull command."
    Assert-Contains -Text $alertmanagerOutput -Pattern "docker pull --platform linux/arm64 $alertmanagerImage" -Message "Platform dry-run must include Alertmanager platform pull command."
    Assert-NotContains -Text $alertmanagerOutput -Pattern "pulling " -Message "Alertmanager dry-run must not pull images."

    $planDir = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-observability-image-plan-" + [Guid]::NewGuid().ToString("N"))
    $planOutput = Invoke-PrepareDryRun -IncludeAlertmanager -Platform "linux/arm64" -OutputDir $planDir
    $planJsonPath = Join-Path $planDir "observability-image-prepare-plan.json"
    $planMarkdownPath = Join-Path $planDir "observability-image-prepare-plan.md"
    if (-not (Test-Path -LiteralPath $planJsonPath -PathType Leaf)) {
        throw "Prepare dry-run must write JSON plan when OutputDir is set."
    }
    if (-not (Test-Path -LiteralPath $planMarkdownPath -PathType Leaf)) {
        throw "Prepare dry-run must write Markdown plan when OutputDir is set."
    }
    Assert-Contains -Text $planOutput -Pattern "observability_image_prepare_plan_json=$planJsonPath" -Message "Prepare dry-run must print JSON plan path."
    Assert-Contains -Text $planOutput -Pattern "observability_image_prepare_plan_report=$planMarkdownPath" -Message "Prepare dry-run must print Markdown plan path."

    $plan = Get-Content -LiteralPath $planJsonPath -Raw | ConvertFrom-Json
    if ($plan.missing_count -ne 3) {
        throw "Prepare JSON plan must record missing_count=3."
    }
    if ($plan.allow_image_pull -ne $false) {
        throw "Prepare JSON plan must record allow_image_pull=false for dry-run."
    }
    if ($plan.include_alertmanager -ne $true) {
        throw "Prepare JSON plan must record include_alertmanager=true."
    }
    if ($plan.platform -ne "linux/arm64") {
        throw "Prepare JSON plan must record selected platform."
    }
    if (@($plan.images).Count -ne 3) {
        throw "Prepare JSON plan must record three image entries."
    }
    $alertmanagerEntry = @($plan.images | Where-Object { $_.name -eq "alertmanager" })[0]
    if (-not $alertmanagerEntry -or $alertmanagerEntry.pull_command -ne "docker pull --platform linux/arm64 $alertmanagerImage") {
        throw "Prepare JSON plan must record Alertmanager platform pull command."
    }

    $planMarkdown = Get-Content -LiteralPath $planMarkdownPath -Raw
    Assert-Contains -Text $planMarkdown -Pattern "NexusIM Observability Image Prepare Plan" -Message "Prepare Markdown plan must have title."
    Assert-Contains -Text $planMarkdown -Pattern $alertmanagerImage -Message "Prepare Markdown plan must include Alertmanager image."
}
finally {
    if ($planDir -and (Test-Path -LiteralPath $planDir)) {
        Remove-Item -LiteralPath $planDir -Recurse -Force
    }
    foreach ($envName in $envNames) {
        $previousValue = $previousEnv[$envName]
        [Environment]::SetEnvironmentVariable($envName, $previousValue)
    }
}

Write-Host "OK   local observability image prepare dry-run self-test"
