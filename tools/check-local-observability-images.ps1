param(
    [switch]$RequireImages
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

$images = @(
    [pscustomobject]@{
        Name = "prometheus"
        Image = Get-ObservabilityImage -EnvName "NEXUSIM_PROMETHEUS_IMAGE" -DefaultImage "prom/prometheus:v2.54.1"
    },
    [pscustomobject]@{
        Name = "grafana"
        Image = Get-ObservabilityImage -EnvName "NEXUSIM_GRAFANA_IMAGE" -DefaultImage "grafana/grafana-oss:11.2.0"
    },
    [pscustomobject]@{
        Name = "alertmanager"
        Image = Get-ObservabilityImage -EnvName "NEXUSIM_ALERTMANAGER_IMAGE" -DefaultImage "prom/alertmanager:v0.27.0"
    }
)

if (-not (Test-DockerAvailable)) {
    if ($RequireImages) {
        Write-Host "FAIL local observability image preflight: docker CLI is unavailable." -ForegroundColor Red
        exit 1
    }

    Write-Host "OK   local observability image preflight skipped: docker CLI is unavailable."
    Write-Host "     require_images=false"
    exit 0
}

$results = @()
$missing = @()
foreach ($entry in $images) {
    $present = Test-DockerImagePresent -Image $entry.Image
    $result = [pscustomobject]@{
        Name = $entry.Name
        Image = $entry.Image
        Present = $present
    }
    $results += $result
    if (-not $present) {
        $missing += $result
    }
}

if ($missing.Count -gt 0 -and $RequireImages) {
    Write-Host "FAIL local observability images missing: $($missing.Name -join ', ')." -ForegroundColor Red
    foreach ($result in $results) {
        $status = if ($result.Present) { "present" } else { "missing" }
        Write-Host "     $($result.Name)=$status image=$($result.Image)"
    }
    exit 1
}

Write-Host "OK   local observability image preflight"
Write-Host "     require_images=$([bool]$RequireImages)"
foreach ($result in $results) {
    $status = if ($result.Present) { "present" } else { "missing" }
    Write-Host "     $($result.Name)=$status image=$($result.Image)"
}
