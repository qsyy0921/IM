# Hotgroup Kafka Async Policy Audit Experiment - 2026-07-02

## Scope

Backend Lab implemented and pressure-tested `NEXUSIM_POLICY_DECISION_AUDIT_SINK=kafka_async` for policy decision audit. This profile removes Kafka ACK wait from the `CheckMessageAction(SEND)` request path by enqueueing a low-sensitive `PolicyEvent` into a bounded in-process queue, then publishing batches to `im.policy.events` from background workers. Main topic publish retries are bounded; exhausted batches are retried to `im.policy.events.dlq`.

This is a pressure profile, not a production-grade crash-durable audit design. It is process-reliable while the policy-service instance is alive, but an instance crash between enqueue and Kafka ACK can lose queued audit events. Production reliable async audit still needs a durable spool, transactional outbox / CDC relay, or an idempotent external sink with replay and retention controls.

## Code and Verification

- Branch: `codex/backend-lab`
- Commit: `9b87f95d` (`feat: add async kafka policy audit sink`)
- Push target: `origin/codex/backend-lab`
- Key files:
  - `services/policy-service/internal/infrastructure/kafka/decision_auditor_async.go`
  - `services/policy-service/internal/infrastructure/kafka/decision_auditor_async_test.go`
  - `services/policy-service/cmd/policy-service/decision_audit_config.go`
  - `deploy/local/docker-compose.services.yml`
  - `tools/record-hotgroup-metrics-window.ps1`
  - `docs/sdd/policy-service.md`

Focused checks:

```text
go test ./services/policy-service/internal/infrastructure/kafka -count=1
go test ./services/policy-service/cmd/policy-service -run DecisionAuditor -count=1
go test ./services/policy-service/internal/infrastructure/kafka ./services/policy-service/cmd/policy-service -count=1
go test ./services/policy-service/... -count=1
go build ./services/policy-service/cmd/policy-service
git diff --check
git diff --cached --check
```

All passed. `go test -race ./services/policy-service/internal/infrastructure/kafka -count=1` was attempted but could not run because the local Go environment has `CGO_ENABLED=0`; the async test observer was still made mutex-protected so it is safe for future race runs.

## Deployment

- Docker image: `nexusim/policy-service:local`
- Archive: `H:\NexusIM\docker-images\archives\nexusim-policy-service-9b87f95d-kafka-async-audit-20260702-220909.tar`
- Ubuntu runtime compose: `/home/qsyy0921/IM/deploy/local/docker-compose.services.yml`
- Runtime env confirmed:
  - `NEXUSIM_POLICY_DECISION_AUDIT_SINK=kafka_async`
  - `NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC=im.policy.events`
  - `NEXUSIM_POLICY_AUDIT_EVENTS_DLQ_TOPIC=im.policy.events.dlq`
  - `NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_QUEUE_SIZE=8192`
  - `NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_BATCH_SIZE=100`
  - `NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_FLUSH_INTERVAL=10ms`
  - `NEXUSIM_POLICY_DECISION_AUDIT_ASYNC_MAX_ATTEMPTS=5`

Startup log confirmed:

```text
policy-service decision audit async kafka sink enabled topic=im.policy.events dlq_topic=im.policy.events.dlq batch_size=100
```

Kafka topics confirmed:

```text
im.policy.events
im.policy.events.dlq
```

## Smoke

Run:

```text
H:\NexusIM\loadtest-results\hotgroup-kafkaasync-smoke-9b87f95d-20260702-221140
```

Result:

- 10 / 10 `SendMessage` succeeded.
- PG audit outbox stayed `253524,total / 0 pending / 0 DLQ`.
- Kafka `im.policy.events` increased by 10 records.
- Kafka DLQ stayed 0.
- Metrics showed `decision_audit_kafka_async`, `decision_audit_kafka_async_enqueue`, `decision_audit_kafka_async_publish`; all async audit errors were 0.

## Formal Run

The first orchestration attempt `hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221417` is excluded: Mac zsh expanded the unquoted `?sslmode=disable` DSN and the Mac runner did not start. That excluded attempt produced a valid Windows-only 2500-message diagnostic, but it is not used as the dual-client result.

Effective run:

```text
H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912
H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912-win
H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912-mac
```

Shape:

- Windows + Mac runners.
- Each runner: 6000 members, 2500 messages, 384 senders / 384 concurrency, target 8000 msg/s.
- `READ_FANOUT + SEQUENCER_BLOCK`.
- Resource sampling started before pressure and stopped 30s after both runners finished.
- Resource scope only counted pressure-related runner processes and Ubuntu service / PostgreSQL / Kafka / Redis containers.

Runner summary:

| Runner | Success | Send errors | Actual send rate | Send p95 | Send p99 | Message outbox | Delivery outbox |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Windows | 2500 / 2500 | 0 | 2245.972 msg/s | 297.090 ms | 386.652 ms | 0 pending | 0 pending |
| Mac | 2500 / 2500 | 0 | 2495.839 msg/s | 242.963 ms | 324.734 ms | 0 pending | 0 pending |

The two send windows did not perfectly overlap because each full runner performs local setup before sending; keep using per-runner rates for this comparable experiment instead of treating the wall-clock combined rate as a service capacity number.

Audit correctness:

| Check | Before | After |
| --- | ---: | ---: |
| PG audit outbox total / pending / DLQ | `253524,0,0` | `253524,0,0` |
| Kafka `im.policy.events` offsets | `65981,94622,110441` | `68481,97122,110441` |
| Kafka `im.policy.events.dlq` offsets | `0,0,0` | `0,0,0` |

Kafka main topic increased by exactly 5000 records; DLQ stayed 0.

## Metrics

Prometheus reports:

- Default 5-minute padding window:
  `H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912\hotgroup-prometheus-window.md`
- Cleaner 1-minute padding window used for this conclusion:
  `H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912\hotgroup-prometheus-window-1m.md`

Key 1-minute-padding metrics:

| Metric | Max | Last |
| --- | ---: | ---: |
| `message_send_p99_recent_ms` | 634.187 | 297.000 |
| `message_dependency_read_p99_recent_ms` | 304.576 | 272.944 |
| `message_policy_check_p99_recent_ms` | 241.992 | 241.992 |
| `message_repository_append_call_p99_recent_ms` | 345.306 | 150.602 |
| `message_repository_pool_acquire_p99_recent_ms` | 206.260 | 42.086 |
| `message_repository_insert_outbox_p99_recent_ms` | 115.390 | 88.464 |
| `policy_evaluator_facts_read_p99_ms` | 20 | 20 |
| `policy_decision_audit_kafka_async_p99_ms` | 80 | 80 |
| `policy_decision_audit_kafka_async_enqueue_p99_ms` | 0 | 0 |
| `policy_decision_audit_kafka_async_publish_p99_ms` | 300 | 300 |
| `policy_decision_audit_kafka_async_errors_5m` | 0 | 0 |
| `policy_audit_pg_pool_acquire_total` | 0 | 0 |
| `policy_audit_outbox_pending` | 0 | 0 |

Interpretation:

- The request path no longer waits on synchronous PG audit insert or synchronous Kafka ACK.
- `decision_audit_kafka_async_enqueue` p99 is 0ms and error count is 0.
- Background Kafka publish still has p99 up to 300ms, but that cost has moved off the policy decision request path.
- Policy facts read is no longer the largest visible p99 in this run (`policy_evaluator_facts_read_p99_ms` max 20ms).
- The remaining end-to-end long tail is still dominated by message-service dependency/read and write path:
  - `message_dependency_read_p99_recent_ms` max about 305ms.
  - `message_policy_check_p99_recent_ms` max about 242ms.
  - `message_repository_append_call_p99_recent_ms` max about 345ms.
  - `message_repository_pool_acquire_p99_recent_ms` max about 206ms.

## Resource Window

Resource report:

```text
H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912\lab-process-resource-summary.md
```

Selected pressure-related resource samples:

| Machine | Process / container | CPU avg | CPU max | Memory max |
| --- | --- | ---: | ---: | ---: |
| Ubuntu | `nexusim-postgres` | 205.571% | 2672.5% | 2021.376 MB |
| Ubuntu | `nexusim-message-service-grpc` | 32.449% | 833.64% | 116.7 MB |
| Ubuntu | `nexusim-policy-service-grpc` | 9.020% | 184.22% | 44.31 MB |
| Ubuntu | `nexusim-kafka` | 47.888% | 339.66% | 1545.216 MB |
| Mac | `hotgroup-loadtest` | 2.294% | 8.6% | 39.813 MB |
| Windows | `hotgroup-loadtest.exe` | 2.071% | 11% | 40.219 MB |

Disk sample:

```text
H:\NexusIM\loadtest-results\hotgroup-kafkaasync-clean-2client-6000x2500x2-384c-9b87f95d-20260702-221912\ubuntu-iostat.txt
```

`nvme0n1` summary from the captured window:

| Device | Samples | Max util | Avg util | Max write await | Avg write await | Max write KiB/s | Avg write KiB/s |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `nvme0n1` | 227 | 26.4% | 7.158% | 12.05 ms | 0.386 ms | 61320 | 5159.134 |

This does not look like a saturated SSD. The current pressure is better explained by PostgreSQL / message-service wait paths than by raw disk utilization.

## Conclusion

`kafka_async` successfully removes policy decision audit PostgreSQL writes from the hot path and avoids synchronous Kafka ACK wait on the request path. The audit correctness checks held: PG audit outbox did not grow, Kafka received exactly the expected 5000 audit records, DLQ stayed 0, and async audit stage errors stayed 0.

The bottleneck did not disappear; it moved back to the main SendMessage pipeline. Current p99 is still shaped by policy RPC / dependency wait plus message repository append / PG pool acquire. In this run, policy facts read itself is relatively small, and policy audit PG pool is unused. The next backend optimization should focus on message-service repository append and message PG pool behavior, especially `message_outbox` insert / index / WAL pressure and transaction concurrency, while keeping the async audit profile as the baseline for removing audit noise from the path.

## Risks and Next Steps

- `kafka_async` is not crash-durable. It is valid for pressure isolation, but production-grade reliable async audit needs a durable spool, transactional outbox / CDC, or idempotent sink replay design.
- The two full runners still spend setup time before sending, so the short 2500-message send windows did not perfectly overlap. For later capacity claims, use longer message counts or a runner-level send barrier so Windows and Mac produce sustained overlapping pressure.
- Continue with message-service write-path profiling:
  - split `repository_append` by `message_log`, `conversation_timeline`, `message_outbox`, commit and WAL-sensitive waits;
  - inspect `message_outbox` indexes and insert payload shape;
  - compare PG wait events during a sustained longer run;
  - avoid blindly increasing pools until write-path contention is understood.
