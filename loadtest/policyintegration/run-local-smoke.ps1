param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "policy-message-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
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
    go build -o bin\message-service.exe ./services/message-service/cmd/message-service
    go build -o bin\policy-message-loadtest.exe ./loadtest/policyintegration
}

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = Resolve-Path $Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "nexusim-postgres:$containerPath"
    docker exec nexusim-postgres psql `
        -U nexusim `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
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

function Start-NexusProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [hashtable]$Env,
        [int]$Port = 0
    )
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    $proc = Start-Process -FilePath $FilePath `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
    if ($Port -gt 0) {
        Wait-Tcp -HostName "127.0.0.1" -Port $Port
    } else {
        Start-Sleep -Milliseconds 800
    }
    return $proc
}

function Stop-Processes {
    param([array]$Processes)
    foreach ($proc in $Processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

function Run-Scenario {
    param(
        [string]$Scenario,
        [string]$ScenarioDir,
        [bool]$Allowed,
        [int64]$PermissionVersion,
        [string]$Classification,
        [string]$Reason
    )
    $policyPort = Get-FreeTcpPort
    $messagePort = Get-FreeTcpPort
    $policyAddr = "127.0.0.1:$policyPort"
    $messageAddr = "127.0.0.1:$messagePort"
    $processes = @()
    try {
        $processes += Start-NexusProcess -Name "policy-$Scenario" -FilePath (Join-Path $repo "bin\policy-service.exe") -Port $policyPort -Env @{
            NEXUSIM_POLICY_SERVICE_MODE = "grpc"
            NEXUSIM_POLICY_GRPC_ADDR = $policyAddr
            NEXUSIM_POLICY_MESSAGE_ALLOWED = [string]$Allowed
            NEXUSIM_POLICY_PERMISSION_VERSION = [string]$PermissionVersion
            NEXUSIM_POLICY_CLASSIFICATION = $Classification
            NEXUSIM_POLICY_DENY_REASON = $Reason
        }
        $processes += Start-NexusProcess -Name "message-$Scenario" -FilePath (Join-Path $repo "bin\message-service.exe") -Port $messagePort -Env @{
            NEXUSIM_MESSAGE_SERVICE_MODE = "grpc"
            NEXUSIM_GRPC_ADDR = $messageAddr
            NEXUSIM_PG_DSN = $PgDsn
            NEXUSIM_POLICY_SERVICE_ADDR = $policyAddr
            NEXUSIM_POLICY_RPC_TIMEOUT = "2s"
            NEXUSIM_MOCK_PERMISSION_VERSION = [string]$PermissionVersion
            NEXUSIM_MOCK_CLASSIFICATION = $Classification
        }
        $runner = Join-Path $repo "bin\policy-message-loadtest.exe"
        $tenant = "tenant-policy-message-$Scenario-" + (Get-Date -Format "yyyyMMddHHmmss")
        $args = @(
            "--target", $messageAddr,
            "--pg-dsn", $PgDsn,
            "--scenario", $Scenario,
            "--result-dir", $ScenarioDir,
            "--tenant-id", $tenant,
            "--conversation-id", "policy-message-$Scenario-conversation",
            "--client-msg-id", "policy-message-$Scenario-client-msg",
            "--expected-permission-version", $PermissionVersion,
            "--expected-classification", $Classification,
            "--cleanup"
        )
        if ($Reason -ne "") {
            $args += @("--expected-reason", $Reason)
        }
        & $runner @args
        if ($LASTEXITCODE -ne 0) {
            throw "policy message smoke runner failed with exit code $LASTEXITCODE"
        }
        $summary = Get-Content -LiteralPath (Join-Path $ScenarioDir "policy-message-summary.json") -Raw | ConvertFrom-Json
        if (-not $summary.success) {
            throw "policy message smoke failed: $($summary.error)"
        }
        return $summary
    } finally {
        Stop-Processes $processes
    }
}

Get-ChildItem -Path "migrations\postgres\message" -Filter "*.sql" |
    Sort-Object Name |
    ForEach-Object {
        Apply-PostgresMigration -Path $_.FullName -Name $_.Name
    }

$allowSummary = Run-Scenario `
    -Scenario "allow" `
    -ScenarioDir $allowDir `
    -Allowed $true `
    -PermissionVersion 41 `
    -Classification "POLICY_RPC_ALLOWED" `
    -Reason ""

$denySummary = Run-Scenario `
    -Scenario "deny" `
    -ScenarioDir $denyDir `
    -Allowed $false `
    -PermissionVersion 42 `
    -Classification "POLICY_RPC_BLOCKED" `
    -Reason "blocked by policy integration smoke"

$combined = [pscustomobject]@{
    success = $true
    result_dir = $resultDir
    allow = $allowSummary
    deny = $denySummary
}
$combined | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $resultDir "policy-message-smoke-summary.json") -Encoding UTF8

Write-Host "result_dir=$resultDir"
Write-Host "allow_message_id=$($allowSummary.send_message.message_id)"
Write-Host "deny_grpc_code=$($denySummary.send_message.grpc_code)"
