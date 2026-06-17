param(
    [switch]$IncludeAlertmanager,
    [switch]$AllowImagePull,
    [string]$Platform = "",
    [string]$OutputDir = ""
)

$ErrorActionPreference = "Stop"

function Get-ObservabilityImage {
    param(
        [string]$EnvName,
        [string]$DefaultImage
    )

    $configured = [Environment]::GetEnvironmentVariable($EnvName)
    if ([string]::IsNullOrWhiteSpace($configured)) {
        return $DefaultImage
    }
    return $configured.Trim()
}

function Test-DockerAvailable {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    docker --version > $null 2> $null
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference
    return $exitCode -eq 0
}

function Test-DockerImagePresent {
    param([string]$Image)

    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    docker image inspect $Image > $null 2> $null
    $exitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference
    return $exitCode -eq 0
}

function New-DockerPullCommand {
    param([string]$Image)

    if ([string]::IsNullOrWhiteSpace($Platform)) {
        return "docker pull $Image"
    }
    return "docker pull --platform $Platform $Image"
}

function Invoke-DockerPull {
    param([string]$Image)

    $pullArgs = @("pull")
    if (-not [string]::IsNullOrWhiteSpace($Platform)) {
        $pullArgs += @("--platform", $Platform.Trim())
    }
    $pullArgs += $Image
    & docker @pullArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker pull failed for image: $Image"
    }
}

function Write-ImagePreparePlan {
    param(
        [string]$PlanOutputDir,
        [object]$Plan
    )

    New-Item -ItemType Directory -Force -Path $PlanOutputDir | Out-Null
    $jsonPath = Join-Path $PlanOutputDir "observability-image-prepare-plan.json"
    $markdownPath = Join-Path $PlanOutputDir "observability-image-prepare-plan.md"

    $Plan | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $jsonPath -Encoding utf8

    $lines = @(
        "# NexusIM Observability Image Prepare Plan",
        "",
        "Generated at UTC: $($Plan.generated_at_utc)",
        "",
        "This file is a low-sensitive local Docker image preparation plan. It does not prove a production observability SLO.",
        "",
        "| Image role | Image | Status | Pull command |",
        "| --- | --- | --- | --- |"
    )
    foreach ($image in $Plan.images) {
        $lines += "| $($image.name) | ``$($image.image)`` | $($image.status) | ``$($image.pull_command)`` |"
    }
    $lines += ""
    $lines += "Missing count: $($Plan.missing_count)"
    $lines += "Allow image pull: $($Plan.allow_image_pull)"
    $lines += "Include Alertmanager: $($Plan.include_alertmanager)"
    if (-not [string]::IsNullOrWhiteSpace([string]$Plan.platform)) {
        $lines += "Platform: $($Plan.platform)"
    }
    $lines | Set-Content -LiteralPath $markdownPath -Encoding utf8

    Write-Host "observability_image_prepare_plan_json=$jsonPath"
    Write-Host "observability_image_prepare_plan_report=$markdownPath"
}

$images = @(
    [pscustomobject]@{
        Name = "prometheus"
        Image = Get-ObservabilityImage -EnvName "NEXUSIM_PROMETHEUS_IMAGE" -DefaultImage "prom/prometheus:v2.54.1"
    },
    [pscustomobject]@{
        Name = "grafana"
        Image = Get-ObservabilityImage -EnvName "NEXUSIM_GRAFANA_IMAGE" -DefaultImage "grafana/grafana-oss:11.2.0"
    }
)

if ($IncludeAlertmanager) {
    $images += [pscustomobject]@{
        Name = "alertmanager"
        Image = Get-ObservabilityImage -EnvName "NEXUSIM_ALERTMANAGER_IMAGE" -DefaultImage "prom/alertmanager:v0.27.0"
    }
}

$dockerAvailable = Test-DockerAvailable
if (-not $dockerAvailable -and $AllowImagePull) {
    throw "Docker CLI is unavailable; cannot pull local observability images."
}

$missing = @()
$results = @()
foreach ($entry in $images) {
    $present = $false
    if ($dockerAvailable) {
        $present = Test-DockerImagePresent -Image $entry.Image
    }

    $pullCommand = New-DockerPullCommand -Image $entry.Image
    if ($present) {
        Write-Host "present $($entry.Name) image=$($entry.Image)"
        $results += [pscustomobject]@{
            name = $entry.Name
            image = $entry.Image
            status = "present"
            pull_command = $pullCommand
        }
        continue
    }

    $missing += $entry
    if ($AllowImagePull) {
        Write-Host "pulling $($entry.Name) image=$($entry.Image)"
        Invoke-DockerPull -Image $entry.Image
        $results += [pscustomobject]@{
            name = $entry.Name
            image = $entry.Image
            status = "pulled"
            pull_command = $pullCommand
        }
    }
    else {
        Write-Host "missing $($entry.Name) image=$($entry.Image)"
        Write-Host "        $pullCommand"
        $results += [pscustomobject]@{
            name = $entry.Name
            image = $entry.Image
            status = "missing"
            pull_command = $pullCommand
        }
    }
}

$plan = [pscustomobject]@{
    generated_at_utc = (Get-Date).ToUniversalTime().ToString("o")
    docker_available = [bool]$dockerAvailable
    include_alertmanager = [bool]$IncludeAlertmanager
    allow_image_pull = [bool]$AllowImagePull
    platform = $Platform
    missing_count = [int]$missing.Count
    images = $results
    boundary = "local observability image preparation only; does not start containers or prove production observability"
}

if (-not [string]::IsNullOrWhiteSpace($OutputDir)) {
    Write-ImagePreparePlan -PlanOutputDir $OutputDir -Plan $plan
}

if ($AllowImagePull) {
    Write-Host "OK   local observability images prepared."
}
else {
    Write-Host "OK   local observability image preparation dry-run."
    Write-Host "     missing_count=$($missing.Count)"
    Write-Host "     rerun_with=-AllowImagePull only after confirming network and traffic budget."
}
