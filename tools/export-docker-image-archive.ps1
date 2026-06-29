param(
    [string]$OutputRoot = "H:\NexusIM\docker-images",
    [string]$Name = "",
    [string[]]$Images = @(),
    [switch]$IncludeNexusIMServices,
    [switch]$IncludeCoreMiddleware,
    [switch]$IncludeOptionalMiddleware,
    [switch]$ListAvailable
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $OutputRoot -RepositoryRoot $repoRoot -Name "OutputRoot" -SuggestedRoot "H:\NexusIM\docker-images"

function Get-DockerImages {
    docker images --format "{{.Repository}}:{{.Tag}}" |
        Where-Object { $_ -and $_ -notmatch "<none>" } |
        Sort-Object -Unique
}

function Test-ImageExists {
    param([string]$Image)
    docker image inspect $Image *> $null
    return $LASTEXITCODE -eq 0
}

$available = @(Get-DockerImages)

if ($ListAvailable) {
    $available | ForEach-Object { Write-Output $_ }
    return
}

$selected = New-Object System.Collections.Generic.List[string]

foreach ($image in $Images) {
    if (-not [string]::IsNullOrWhiteSpace($image)) {
        $selected.Add($image.Trim())
    }
}

if ($IncludeNexusIMServices) {
    foreach ($image in $available | Where-Object { $_ -like "nexusim/*:local" }) {
        $selected.Add($image)
    }
}

if ($IncludeCoreMiddleware) {
    foreach ($image in @(
        "postgres:16-alpine",
        "redis:7-alpine",
        "confluentinc/cp-kafka:7.7.1"
    )) {
        $selected.Add($image)
    }
}

if ($IncludeOptionalMiddleware) {
    foreach ($image in @(
        "confluentinc/cp-schema-registry:7.7.1",
        "provectuslabs/kafka-ui:latest"
    )) {
        $selected.Add($image)
    }
}

$selected = @($selected | Sort-Object -Unique)
if ($selected.Count -eq 0) {
    throw "No images selected. Pass -Images, -IncludeNexusIMServices, -IncludeCoreMiddleware or -IncludeOptionalMiddleware."
}

$missing = @()
foreach ($image in $selected) {
    if (-not (Test-ImageExists -Image $image)) {
        $missing += $image
    }
}
if ($missing.Count -gt 0) {
    throw "Missing Docker images: $($missing -join ', ')"
}

if ([string]::IsNullOrWhiteSpace($Name)) {
    $Name = "docker-images-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$archiveDir = Join-Path $OutputRoot "archives"
New-Item -ItemType Directory -Force -Path $archiveDir | Out-Null

$tarPath = Join-Path $archiveDir "$Name.tar"
$manifestPath = Join-Path $archiveDir "$Name.manifest.json"
$hostName = $env:COMPUTERNAME
if ([string]::IsNullOrWhiteSpace($hostName)) {
    $hostName = (hostname)
}

Write-Host "Exporting $($selected.Count) images to $tarPath"
docker save -o $tarPath @selected

$manifest = [pscustomobject]@{
    schema_version = 1
    name = $Name
    tar_path = $tarPath
    created_at = (Get-Date).ToUniversalTime().ToString("o")
    host = $hostName
    image_count = $selected.Count
    images = $selected
    load_command = "docker load -i `"$tarPath`""
}

$manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $manifestPath -Encoding UTF8

$file = Get-Item -LiteralPath $tarPath
Write-Host "OK   archive: $tarPath"
Write-Host "OK   manifest: $manifestPath"
Write-Host ("OK   size_gib: {0:N2}" -f ($file.Length / 1GB))
