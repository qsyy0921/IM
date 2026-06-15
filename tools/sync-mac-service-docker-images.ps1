param(
    [string]$MacHost = "172.31.50.2",
    [string]$MacUser = "qsyy0921",
    [string]$MacPath = "/Users/qsyy0921/Desktop/IM/_local/distributed-smoke",
    [string]$ImagePrefix = "nexusim",
    [string]$ImageTag = "local",
    [string]$BundleRoot = "H:\NexusIM",
    [string[]]$Services = @(),
    [switch]$SkipBuild,
    [switch]$SkipWindowsImages,
    [switch]$ListServices
)

$ErrorActionPreference = "Stop"

if ($MacPath -notmatch "/IM/_local/distributed-smoke$") {
    throw "Refusing to write non-smoke Mac path: $MacPath"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

function Get-ImplementedServices {
    Get-ChildItem -LiteralPath (Join-Path $repoRoot "services") -Directory |
        Sort-Object Name |
        Select-Object -ExpandProperty Name
}

function Get-ServiceCommand {
    param([string]$Service)

    $commandPath = Join-Path $repoRoot "services\$Service\cmd\$Service"
    if (-not (Test-Path -LiteralPath $commandPath)) {
        throw "Unknown service or missing service command: $Service"
    }
    return "./services/$Service/cmd/$Service"
}

if ($Services.Count -eq 0) {
    $Services = @(Get-ImplementedServices)
}

if ($ListServices) {
    $Services | ForEach-Object { Write-Output $_ }
    return
}

foreach ($service in $Services) {
    [void](Get-ServiceCommand -Service $service)
}

New-Item -ItemType Directory -Force $BundleRoot | Out-Null
$buildRoot = Join-Path $BundleRoot "docker-build"
New-Item -ItemType Directory -Force $buildRoot | Out-Null

function Build-GoBinary {
    param(
        [string]$Service,
        [string]$Goos,
        [string]$Goarch,
        [string]$OutputPath
    )
    $env:GOOS = $Goos
    $env:GOARCH = $Goarch
    $env:CGO_ENABLED = "0"
    try {
        go build -o $OutputPath (Get-ServiceCommand -Service $Service)
    }
    finally {
        Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

function New-DockerContext {
    param(
        [string]$ContextPath,
        [string]$BinaryPath
    )
    if (Test-Path -LiteralPath $ContextPath) {
        Remove-Item -LiteralPath $ContextPath -Recurse -Force
    }
    New-Item -ItemType Directory -Force $ContextPath | Out-Null
    Copy-Item -LiteralPath $BinaryPath -Destination (Join-Path $ContextPath "nexusim-service")
    @'
FROM scratch
COPY nexusim-service /nexusim-service
ENTRYPOINT ["/nexusim-service"]
'@ | Set-Content -LiteralPath (Join-Path $ContextPath "Dockerfile") -Encoding ascii
}

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    New-Item -ItemType Directory -Force .\bin\linux-amd64 | Out-Null
    New-Item -ItemType Directory -Force .\bin\linux-arm64 | Out-Null
    foreach ($service in $Services) {
        Build-GoBinary -Service $service -Goos "linux" -Goarch "amd64" -OutputPath ".\bin\linux-amd64\$service"
        Build-GoBinary -Service $service -Goos "linux" -Goarch "arm64" -OutputPath ".\bin\linux-arm64\$service"
    }
}

if (-not $SkipWindowsImages) {
    foreach ($service in $Services) {
        $contextPath = Join-Path $buildRoot "$service-linux-amd64"
        New-DockerContext -ContextPath $contextPath -BinaryPath ".\bin\linux-amd64\$service"
        docker build --platform linux/amd64 -t "$ImagePrefix/${service}:$ImageTag" $contextPath
    }
}

$remoteBuildRoot = "$MacPath/docker/service-images"
ssh -o BatchMode=yes "${MacUser}@${MacHost}" "rm -rf '$remoteBuildRoot'; mkdir -p '$remoteBuildRoot'"

foreach ($service in $Services) {
    $remoteContext = "$remoteBuildRoot/$service"
    $armContextPath = Join-Path $buildRoot "$service-linux-arm64"
    New-DockerContext -ContextPath $armContextPath -BinaryPath ".\bin\linux-arm64\$service"
    ssh -o BatchMode=yes "${MacUser}@${MacHost}" "mkdir -p '$remoteContext'"
    scp (Join-Path $armContextPath "nexusim-service") (Join-Path $armContextPath "Dockerfile") "${MacUser}@${MacHost}:$remoteContext/"
    ssh -o BatchMode=yes "${MacUser}@${MacHost}" "chmod +x '$remoteContext/nexusim-service'; docker build --platform linux/arm64 -t '$ImagePrefix/${service}:$ImageTag' '$remoteContext'"
}

Write-Host "windows_images="
if (-not $SkipWindowsImages) {
    docker images --format "{{.Repository}}:{{.Tag}} {{.ID}} {{.Size}}" | Select-String "^$ImagePrefix/"
}
Write-Host "mac_images="
ssh -o BatchMode=yes "${MacUser}@${MacHost}" "docker images --format '{{.Repository}}:{{.Tag}} {{.ID}} {{.Size}}' | grep '^$ImagePrefix/' || true"
