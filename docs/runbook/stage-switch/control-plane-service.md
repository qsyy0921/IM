# control-plane-service Stage-Switch Review

Date: 2026-06-20

## Result

`control-plane-service` SDD v0.1 is ready to enter the implementation stage.
No P0/P1 blocker was found in the stage-switch review.

This review does not create `services/control-plane-service` yet. The
implementation slice must switch `control-plane-service` out of `future` in
`service-registry.json` and create the service directory in the same coherent
change.

## Why Promotion Is Justified

- Independent data model: config bundles, immutable versions, rollout rules,
  applied ACKs and control outbox are not owned by api-gateway, policy,
  notification, model or admin services.
- Independent scale profile: config authoring, snapshot reads, drift checks and
  applied ACK monitoring scale differently from IM request hot paths.
- Independent failure boundary: failed config publication, drift detection or
  applied ACK lag must not block message send, delivery, push or policy checks.
- Security boundary: runtime configuration can affect quota, feature flags,
  provider routes and rollout rules, so secret hygiene, checksum verification
  and service-identity reads need a dedicated owner.
- Complexity reduction: keeping quota/config snapshots in api-gateway,
  policy-service and notification-service independently would duplicate version,
  rollback, checksum and drift logic.

## Boundary Checks

- api-gateway still executes request rate limiting and validates local quota
  snapshots; control-plane does not sit in the request hot path.
- policy-service still owns authorization / ReBAC / moderation decisions.
- Dynamic config cannot disable startup safety gates, trusted metadata, mTLS,
  mock-auth guards or secret-manager boundaries.
- payload events, metrics and audit records must not expose `payload_json`,
  tenant plan details, DSN, endpoint secrets, provider tokens or private keys.
- admin-service and workflow-service may call public control-plane APIs, but
  control-plane must not directly mutate other service private tables.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `control-plane-service` from
  `future` to `product-active` with local process / debug / observability
  metadata.
- `api/proto/nexusim/controlplane/v1/control_plane_service.proto`.
- `migrations/postgres/control-plane/000001_control_plane_core.sql`.
- `services/control-plane-service` six-layer skeleton and
  `cmd/control-plane-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
PublishConfigVersion
GetConfigSnapshot
AckAppliedConfigVersion
```

Focus first on `API_GATEWAY_TENANT_QUOTA` and `FEATURE_FLAG` snapshots.
Rollback, Kafka outbox relay, ACK monitor, expiry worker and cleanup operator
are later slices.

## Focused Acceptance For First Smoke

- publish is idempotent by tenant + environment + bundle + idempotency key.
- server computes and verifies checksum; it must not blindly trust a caller
  supplied checksum.
- snapshot output matches the existing api-gateway quota snapshot contract.
- ACK upsert is idempotent by service + instance + bundle + version.
- outbox payload contains only low-sensitive refs and checksum presence, not
  full config payload or tenant plan details.
- malformed config kind, unknown schema field or unsafe secret-like field fails
  closed.
