# Contacts Stack Capacity Baseline

## Scope

This report records a local short capacity baseline for `contacts-service`. It proves that the stack runner can exercise:

```text
contacts-service gRPC
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> Kafka readback in loadtest/contacts
```

This is a local development / interview evidence run. It is not a production SLO, HA proof, or sizing claim.

## Environment

| Item | Value |
| --- | --- |
| Commit | `8a1faa40` |
| Raw result root | `H:\NexusIM\loadtest-results\capacity-baseline-contacts-stack-20260616-r2` |
| Service target | `127.0.0.1:10500` |
| Kafka brokers | `localhost:9092` |
| Kafka topic | `im.contact.events` |
| PostgreSQL DSN | `postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable` |
| Runner | `.\tools\run-loadtest-capacity-baseline-suite.ps1 -Services contacts-service -IncludeStackRunners` |

The first attempt failed because the default `im.contact.events` topic did not exist while Kafka auto topic creation is disabled. The topic was created explicitly before the passing run:

```powershell
docker exec nexusim-kafka kafka-topics `
  --bootstrap-server localhost:9092 `
  --create `
  --if-not-exists `
  --topic im.contact.events `
  --partitions 1 `
  --replication-factor 1
```

## Result

| Metric | Value |
| --- | ---: |
| Suite status | `passed` |
| Scenario | `accept` |
| Operation count | `9` |
| Contact event count | `2` |
| `contacts_outbox_total` | `2` |
| `contacts_outbox_pending` | `0` |
| `contacts_outbox_dlq` | `0` |
| `operations_per_second` | `0.8913272962745193` |
| `events_per_second` | `0.19807273250544874` |
| `latency_p95_ms` | `27.93` |
| `latency_p99_ms` | `27.93` |

Kafka readback returned:

```text
contact.request.created.v1
contact.request.accepted.v1
```

Both events used the same contact request aggregate and versions `1 -> 2`.

## Limits

- This is a single short local run, not a long-duration capacity curve.
- It does not validate Redis / Kafka / PostgreSQL HA behavior.
- It does not cover all contact scenarios; only the `accept` path is included.
- `im.contact.events` must exist before using the default contacts capacity stack runner.
