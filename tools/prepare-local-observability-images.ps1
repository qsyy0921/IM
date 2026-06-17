param(
    [switch]$IncludeAlertmanager,
    [switch]$AllowImagePull,
    [string]$Platform = ""
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
foreach ($entry in $images) {
    $present = $false
    if ($dockerAvailable) {
        $present = Test-DockerImagePresent -Image $entry.Image
    }

    if ($present) {
        Write-Host "present $($entry.Name) image=$($entry.Image)"
        continue
    }

    $missing += $entry
    if ($AllowImagePull) {
        Write-Host "pulling $($entry.Name) image=$($entry.Image)"
        Invoke-DockerPull -Image $entry.Image
    }
    else {
        Write-Host "missing $($entry.Name) image=$($entry.Image)"
        Write-Host "        $(New-DockerPullCommand -Image $entry.Image)"
    }
}

if ($AllowImagePull) {
    Write-Host "OK   local observability images prepared."
}
else {
    Write-Host "OK   local observability image preparation dry-run."
    Write-Host "     missing_count=$($missing.Count)"
    Write-Host "     rerun_with=-AllowImagePull only after confirming network and traffic budget."
}
