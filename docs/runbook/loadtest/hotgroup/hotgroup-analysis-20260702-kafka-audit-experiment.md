# Hotgroup Kafka Policy Audit Experiment - 2026-07-02

## Scope

- Backend Lab branch: `codex/backend-lab`
- Commit under test: `19771df006ea3094bfa3d3b93c5a83c13a107c16`
- Runtime profile: `policy_decision_audit_kafka_direct`
- Policy-service Docker image archive:
  `H:\NexusIM\docker-images\archives\nexusim-policy-service-19771df0-kafka-audit-20260702-213004.tar`
- Batch directory:
  `H:\NexusIM\loadtest-results\hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651`

## Code / Deploy Validation

- Focused tests before deploy:
  - `go test ./services/policy-service/internal/app ./services/policy-service/internal/infrastructure/kafka ./services/policy-service/cmd/policy-service -count=1`
  - `go test ./services/policy-service/internal/trigger/outbox ./services/policy-service/internal/infrastructure/postgres -run "DecisionAudit|PolicyEvent|Outbox" -count=1`
  - `go build ./services/policy-service/cmd/policy-service`
  - `go test ./services/policy-service/... -count=1`
  - `git diff --check`
- Ubuntu redeploy:
  - image loaded as `nexusim/policy-service:local`
  - `NEXUSIM_POLICY_DECISION_AUDIT_SINK=kafka`
  - `NEXUSIM_POLICY_AUDIT_EVENTS_TOPIC=im.policy.events`
  - `NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_BATCH_SIZE=1`
  - `NEXUSIM_POLICY_DECISION_AUDIT_KAFKA_BATCH_TIMEOUT=1ms`
- Smoke:
  - run:
    `H:\NexusIM\loadtest-results\hotgroup-kafka-audit-smoke-19771df0-20260702-213238`
  - 10 / 10 SendMessage success
  - `decision_audit_kafka` count increased by 10, error count 0
  - `policy_decision_audit_outbox` stayed `253524`, pending stayed `0`

## Pressure Run

- Batch:
  `hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651`
- Windows runner:
  `hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651-win`
- Mac runner:
  `hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651-mac`
- Shape:
  - two full runners, Windows + Mac
  - each runner: 6000 group members, 2500 SendMessage, 384 senders, 384 concurrency
  - target: 8000 msg/s per runner
  - fanout mode: `READ_FANOUT`
  - delivery outbox drain required
- Resource sampling:
  - `H:\NexusIM\loadtest-results\hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651\lab-process-resource\lab-process-resource-summary.md`
  - sampler started before pressure and stopped after the runners finished plus 30s

## Results

| Runner | Success | Achieved msg/s | Send p95 ms | Send p99 ms | Pull p95 ms | Message outbox pending | Delivery outbox pending |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Windows | true | 1790.630 | 343.062 | 373.781 | 677.910 | 0 | 0 |
| Mac | true | 1854.436 | 237.801 | 244.973 | 668.311 | 0 | 0 |

For comparison with the nearest previous two-client runs:

| Profile | Windows msg/s | Windows p99 ms | Mac msg/s | Mac p99 ms |
| --- | ---: | ---: | ---: | ---: |
| default main PG audit | 915.777 | 785.565 | 1008.686 | 681.752 |
| independent audit PG | 889.582 | 757.653 | 993.732 | 707.529 |
| direct synchronous Kafka audit | 1790.630 | 373.781 | 1854.436 | 244.973 |

## Prometheus / Resource Evidence

- Prometheus window:
  `H:\NexusIM\loadtest-results\hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651\hotgroup-prometheus-window.json`
- Low-sensitive metrics markdown:
  `H:\NexusIM\loadtest-results\hotgroup-kafkaaudit-clean-2client-6000x2500x2-384c-19771df0-20260702-213651\hotgroup-prometheus-window.md`
- `policy_decision_audit_outbox` count stayed `253524 -> 253524`, pending `0 -> 0`.
- Kafka audit stages:
  - `decision_audit_kafka` total: `5010`
  - `decision_audit_kafka` errors: `0`
  - `decision_audit_kafka p99`: `223ms`
  - `decision_audit_kafka_publish p99`: `222ms`
  - build / marshal p99: `0ms`
- Policy facts read is no longer the bottleneck:
  - `policy_evaluator_facts_read_p99_ms`: `11ms`
- Message write path improved versus independent audit PG:
  - `message_repository_append_recent p99`: last `32.058ms`, max `143.529ms`
  - `message_repository_pool_acquire_recent p99`: last `15.342ms`
- Request-stage pressure is now dominated by policy check / Kafka publish:
  - `message_dependency_read_recent p99`: last `236.334ms`
  - `message_policy_check_recent p99`: last `227.421ms`
- Resource window:
  - PostgreSQL avg / max CPU: `144.418% / 438.7%`
  - Kafka avg / max CPU: `45.846% / 337.51%`
  - policy-service avg / max CPU: `1.877% / 52.93%`
  - Windows runner avg / max CPU: `1.968% / 12%`
  - Mac runner avg / max CPU: `1.984% / 8%`
- Ubuntu NVMe iostat:
  - max `%util`: `42.8`
  - max `w_await`: `7.36ms`
  - max `aqu-sz`: `12.22`

## Conclusion

The experiment proves that Kafka can remove the synchronous PG audit outbox
write from the SendMessage path: the PG audit outbox row count did not change,
and all new SEND decisions went through Kafka with zero publish errors.

However, this direct implementation still waits for Kafka ACK inside
`CheckMessageAction`. That moved the hot wait from `decision_audit_pool_acquire`
/ `decision_audit_insert_exec` to `decision_audit_kafka_publish`. In this run,
Kafka publish p99 was about `222ms`, almost exactly matching the remaining
`message_policy_check_recent p99` of about `227ms`.

So Kafka is useful, but the correct production direction is not "replace sync PG
audit with sync Kafka audit and stop". The next optimization should implement a
reliable asynchronous audit boundary:

- keep the request path fail-closed only for permission decision correctness;
- enqueue the audit event to a bounded durable handoff path outside the
  permission read / decision critical section;
- use Kafka idempotent producer semantics or an event idempotency key at the
  downstream sink;
- keep explicit delivery / DLQ / redrive metrics so audit reliability remains
  observable.

This run should be treated as a Kafka feasibility and bottleneck migration
experiment, not the final audit architecture.

## Risks / Follow-Up

- Current code uses `segmentio/kafka-go`; it supports required ACKs but does not
  expose Kafka idempotent producer semantics. Downstream consumers must use
  `event_id` idempotency until the producer layer is upgraded or wrapped.
- The Kafka path is still synchronous and therefore still adds Kafka broker /
  network / ACK latency directly to SendMessage p99.
- PullInbox p95 in this run is high even though message / delivery outbox pending
  are zero. That should be checked separately before using this run as a read
  path capacity signal.
- Next code experiment: reliable async Kafka audit with bounded local queue plus
  explicit fail / retry / DLQ metrics, followed by the same two-client run.
