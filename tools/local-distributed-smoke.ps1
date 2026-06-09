param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$KafkaBrokers = "localhost:9092",
    [ValidateSet("single", "sentinel")]
    [string]$RedisMode = "single",
    [string]$RedisAddr = "127.0.0.1:6379",
    [string]$RedisSentinelAddrs = "127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381",
    [string]$RedisSentinelMasterName = "mymaster",
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
    RedisMode = $RedisMode
    RedisAddr = $RedisAddr
    RedisSentinelAddrs = $RedisSentinelAddrs
    RedisSentinelMasterName = $RedisSentinelMasterName
    ResultRoot = $ResultRoot
    RunName = $RunName
}

if ($SkipBuild) {
    $scriptArgs.SkipBuild = $true
}

& .\loadtest\pushgateway\run-local-smoke.ps1 @scriptArgs
