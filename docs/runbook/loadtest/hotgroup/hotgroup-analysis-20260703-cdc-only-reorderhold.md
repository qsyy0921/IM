# Hotgroup CDC-Only Reorder Hold Analysis - 2026-07-03

## Scope

This report records the first remote `cdc_only` pressure validation for the
message timeline DB-first CDC/WAL path. PostgreSQL remains the message fact
source; `message-service` writes `message_log` and
`conversation_timeline_events`, Debezium reads WAL, and
`message-service cdc-bridge` publishes `ConversationTimelineEvent` records to
`conversation.timeline.events.cdc`.

The goal is to remove per-message `message_outbox` write amplification from the
SendMessage hot path while preserving delivery projection correctness.

## Runtime

| Field | Value |
| --- | --- |
| Branch | `codex/backend-lab` |
| Final code commit | `aad98064 Tune CDC bridge reorder hold` |
| Prior bridge implementation commit | `5d8dc543 Add CDC bridge batch reorder` |
| Message export mode | `NEXUSIM_MESSAGE_EVENT_EXPORT_MODE=cdc_only` |
| Delivery topic | `NEXUSIM_TIMELINE_TOPIC=conversation.timeline.events.cdc` |
| Delivery consumer group | `nexusim-delivery-service-cdc-local` |
| Message outbox relay | stopped |
| CDC connector | `nexusim-message-timeline-cdc`, connector/task `RUNNING` |
| CDC bridge reorder | `reorder_flush_delay=3s`, `reorder_max_records=10000` |

## Smoke

Before the formal run, a small remote `cdc_only` smoke confirmed that new WAL
events still flowed after the Ubuntu runtime restart:

```text
run = hotgroup-cdcreorder-smoke-61x20-16c-5d8dc543-20260703-0222
tenant = tenant-cdc-remote-20260703-012535
conversation = conv-cdc-remote-20260703-012535
SendMessage = 20/20
send_p95_ms = 69.49
send_p99_ms = 73.546
delivery_timeline_rows = 440
message_outbox_pending = 0
delivery_outbox_pending = 0
message-cdc-bridge lag = 0
delivery CDC consumer lag = 0
shadow-check expected_count = 20
shadow-check observed_count = 20
missing / unexpected / duplicate / out_of_order = empty
```

## Failed 200ms Reorder Gate

Commit `5d8dc543` added batch sorting by `tenant:conversation` and
`aggregate_version`, but the initial default hold was only `200ms`. The first
6000-member dual-client run proved that this was not enough:

```text
run = hotgroup-cdcreorder-2client-6000x2500x2-384c-5d8dc543-20260703-0225
Windows SendMessage = 2500/2500, p95 = 635.216ms, p99 = 814.206ms
Mac SendMessage = 2500/2500, p95 = 647.998ms, p99 = 834.082ms
message_log_count = 12500
delivery_timeline_rows = 12500
user_inbox_rows = 0
message_outbox_pending = 0
delivery_outbox_pending = 0
CDC source lag = 0
CDC delivery lag = 0
shadow-check expected_count = 5000
shadow-check observed_count = 5000
shadow-check out_of_order = 2 records
```

Interpretation: all events were present and delivery caught up, but high
concurrency can commit a higher allocated `conversation_seq` before a lower seq.
If the bridge flushes after a 200ms quiet gap, the lower seq can arrive in the
next bridge batch and appear out of order on the target Kafka topic.

## Passing 3s Reorder Hold Run

The bridge default hold was then changed to 3 seconds in commit `aad98064`.
The same prepared 6000-member conversation was retested with Windows and Mac
sending concurrently:

```text
run = hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230
tenant = tenant-hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912-win
conversation = conv-hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912-win
group_size = 6000
fanout_mode = READ_FANOUT
conversation_mode = SEQUENCER_BLOCK
Windows SendMessage = 2500/2500, p95 = 638.253ms, p99 = 817.526ms
Mac SendMessage = 2500/2500, p95 = 679.259ms, p99 = 940.766ms
message_log_count = 17500
delivery_timeline_rows = 17500
user_inbox_rows = 0
message_outbox_pending = 0
delivery_outbox_pending = 0
```

CDC / Kafka checks:

```text
message-cdc-bridge source offsets:
  partition 0 = 15000 / 15000, lag 0
  partition 1 = 501 / 501, lag 0
delivery CDC consumer offsets:
  partition 0 = 15000 / 15000, lag 0
  partition 1 = 501 / 501, lag 0
shadow-check expected_count = 5000
shadow-check observed_count = 5000
shadow-check scanned_observed_count = 15000
shadow-check ignored_observed_count = 10000
missing / unexpected / duplicate / out_of_order = empty
```

Artifacts:

```text
Windows summary:
H:\NexusIM\loadtest-results\hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230-win\hotgroup-summary.json

Mac summary copy:
H:\NexusIM\loadtest-results\hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230-mac\hotgroup-summary.json

Process/container resource window:
H:\NexusIM\loadtest-results\hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230\lab-process-resource\lab-process-resource-summary.md

Prometheus raw JSON:
H:\NexusIM\loadtest-results\hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230-win\hotgroup-prometheus-window.json

Prometheus low-sensitive report:
docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230-win.md
```

## Resource Window

The process/container sampler started before pressure and stopped 30 seconds
after both runners completed. It only counted pressure-related runner processes
and Ubuntu service containers, not whole-machine utilization.

Key rows:

```text
ubuntu nexusim-postgres avg_cpu = 141.164%, max_cpu = 2687.65%
ubuntu nexusim-message-service-grpc avg_cpu = 26.071%, max_cpu = 987.36%
ubuntu nexusim-kafka avg_cpu = 27.469%, max_cpu = 282.42%
ubuntu nexusim-message-service-cdc-bridge avg_cpu = 2.239%, max_cpu = 8.46%
ubuntu nexusim-delivery-service-timeline-consumer avg_cpu = 5.917%, max_cpu = 21.36%
mac hotgroup-loadtest avg_cpu = 0.907%, max_cpu = 27.2%
windows hotgroup-loadtest.exe avg_cpu = 0.138%, max_cpu = 4%
sample_errors = none
```

This confirms the run did reach the Ubuntu service side. The low client CPU
numbers are expected because both runners spent most of the window waiting on
remote gRPC / database-dependent work rather than burning local CPU.

## Prometheus Window

Selected maxima from the low-sensitive window:

```text
core_targets_up = 8
message_send_p99_recent_ms = 846.446
message_dependency_read_p99_recent_ms = 833.628
message_policy_check_p99_recent_ms = 769.226
message_conversation_context_p99_recent_ms = 138.687
message_repository_append_p99_recent_ms = 173.026
message_repository_pool_acquire_p99_recent_ms = 12.716
message_repository_insert_message_p99_recent_ms = 129.456
message_repository_insert_timeline_p99_recent_ms = 48.336
message_repository_insert_outbox_p99_recent_ms = 0
message_repository_commit_p99_recent_ms = 32.609
message_outbox_process_ready_active_samples_window = 0
message_kafka_publish_call_samples_window = 0
policy_decision_audit_kafka_async_enqueue_p99_ms = 0
policy_decision_audit_kafka_async_publish_p99_ms = 472
policy_evaluator_facts_read_p99_ms = 87
policy_grpc_check_max_ms = 1031
policy_pg_pool_empty_acquire_total max = 5207
```

Interpretation:

- The message table outbox path is structurally out of the per-message hot path:
  `repository_insert_outbox_p99_recent_ms = 0`, and the message outbox relay
  samples are 0.
- Delivery projection from CDC is correct for this finite 5000-message burst:
  no pending outbox, no CDC lag and no shadow out-of-order.
- The remaining SendMessage long tail is back in dependency / policy waiting
  and the primary PostgreSQL write path. This matches the earlier bottleneck
  curve: we removed one write-amplification leg, but the request still performs
  conversation dependency read, policy check, sequencer work and PG append.

## Correctness Notes

The 3-second bridge hold is a bounded reorder window, not a full durable reorder
log. It is enough for the observed 6000-member / 5000-message dual-client burst,
but it is not a proof of unbounded continuous-stream ordering. A production
solution needs one of these explicit contracts:

1. Keep Kafka target order strict by adding durable per-conversation bridge
   reorder state and a safe watermark.
2. Treat CDC Kafka order as commit order, and require every downstream consumer
   to be order-insensitive and sort / idempotently apply by `conversation_seq`.
3. Keep the current bounded hold as a local loadtest profile only, with a clear
   max skew assumption.

Do not describe this as a completed production CDC cutover until that contract
is decided.

## Next Bottleneck

The next bottleneck for OpenIM-like large groups is not the removed
`message_outbox` write amplification anymore. It is now:

```text
SendMessage dependency read / policy check
-> policy-service PG pool and facts read under burst
-> primary PG message_log + timeline append
-> residual repository insert / commit latency
```

The next backend step should not be another table-outbox optimization. It should
compare:

- policy decision revision cache hit path for unchanged conversation / sender;
- conversation dependency context cache with explicit revision invalidation;
- message table partition / index profile for append-heavy conversations;
- larger prepared-group runs that keep the same `cdc_only` path but vary sender
  concurrency and message count.

