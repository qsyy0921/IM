param(
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$PgDsn = "postgres://nexusim:nexusim@127.0.0.1:15432/nexusim?sslmode=disable",
    [string]$PostgresExecContainer = "nexusim-pgpool",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "postgres-quorum-observation-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$repo = (Get-Location).Path
$resultDir = Join-Path $ResultRoot $RunName
New-Item -ItemType Directory -Force $resultDir | Out-Null

$hostToContainer = @{
    "postgres-ha-0" = "nexusim-postgres-ha-0"
    "postgres-ha-1" = "nexusim-postgres-ha-1"
    "postgres-ha-2" = "nexusim-postgres-ha-2"
}

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

function Wait-ForPGPoolNodes {
    param([int]$TimeoutSeconds = 120)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $nodes = @()
    $lastError = ""
    do {
        Start-Sleep -Seconds 2
        try {
            $nodes = @(Get-PGPoolNodes)
            $lastError = ""
        } catch {
            $nodes = @()
            $lastError = $_.Exception.Message
        }
    } while ($nodes.Count -eq 0 -and (Get-Date) -lt $deadline)
    if ($nodes.Count -eq 0) {
        throw "pgpool nodes did not become queryable before timeout; last_error=$lastError"
    }
    return $nodes
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
            Apply-PostgresMigration -Path $migration.FullName -Name ("postgres-quorum-" + $migration.Name)
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
            -c "CREATE TABLE IF NOT EXISTS nexusim_pg_quorum_probe (id integer PRIMARY KEY, touched_at timestamptz NOT NULL); INSERT INTO nexusim_pg_quorum_probe (id, touched_at) VALUES (1, now()) ON CONFLICT (id) DO UPDATE SET touched_at = EXCLUDED.touched_at;" | Out-Null
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

function Wait-ForPGPool {
    param([int]$TimeoutSeconds = 120)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $ready = $false
    do {
        Start-Sleep -Seconds 2
        try {
            $ready = (Invoke-PGPoolScalar -Sql "SELECT 1;") -eq "1"
        } catch {
            $ready = $false
        }
    } while (-not $ready -and (Get-Date) -lt $deadline)
    if (-not $ready) {
        throw "pgpool did not become ready before timeout"
    }
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
$summary.before_pool_nodes = @(Get-PGPoolNodes)

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

$standbyHosts = @($hostToContainer.Keys | Where-Object { $_ -ne $summary.before_primary } | Sort-Object)
$standbyContainers = @($standbyHosts | ForEach-Object { $hostToContainer[$_] })
$summary.stopped_standby_containers = $standbyContainers

$standbysStopped = $false
try {
    foreach ($container in $standbyContainers) {
        docker stop $container | Out-Null
    }
    $standbysStopped = $true
    $summary.after_standby_stop_pool_nodes = @(Wait-ForPGPoolNodes -TimeoutSeconds 120)
    $summary.write_probe_with_only_primary = Test-PGPoolWriteReady

    $duringRun = "$RunName-only-primary"
    $duringArgs = @{
        PgDsn = $PgDsn
        PostgresExecContainer = $PostgresExecContainer
        ResultRoot = $ResultRoot
        RunName = $duringRun
        SkipBuild = $true
    }
    if ($summary.write_probe_with_only_primary) {
        & .\tools\local-distributed-smoke.ps1 @duringArgs
        $summary.only_primary_summary = Join-Path (Join-Path $ResultRoot $duringRun) "pushgateway-summary.json"
    } else {
        $summary.only_primary_summary = ""
    }
} finally {
    if ($standbysStopped) {
        foreach ($container in $standbyContainers) {
            $running = docker inspect -f "{{.State.Running}}" $container 2>$null
            if ($running -ne "true") {
                docker start $container | Out-Null
            }
        }
        Wait-ForPGPool
        Start-Sleep -Seconds 10
        $summary.after_restore_pool_nodes = @(Wait-ForPGPoolNodes -TimeoutSeconds 120)
    }
}

$afterRun = "$RunName-after-restore"
$afterArgs = @{
    PgDsn = $PgDsn
    PostgresExecContainer = $PostgresExecContainer
    ResultRoot = $ResultRoot
    RunName = $afterRun
    SkipBuild = $true
}
& .\tools\local-distributed-smoke.ps1 @afterArgs

$summary.before_summary = Join-Path (Join-Path $ResultRoot $beforeRun) "pushgateway-summary.json"
$summary.after_restore_summary = Join-Path (Join-Path $ResultRoot $afterRun) "pushgateway-summary.json"
$summary.completed_at = (Get-Date).ToString("o")

$summaryPath = Join-Path $resultDir "postgres-quorum-observation-summary.json"
$summary | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $summaryPath -Encoding UTF8

Write-Host "postgres_quorum_before_primary=$($summary.before_primary)"
Write-Host "postgres_quorum_stopped_standbys=$($standbyContainers -join ',')"
Write-Host "postgres_quorum_write_probe_with_only_primary=$($summary.write_probe_with_only_primary)"
Write-Host "postgres_quorum_observation_summary=$summaryPath"
