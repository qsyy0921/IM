param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$RedisAddr = "127.0.0.1:6379",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "nexusim-distributed-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$scriptArgs = @{
    Scenario = "full"
    RouteBackend = "redis"
    PgDsn = $PgDsn
    KafkaBrokers = $KafkaBrokers
    RedisAddr = $RedisAddr
    ResultRoot = $ResultRoot
    RunName = $RunName
}

if ($SkipBuild) {
    $scriptArgs.SkipBuild = $true
}

& .\loadtest\pushgateway\run-local-smoke.ps1 @scriptArgs
