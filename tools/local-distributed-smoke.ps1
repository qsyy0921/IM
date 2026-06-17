param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-postgres",
    [string]$KafkaBrokers = "localhost:9092",
    [string]$KafkaExecContainer = "nexusim-kafka",
    [string]$KafkaAdminBootstrap = "localhost:9092",
    [int]$KafkaTopicReplicationFactor = 1,
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

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "nexusim-distributed-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$scriptArgs = @{
    Scenario = "full"
    RouteBackend = "redis"
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    KafkaBrokers = $KafkaBrokers
    KafkaExecContainer = $KafkaExecContainer
    KafkaAdminBootstrap = $KafkaAdminBootstrap
    KafkaTopicReplicationFactor = $KafkaTopicReplicationFactor
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
