$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-postgres-smoke-summary.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing PostgreSQL smoke summary validator: $validator"
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
        [string]$Scenario = "full",
        [int]$Seq = 2
    )

    return [ordered]@{
        commit = "abcdef0"
        commit_full = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        route_backend = "redis"
        scenario = $Scenario
        success = $true
        send_message = [ordered]@{
            message_id = "msg_1"
            conversation_seq = $Seq
        }
        pull_inbox = [ordered]@{
            item_count = 1
            max_seq = $Seq
        }
        delivery_ack_ok = [ordered]@{
            op = "delivery.ack.ok"
            last_received_seq = $Seq
        }
        delivery_outbox_published = 2
        delivery_outbox_pending = 0
        delivery_outbox_dlq = 0
    }
}

function Invoke-Validator {
    param(
        [string]$SummaryPath,
        [string]$ExpectedScenario
    )

    try {
        $output = & $validator `
            -SummaryPath $SummaryPath `
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-postgres-smoke-summary-" + [System.Guid]::NewGuid().ToString("N"))
try {
    $failoverDir = Join-Path $tempRoot "failover"
    $failoverBeforeDir = Join-Path $tempRoot "failover-before"
    $failoverAfterDir = Join-Path $tempRoot "failover-after"
    New-Item -ItemType Directory -Force -Path $failoverDir, $failoverBeforeDir, $failoverAfterDir | Out-Null
    $failoverBeforePush = Join-Path $failoverBeforeDir "pushgateway-summary.json"
    $failoverAfterPush = Join-Path $failoverAfterDir "pushgateway-summary.json"
    Write-JsonFile -Path $failoverBeforePush -Value (New-PushGatewaySummary)
    Write-JsonFile -Path $failoverAfterPush -Value (New-PushGatewaySummary)
    $failoverSummary = Join-Path $failoverDir "postgres-failover-summary.json"
    Write-JsonFile -Path $failoverSummary -Value ([ordered]@{
        run_name = "postgres-failover-fixture"
        git_commit = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        pg_dsn = "postgres://fixture"
        postgres_exec_container = "nexusim-pgpool"
        before_primary = "postgres-ha-0"
        stopped_container = "nexusim-postgres-ha-0"
        after_primary = "postgres-ha-1"
        before_summary = $failoverBeforePush
        after_summary = $failoverAfterPush
        completed_at = (Get-Date).ToUniversalTime().ToString("o")
    })
    $failoverResult = Invoke-Validator -SummaryPath $failoverSummary -ExpectedScenario "postgres-failover"
    if ($failoverResult.ExitCode -ne 0) {
        Write-Host "FAIL postgres failover fixture should pass." -ForegroundColor Red
        Write-Host $failoverResult.Output -ForegroundColor Red
        exit 1
    }

    $quorumDir = Join-Path $tempRoot "quorum"
    $quorumBeforeDir = Join-Path $tempRoot "quorum-before"
    $quorumOnlyPrimaryDir = Join-Path $tempRoot "quorum-only-primary"
    $quorumAfterRestoreDir = Join-Path $tempRoot "quorum-after-restore"
    New-Item -ItemType Directory -Force -Path $quorumDir, $quorumBeforeDir, $quorumOnlyPrimaryDir, $quorumAfterRestoreDir | Out-Null
    $quorumBeforePush = Join-Path $quorumBeforeDir "pushgateway-summary.json"
    $quorumOnlyPrimaryPush = Join-Path $quorumOnlyPrimaryDir "pushgateway-summary.json"
    $quorumAfterRestorePush = Join-Path $quorumAfterRestoreDir "pushgateway-summary.json"
    Write-JsonFile -Path $quorumBeforePush -Value (New-PushGatewaySummary)
    Write-JsonFile -Path $quorumOnlyPrimaryPush -Value (New-PushGatewaySummary)
    Write-JsonFile -Path $quorumAfterRestorePush -Value (New-PushGatewaySummary)
    $quorumSummary = Join-Path $quorumDir "postgres-quorum-observation-summary.json"
    Write-JsonFile -Path $quorumSummary -Value ([ordered]@{
        run_name = "postgres-quorum-observation-fixture"
        git_commit = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        pg_dsn = "postgres://fixture"
        postgres_exec_container = "nexusim-pgpool"
        before_primary = "postgres-ha-0"
        before_pool_nodes = @("0|postgres-ha-0|5432|up|up|0.333333|primary|primary", "1|postgres-ha-1|5432|up|up|0.333333|standby|standby", "2|postgres-ha-2|5432|up|up|0.333333|standby|standby")
        stopped_standby_containers = @("nexusim-postgres-ha-1", "nexusim-postgres-ha-2")
        after_standby_stop_pool_nodes = @("0|postgres-ha-0|5432|up|up|0.333333|primary|primary")
        write_probe_with_only_primary = $true
        only_primary_summary = $quorumOnlyPrimaryPush
        after_restore_pool_nodes = @("0|postgres-ha-0|5432|up|up|0.333333|primary|primary", "1|postgres-ha-1|5432|up|up|0.333333|standby|standby", "2|postgres-ha-2|5432|up|up|0.333333|standby|standby")
        before_summary = $quorumBeforePush
        after_restore_summary = $quorumAfterRestorePush
        completed_at = (Get-Date).ToUniversalTime().ToString("o")
    })
    $quorumResult = Invoke-Validator -SummaryPath $quorumSummary -ExpectedScenario "postgres-quorum-observation"
    if ($quorumResult.ExitCode -ne 0) {
        Write-Host "FAIL postgres quorum observation fixture should pass." -ForegroundColor Red
        Write-Host $quorumResult.Output -ForegroundColor Red
        exit 1
    }

    $badFailoverDir = Join-Path $tempRoot "bad-failover"
    New-Item -ItemType Directory -Force -Path $badFailoverDir | Out-Null
    $badFailoverSummary = Join-Path $badFailoverDir "postgres-failover-summary.json"
    Write-JsonFile -Path $badFailoverSummary -Value ([ordered]@{
        run_name = "bad-postgres-failover-fixture"
        git_commit = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        pg_dsn = "postgres://fixture"
        postgres_exec_container = "nexusim-pgpool"
        before_primary = "postgres-ha-0"
        stopped_container = "nexusim-postgres-ha-0"
        after_primary = "postgres-ha-0"
        before_summary = $failoverBeforePush
        after_summary = $failoverAfterPush
        completed_at = (Get-Date).ToUniversalTime().ToString("o")
    })
    $badFailoverResult = Invoke-Validator -SummaryPath $badFailoverSummary -ExpectedScenario "postgres-failover"
    if ($badFailoverResult.ExitCode -eq 0) {
        Write-Host "FAIL postgres failover fixture with unchanged primary should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badFailoverResult.Output.Contains("after_primary")) {
        Write-Host "FAIL bad failover fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badFailoverResult.Output -ForegroundColor Red
        exit 1
    }

    $badQuorumDir = Join-Path $tempRoot "bad-quorum"
    New-Item -ItemType Directory -Force -Path $badQuorumDir | Out-Null
    $badQuorumSummary = Join-Path $badQuorumDir "postgres-quorum-observation-summary.json"
    Write-JsonFile -Path $badQuorumSummary -Value ([ordered]@{
        run_name = "bad-postgres-quorum-fixture"
        git_commit = "abcdef0123456789abcdef0123456789abcdef01"
        git_dirty = $false
        pg_dsn = "postgres://fixture"
        postgres_exec_container = "nexusim-pgpool"
        before_primary = "postgres-ha-0"
        before_pool_nodes = @("0|postgres-ha-0|5432|up|up|0.333333|primary|primary")
        stopped_standby_containers = @("nexusim-postgres-ha-1", "nexusim-postgres-ha-2")
        after_standby_stop_pool_nodes = @("0|postgres-ha-0|5432|up|up|0.333333|primary|primary")
        write_probe_with_only_primary = $true
        after_restore_pool_nodes = @("0|postgres-ha-0|5432|up|up|0.333333|primary|primary")
        before_summary = $quorumBeforePush
        after_restore_summary = $quorumAfterRestorePush
        completed_at = (Get-Date).ToUniversalTime().ToString("o")
    })
    $badQuorumResult = Invoke-Validator -SummaryPath $badQuorumSummary -ExpectedScenario "postgres-quorum-observation"
    if ($badQuorumResult.ExitCode -eq 0) {
        Write-Host "FAIL postgres quorum fixture missing only_primary_summary should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badQuorumResult.Output.Contains("only_primary_summary")) {
        Write-Host "FAIL bad quorum fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badQuorumResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   PostgreSQL smoke summary self-test"
