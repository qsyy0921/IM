# Policy hot path experiment, 2026-07-01

Scope: Backend Lab diagnostic experiment for `policy-service` SendMessage hot path.
This is not a production capacity claim.

## Code under test

- Branch: `codex/backend-lab`
- Commit: `43207080`
- Image archive:
  `H:\NexusIM\docker-images\archives\nexusim-policy-service-43207080-20260701-215625.tar`
- Deployed service: `policy-service-grpc`

Focused checks before deployment:

- `go test ./services/policy-service/... -count=1`: pass
- `go build ./services/policy-service/cmd/policy-service`: pass
- `git diff --check`: pass

## Experiment 1: PG pool A/B

All runs used 6000 users, 5000 messages, 768 sender concurrency, target
16000 msg/s, `READ_FANOUT + SEQUENCER_BLOCK`, send-only subscribers.

| Run | Policy PG pool | Send result | Send rate | Send p95 | Send p99 | Notes |
| --- | ---: | --- | ---: | ---: | ---: | --- |
| `hotgroup-policy-hotpath-pool64-6000x5000-768c-43207080-20260701-2200` | 64 | 5000/5000 | 2262.690 msg/s | 481.945ms | 531.929ms | send-path pass; delivery outbox still pending at 60s |
| `hotgroup-policy-hotpath-pool96-6000x5000-768c-43207080-20260701-2215` | 96 | 5000/5000 | 2080.398 msg/s | 585.915ms | 654.204ms | send-path pass; worse than 64 |
| `hotgroup-policy-hotpath-pool128-6000x5000-768c-43207080-20260701-2222` | 128 | 5000/5000 | 2062.255 msg/s | 631.077ms | 739.645ms | runner failed waiting delivery timeline rows; useful as overload diagnostic only |

Policy-service pool counters after each run still showed large empty-acquire
counts:

- pool 64: `empty_acquire_total=30007`, `acquire_duration_ms_total=998086`
- pool 96: `empty_acquire_total=26228`, `acquire_duration_ms_total=555796`
- pool 128: `empty_acquire_total=19667`, `acquire_duration_ms_total=305417`

Increasing the pool reduced acquire wait totals, but did not improve end-to-end
SendMessage latency or throughput. It pushed more concurrent work into
PostgreSQL and the audit/write path.

## Experiment 2: EXPLAIN / ANALYZE

Raw SQL and output:

- `H:\NexusIM\loadtest-results\policy-hotpath-explain-43207080-20260701-2158\policy-hotpath-explain.sql`
- `H:\NexusIM\loadtest-results\policy-hotpath-explain-43207080-20260701-2158\policy-hotpath-explain.out`

Representative tenant/conversation/user:

- tenant: `tenant-hotgroup-20260701-214027`
- conversation: `conv-hotgroup-20260701-214027`
- user: `hot-sender-000001`

Findings:

- `policy_user_message_action_restrictions`: index scan, execution about `0.042ms`.
- `policy_conversation_role_action_rules`: primary key index scan, execution about `0.036ms`.
- `policy_rebac_message_action_rules`: indexed tenant scan plus filter/sort, execution about `0.179ms`.
- `policy_tenant_message_action_quotas`: primary key index scan, execution about `0.085ms`; quota subplan now uses the new audit index if executed.
- combined exact/tenant message rule lookup: primary key scans on both tables, execution about `0.134ms`.

The bottleneck is therefore not a single slow SQL statement. The hot path does
many indexed DB roundtrips per message under high concurrency, and those
roundtrips contend for pool/PG/audit capacity.

## Experiment 3: SQL / index optimization

Added migration:

- `migrations/postgres/policy/000015_policy_hotpath_lookup_indexes.sql`

The new partial index targets the quota subquery:

```sql
CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_quota_allowed_recent
    ON policy_decision_audit_outbox (tenant_id, action, created_at DESC)
    WHERE allowed = true;
```

EXPLAIN confirmed the quota subplan can use
`idx_policy_decision_audit_outbox_quota_allowed_recent`.

## Experiment 4: repeated policy DB checks

The final exact-user rule lookup and tenant-default rule lookup were combined
into one ordered `UNION ALL` query in
`services/policy-service/internal/infrastructure/postgres/evaluator.go`.

Correctness constraint:

- exact user/conversation rule still has precedence `1`;
- tenant default rule has precedence `2`;
- existing deny/allow validation and decision source checks remain fail-closed;
- existing integration tests for exact-over-tenant, user restriction, tenant
  quota, role gate and ReBAC all pass.

Unsafe cache was deliberately not added. A local/Redis cache that skips user
restriction or tenant rule reads can miss a newly inserted deny rule unless it
has a versioned invalidation source. That would be a hidden allow fallback, so
it is not acceptable for this policy path. A future Redis design should cache
only versioned snapshots keyed by tenant/action/conversation permission version
and invalidated by policy-owned version events.

## Bottleneck conclusion

The current bottleneck is the policy hot path as a serial dependency chain over
PostgreSQL plus decision audit outbox writes, not policy-service CPU and not one
bad sequential scan. Raising `NEXUSIM_POLICY_PG_MAX_CONNS` from 64 to 96/128
does not increase throughput; it lowers pool wait counters but makes
SendMessage p99 worse and starts to starve downstream delivery projection.

The next safe optimization should reduce the number of synchronous policy DB
roundtrips and/or move non-blocking audit writes off the SendMessage critical
path with explicit fail-closed durability semantics, rather than only adding
more PG connections.
