$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-redis-smoke-summary.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing Redis smoke summary validator: $validator"
}

function Write-JsonFile {
    param(
        [string]$Path,
        $Value
    )

    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $Value | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function New-PushGatewaySummary {
    param(
        [string]$Scenario,
        [string]$RedisMode,
        [int]$MessageCount = 1,
        [bool]$NotifyReceived = $false,
        [bool]$IncludeFault = $true
    )

    $sendSeq = 2 + [Math]::Max(0, $MessageCount - 1)
    $summary = [ordered]@{
        commit = "abcdef0"
        commit_full = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        route_backend = "redis"
        redis_mode = $RedisMode
        scenario = $Scenario
        success = $true
        message_count = $MessageCount
        send_message = [ordered]@{
            message_id = "msg_1"
            conversation_seq = $sendSeq
        }
        pull_inbox = [ordered]@{
            item_count = $MessageCount
            max_seq = $sendSeq
        }
        delivery_outbox_published = $MessageCount + 1
        delivery_outbox_pending = 0
        delivery_outbox_dlq = 0
        capacity_summary = [ordered]@{
            duration_ms = 1000
            device_count = 1
            message_count = $MessageCount
            notify_frame_count = $MessageCount
            ack_frame_count = 1
            pull_inbox_item_count = $MessageCount
            delivery_outbox_published = $MessageCount + 1
        }
    }

    if ($IncludeFault) {
        $summary.redis_fault = [ordered]@{
            fault_command = "fixture"
            command_output = "fixture"
            notify_received = $NotifyReceived
            notify_wait_error = "fixture"
            recovery_pull_inbox = [ordered]@{
                item_count = 1
                max_seq = $sendSeq
            }
            ack_ok = [ordered]@{
                op = "delivery.ack.ok"
            }
            delivery_outbox_total = $MessageCount + 1
        }
    }

    return $summary
}

function New-WrapperSummary {
    param(
        [string]$RunDir,
        [string]$Scenario,
        [string]$RedisMode,
        [int]$MessageCount = 1
    )

    $wrapperName = switch -Regex ($Scenario) {
        "sentinel" { "redis-sentinel-summary.json"; break }
        "cluster" { "redis-cluster-summary.json"; break }
        default { "redis-summary.json" }
    }
    $wrapperPath = Join-Path $RunDir $wrapperName
    $pushPath = Join-Path $RunDir "pushgateway-summary.json"
    Write-JsonFile -Path $wrapperPath -Value ([ordered]@{
        run_name = Split-Path -Leaf $RunDir
        git_commit = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        redis_mode = $RedisMode
        redis_sentinel_addrs = if ($RedisMode -eq "sentinel") { "127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381" } else { $null }
        redis_cluster_addrs = if ($RedisMode -eq "cluster") { "127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002,127.0.0.1:7003,127.0.0.1:7004,127.0.0.1:7005" } else { $null }
        message_count = $MessageCount
        pushgateway_summary = $pushPath
        completed_at = (Get-Date).ToUniversalTime().ToString("o")
    })
    return $wrapperPath
}

function Invoke-Validator {
    param(
        [string]$SummaryPath,
        [string]$ExpectedRedisMode,
        [string]$ExpectedScenario
    )

    try {
        $output = & $validator `
            -SummaryPath $SummaryPath `
            -ExpectedRedisMode $ExpectedRedisMode `
            -ExpectedScenario $ExpectedScenario `
            -RequireCleanGit 2>&1
        return [pscustomobject]@{
            ExitCode = 0
            Output = (($output | Out-String).Trim())
        }
    }
    catch {
        return [pscustomobject]@{
            ExitCode = 1
            Output = [string]$_.Exception.Message
        }
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-redis-smoke-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $sentinelDir = Join-Path $tempRoot "sentinel-quorum-loss"
    New-Item -ItemType Directory -Force -Path $sentinelDir | Out-Null
    Write-JsonFile `
        -Path (Join-Path $sentinelDir "pushgateway-summary.json") `
        -Value (New-PushGatewaySummary -Scenario "redis-sentinel-quorum-loss" -RedisMode "sentinel" -NotifyReceived $false)
    $sentinelSummary = New-WrapperSummary -RunDir $sentinelDir -Scenario "redis-sentinel-quorum-loss" -RedisMode "sentinel"
    $sentinelResult = Invoke-Validator -SummaryPath $sentinelSummary -ExpectedRedisMode "sentinel" -ExpectedScenario "redis-sentinel-quorum-loss"
    if ($sentinelResult.ExitCode -ne 0) {
        Write-Host "FAIL sentinel recovery fixture should pass." -ForegroundColor Red
        Write-Host $sentinelResult.Output -ForegroundColor Red
        exit 1
    }

    $clusterFailoverDir = Join-Path $tempRoot "cluster-failover"
    New-Item -ItemType Directory -Force -Path $clusterFailoverDir | Out-Null
    Write-JsonFile `
        -Path (Join-Path $clusterFailoverDir "pushgateway-summary.json") `
        -Value (New-PushGatewaySummary -Scenario "redis-cluster-failover" -RedisMode "cluster" -NotifyReceived $true)
    $clusterFailoverSummary = New-WrapperSummary -RunDir $clusterFailoverDir -Scenario "redis-cluster-failover" -RedisMode "cluster"
    $clusterFailoverResult = Invoke-Validator -SummaryPath $clusterFailoverSummary -ExpectedRedisMode "cluster" -ExpectedScenario "redis-cluster-failover"
    if ($clusterFailoverResult.ExitCode -ne 0) {
        Write-Host "FAIL cluster failover fixture should pass." -ForegroundColor Red
        Write-Host $clusterFailoverResult.Output -ForegroundColor Red
        exit 1
    }

    $capacityDir = Join-Path $tempRoot "cluster-capacity"
    New-Item -ItemType Directory -Force -Path $capacityDir | Out-Null
    Write-JsonFile `
        -Path (Join-Path $capacityDir "pushgateway-summary.json") `
        -Value (New-PushGatewaySummary -Scenario "full" -RedisMode "cluster" -MessageCount 16 -IncludeFault $false)
    $capacitySummary = New-WrapperSummary -RunDir $capacityDir -Scenario "redis-cluster-capacity" -RedisMode "cluster" -MessageCount 16
    $capacityResult = Invoke-Validator -SummaryPath $capacitySummary -ExpectedRedisMode "cluster" -ExpectedScenario "full"
    if ($capacityResult.ExitCode -ne 0) {
        Write-Host "FAIL cluster capacity fixture should pass." -ForegroundColor Red
        Write-Host $capacityResult.Output -ForegroundColor Red
        exit 1
    }

    $badDir = Join-Path $tempRoot "bad-sentinel"
    New-Item -ItemType Directory -Force -Path $badDir | Out-Null
    Write-JsonFile `
        -Path (Join-Path $badDir "pushgateway-summary.json") `
        -Value (New-PushGatewaySummary -Scenario "redis-sentinel-network-partition" -RedisMode "sentinel" -NotifyReceived $true)
    $badSummary = New-WrapperSummary -RunDir $badDir -Scenario "redis-sentinel-network-partition" -RedisMode "sentinel"
    $badResult = Invoke-Validator -SummaryPath $badSummary -ExpectedRedisMode "sentinel" -ExpectedScenario "redis-sentinel-network-partition"
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL sentinel recovery fixture with unexpected notify should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("PullInbox recovery")) {
        Write-Host "FAIL bad sentinel fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   Redis smoke summary self-test"
