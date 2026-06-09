param(
    [string]$MacHost = "172.31.50.2",
    [string]$MacUser = "qsyy0921",
    [string]$MacPath = "/Users/qsyy0921/Desktop/IM/_local/distributed-smoke",
    [string]$ImageTag = "nexusim/push-gateway:local",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if ($MacPath -notmatch "/IM/_local/distributed-smoke$") {
    throw "Refusing to write non-smoke Mac path: $MacPath"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot
$buildRoot = Join-Path "H:\NexusIM" "docker-build"
$contextPath = Join-Path $buildRoot "push-gateway-linux-arm64"

if (-not $SkipBuild) {
    . .\tools\go-env.ps1
    New-Item -ItemType Directory -Force .\bin\linux-arm64 | Out-Null
    $env:GOOS = "linux"
    $env:GOARCH = "arm64"
    $env:CGO_ENABLED = "0"
    try {
        go build -o .\bin\linux-arm64\push-gateway .\services\push-gateway\cmd\push-gateway
    }
    finally {
        Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

ssh -o BatchMode=yes "${MacUser}@${MacHost}" "mkdir -p '$MacPath/bin/linux-arm64' '$MacPath/docker/push-gateway'"
scp .\bin\linux-arm64\push-gateway "${MacUser}@${MacHost}:$MacPath/bin/linux-arm64/push-gateway"

if (Test-Path -LiteralPath $contextPath) {
    Remove-Item -LiteralPath $contextPath -Recurse -Force
}
New-Item -ItemType Directory -Force $contextPath | Out-Null
Copy-Item -LiteralPath .\bin\linux-arm64\push-gateway -Destination (Join-Path $contextPath "push-gateway")
@'
FROM scratch
COPY push-gateway /push-gateway
ENTRYPOINT ["/push-gateway"]
'@ | Set-Content -LiteralPath (Join-Path $contextPath "Dockerfile") -Encoding ascii

scp (Join-Path $contextPath "push-gateway") (Join-Path $contextPath "Dockerfile") "${MacUser}@${MacHost}:$MacPath/docker/push-gateway/"
ssh -o BatchMode=yes "${MacUser}@${MacHost}" "chmod +x '$MacPath/docker/push-gateway/push-gateway'; docker build --platform linux/arm64 -t '$ImageTag' '$MacPath/docker/push-gateway'; docker image inspect '$ImageTag' --format 'image={{.RepoTags}} id={{.Id}} size={{.Size}}'"
