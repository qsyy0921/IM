# Push Gateway PostgreSQL Quorum Observation Smoke

## Summary

- Date: 2026-06-15
- Commit under test: `3540572 test: add postgres quorum observation smoke`
- Result: observation captured
- Raw result root: `H:\NexusIM\loadtest-results\postgres-quorum-observation-smoke-20260615-202747`
- Summary JSON: `H:\NexusIM\loadtest-results\postgres-quorum-observation-smoke-20260615-202747\postgres-quorum-observation-summary.json`
- Baseline smoke summary: `H:\NexusIM\loadtest-results\postgres-quorum-observation-smoke-20260615-202747-before\pushgateway-summary.json`
- Only-primary smoke summary: `H:\NexusIM\loadtest-results\postgres-quorum-observation-smoke-20260615-202747-only-primary\pushgateway-summary.json`
- Restore smoke summary: `H:\NexusIM\loadtest-results\postgres-quorum-observation-smoke-20260615-202747-after-restore\pushgateway-summary.json`

This is a local quorum-loss observation against the current `bitnamilegacy/postgresql-repmgr + pgpool` topology. It is not a production PostgreSQL HA, split-brain prevention, or quorum-fencing proof.

## Scenario

The wrapper started the local PostgreSQL HA topology and ran a baseline NexusIM distributed smoke through the stable pgpool endpoint:

```text
postgres://nexusim:nexusim@127.0.0.1:15432/nexusim?sslmode=disable
```

It then stopped both standby containers while keeping the current primary and pgpool running:

```text
before_primary=postgres-ha-0
stopped_standbys=nexusim-postgres-ha-1,nexusim-postgres-ha-2
```

The goal was to observe whether the local writer endpoint is quorum-fenced when only one PostgreSQL node remains.

## Evidence

Before the fault, pgpool reported one primary and two async streaming standbys:

```text
postgres-ha-0 up primary
postgres-ha-1 up standby streaming async
postgres-ha-2 up standby streaming async
```

After stopping both standbys, pgpool eventually reported:

```text
postgres-ha-0 up primary
postgres-ha-1 down standby unknown
postgres-ha-2 down standby unknown
```

The direct write probe still succeeded:

```text
write_probe_with_only_primary=true
```

The full NexusIM chain also completed while only the primary remained:

```text
commit=3540572
git_dirty=false
success=true
delivery.notify conversation_seq=2
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox PUBLISHED=2 / PENDING=0 / DLQ=0
```

After restoring both standby containers, pgpool again reported one primary and two streaming async standbys, and the full smoke completed again:

```text
success=true
delivery.notify conversation_seq=2
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox PUBLISHED=2 / PENDING=0 / DLQ=0
```

## Interpretation

This result is intentionally not a pass/fail HA claim. It proves a boundary:

```text
the current local repmgr + pgpool topology keeps accepting writes
when both standbys are down and only the primary remains.
```

That is useful for development and demos, but it means this topology should not be described as quorum-fenced production PostgreSQL HA.

For production-grade split-brain resistance, the project still needs a different or hardened design, such as:

- Patroni + etcd/Consul style quorum and leader lock
- managed cloud PostgreSQL HA with documented fencing semantics
- strict synchronous replication policy for selected data classes
- explicit failure-domain runbooks and split-brain repair procedures

## Limits

This run does not prove:

- split-brain prevention
- quorum-based write fencing
- in-flight transaction continuity
- cross-machine PostgreSQL HA
- automatic failback safety
- production-grade RPO/RTO

It is a local observation smoke that prevents over-claiming the current `repmgr + pgpool` setup.
