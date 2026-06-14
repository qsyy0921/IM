# PostgreSQL Failover Smoke

Date: 2026-06-14

## Scope

This run verifies the local distributed NexusIM chain against a PostgreSQL HA write endpoint backed by:

```text
bitnamilegacy/postgresql-repmgr x 3
+ pgpool stable writer endpoint
```

The goal is narrow:

```text
same pgpool DSN
-> smoke before primary stop
-> stop current primary
-> wait for new primary + write probe
-> smoke again
```

It does not claim production PostgreSQL HA.

## Topology

Write DSN:

```text
postgres://nexusim:nexusim@127.0.0.1:15432/nexusim?sslmode=disable
```

Runtime topology:

```text
conversation-service grpc
message-service grpc + outbox-relay
delivery-service grpc + timeline-consumer + outbox-relay
push-gateway ws + delivery-consumer
Kafka localhost:9092
Redis localhost:6379
PostgreSQL HA via pgpool localhost:15432
```

## Command

```powershell
.\tools\local-postgres-failover-smoke.ps1 `
  -RunName postgres-failover-smoke-20260614f `
  -SkipBuild
```

## Evidence

Main summary:

```text
H:\NexusIM\loadtest-results\postgres-failover-smoke-20260614f\postgres-failover-summary.json
```

Before smoke summary:

```text
H:\NexusIM\loadtest-results\postgres-failover-smoke-20260614f-before\pushgateway-summary.json
```

After smoke summary:

```text
H:\NexusIM\loadtest-results\postgres-failover-smoke-20260614f-after\pushgateway-summary.json
```

Primary switch:

```text
before_primary = postgres-ha-0
stopped_container = nexusim-postgres-ha-0
after_primary = postgres-ha-1
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
the local pgpool writer endpoint stayed usable across a primary switch,
and the NexusIM distributed smoke chain still completed after failover
without changing service DSNs.
```

The smoke also proved the script-side stabilization gate mattered:

```text
new primary visible
+ pgpool SELECT 1
+ repeated write probe success
-> then start the post-failover smoke
```

Without that gate, the services could start inside the failover window and produce flaky false negatives.

## Limits

This run does not prove:

- production PostgreSQL HA
- split-brain resistance
- quorum behavior
- automatic failback policy
- in-flight transaction continuity during the exact failover window
- cross-machine PostgreSQL HA
- Patroni / etcd or managed cloud failover semantics

It is a first-stage local failover smoke only.
