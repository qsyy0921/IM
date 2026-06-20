# control-plane-service gRPC smoke report 2026-06-20

## Scope

Local first-stage `control-plane-service` smoke for the product-active slice.
The run verifies the minimal versioned config path:

```text
PublishConfigVersion -> idempotent replay -> GetConfigSnapshot
-> AckAppliedConfigVersion -> control_outbox low-sensitive payload check
```

Raw summary and logs are stored outside the repository:

```text
H:\NexusIM\loadtest-results\control-plane-grpc-smoke-20260620
```

## Environment

- commit: `93e58628feb6744d0b5ed3b423ded08832d0ecac`
- git_dirty: `false`
- target: `127.0.0.1:52243`
- tenant: `tenant-control-plane-grpc-smoke-20260620`

## Result

Status: passed.

Key facts from `control-plane-grpc-summary.json`:

| Check | Result |
| --- | --- |
| `PublishConfigVersion` | passed |
| same request replay | `replay_same_version=true` |
| snapshot version | `quota-v1.smoke` |
| snapshot checksum | `sha256:fc2848373abd981762936801f878cea64f5117616c5ecc56a8d52e0c5dd518a4` |
| applied ACK status | `IN_SYNC` |
| `control_outbox.total` | `2` |
| `control_outbox.pending` | `2` |
| `control_outbox.dlq` | `0` |
| outbox payload safety scan | `true` |

## Boundaries

This smoke proves only the local gRPC + PostgreSQL first path.

It does not prove:

- rollback.
- Kafka outbox relay.
- drift monitor.
- expiry / cleanup worker.
- api-gateway dynamic quota consumer.
- production HA, sizing, long-run SLO, or provider-grade rollout.

`control-plane-service` remains outside the request hot path; api-gateway still
owns rate-limit execution and local snapshot fail-closed behavior.
