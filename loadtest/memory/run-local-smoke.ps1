param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$Topic = "",
    [string]$ConsumerGroup = "",
    [string]$MemoryGrpcAddr = "127.0.0.1:10580",
    [int]$TopicReplicationFactor = 1,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "memory-service-projection-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}
if (-not $Topic) {
    $Topic = "conversation.timeline.memory." + (Get-Date -Format "yyyyMMddHHmmss")
}
if (-not $ConsumerGroup) {
    $ConsumerGroup = "nexusim-memory-smoke-" + (Get-Date -Format "yyyyMMddHHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\memory-service.exe") ./services/memory-service/cmd/memory-service
    go build -o (Join-Path $repoRoot "bin\memory-smoke.exe") ./loadtest/memory
}

function Ensure-KafkaTopic {
    param([string]$Name)
    docker exec $KafkaExecContainer kafka-topics `
        --bootstrap-server localhost:9092 `
        --create `
        --if-not-exists `
        --topic $Name `
        --partitions 1 `
        --replication-factor $TopicReplicationFactor | Out-Null
}

function Start-MemoryProcess {
    param(
        [string]$Name,
        [hashtable]$Env
    )

    $exe = Join-Path $repoRoot "bin\memory-service.exe"
    $stdout = Join-Path $logDir "$Name.out.log"
    $stderr = Join-Path $logDir "$Name.err.log"
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $exe
    $psi.WorkingDirectory = $repoRoot
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    foreach ($key in $Env.Keys) {
        $psi.Environment[$key] = [string]$Env[$key]
    }

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $psi
    $null = $process.Start()
    Register-ObjectEvent `
        -InputObject $process `
        -EventName OutputDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stdout | Out-Null
    Register-ObjectEvent `
        -InputObject $process `
        -EventName ErrorDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stderr | Out-Null
    $process.BeginOutputReadLine()
    $process.BeginErrorReadLine()
    return $process
}

$processes = @()
try {
    Ensure-KafkaTopic -Name $Topic

    $processes += Start-MemoryProcess -Name "memory-grpc" -Env @{
        NEXUSIM_MEMORY_SERVICE_MODE = "grpc"
        NEXUSIM_MEMORY_GRPC_ADDR = $MemoryGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEMORY_DEBUG_ADDR = ""
    }
    Start-Sleep -Milliseconds 500

    $processes += Start-MemoryProcess -Name "memory-timeline-consumer" -Env @{
        NEXUSIM_MEMORY_SERVICE_MODE = "timeline-consumer"
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_TIMELINE_TOPIC = $Topic
        NEXUSIM_MEMORY_CONSUMER_GROUP = $ConsumerGroup
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEMORY_DEBUG_ADDR = ""
    }
    Start-Sleep -Seconds 2

    $runner = Join-Path $repoRoot "bin\memory-smoke.exe"
    & $runner `
        --pg-dsn $PgDsn `
        --kafka-brokers $KafkaBrokers `
        --topic $Topic `
        --consumer-group $ConsumerGroup `
        --memory-target $MemoryGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName `
        --ensure-topic=false
    if ($LASTEXITCODE -ne 0) {
        throw "memory smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "topic=$Topic"
    Write-Host "consumer_group=$ConsumerGroup"
    Write-Host "result_dir=$resultDir"
}
finally {
    foreach ($process in $processes) {
        if ($process -and -not $process.HasExited) {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
        }
        if ($process) {
            $process.Dispose()
        }
    }
    Get-EventSubscriber |
        Where-Object { $_.SourceObject -is [System.Diagnostics.Process] } |
        Unregister-Event -ErrorAction SilentlyContinue
}
