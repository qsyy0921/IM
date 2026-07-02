# Policy Audit Index Experiment

Scope: Backend Lab hotgroup pressure result. This is not a production SLO.

## What Changed

- Code commit `e8b701d`: added `migrations/postgres/policy/000017_policy_audit_outbox_partition_order_partial.sql`.
- The migration replaces the full `idx_policy_decision_audit_outbox_partition_order` btree with a smaller partial index:
  `idx_policy_decision_audit_outbox_partition_order_ready`.
- The new index only covers rows where `status IN ('PENDING', 'DLQ')`.
- Code commit `86e5c30`: fixed process resource sampling so Windows process-name matches and Mac defaults include `hotgroup-loadtest`.

## Correctness Review

- Current policy outbox relay starts `OutboxStore` with `WithOutboxOrderedPartitionPublishing(false)`, so the old full partition-order index is not used by the current relay hot path.
- The optional ordered path only checks blockers with `previous.status IN ('PENDING', 'DLQ')`, so the partial index still supports the ordered blocker query if that path is enabled later.
- The migration does not change policy allow/deny semantics, audit payload content, fail-closed behavior, Redis cache behavior, or service APIs.
- Ubuntu PG verification after applying the migration:
  - before: `policy_decision_audit_outbox` had `238524` rows, all `PUBLISHED`; total/table/index size was about `681MB / 452MB / 228MB`;
  - after: only `idx_policy_decision_audit_outbox_partition_order_ready` remains for partition order; total/table/index size is about `550MB / 452MB / 97MB`.

## Runtime Setup

- policy-service image: `sha256:726d4407a83cfc37aca0c501a56909294d9336cfa794bb2f57a0794c8d1fb345`.
- message-service image for clean repeat: `sha256:fa850212bdf579e843ecfd15c29ce70de5b67f8a5cbd0c131574023dae7a8d3a`.
- `NEXUSIM_POLICY_PG_MAX_CONNS=32`.
- `NEXUSIM_POLICY_AUDIT_PG_MAX_CONNS=32`.
- `NEXUSIM_POLICY_DECISION_CACHE_BACKEND=disabled`.
- policy-service and message-service were force-recreated before the clean repeat to reset recent latency and pool counter history.

## Runs

All runs are two-client READ_FANOUT send-only pressure:

- Windows runner: 6000 group members, 2500 messages, 384 send concurrency, target 8000 msg/s.
- Mac runner: same as Windows, separate tenant and conversation.
- Each run requires delivery outbox drain.
- Resource sampler starts before pressure and stops about 30 seconds after the last runner finishes.
- Resource scope is pressure-related local runner processes and Ubuntu service containers only.

| run | role | achieved msg/s | send p99 ms | pull p95 ms | status |
| --- | --- | ---: | ---: | ---: | --- |
| baseline32 `hotgroup-splitmetrics...182542` | Windows | 1561.609 | 677.368 | 39.131 | success |
| baseline32 `hotgroup-splitmetrics...182542` | Mac | 2223.475 | 257.043 | 35.376 | success |
| audit64 `hotgroup-audit64...191754` | Windows | 1548.04 | 773.262 | 609.339 | success |
| audit64 `hotgroup-audit64...191754` | Mac | 2175.682 | 258.652 | 578.649 | success |
| partial-index diagnostic `hotgroup-auditidxpartial...193245` | Windows | 1823.912 | 370.034 | 703.668 | success, Windows resource row missing |
| partial-index diagnostic `hotgroup-auditidxpartial...193245` | Mac | 2073.61 | 278.453 | 693.437 | success |
| partial-index clean `hotgroup-auditidxpartial-clean...194249` | Windows | 1482.818 | 730.397 | 689.828 | success |
| partial-index clean `hotgroup-auditidxpartial-clean...194249` | Mac | 2131.325 | 235.026 | 704.343 | success |

## Key Metrics

| run | message pool avg wait ms/acquire | message empty acquires | audit pool avg wait ms/acquire | audit empty acquires | audit p99 ms | audit acquire p99 ms | audit insert p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| baseline32 | 9.222 | 587 | 52.56 | 4592 | 126 | 104 | 40 |
| audit64 | 19.178 | 1887 | 20.715 | 3484 | 115 | 66 | 66 |
| partial-index diagnostic | 0.498 | 266 | 66.932 | 4748 | 136 | 118 | 40 |
| partial-index clean | 10.847 | 751 | 58.78 | 4662 | 133 | 111 | 48 |

Clean repeat process-resource highlights:

- Ubuntu PostgreSQL avg `267.766%`, max `4003.22%`.
- Ubuntu message-service avg `14.319%`, max `455.02%`.
- Ubuntu policy-service avg `5.58%`, max `168.02%`.
- Ubuntu Kafka avg `33.003%`, max `318.01%`.
- Windows runner avg `1%`, max `8%`.
- Mac runner avg `2.358%`, max `11.7%`.

During the clean repeat, `pg_stat_activity` showed active PostgreSQL wait events including `BufferContent`, `WALWrite`, and `WALInsert`. That means pressure is entering PostgreSQL write/WAL contention, not just sitting idle on the clients.

## Conclusion

The index change is correct and useful as index hygiene: it removes a large unused full btree from the synchronous audit insert path while preserving the optional ordered blocker query. It reduced audit outbox index size by about `131MB`.

It is not a complete bottleneck fix. The clean repeat did not show stable end-to-end p99 improvement, and the policy audit pool still queued heavily:

- baseline audit avg wait was `52.56ms/acquire`;
- partial-index clean audit avg wait was `58.78ms/acquire`;
- audit empty acquire count stayed above `4600` for `5000` audit inserts.

The current bottleneck is still synchronous policy decision audit writes plus shared PostgreSQL write/WAL contention. In simple terms: every `SendMessage` still waits for policy-service to durably insert an audit outbox row. Under high concurrency, thousands of requests line up for the audit PG pool and then compete with message/delivery writes in the same PostgreSQL write-ahead log path.

## Next Step

Do not keep increasing PG pool size blindly. The better next experiments are:

1. Isolate policy audit writes to a separate PostgreSQL instance using `NEXUSIM_POLICY_AUDIT_PG_DSN`, while keeping fail-closed synchronous audit durability.
2. Partition `policy_decision_audit_outbox` by time or tenant hash if audit must stay in the same PostgreSQL instance.
3. Minimize audit payload/indexes further, but treat this as secondary because current insert exec p99 is much smaller than pool acquire p99.
4. Only if product semantics allow it, move audit out of the request success path into a reliable async path. This needs explicit design approval because current behavior fails closed when audit cannot be written.

Redis does not address this bottleneck. Redis can help policy decision reads only when cache keys are revision-bound; it cannot remove the durable synchronous audit write.
