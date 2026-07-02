# Hotgroup Message CDC/WAL Shadow Plan - 2026-07-02

## Scope

This module implements the first DB-first CDC/WAL path for message timeline
events. PostgreSQL remains the message fact source. The request path commits
`message_log` and `conversation_timeline_events`; Debezium reads committed WAL
rows and `message-service cdc-bridge` converts them to the existing
`ConversationTimelineEvent` protobuf.

This is implementation and operator wiring only. It has not yet been used as a
formal hotgroup pressure-test result.

## Why This Exists

The current SendMessage write path still pays write amplification for
`message_outbox` insert/update even after policy audit was removed from the
request path. CDC/WAL lets us test a structural alternative:

```text
PG commit: message_log + conversation_timeline_events
-> Debezium reads WAL after commit
-> cdc-bridge publishes Kafka timeline event
-> delivery / push consume Kafka
```

This avoids the unsafe pattern:

```text
Kafka publish succeeds
PG write fails
user sees accepted message but history cannot find it
```

## Implemented Pieces

| Area | Files |
| --- | --- |
| Message export mode | `services/message-service/internal/infrastructure/postgres/repository.go`, mutation repositories, `services/message-service/cmd/message-service/main.go` |
| Timeline CDC envelope columns | `migrations/postgres/message/000007_conversation_timeline_cdc_envelope.sql` |
| CDC bridge runtime | `services/message-service/internal/trigger/cdc/bridge.go` |
| Local Kafka Connect / Debezium profile | `deploy/local/docker-compose.yml`, `deploy/local/debezium/message-timeline-connector.json` |
| Local worker wiring | `deploy/local/docker-compose.service-workers.yml`, `deploy/local/docker-compose.services.yml` |
| Operator scripts | `tools/ensure-message-cdc-kafka-topics.ps1`, `tools/register-message-timeline-cdc-connector.ps1`, `tools/check-message-cdc-runtime.ps1` |
| Shadow compare tool | `tools/message-cdc-shadow-check/main.go` |

## Modes

| Mode | `message_outbox` write | Primary delivery path | Use |
| --- | --- | --- | --- |
| `table_outbox` | yes | `conversation.timeline.events` from outbox relay | Existing compatibility mode. |
| `cdc_shadow` | yes | outbox relay remains primary; CDC bridge writes `conversation.timeline.events.cdc` | Compare CDC output without changing delivery. |
| `cdc_only` | no | `conversation.timeline.events.cdc` from Debezium + cdc-bridge | Explicit cutover experiment after shadow checks pass. |

Local compose now defaults message-service gRPC to `cdc_shadow`. That preserves
the old outbox path while producing a comparable CDC stream.

## Local Runtime Steps

Start the CDC profile together with the normal local stack:

```powershell
docker compose --profile cdc -f deploy/local/docker-compose.yml -f deploy/local/docker-compose.services.yml -f deploy/local/docker-compose.service-workers.yml up -d postgres kafka schema-registry kafka-connect message-service-cdc-bridge
```

Prepare the target topic and register the Debezium connector:

```powershell
.\tools\ensure-message-cdc-kafka-topics.ps1
.\tools\register-message-timeline-cdc-connector.ps1
.\tools\check-message-cdc-runtime.ps1
```

Expected runtime facts:

- PostgreSQL reports `wal_level=logical`.
- Replication slot `nexusim_message_timeline_slot` exists and is active once
  the connector is running.
- Connector `nexusim-message-timeline-cdc` is `RUNNING`.
- Target topic `conversation.timeline.events.cdc` has partitions and offsets
  increasing after new timeline rows are committed.

## Shadow Validation

Keep:

```text
NEXUSIM_MESSAGE_EVENT_EXPORT_MODE=cdc_shadow
NEXUSIM_TIMELINE_TOPIC=conversation.timeline.events
```

Run a small hotgroup or SendMessage smoke, then compare committed timeline rows
with the CDC topic:

```powershell
go run .\tools\message-cdc-shadow-check `
  -tenant-id <tenant> `
  -conversation-id <conversation> `
  -created-after <run-start-rfc3339> `
  -timeout 20s
```

Pass condition:

```text
expected_count == observed_count
missing_event_ids is empty
unexpected_event_ids is empty
duplicate_event_ids is empty
out_of_order is empty
```

The checker reads `conversation_timeline_events` as the expected source and
compares against `conversation.timeline.events.cdc`.

## Consumer Switch Experiment

Only after shadow validation passes:

```text
NEXUSIM_TIMELINE_TOPIC=conversation.timeline.events.cdc
```

Use a fresh consumer group or reset checkpoints for an isolated A/B projection
run. Do not point the production-like delivery consumer at CDC while old
checkpoints from `conversation.timeline.events` are still assumed to represent
the same topic.

## `cdc_only` Experiment

Only after CDC runtime health and shadow compare pass:

```text
NEXUSIM_MESSAGE_EVENT_EXPORT_MODE=cdc_only
NEXUSIM_TIMELINE_TOPIC=conversation.timeline.events.cdc
```

For a clean bottleneck experiment, stop `message-service-outbox-relay` or leave
it idle with no new rows. The expected database effect is:

```text
message_log rows increase
conversation_timeline_events rows increase
message_outbox rows do not increase for message-service events
```

## Risks

- CDC bridge is at-least-once. A publish can succeed and source offset commit can
  fail, so downstream consumers must keep `event_id` idempotency.
- `snapshot.mode=no_data` exports only new rows after connector registration.
  Historical backfill needs a separate operator plan.
- A stopped connector can retain WAL through the replication slot. Monitor
  retained WAL bytes and disk usage before long pressure tests.
- `cdc_only` removes the table outbox safety queue for message timeline events.
  It must not be enabled unless Kafka Connect, Debezium, the bridge and the
  target consumer are healthy.
- This does not remove PostgreSQL as the write bottleneck. It removes
  `message_outbox` write amplification; pressure tests still need to measure
  `message_log` and `conversation_timeline_events` insert / index / WAL costs.

## Focused Checks

Already run before this report:

```powershell
go test ./services/message-service/internal/trigger/cdc -count=1
go test ./services/message-service/internal/infrastructure/postgres -run 'TestMessageRepositoryEventExportModesIntegration|TestMessageRepositoryAppendMessageIntegration' -count=1
go build ./services/message-service/cmd/message-service
go build ./tools/message-cdc-shadow-check
go test ./services/message-service/... -count=1
docker compose -f deploy/local/docker-compose.yml -f deploy/local/docker-compose.services.yml -f deploy/local/docker-compose.service-workers.yml config --quiet
docker compose --profile cdc -f deploy/local/docker-compose.yml -f deploy/local/docker-compose.services.yml -f deploy/local/docker-compose.service-workers.yml config --quiet
git diff --check
```

All listed checks passed. Docker CDC runtime registration and hotgroup pressure
test are the next step after commit / redeploy.
