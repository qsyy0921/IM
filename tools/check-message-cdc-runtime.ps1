param(
    [string]$ConnectUrl = "http://localhost:18083",
    [string]$ConnectorName = "nexusim-message-timeline-cdc",
    [string]$PostgresContainer = "nexusim-postgres",
    [string]$PostgresUser = "nexusim",
    [string]$PostgresDb = "nexusim",
    [string]$ReplicationSlot = "nexusim_message_timeline_slot",
    [string]$KafkaContainer = "nexusim-kafka",
    [string]$BootstrapServer = "localhost:9092",
    [string]$TargetTopic = "conversation.timeline.events.cdc"
)

$ErrorActionPreference = "Stop"

function Get-TopicEndOffsets {
    param([string]$Topic)
    $output = docker exec $KafkaContainer kafka-get-offsets `
        --bootstrap-server $BootstrapServer `
        --topic $Topic
    $sum = 0L
    $partitions = @()
    foreach ($line in $output) {
        if ($line -match "^[^:]+:(\d+):(\d+)$") {
            $offset = [int64]$Matches[2]
            $sum += $offset
            $partitions += [pscustomobject]@{
                partition = [int]$Matches[1]
                end_offset = $offset
            }
        }
    }
    [pscustomobject]@{
        topic = $Topic
        total_end_offset = $sum
        partitions = $partitions
    }
}

$connectorStatus = Invoke-RestMethod -Method Get -Uri "$($ConnectUrl.TrimEnd('/'))/connectors/$ConnectorName/status" -TimeoutSec 10

$slotSql = @"
SELECT json_build_object(
  'wal_level', current_setting('wal_level'),
  'slot', (
    SELECT json_build_object(
      'slot_name', slot_name,
      'active', active,
      'plugin', plugin,
      'database', database,
      'restart_lsn', restart_lsn::text,
      'confirmed_flush_lsn', confirmed_flush_lsn::text,
      'retained_wal_bytes', pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)
    )
    FROM pg_replication_slots
    WHERE slot_name = '$ReplicationSlot'
  )
)::text;
"@

$slotRaw = docker exec $PostgresContainer psql -U $PostgresUser -d $PostgresDb -t -A -c $slotSql
if ($LASTEXITCODE -ne 0) {
    throw "failed to query PostgreSQL replication slot state"
}
$slot = $slotRaw | ConvertFrom-Json
$targetOffsets = Get-TopicEndOffsets -Topic $TargetTopic

[pscustomobject]@{
    connector = $connectorStatus
    postgres = $slot
    kafka_target = $targetOffsets
} | ConvertTo-Json -Depth 20
