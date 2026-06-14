param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@127.0.0.1:15432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-pgpool",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not $RunName) {
    $RunName = "postgres-failover-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

function Invoke-PGPoolScalar {
    param([string]$Sql)
    $output = docker exec $PostgresExecContainer env PGPASSWORD=nexusim psql `
        -U nexusim `
        -h 127.0.0.1 `
        -p 5432 `
        -d postgres `
        -At `
        -v ON_ERROR_STOP=1 `
        -c $Sql
    if ($LASTEXITCODE -ne 0) {
        throw "pgpool scalar query failed: $Sql"
    }
    if ($null -eq $output) {
        return ""
    }
    return (($output | Select-Object -Last 1) -as [string]).Trim()
}

function Get-PGPoolNodes {
    $lines = @(
        docker exec $PostgresExecContainer env PGPASSWORD=nexusim psql `
            -U nexusim `
            -h 127.0.0.1 `
            -p 5432 `
            -d postgres `
            -At `
            -v ON_ERROR_STOP=1 `
            -c "show pool_nodes;"
    )
    if ($LASTEXITCODE -ne 0 -or $lines.Count -lt 1) {
        throw "failed to query pgpool nodes"
    }
    return $lines
}

function Get-CurrentPrimaryHost {
    $primaryLine = Get-PGPoolNodes | Where-Object { $_ -match "\|primary\|" } | Select-Object -First 1
    if (-not $primaryLine) {
        throw "pgpool did not report a primary backend"
    }
    return ($primaryLine -split "\|")[1].Trim()
}

function Apply-PostgresMigration {
    param(
        [string]$Path,
        [string]$Name
    )
    $resolved = (Resolve-Path $Path).Path
    $containerPath = "/tmp/$Name"
    docker cp $resolved "${PostgresExecContainer}:$containerPath" | Out-Null
    docker exec $PostgresExecContainer env PGPASSWORD=nexusim psql `
        -U nexusim `
        -h 127.0.0.1 `
        -p 5432 `
        -d nexusim `
        -v ON_ERROR_STOP=1 `
        -f $containerPath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "failed to apply migration $Name"
    }
}

function Apply-CoreMigrations {
    $migrationSets = @(
        "migrations\postgres\message",
        "migrations\postgres\conversation",
        "migrations\postgres\delivery"
    )
    foreach ($dir in $migrationSets) {
        foreach ($migration in Get-ChildItem -LiteralPath (Join-Path $repo $dir) -Filter "*.sql" | Sort-Object Name) {
            Apply-PostgresMigration -Path $migration.FullName -Name ("postgres-ha-" + $migration.Name)
        }
    }
}

function Test-PGPoolWriteReady {
    try {
        docker exec $PostgresExecContainer env PGPASSWORD=nexusim psql `
            -U nexusim `
            -h 127.0.0.1 `
            -p 5432 `
            -d nexusim `
            -v ON_ERROR_STOP=1 `
            -c "CREATE TABLE IF NOT EXISTS nexusim_pg_failover_probe (id integer PRIMARY KEY, touched_at timestamptz NOT NULL); INSERT INTO nexusim_pg_failover_probe (id, touched_at) VALUES (1, now()) ON CONFLICT (id) DO UPDATE SET touched_at = EXCLUDED.touched_at;" | Out-Null
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

function Wait-ForFailover {
    param(
        [string]$PreviousPrimaryHost,
        [int]$TimeoutSeconds = 120
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $currentPrimaryHost = ""
    $poolReady = ""
    $lastError = ""
    $stableWriteProbeCount = 0
    do {
        Start-Sleep -Seconds 2
        try {
            $currentPrimaryHost = Get-CurrentPrimaryHost
            $poolReady = Invoke-PGPoolScalar -Sql "SELECT 1;"
            if ($currentPrimaryHost -ne $PreviousPrimaryHost -and $poolReady -eq "1" -and (Test-PGPoolWriteReady)) {
                $stableWriteProbeCount++
            } else {
                $stableWriteProbeCount = 0
            }
            $lastError = ""
        } catch {
            $lastError = $_.Exception.Message
            $currentPrimaryHost = ""
            $poolReady = ""
            $stableWriteProbeCount = 0
        }
    } while (($currentPrimaryHost -eq $PreviousPrimaryHost -or $poolReady -ne "1" -or $stableWriteProbeCount -lt 3) -and (Get-Date) -lt $deadline)

    if ($currentPrimaryHost -eq $PreviousPrimaryHost -or $poolReady -ne "1" -or $stableWriteProbeCount -lt 3) {
        throw "PostgreSQL failover did not complete before timeout; previous=$PreviousPrimaryHost current=$currentPrimaryHost stable_write_probes=$stableWriteProbeCount last_error=$lastError"
    }

    Start-Sleep -Seconds 5
    return $currentPrimaryHost
}

docker compose -f deploy/local/docker-compose.postgres-ha.yml down -v | Out-Null
& .\tools\local-up-postgres-ha.ps1
Apply-CoreMigrations

$summary = [ordered]@{
    run_name = $RunName
    git_commit = (git rev-parse HEAD)
    git_dirty = -not [string]::IsNullOrWhiteSpace((git status --short))
    pg_dsn = $PgDsn
    postgres_exec_container = $PostgresExecContainer
}

$summary.before_primary = Get-CurrentPrimaryHost

$beforeRun = "$RunName-before"
$beforeArgs = @{
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    ResultRoot = $ResultRoot
    RunName = $beforeRun
}
if ($SkipBuild) {
    $beforeArgs.SkipBuild = $true
}
& .\tools\local-distributed-smoke.ps1 @beforeArgs

$stoppedContainer = switch ($summary.before_primary) {
    "postgres-ha-0" { "nexusim-postgres-ha-0" }
    "postgres-ha-1" { "nexusim-postgres-ha-1" }
    "postgres-ha-2" { "nexusim-postgres-ha-2" }
    default { throw "unknown primary host $($summary.before_primary)" }
}

$summary.stopped_container = $stoppedContainer
docker stop $stoppedContainer | Out-Null
$summary.after_primary = Wait-ForFailover -PreviousPrimaryHost $summary.before_primary

$afterRun = "$RunName-after"
$afterArgs = @{
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    ResultRoot = $ResultRoot
    RunName = $afterRun
    SkipBuild = $true
}
& .\tools\local-distributed-smoke.ps1 @afterArgs

docker start $stoppedContainer | Out-Null

$summary.before_summary = Join-Path (Join-Path $ResultRoot $beforeRun) "pushgateway-summary.json"
$summary.after_summary = Join-Path (Join-Path $ResultRoot $afterRun) "pushgateway-summary.json"
$summary.completed_at = (Get-Date).ToString("o")

$summaryPath = Join-Path $resultDir "postgres-failover-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

Write-Host "postgres_failover_before_primary=$($summary.before_primary)"
Write-Host "postgres_failover_after_primary=$($summary.after_primary)"
Write-Host "postgres_failover_summary=$summaryPath"
