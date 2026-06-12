param(
    [string]$MacHost = "172.31.50.2",
    [string]$MacUser = "qsyy0921",
    [string]$MacPath = "/Users/qsyy0921/Desktop/IM/_local/distributed-smoke",
    [string]$ImagePrefix = "nexusim",
    [string]$ImageTag = "local",
    [string]$BundleRoot = "H:\NexusIM",
    [string[]]$Services = @(
        "conversation-service",
        "message-service",
        "delivery-service",
        "push-gateway",
        "receipt-service",
        "contacts-service",
        "identity-service"
    ),
    [switch]$SkipBuild,
    [switch]$SkipWindowsImages
)

$ErrorActionPreference = "Stop"

if ($MacPath -notmatch "/IM/_local/distributed-smoke$") {
    throw "Refusing to write non-smoke Mac path: $MacPath"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$serviceCommands = @{
    "conversation-service" = "./services/conversation-service/cmd/conversation-service"
    "message-service" = "./services/message-service/cmd/message-service"
    "delivery-service" = "./services/delivery-service/cmd/delivery-service"
    "push-gateway" = "./services/push-gateway/cmd/push-gateway"
    "receipt-service" = "./services/receipt-service/cmd/receipt-service"
    "contacts-service" = "./services/contacts-service/cmd/contacts-service"
    "identity-service" = "./services/identity-service/cmd/identity-service"
}

foreach ($service in $Services) {
    if (-not $serviceCommands.ContainsKey($service)) {
        throw "Unknown service: $service"
    }
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
        go build -o $OutputPath $serviceCommands[$Service]
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
