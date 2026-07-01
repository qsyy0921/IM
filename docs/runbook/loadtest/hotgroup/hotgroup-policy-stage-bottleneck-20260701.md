# Hot Group Policy Stage Bottleneck - 2026-07-01

Scope: diagnostic send-only bottleneck attribution for 6000-member
`READ_FANOUT + SEQUENCER_BLOCK` hot group after adding policy-service evaluator
stage metrics. This is not a production capacity result.

## Change Under Test

- Commit: `213d7956`
- Change: policy-service now exposes low-cardinality stage latency metrics for
  `CheckMessageAction(SEND)`:
  - `contact_block_lookup`
  - `user_restriction_lookup`
  - `role_gate`
  - `rebac_gate`
  - `tenant_quota_lookup`
  - `exact_rule_lookup`
  - `tenant_rule_lookup`
  - `decision_audit_outbox`
- Ubuntu redeploy: `nexusim/policy-service:local` rebuilt, archived at
  `H:\NexusIM\docker-images\archives\nexusim-policy-service-213d7956-20260701-213320.tar`,
  loaded on `qsyy0921@172.31.50.2`, and `policy-service-grpc` recreated via
  docker compose.

## Runs

| run | status | notes |
| --- | --- | --- |
| `hotgroup-policy-stage-6000x5000-512c-213d7956-20260701-2134` | SendMessage complete; runner failed waiting delivery outbox drain | `git_dirty=false`, 5000/5000 SendMessage succeeded, delivery outbox still had 2711 rows at drain timeout. Useful for send-path attribution, not a clean end-to-end pass. |
| `hotgroup-policy-stage-6000x5000-768c-213d7956-20260701-2140` | Diagnostic pass | 5000/5000 SendMessage succeeded, but `git_dirty=true` because the 512c reports already existed in the workspace. Useful for bottleneck attribution, not formal capacity evidence. |

## Key Evidence

512c:

- run-local SendMessage p95 / p99: `421.5ms / 495.772ms`.
- Prometheus recent `message_policy_check_p99`: max `285.685ms`, last `213.02ms`.
- policy-service `CheckMessageAction` avg / max: max `162ms / 416ms`.
- policy stage p99:
  - `user_restriction_lookup`: `42ms`
  - `role_gate`: `41ms`
  - `rebac_gate`: `40ms`
  - `tenant_quota_lookup`: `43ms`
  - `exact_rule_lookup`: `42ms`
  - `tenant_rule_lookup`: `40ms`
  - `decision_audit_outbox`: `48ms`
- process resource: policy-service avg CPU `0.038%`, PostgreSQL avg CPU
  `110.782%`.

768c:

- run-local SendMessage p95 / p99: `390.545ms / 446.314ms`.
- achieved send rate: about `2288.829 msg/s`.
- Prometheus recent `message_policy_check_p99`: max / last `309.066ms`.
- policy-service `CheckMessageAction` avg / max: max `199ms / 325ms`.
- policy stage p99:
  - `user_restriction_lookup`: `60ms`
  - `role_gate`: `58ms`
  - `rebac_gate`: `58ms`
  - `tenant_quota_lookup`: `55ms`
  - `exact_rule_lookup`: `60ms`
  - `tenant_rule_lookup`: `60ms`
  - `decision_audit_outbox`: `64ms`
- policy PG pool:
  - `NEXUSIM_POLICY_PG_MAX_CONNS=32`
  - `policy_pg_pool_acquire_total`: `70701`
  - `policy_pg_pool_empty_acquire_total`: `69307`
  - `policy_pg_pool_acquire_duration_ms_total`: `1806423`
- process resource: policy-service avg CPU `0.087%`, PostgreSQL avg CPU
  `123.383%`, Kafka avg CPU `50.012%`.

## Interpretation

The concrete bottleneck is not policy-service CPU and not a single isolated slow
query. The hot send path is serialized through multiple policy PostgreSQL
lookups plus policy decision audit writing. Under 768 send concurrency, almost
every policy PostgreSQL acquire is an empty acquire, so requests queue on the
policy-service PG pool and then pay several 55-64ms p99 stage costs in sequence.

In plain terms: each SendMessage has to pass several policy database checkpoints
before the message is appended. Increasing client concurrency puts more requests
in that checkpoint queue; Ubuntu still has spare CPU because the blocked
goroutines are waiting for PostgreSQL connections and query results.

## Next Step

Run a controlled A/B with only `NEXUSIM_POLICY_PG_MAX_CONNS` changed:

- baseline: `32`
- candidate: `64`
- optional next candidates: `96`, `128`

For each run, compare:

- `message_policy_check_p99_recent_ms`
- policy stage p99 per stage
- `policy_pg_pool_empty_acquire_total` delta
- `policy_pg_pool_acquire_duration_ms_total` delta
- PostgreSQL CPU / IO and connection count
- SendMessage achieved rate and p99

If larger policy PG pool reduces `empty_acquire` and `policy_check` p99, then
the first fix is runtime profile sizing. If p99 remains high after pool sizing,
the next fix is reducing policy DB work per send: skip empty rule stages, merge
lookups, make audit writing cheaper, and evaluate versioned Redis/local caching
for hot permission decisions.
