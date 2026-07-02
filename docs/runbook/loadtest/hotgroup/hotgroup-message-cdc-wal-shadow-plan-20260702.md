# Hotgroup Message CDC/WAL Shadow Plan - 2026-07-02

## Scope

This module implements the first DB-first CDC/WAL path for message timeline
events. PostgreSQL remains the message fact source. The request path commits
`message_log` and `conversation_timeline_events`; Debezium reads committed WAL
rows and `message-service cdc-bridge` converts them to the existing
`ConversationTimelineEvent` protobuf.

This is implementation, operator wiring and one local CDC runtime smoke. It has
not yet been used as a formal hotgroup pressure-test result.

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
- The connector must set `message.key.columns` to
  `public.conversation_timeline_events:tenant_id,conversation_id`. Otherwise
  Debezium can partition rows for the same conversation by the table primary key
  and the CDC bridge can publish seq out of order.
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
message_outbox rows do not increase for message-service message events
```

Conversation-service currently also writes membership boundary events into the
same `message_outbox` table. A hotgroup setup can therefore still create
`conversation.member.*` outbox rows even when message-service runs in
`cdc_only`. Removing those rows requires a coordinated conversation-service
timeline CDC cutover and is not part of this message-service-only experiment.

## Risks

- CDC bridge is at-least-once. A publish can succeed and source offset commit can
  fail, so downstream consumers must keep `event_id` idempotency.
- Source partitioning is part of correctness, not just throughput tuning. The
  Debezium source topic must use conversation-level keys so every event for the
  same `(tenant_id, conversation_id)` is emitted in per-conversation order.
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

All listed checks passed.

## Local CDC Runtime Smoke

Executed after commit `5c54fc05` with follow-up fixes in the working tree:

```powershell
docker pull quay.io/debezium/connect:3.3
.\tools\build-service-docker-images.ps1 -Services message-service -Platform linux/amd64
docker compose --profile cdc -f deploy/local/docker-compose.yml -f deploy/local/docker-compose.services.yml -f deploy/local/docker-compose.service-workers.yml up -d postgres kafka schema-registry kafka-connect message-service-cdc-bridge
.\tools\ensure-message-cdc-kafka-topics.ps1
.\tools\register-message-timeline-cdc-connector.ps1
.\tools\check-message-cdc-runtime.ps1
go run ./tools/message-cdc-shadow-check -tenant-id tenant-cdc-smoke-20260703003533 -conversation-id conv-cdc-smoke-20260703003533 -created-after 2026-07-02T16:35:00Z -timeout 20s -include-rows
```

Result:

```text
connector = RUNNING
task = RUNNING
postgres wal_level = logical
replication slot nexusim_message_timeline_slot active = true
conversation.timeline.events.cdc total_end_offset = 1
shadow-check expected_count = 1
shadow-check observed_count = 1
missing / unexpected / duplicate / out_of_order = empty
```

Issues found and fixed during smoke:

- Debezium's JSON output can include a `schema/payload` wrapper when worker
  converter settings are not applied. The bridge now supports both wrapped and
  schemaless envelopes, and the local connector config explicitly sets JSON
  converter schemas disabled.
- The connector registration script now waits until connector and tasks are
  `RUNNING`, not merely until `/status` exists.
- The runtime health script now uses `psql -c` instead of piping SQL through
  Windows PowerShell stdin, avoiding a BOM before `SELECT`.
- The shadow checker now reads each Kafka partition only up to its current end
  offset, so empty lower-numbered partitions cannot consume the whole timeout.

## Local Hotgroup CDC Smoke

After commit `ebdc45be`, the minimal local stack was expanded to
conversation-service, timeline-service, policy-service, message-service,
delivery-service, push-gateway, Redis, Kafka, Kafka Connect and the CDC bridge.
The first `cdc_shadow` hotgroup run exposed a correctness issue:

```text
run = hotgroup-cdc-shadow-smoke-20260703-005102
business result = success
shadow expected_count = 81
shadow observed_count = 81
shadow out_of_order = non-empty
```

Root cause: the Debezium source topic had 3 partitions and used the default
table key. Rows for one conversation were spread across source partitions, then
the bridge re-published them into the same target key out of seq order.

Fix: set connector `message.key.columns` to
`public.conversation_timeline_events:tenant_id,conversation_id` and re-register
the connector.

Passing `cdc_shadow` run after the key fix:

```text
run = hotgroup-cdc-shadow-keyed-smoke-20260703-005318
tenant = tenant-hotgroup-20260703-005319
conversation = conv-hotgroup-20260703-005319
business result = success
fanout_mode = WRITE_FANOUT
SendMessage = 20/20
send_p95_ms = 12.38
send_p99_ms = 16.317
message_outbox_pending = 0
delivery_outbox_pending = 0
delivery_timeline_rows = 20
shadow expected_count = 81
shadow observed_count = 81
missing / unexpected / duplicate / out_of_order = empty
message-cdc-bridge lag = 0
```

Passing `cdc_only` run:

```text
run = hotgroup-cdc-only-smoke-20260703-005525
tenant = tenant-hotgroup-20260703-005526
conversation = conv-hotgroup-20260703-005526
message-service export mode = cdc_only
delivery timeline topic = conversation.timeline.events.cdc
delivery consumer group = nexusim-delivery-service-cdc-local
message-service-outbox-relay = stopped
business result = success
fanout_mode = WRITE_FANOUT
SendMessage = 20/20
send_p95_ms = 18.001
send_p99_ms = 40.943
delivery_outbox_pending = 0
delivery_timeline_rows = 20
user_inbox_rows = 1220
shadow expected_count = 81
shadow observed_count = 81
missing / unexpected / duplicate / out_of_order = empty
delivery CDC consumer lag = 0
message-cdc-bridge lag = 0
```

The `cdc_only` run still had 61 `message_outbox` rows, all
`conversation.member.joined.v1` from conversation-service setup. There were no
`message.persisted.v1` rows in `message_outbox` for the 20 SendMessage calls.

Next step: run `cdc_shadow` and then `cdc_only` under a send-only hotgroup load
with an already prepared group, so the measurement isolates per-message
`message_outbox` write amplification instead of setup-time membership events.
