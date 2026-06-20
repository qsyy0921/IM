# admin-service Stage-Switch Review

Date: 2026-06-21

## Result

`admin-service` SDD v0.1 is ready to enter the implementation stage. No P0/P1
blocker was found in the stage-switch review.

This review does not create `services/admin-service` yet. The implementation
slice must switch `admin-service` out of `future` in `service-registry.json` and
create the service directory in the same coherent change.

## Why Promotion Is Justified

- Independent data model: admin operations, approvals, operation results and
  admin outbox are not owned by identity, contacts, policy, control-plane,
  workflow, audit or individual business services.
- Independent failure boundary: an operator workflow, approval wait, downstream
  admin API failure or audit export request must not block user IM traffic,
  delivery, push, retrieval or model invocation.
- Security boundary: high-risk operator actions need verified admin metadata,
  policy precheck, separation of duty, idempotency, audit refs and controlled
  execution rather than ad hoc scripts.
- Complexity reduction: keeping admin operation state in each service would
  duplicate approval, payload redaction, result tracking and repair-safety
  logic.

## Boundary Checks

- `admin-service` is not the user-facing IM gateway and must not carry normal
  message, delivery, contact or identity user traffic.
- It must not directly write private tables owned by other services.
- It calls public admin/operator APIs, workflow-service, control-plane-service
  or service-specific operator commands.
- High-risk actions must retain policy, approval, execution and audit boundaries.
- `workflow-service` owns long approval wait and compensation state; admin
  creates or queries workflow refs.
- `audit-service` owns long-term audit and export; admin emits low-sensitive
  events and result refs.
- Events, metrics and debug snapshots must not contain raw payload, raw operator
  reason, downstream response body, password, token, TOTP/recovery code, message
  body, prompt, EvidencePack text, object key, DSN or secret.

## Gate Impact For Next Slice

The implementation slice is broader than docs-only. It must update:

- `docs/runbook/service-registry.json`: switch `admin-service` from `future` to
  `product-active` with local process / debug / observability metadata.
- `api/proto/nexusim/admin/v1/admin_service.proto`.
- `migrations/postgres/admin/000001_admin_core.sql`.
- `services/admin-service` six-layer skeleton and `cmd/admin-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
CreateAdminOperation
ApproveAdminOperation
GetAdminOperation
ListAdminOperations
```

Support low-sensitive operation metadata and approval state first. Real
downstream execution, operation worker, outbox relay, admin UI and audit export
can follow after the first gRPC / repository path unless needed to prove the
first path.

## Focused Acceptance For First Smoke

- `CreateAdminOperation` is idempotent by tenant + operator + idempotency key.
- Operation payload is a low-sensitive command summary; raw reason, secrets,
  raw EvidencePack, downstream response body and object keys are rejected or not
  persisted.
- `ApproveAdminOperation` requires approver identity and enforces
  separation-of-duty for high / critical risk operations.
- Status transitions are monotonic and reject replay with a different command
  hash.
- `Get/List` return public-safe state only and do not expose raw operation
  payload values beyond allowlisted low-sensitive fields.
- No admin code reads or writes private tables owned by identity, contacts,
  policy, control-plane, audit, workflow or IM business services.
