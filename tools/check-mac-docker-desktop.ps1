param(
    [string]$MacHost = "172.31.50.2",
    [string]$MacUser = "qsyy0921",
    [int]$ExpectedCPU = 8,
    [int]$ExpectedMemoryMiB = 8192,
    [string]$ExpectedProxy = "http://127.0.0.1:7890"
)

$ErrorActionPreference = "Stop"

$python = @'
import json
import pathlib
import subprocess

settings_path = pathlib.Path.home() / "Library/Group Containers/group.com.docker/settings-store.json"
proxy_path = pathlib.Path.home() / "Library/Group Containers/group.com.docker/http_proxy.json"

settings = json.loads(settings_path.read_text())
try:
    proxy = json.loads(proxy_path.read_text())
except FileNotFoundError:
    proxy = {}

def run(cmd):
    return subprocess.check_output(cmd, text=True, stderr=subprocess.STDOUT).strip()

print("docker_cli=" + run(["docker", "--version"]))
print("docker_context=" + run(["docker", "context", "show"]))
print("settings_path=" + str(settings_path))
print("cpus=" + str(settings.get("Cpus")))
print("memory_mib=" + str(settings.get("MemoryMiB")))
print("swap_mib=" + str(settings.get("SwapMiB")))
print("disk_size_mib=" + str(settings.get("DiskSizeMiB")))
print("proxy_http=" + str(proxy.get("http", "")))
print("proxy_https=" + str(proxy.get("https", "")))
print("proxy_exclude=" + str(proxy.get("exclude", "")))
'@

New-Item -ItemType Directory -Force $env:TEMP | Out-Null
$localScript = Join-Path $env:TEMP "nexusim-check-mac-docker.py"
$remoteScript = "/tmp/nexusim-check-mac-docker.py"
[System.IO.File]::WriteAllText($localScript, $python, [System.Text.UTF8Encoding]::new($false))
try {
    scp $localScript "${MacUser}@${MacHost}:$remoteScript" | Out-Null
    $output = ssh -o BatchMode=yes "${MacUser}@${MacHost}" "python3 $remoteScript; rm -f $remoteScript"
}
finally {
    Remove-Item $localScript -Force -ErrorAction SilentlyContinue
}
$output | ForEach-Object { Write-Host $_ }

$values = @{}
foreach ($line in $output) {
    $parts = $line -split "=", 2
    if ($parts.Count -eq 2) {
        $values[$parts[0]] = $parts[1]
    }
}

if ([int]$values["cpus"] -ne $ExpectedCPU) {
    throw "Unexpected Mac Docker CPU setting: $($values["cpus"])"
}
if ([int]$values["memory_mib"] -ne $ExpectedMemoryMiB) {
    throw "Unexpected Mac Docker memory setting: $($values["memory_mib"]) MiB"
}
if ($values["proxy_http"] -ne $ExpectedProxy -or $values["proxy_https"] -ne $ExpectedProxy) {
    throw "Unexpected Mac Docker proxy: http=$($values["proxy_http"]) https=$($values["proxy_https"])"
}
if ($values["proxy_exclude"] -notmatch "172\.16\.0\.0/12") {
    throw "Mac Docker proxy exclude does not cover 172.16.0.0/12"
}

Write-Host "mac_docker_desktop_config=OK"
