# audit-service Stage-Switch Review

Date: 2026-06-20

## Result

`audit-service` SDD v0.1 is ready to enter the implementation stage. No P0/P1
blocker was found in the stage-switch review.

This review does not create `services/audit-service` yet. The implementation
slice must switch `audit-service` out of `future` in `service-registry.json`
and create the service directory in the same coherent change.

## Why Promotion Is Justified

- Independent data model: audit records, hash segments, export jobs,
  ingestion checkpoints and audit outbox are not owned by identity, policy,
  agent, notification or admin services.
- Independent scale profile: audit ingestion, export generation, proof
  verification and retention cleanup scale differently from user-facing IM
  request paths.
- Independent failure boundary: audit export, hash-chain sealing or SIEM sink
  failures must not block login, message send, delivery, push or Agent
  proposal creation.
- Security boundary: audit data needs strict low-sensitive schemas, redaction,
  retention and proof verification without leaking raw prompts, message bodies,
  provider bodies, secrets or full PII.
- Complexity reduction: keeping cross-service audit export, proof verification
  and Agent action trace joins inside individual services would duplicate
  redaction and retention policy.

## Boundary Checks

- Upstream services remain the owners of their business facts and local
  transaction-scoped audit.
- `audit-service` may ingest low-sensitive audit events or accept internal
  append calls, but it must not query another service's private tables.
- Agent write-action audit must link proposal, approval, policy decision,
  tool prepare and executor result by refs, not by copying raw EvidencePack or
  provider output.
- Hash-chain proof only demonstrates tamper evidence after audit ingestion; it
  does not prove the upstream business fact was complete or true.
- Query and export APIs must default to redacted views and require policy /
  admin authorization.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `audit-service` from `future` to
  `product-active` with local process / debug / observability metadata.
- `api/proto/nexusim/audit/v1/audit_service.proto`.
- `migrations/postgres/audit/000001_audit_core.sql`.
- `services/audit-service` six-layer skeleton and `cmd/audit-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full
`check-local` before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
AppendAuditRecord
QueryAuditRecords
VerifyAuditProof
```

Use PostgreSQL only for the first slice. Kafka ingestion, export worker,
external SIEM forwarding, retention cleanup and provider-grade object storage
exports are later slices.

## Focused Acceptance For First Smoke

- append is idempotent by tenant + source_service + source_event_id or
  idempotency key.
- attributes are allowlisted and reject raw token, password, TOTP/recovery
  code, raw prompt, raw EvidencePack, message body, provider body, SQL error,
  object key, destination and full PII.
- append writes `audit_records` and hash pointer in one transaction.
- query returns redacted public fields and does not expose attributes that were
  rejected or raw canonical payload.
- proof verification detects changed record hash, missing predecessor and
  segment mismatch.
- audit outbox events contain only low-sensitive refs, counts, stream IDs and
  hash/proof metadata.
