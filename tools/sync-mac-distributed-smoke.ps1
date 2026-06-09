param(
    [string]$MacHost = "172.31.50.2",
    [string]$MacUser = "qsyy0921",
    [string]$MacPath = "/Users/qsyy0921/Desktop/IM/_local/distributed-smoke",
    [string]$BundleRoot = "H:\NexusIM",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if ($MacPath -notmatch "/IM/_local/distributed-smoke$") {
    throw "Refusing to reset non-smoke Mac path: $MacPath"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$head = (git rev-parse --short HEAD).Trim()
New-Item -ItemType Directory -Force $BundleRoot | Out-Null
$bundlePath = Join-Path $BundleRoot "nexusim-main-$head.bundle"

git bundle create $bundlePath HEAD

if (-not $SkipBuild) {
    . .\tools\go-env.ps1
    New-Item -ItemType Directory -Force .\bin\darwin-arm64 | Out-Null
    $env:GOOS = "darwin"
    $env:GOARCH = "arm64"
    $env:CGO_ENABLED = "0"
    try {
        go build -o .\bin\darwin-arm64\push-gateway .\services\push-gateway\cmd\push-gateway
        go build -o .\bin\darwin-arm64\pushgateway-smoke .\loadtest\pushgateway
    }
    finally {
        Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

$remoteBundleDir = "/Users/$MacUser/Desktop/IM/_local/artifacts/bundles"
$remoteBundle = "$remoteBundleDir/nexusim-main-$head.bundle"
ssh -o BatchMode=yes "${MacUser}@${MacHost}" "mkdir -p '$remoteBundleDir'"
scp $bundlePath "${MacUser}@${MacHost}:$remoteBundle"

$remoteScript = @"
set -e
rm -rf '$MacPath'
git clone '$remoteBundle' '$MacPath'
cd '$MacPath'
git switch -c main >/dev/null 2>&1 || git switch main >/dev/null 2>&1 || true
mkdir -p '$MacPath/bin/darwin-arm64'
"@

ssh -o BatchMode=yes "${MacUser}@${MacHost}" $remoteScript

scp .\bin\darwin-arm64\push-gateway .\bin\darwin-arm64\pushgateway-smoke "${MacUser}@${MacHost}:$MacPath/bin/darwin-arm64/"

ssh -o BatchMode=yes "${MacUser}@${MacHost}" "chmod +x '$MacPath/bin/darwin-arm64/push-gateway' '$MacPath/bin/darwin-arm64/pushgateway-smoke'; cd '$MacPath'; git status --short --branch; git rev-parse --short HEAD; NEXUSIM_PUSH_GATEWAY_MODE=noop ./bin/darwin-arm64/push-gateway"
