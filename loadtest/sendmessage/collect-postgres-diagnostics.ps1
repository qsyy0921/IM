param(
    [string]$ResultDir = "",
    [string]$ContainerName = $(if ($env:NEXUSIM_POSTGRES_CONTAINER) { $env:NEXUSIM_POSTGRES_CONTAINER } else { "nexusim-postgres" }),
    [string]$Database = $(if ($env:NEXUSIM_POSTGRES_DB) { $env:NEXUSIM_POSTGRES_DB } else { "nexusim" }),
    [string]$User = $(if ($env:NEXUSIM_POSTGRES_USER) { $env:NEXUSIM_POSTGRES_USER } else { "nexusim" })
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

if (-not $ResultDir) {
    $ResultDir = Join-Path "loadtest\results" ("postgres-diagnostics-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
}
New-Item -ItemType Directory -Force $ResultDir | Out-Null

$sql = @"
WITH
settings AS (
    SELECT jsonb_object_agg(name, setting) AS value
    FROM pg_settings
    WHERE name IN (
        'max_connections',
        'shared_buffers',
        'work_mem',
        'maintenance_work_mem',
        'wal_buffers',
        'checkpoint_timeout',
        'max_wal_size',
        'synchronous_commit',
        'effective_cache_size'
    )
),
activity_summary AS (
    SELECT jsonb_build_object(
        'total', count(*),
        'active', count(*) FILTER (WHERE state = 'active'),
        'idle', count(*) FILTER (WHERE state = 'idle'),
        'idle_in_transaction', count(*) FILTER (WHERE state = 'idle in transaction'),
        'waiting', count(*) FILTER (WHERE wait_event_type IS NOT NULL)
    ) AS value
    FROM pg_stat_activity
),
activity_by_state AS (
    SELECT COALESCE(jsonb_agg(row_to_json(t) ORDER BY t.count DESC), '[]'::jsonb) AS value
    FROM (
        SELECT COALESCE(state, 'unknown') AS state, count(*)::bigint AS count
        FROM pg_stat_activity
        GROUP BY COALESCE(state, 'unknown')
    ) AS t
),
activity_by_wait AS (
    SELECT COALESCE(jsonb_agg(row_to_json(t) ORDER BY t.count DESC), '[]'::jsonb) AS value
    FROM (
        SELECT
            COALESCE(wait_event_type, 'none') AS wait_event_type,
            COALESCE(wait_event, 'none') AS wait_event,
            count(*)::bigint AS count
        FROM pg_stat_activity
        GROUP BY COALESCE(wait_event_type, 'none'), COALESCE(wait_event, 'none')
    ) AS t
),
locks_waiting AS (
    SELECT COALESCE(jsonb_agg(row_to_json(t) ORDER BY t.waiting_locks DESC), '[]'::jsonb) AS value
    FROM (
        SELECT locktype, mode, count(*)::bigint AS waiting_locks
        FROM pg_locks
        WHERE NOT granted
        GROUP BY locktype, mode
    ) AS t
),
database_stats AS (
    SELECT row_to_json(t)::jsonb AS value
    FROM (
        SELECT
            numbackends,
            xact_commit,
            xact_rollback,
            blks_read,
            blks_hit,
            tup_returned,
            tup_fetched,
            tup_inserted,
            tup_updated,
            tup_deleted,
            conflicts,
            deadlocks,
            temp_files,
            temp_bytes
        FROM pg_stat_database
        WHERE datname = current_database()
    ) AS t
),
table_stats AS (
    SELECT COALESCE(jsonb_agg(row_to_json(t) ORDER BY t.n_tup_ins DESC), '[]'::jsonb) AS value
    FROM (
        SELECT
            relname,
            n_live_tup,
            n_dead_tup,
            n_tup_ins,
            n_tup_upd,
            n_tup_del,
            seq_scan,
            idx_scan,
            vacuum_count,
            autovacuum_count,
            analyze_count,
            autoanalyze_count
        FROM pg_stat_user_tables
        WHERE schemaname = 'public'
          AND relname IN ('conversation_seq', 'message_log', 'conversation_timeline_events', 'message_outbox')
    ) AS t
),
index_stats AS (
    SELECT COALESCE(jsonb_agg(row_to_json(t) ORDER BY t.idx_scan DESC), '[]'::jsonb) AS value
    FROM (
        SELECT
            relname,
            indexrelname,
            idx_scan,
            idx_tup_read,
            idx_tup_fetch
        FROM pg_stat_user_indexes
        WHERE schemaname = 'public'
          AND relname IN ('conversation_seq', 'message_log', 'conversation_timeline_events', 'message_outbox')
    ) AS t
)
SELECT jsonb_pretty(jsonb_build_object(
    'captured_at', now(),
    'database', current_database(),
    'settings', (SELECT value FROM settings),
    'activity_summary', (SELECT value FROM activity_summary),
    'activity_by_state', (SELECT value FROM activity_by_state),
    'activity_by_wait', (SELECT value FROM activity_by_wait),
    'locks_waiting', (SELECT value FROM locks_waiting),
    'database_stats', (SELECT value FROM database_stats),
    'table_stats', (SELECT value FROM table_stats),
    'index_stats', (SELECT value FROM index_stats)
));
"@

$output = docker exec $ContainerName psql -U $User -d $Database -X -v ON_ERROR_STOP=1 -At -c $sql
$jsonPath = Join-Path $ResultDir "postgres-diagnostics.json"
$output | Set-Content -Path $jsonPath -Encoding utf8

Write-Host "PostgreSQL diagnostics written to $jsonPath"
