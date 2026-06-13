param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "policy-contact-projection-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
$topicSuffix = $RunName -replace '[^A-Za-z0-9._-]', '-'
$topic = "im.contact.events.$topicSuffix"
$consumerGroup = "nexusim-policy-contact-$topicSuffix"

New-Item -ItemType Directory -Force $logDir | Out-Null

. .\tools\go-env.ps1

if (-not $SkipBuild) {
    go build -o bin\policy-service.exe ./services/policy-service/cmd/policy-service
    go build -o bin\policy-contact-loadtest.exe ./loadtest/policycontacts
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

$processes = @()
try {
    $processes += Start-NexusProcess -Name "policy-contact-consumer" -FilePath (Join-Path $repo "bin\policy-service.exe") -Env @{
        NEXUSIM_POLICY_SERVICE_MODE = "contact-consumer"
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_KAFKA_BROKERS = $KafkaBrokers
        NEXUSIM_CONTACT_EVENTS_TOPIC = $topic
        NEXUSIM_POLICY_CONTACT_CONSUMER_GROUP = $consumerGroup
    }

    Start-Sleep -Milliseconds 1000

    $runner = Join-Path $repo "bin\policy-contact-loadtest.exe"
    & $runner `
        --brokers $KafkaBrokers `
        --topic $topic `
        --consumer-group $consumerGroup `
        --pg-dsn $PgDsn `
        --result-dir $resultDir `
        --cleanup=true
    if ($LASTEXITCODE -ne 0) {
        throw "policy contact projection smoke failed with exit code $LASTEXITCODE"
    }
} finally {
    Stop-Processes $processes
}

$summary = Get-Content -LiteralPath (Join-Path $resultDir "policy-contact-summary.json") -Raw | ConvertFrom-Json
if (-not $summary.success) {
    throw "policy contact projection smoke failed: $($summary.error)"
}

Write-Host "result_dir=$resultDir"
Write-Host "topic=$topic"
Write-Host "after_blocked=$($summary.after_blocked.status)/$($summary.after_blocked.edge_version)"
Write-Host "after_unblocked=$($summary.after_unblocked.status)/$($summary.after_unblocked.edge_version)"
