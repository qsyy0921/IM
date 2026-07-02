param(
    [string]$KafkaContainer = "nexusim-kafka",
    [string]$BootstrapServer = "localhost:9092",
    [string[]]$Topics = @("conversation.timeline.events.cdc"),
    [int]$Partitions = 3,
    [int]$ReplicationFactor = 1
)

$ErrorActionPreference = "Stop"

foreach ($topic in $Topics) {
    if ([string]::IsNullOrWhiteSpace($topic)) {
        continue
    }
    docker exec $KafkaContainer kafka-topics `
        --bootstrap-server $BootstrapServer `
        --create `
        --if-not-exists `
        --topic $topic `
        --partitions $Partitions `
        --replication-factor $ReplicationFactor | Out-Host
}
