# audit-service gRPC smoke report 2026-06-20

## Scope

Local first-stage `audit-service` smoke for the current product-active slice.
The run verifies the minimal gRPC API and PostgreSQL audit append model:

```text
AppendAuditRecord -> idempotent replay -> QueryAuditRecords -> VerifyAuditProof
-> audit_outbox low-sensitive payload check
```

Raw summary and logs are stored outside the repository:

```text
H:\NexusIM\loadtest-results\audit-service-grpc-smoke-20260620
```

## Environment

- commit: `5ce71f92a6d87d8ef362167a4572688004c3b4fd`
- git_dirty: `false`
- audit target: `127.0.0.1:52303`
- TLS: disabled for local smoke
- tenant: `tenant-audit-service-grpc-smoke-20260620`

## Result

Status: passed.

Key facts from `audit-grpc-summary.json`:

| Check | Result |
| --- | --- |
| `AppendAuditRecord` first append | passed |
| same request replay | `replay_same_audit_id=true` |
| `QueryAuditRecords` count | `2` |
| `VerifyAuditProof` | `valid=true` |
| proof previous hash | points to first record hash |
| `audit_outbox.total` | `2` |
| `audit_outbox.pending` | `2` |
| `audit_outbox.dlq` | `0` |
| outbox payload safety scan | `true` |

## Boundaries

This smoke proves only the local gRPC + PostgreSQL first path.

It does not prove:

- Kafka ingestion into `audit-service`.
- export worker / SIEM forwarding.
- retention cleanup.
- segment sealing.
- upstream business-event completeness.
- production HA, sizing, long-run SLO, or provider-grade operations.

Hash-chain proof only proves records already accepted by `audit-service` were
linked consistently for this local run.
