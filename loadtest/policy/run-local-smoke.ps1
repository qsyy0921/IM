param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "policy-service-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$allowDir = Join-Path $resultDir "allow"
$denyDir = Join-Path $resultDir "deny"
$logDir = Join-Path $resultDir "logs"

New-Item -ItemType Directory -Force $allowDir | Out-Null
New-Item -ItemType Directory -Force $denyDir | Out-Null
New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\policy-service.exe ./services/policy-service/cmd/policy-service
    go build -o bin\policy-loadtest.exe ./loadtest/policy
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Wait-Tcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 20
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 200
    }
    throw "Timed out waiting for ${HostName}:${Port}"
}

function Start-PolicyService {
    param(
        [string]$Name,
        [bool]$Allowed,
        [int64]$PermissionVersion,
        [string]$Classification,
        [string]$Reason
    )
    $port = Get-FreeTcpPort
    $addr = "127.0.0.1:$port"
    [Environment]::SetEnvironmentVariable("NEXUSIM_POLICY_SERVICE_MODE", "grpc", "Process")
    [Environment]::SetEnvironmentVariable("NEXUSIM_POLICY_GRPC_ADDR", $addr, "Process")
    [Environment]::SetEnvironmentVariable("NEXUSIM_POLICY_MESSAGE_ALLOWED", [string]$Allowed, "Process")
    [Environment]::SetEnvironmentVariable("NEXUSIM_POLICY_PERMISSION_VERSION", [string]$PermissionVersion, "Process")
    [Environment]::SetEnvironmentVariable("NEXUSIM_POLICY_CLASSIFICATION", $Classification, "Process")
    [Environment]::SetEnvironmentVariable("NEXUSIM_POLICY_DENY_REASON", $Reason, "Process")

    $service = Join-Path $repo "bin\policy-service.exe"
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $proc = Start-Process -FilePath $service `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
    Wait-Tcp -HostName "127.0.0.1" -Port $port
    return [pscustomobject]@{
        Process = $proc
        Address = $addr
    }
}

function Stop-PolicyService {
    param([object]$Started)
    if ($null -ne $Started -and $null -ne $Started.Process -and -not $Started.Process.HasExited) {
        Stop-Process -Id $Started.Process.Id -Force -ErrorAction SilentlyContinue
    }
}

function Run-PolicyProbe {
    param(
        [string]$Target,
        [string]$ScenarioDir,
        [bool]$ExpectedAllowed,
        [int64]$ExpectedPermissionVersion,
        [string]$ExpectedClassification,
        [string]$ExpectedReason
    )
    $runner = Join-Path $repo "bin\policy-loadtest.exe"
    $args = @(
        "--target", $Target,
        "--result-dir", $ScenarioDir,
        "--expected-allowed=$ExpectedAllowed",
        "--expected-permission-version", $ExpectedPermissionVersion,
        "--expected-classification", $ExpectedClassification
    )
    if ($ExpectedReason -ne "") {
        $args += @("--expected-reason", $ExpectedReason)
    }
    & $runner @args
    if ($LASTEXITCODE -ne 0) {
        throw "policy smoke runner failed with exit code $LASTEXITCODE"
    }
    $summary = Get-Content -LiteralPath (Join-Path $ScenarioDir "policy-summary.json") -Raw | ConvertFrom-Json
    if (-not $summary.success) {
        throw "policy smoke failed: $($summary.error)"
    }
    if ($summary.actions.Count -ne 4) {
        throw "expected 4 policy actions, got $($summary.actions.Count)"
    }
    return $summary
}

$allowService = $null
$denyService = $null
try {
    $allowService = Start-PolicyService `
        -Name "policy-allow" `
        -Allowed $true `
        -PermissionVersion 31 `
        -Classification "CONTACT_ALLOWED" `
        -Reason ""
    $allowSummary = Run-PolicyProbe `
        -Target $allowService.Address `
        -ScenarioDir $allowDir `
        -ExpectedAllowed $true `
        -ExpectedPermissionVersion 31 `
        -ExpectedClassification "CONTACT_ALLOWED" `
        -ExpectedReason ""
    Stop-PolicyService $allowService
    $allowService = $null

    $denyService = Start-PolicyService `
        -Name "policy-deny" `
        -Allowed $false `
        -PermissionVersion 32 `
        -Classification "CONTACT_BLOCKED" `
        -Reason "blocked by policy smoke"
    $denySummary = Run-PolicyProbe `
        -Target $denyService.Address `
        -ScenarioDir $denyDir `
        -ExpectedAllowed $false `
        -ExpectedPermissionVersion 32 `
        -ExpectedClassification "CONTACT_BLOCKED" `
        -ExpectedReason "blocked by policy smoke"
    Stop-PolicyService $denyService
    $denyService = $null

    $combined = [pscustomobject]@{
        success = $true
        result_dir = $resultDir
        allow = $allowSummary
        deny = $denySummary
    }
    $combined | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $resultDir "policy-smoke-summary.json") -Encoding UTF8
} finally {
    Stop-PolicyService $allowService
    Stop-PolicyService $denyService
}

Write-Host "result_dir=$resultDir"
Write-Host "allow_actions=$($allowSummary.actions.Count)"
Write-Host "deny_actions=$($denySummary.actions.Count)"
