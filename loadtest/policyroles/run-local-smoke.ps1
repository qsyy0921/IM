param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "policy-conversation-role-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$topicSuffix = $RunName -replace '[^A-Za-z0-9._-]', '-'
$topic = "conversation.timeline.events.$topicSuffix"
$consumerGroup = "nexusim-policy-timeline-$topicSuffix"

New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\policy-service.exe ./services/policy-service/cmd/policy-service
    go build -o bin\policy-role-loadtest.exe ./loadtest/policyroles
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

function Ensure-KafkaTopic {
    param([string]$Topic)
    docker exec nexusim-kafka kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Topic `
        --partitions 1 `
        --replication-factor 1 | Out-Null
}

function Get-FreeTcpPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try {
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
        [hashtable]$Env
    )
    foreach ($key in $Env.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$Env[$key], "Process")
    }
    $out = Join-Path $logDir "$Name.out.log"
    $err = Join-Path $logDir "$Name.err.log"
    return Start-Process -FilePath $FilePath `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $out `
        -RedirectStandardError $err
}

function Stop-Processes {
    param([array]$Processes)
    foreach ($proc in $Processes) {
        if ($null -ne $proc -and -not $proc.HasExited) {
            Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        }
    }
}

Get-ChildItem -Path "migrations\postgres\policy" -Filter "*.sql" |
    Sort-Object Name |
    ForEach-Object {
        Apply-PostgresMigration -Path $_.FullName -Name $_.Name
    }
Ensure-KafkaTopic -Topic $topic

$policyGrpcPort = Get-FreeTcpPort
$policyGrpcAddr = "127.0.0.1:$policyGrpcPort"

$processes = @()
try {
    $processes += Start-NexusProcess -Name "policy-timeline-consumer" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "timeline-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_CONVERSATION_TIMELINE_TOPIC = $topic
        NEXUSIM_POLICY_TIMELINE_CONSUMER_GROUP = $consumerGroup
    }
    $processes += Start-NexusProcess -Name "policy-grpc" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "grpc"
        NEXUSIM_POLICY_GRPC_ADDR = $policyGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_POLICY_RULES_ENABLED = "true"
        NEXUSIM_POLICY_MESSAGE_ALLOWED = "false"
        NEXUSIM_POLICY_PERMISSION_VERSION = "1"
        NEXUSIM_POLICY_CLASSIFICATION = "POLICY_STATIC_DENY"
        NEXUSIM_POLICY_DENY_REASON = "static fallback should not decide role smoke"
    }
    Wait-Tcp -HostName "127.0.0.1" -Port $policyGrpcPort
    Start-Sleep -Milliseconds 800

    $runner = Join-Path $repo "bin\policy-role-loadtest.exe"
    & $runner `
        --brokers $KafkaBrokers `
        --topic $topic `
        --consumer-group $consumerGroup `
        --policy-grpc-addr $policyGrpcAddr `
        --pg-dsn $PgDsn `
        --result-dir $resultDir `
        --cleanup=true
    if ($LASTEXITCODE -ne 0) {
        throw "policy conversation role smoke failed with exit code $LASTEXITCODE"
    }
} finally {
    Stop-Processes $processes
}

$summary = Get-Content -LiteralPath (Join-Path $resultDir "policy-role-summary.json") -Raw | ConvertFrom-Json
if (-not $summary.success) {
    throw "policy conversation role smoke failed: $($summary.error)"
}

Write-Host "result_dir=$resultDir"
Write-Host "topic=$topic"
Write-Host "joined=$($summary.joined_projection.role)/$($summary.joined_projection.status)/v$($summary.joined_projection.permission_version)"
Write-Host "role_denied=$($summary.role_denied_decision.allowed)/$($summary.role_denied_decision.classification)/v$($summary.role_denied_decision.permission_version)"
Write-Host "stale_error=$($summary.stale_decision.error_code)"
Write-Host "inactive_denied=$($summary.inactive_denied_decision.allowed)/$($summary.inactive_denied_decision.classification)/v$($summary.inactive_denied_decision.permission_version)"
