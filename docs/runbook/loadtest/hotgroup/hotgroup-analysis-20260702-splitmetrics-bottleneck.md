# Hot Group Split Metrics Bottleneck Report

Date: 2026-07-02

Commit under test: `fb3f2155b0e2d8291696745a13345b636bd7c13b`

## Run

This run used two full hotgroup runners against the Ubuntu service host:

| runner | group | messages | send concurrency | target msg/s | achieved msg/s | send p99 | result |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Windows | 6000 | 2500 | 384 | 8000 | 1561.609 | 677.368 ms | success |
| Mac | 6000 | 2500 | 384 | 8000 | 2223.475 | 257.043 ms | success |

Aggregate pressure was 5000 SendMessage calls at 768 client-side concurrency. Both
runs used `READ_FANOUT`, `conversation-subscriber-count=0`, and required delivery
outbox drain. Message and delivery outbox pending counts were `0` at the end of
both runs.

Raw result paths:

- `H:\NexusIM\loadtest-results\hotgroup-splitmetrics-2client-6000x2500x2-384c-fb3f2155-20260702-182542`
- `H:\NexusIM\loadtest-results\hotgroup-splitmetrics-2client-6000x2500x2-384c-fb3f2155-20260702-182542-win`
- `H:\NexusIM\loadtest-results\hotgroup-splitmetrics-2client-6000x2500x2-384c-fb3f2155-20260702-182542-mac`

## Resource Window

The original sampler stopped late because the local orchestration script failed
during cleanup after both runners completed. The raw samples are preserved, and
the resource conclusion below uses the trimmed window ending at max runner finish
time plus 30 seconds:

- trimmed summary: `H:\NexusIM\loadtest-results\hotgroup-splitmetrics-2client-6000x2500x2-384c-fb3f2155-20260702-182542\lab-process-resource\lab-process-resource-summary-trimmed-30s.md`
- trimmed CSV: `H:\NexusIM\loadtest-results\hotgroup-splitmetrics-2client-6000x2500x2-384c-fb3f2155-20260702-182542\lab-process-resource\lab-process-resource-samples-trimmed-30s.csv`

Only hotgroup runner processes and NexusIM service containers are counted. The
trimmed host utilization was:

| host | logical CPUs | pressure CPU avg | pressure CPU max | pressure memory max |
| --- | ---: | ---: | ---: | ---: |
| Ubuntu | 72 | 4.986% | 25.755% | 3.103% |
| Windows | 16 | 0.188% | 0.938% | 0.078% |
| Mac | 8 | 0.334% | 1.000% | 0.241% |

Ubuntu was not CPU-bound. The busiest service was PostgreSQL, peaking at
`1236.54%` Docker CPU, roughly 12.4 logical cores. The latency therefore points
to queueing around finite DB pools and synchronous PostgreSQL writes, not lack of
available host CPU.

## Prometheus Evidence

Prometheus report:

- `docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260702-splitmetrics-2client-6000x2500x2.md`

Key send-path p99 metrics in the captured window:

| metric | max |
| --- | ---: |
| `message_send_p99_recent_ms` | 656.678 ms |
| `message_dependency_read_p99_recent_ms` | 381.408 ms |
| `message_policy_check_p99_recent_ms` | 322.690 ms |
| `message_repository_append_call_p99_recent_ms` | 375.935 ms |
| `message_repository_pool_acquire_p99_recent_ms` | 196.849 ms |
| `message_repository_insert_message_p99_recent_ms` | 69.234 ms |
| `message_repository_insert_timeline_p99_recent_ms` | 58.155 ms |
| `message_repository_insert_outbox_p99_recent_ms` | 84.996 ms |
| `message_repository_commit_p99_recent_ms` | 25.112 ms |

Policy-service split metrics:

| metric | value |
| --- | ---: |
| `policy_evaluator_facts_read_p99_ms` | 15 ms |
| `policy_decision_audit_p99_ms` | 126 ms |
| `policy_decision_audit_pool_acquire_p99_ms` | 104 ms |
| `policy_decision_audit_insert_exec_p99_ms` | 40 ms |
| `policy_decision_audit_split_errors_5m` | 0 |

Pool counter deltas over the same window:

| pool | acquire delta | empty acquire delta | empty ratio | acquire wait total | avg wait / acquire |
| --- | ---: | ---: | ---: | ---: | ---: |
| message PG pool | 5002 | 587 | 11.735% | 46126 ms | 9.222 ms |
| policy audit PG pool | 5000 | 4592 | 91.840% | 262799 ms | 52.560 ms |

## Bottleneck

The current bottleneck is no longer the policy facts read itself. It is the
synchronous PostgreSQL write path around SendMessage, with the clearest single
queue in the policy decision audit PG pool.

In plain terms:

1. Each SendMessage asks policy-service whether the sender is allowed to send.
2. The policy facts read is now cheap: p99 was only `15 ms`.
3. After deciding, policy-service still synchronously writes a decision audit
   outbox row before returning.
4. Under 768 concurrent sends, the audit pool had only 32 PG connections. In
   this run, `4592 / 5000` audit writes found the pool empty and had to wait.
5. That wait alone averaged about `52.6 ms` per audit write, and the p99 pool
   acquire stage was `104 ms`.
6. The message-service repository write path also queued on PG, but less
   severely: `587 / 5002` empty acquires and about `9.2 ms` average acquire wait.

So the request is not slow because one SQL statement is individually bad. It is
slow because many requests arrive at the same time, and each request performs
synchronous DB work. The code makes requests line up at the PG pools and at the
shared PostgreSQL writer.

## What This Rules Out

- Policy semantic checks are not the dominant cost in this run. The combined
  facts read p99 was `15 ms`.
- Redis decision cache is not the first fix for this specific measurement.
  Caching the policy decision would skip some reads, but the largest measured
  wait is the synchronous audit DB write and pool acquire.
- Ubuntu still has CPU capacity. Adding more client concurrency without changing
  the synchronous DB write path is likely to raise p99 before it raises useful
  throughput.
- Message/delivery async outbox drain was not the end-state blocker in this
  no-subscriber 2500x2 run because pending counts ended at `0`. A larger message
  run can still expose relay backlog, as previous 5000-message diagnostics did.

## Next Experiments

1. Keep the current split metrics and run an A/B test with `NEXUSIM_POLICY_AUDIT_PG_MAX_CONNS=64`.
   Treat it as diagnostic only: if send p99 improves but message/delivery relay
   backlog grows, the true bottleneck is shared PG write pressure, not just pool
   size.
2. Test audit outbox write reduction without changing fail-closed semantics:
   smaller payload, minimal indexes, and table partitioning. The goal is to
   reduce `decision_audit_insert_exec` and the time each audit connection is held.
3. Add or verify message outbox relay Prometheus exposure for this deployed
   profile. The current report saw message relay pending drain cleanly, but some
   relay metrics had no series.
4. Only after audit insert remains dominant should we consider an isolated audit
   PostgreSQL instance. Multiple PG instances can help isolate audit writes from
   message writes, but it increases operational complexity and does not remove
   the fail-closed latency unless the synchronous audit path is redesigned.
