# Kafka Failover Smoke

Date: 2026-06-14

## Scope

This run verifies the local distributed NexusIM chain against a local Kafka HA topology backed by:

```text
Kafka KRaft brokers x 3
+ RF=3 topics
```

The goal is narrow:

```text
same Kafka broker list
-> smoke before broker stop
-> stop current im.delivery.events leader broker
-> wait for leader re-election
-> smoke again
```

It does not claim production Kafka HA.

## Topology

Kafka brokers:

```text
127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094
```

Kafka admin listener inside broker network:

```text
kafka-ha-x:29092
```

Runtime topology:

```text
conversation-service grpc
message-service grpc + outbox-relay
delivery-service grpc + timeline-consumer + outbox-relay
push-gateway ws + delivery-consumer
PostgreSQL localhost:5432
Redis localhost:6379
Kafka HA KRaft localhost:19092,19093,19094
```

## Command

```powershell
.\tools\local-kafka-failover-smoke.ps1 `
  -RunName kafka-failover-smoke-20260614b `
  -SkipBuild
```

## Evidence

Main summary:

```text
H:\NexusIM\loadtest-results\kafka-failover-smoke-20260614b\kafka-failover-summary.json
```

Before smoke summary:

```text
H:\NexusIM\loadtest-results\kafka-failover-smoke-20260614b-before\pushgateway-summary.json
```

After smoke summary:

```text
H:\NexusIM\loadtest-results\kafka-failover-smoke-20260614b-after\pushgateway-summary.json
```

Leader switch:

```text
before_leader_broker_id = 2
stopped_container = nexusim-kafka-ha-1
after_leader_broker_id = 3
```

Before smoke key facts:

```text
delivery.notify conversation_seq = 2
PullInbox item_count = 1
PullInbox max_seq = 2
delivery.ack.ok last_received_seq = 2
delivery_outbox PUBLISHED = 2 / PENDING = 0 / DLQ = 0
```

After failover smoke key facts:

```text
delivery.notify conversation_seq = 2
PullInbox item_count = 1
PullInbox max_seq = 2
delivery.ack.ok last_received_seq = 2
delivery_outbox PUBLISHED = 2 / PENDING = 0 / DLQ = 0
```

## Interpretation

This proves a limited but useful property:

```text
the local Kafka broker list stayed usable across a single broker loss
and leader re-election,
and the NexusIM distributed smoke chain still completed after failover
without changing service Kafka brokers.
```

One implementation detail mattered:

```text
HA admin commands executed inside broker containers must use
the internal listener kafka-ha-x:29092,
not the host listener 127.0.0.1:1909x.
```

If the admin client connects through the host listener from inside the container,
Kafka returns host-advertised addresses that are not routable from the broker container,
which produces false-negative topic and consumer-group admin failures.

## Limits

This run does not prove:

- production Kafka HA
- controller failover policy beyond this local KRaft setup
- quorum loss behavior
- multi-broker loss
- ISR instability behavior under load
- cross-machine Kafka HA
- in-flight produce / commit continuity at the exact broker-stop window
- tuned producer durability policy for all services

It is a first-stage local Kafka failover smoke only.
