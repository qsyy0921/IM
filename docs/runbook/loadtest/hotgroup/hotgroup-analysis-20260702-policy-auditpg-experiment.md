# Hot Group Policy Audit PG Experiment - 2026-07-02

Scope: validate whether moving synchronous policy decision audit writes to an
independent PostgreSQL instance removes the current SendMessage bottleneck. This
is a local three-machine pressure result, not a production sizing claim.

## Setup

- Commit under test: `4cd29903`.
- Runtime profile: `policy_audit_pg_dsn_independent_postgres`.
- Main service DB: existing `nexusim-postgres`.
- Audit DB: temporary `nexusim-policy-audit-postgres`, same migration set, only
  used by policy decision audit writes.
- `policy-service-grpc` override:
  - `NEXUSIM_POLICY_AUDIT_PG_DSN=postgres://nexusim:nexusim@policy-audit-postgres:5432/nexusim?sslmode=disable`
  - `NEXUSIM_POLICY_AUDIT_PG_MAX_CONNS=32`
  - `NEXUSIM_POLICY_PG_MAX_CONNS=32`
  - `NEXUSIM_POLICY_DECISION_CACHE_BACKEND=disabled`
- Separate audit relay: `nexusim-policy-service-outbox-relay-auditpg`.

The experiment did not change permission semantics. Policy decisions still write
audit synchronously and fail closed if audit write fails; only the audit write
pool and physical PostgreSQL instance were isolated.

## Run

- Batch:
  `H:\NexusIM\loadtest-results\hotgroup-auditpg-clean-splitmetrics-2client-6000x2500x2-384c-4cd29903-20260702-202608`
- Windows run:
  `hotgroup-auditpg-clean-splitmetrics-2client-6000x2500x2-384c-4cd29903-20260702-202608-win`
- Mac run:
  `hotgroup-auditpg-clean-splitmetrics-2client-6000x2500x2-384c-4cd29903-20260702-202608-mac`
- Parameters per client: 6000 group members, 2500 messages, 384 senders,
  384 send concurrency, target 8000 msg/s, `READ_FANOUT`,
  `--require-delivery-outbox-drain`.
- Resource sampling:
  `H:\NexusIM\loadtest-results\hotgroup-auditpg-clean-splitmetrics-2client-6000x2500x2-384c-4cd29903-20260702-202608\lab-process-resource\lab-process-resource-summary.md`
- Prometheus window:
  `H:\NexusIM\loadtest-results\hotgroup-auditpg-clean-splitmetrics-2client-6000x2500x2-384c-4cd29903-20260702-202608-win\hotgroup-prometheus-window.json`
- Low-sensitive reports:
  - `docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260702-auditpg-clean-splitmetrics-2client-6000x2500x2.md`
  - `docs/runbook/loadtest/hotgroup/hotgroup-analysis-20260702-auditpg-ab.md`

## Correctness Checks

Both runners passed and drained async outboxes:

| runner | success | messages | errors | achieved msg/s | send p95 ms | send p99 ms | message pending | delivery pending |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Windows | true | 2500 | 0 | 889.582 | 628.614 | 757.653 | 0 | 0 |
| Mac | true | 2500 | 0 | 993.732 | 633.635 | 707.529 | 0 | 0 |

Audit write routing was validated by SQL after the run:

| database | `policy_decision_audit_outbox` status | rows |
| --- | --- | ---: |
| independent audit PG | `PUBLISHED` | 5000 |
| main PG | `PUBLISHED` | 248524 |

The main PG count stayed unchanged from before the run, while the independent
audit PG received and published exactly the 5000 decisions from this pressure
run.

## Resource Window

Process/container sampling is pressure-process scoped, not whole-machine usage.
The sampler had no errors and continued after runner exit before stop.

Top CPU users:

| process/container | CPU avg % | CPU max % |
| --- | ---: | ---: |
| `nexusim-postgres` | 115.225 | 286.62 |
| `nexusim-kafka` | 52.617 | 326.59 |
| `nexusim-conversation-service-grpc` | 14.591 | 92.34 |
| `nexusim-delivery-service-timeline-consumer` | 8.103 | 47.44 |
| `nexusim-message-service-outbox-relay` | 6.839 | 22.86 |
| `nexusim-message-service-grpc` | 6.389 | 373.56 |
| `nexusim-policy-audit-postgres` | 3.747 | 169.45 |
| `nexusim-policy-service-grpc` | 3.276 | 183.38 |
| Windows runner | 2.595 | 27 |
| Mac runner | 1.843 | 8.1 |

Interpretation: independent audit PG did receive load, but the main PG remained
the dominant steady resource consumer. Client runner CPU was low, so the result
is not explained by local runner CPU saturation.

## Stage Metrics

Key Prometheus maxima from the run window:

| metric | max |
| --- | ---: |
| `message_send_p99_recent_ms` | 748.761 |
| `message_dependency_read_p99_recent_ms` | 326.781 |
| `message_policy_check_p99_recent_ms` | 236.125 |
| `message_repository_append_call_p99_recent_ms` | 485.394 |
| `message_repository_pool_acquire_p99_recent_ms` | 260.508 |
| `policy_evaluator_facts_read_p99_ms` | 38 |
| `policy_decision_audit_p99_ms` | 69 |
| `policy_decision_audit_pool_acquire_p99_ms` | 49 |
| `policy_decision_audit_insert_exec_p99_ms` | 21 |
| `policy_pg_pool_empty_acquire_total` | 1710 |
| `policy_audit_pg_pool_empty_acquire_total` | 1325 |

Compared with the partial-index run, audit latency improved:

| run | audit p99 ms | audit pool p99 ms | audit insert p99 ms | message repository append p99 ms | message pool acquire p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| baseline audit pool 32 | 126 | 104 | 40 | 375.935 | 196.849 |
| audit pool 64 | 115 | 66 | 66 | 163.527 | 38.023 |
| partial audit index | 133 | 111 | 48 | 328.246 | 238.175 |
| independent audit PG | 69 | 49 | 21 | 485.394 | 260.508 |

The independent audit PG experiment therefore proves audit isolation works, but
it does not improve end-to-end throughput in this topology. Once audit is less
contended, the visible bottleneck moves to the main message PostgreSQL write
path: message repository append and message PG pool acquire dominate the p99.

## Conclusion

Current bottleneck after this validation is no longer "audit outbox alone". The
current bottleneck is the shared main PostgreSQL message write path under the
same SendMessage pressure:

1. Policy facts read remains bounded at tens of milliseconds p99, not the main
   issue.
2. Audit write isolation reduces audit p99 materially, proving the previous
   diagnosis was real.
3. End-to-end p99 and achieved send rate do not improve because message-service
   now waits on its own PG pool/write path.
4. Blindly adding more audit capacity is not the next highest-value fix.

Next work should profile and optimize the message repository append path:
message PG pool sizing under the current main PG profile, insert/index cost on
`message_log` / timeline / `message_outbox`, transaction shape, WAL/checkpoint
behavior, and whether message outbox/write tables need partitioning or tighter
indexes. Independent audit PG is still a valid architectural option, but it only
removes one pressure source; it is not sufficient by itself.

## Observability Gaps

- `policy_audit_outbox_*` Prometheus row-count metrics still represented the
  main policy outbox relay in this run, not the temporary audit PG relay. Audit
  PG correctness was therefore verified with SQL counts.
- If audit PG becomes a supported runtime profile, the audit relay debug target
  and row-count metrics should be added to the standard Prometheus scrape and
  report scripts.
