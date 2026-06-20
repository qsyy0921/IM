# presence-service Stage-Switch Review

Date: 2026-06-20

## Result

`presence-service` SDD v0.1 is ready to enter the implementation stage. No
P0/P1 blocker was found in the stage-switch review.

This review does not create `services/presence-service` yet. The implementation
slice must switch `presence-service` out of `future` in
`service-registry.json` and create the service directory in the same coherent
change.

## Why Promotion Is Justified

- Independent data model: presence sessions, user aggregate state, typing
  indicators, subscriptions and presence outbox are not owned by push-gateway,
  delivery-service, contacts-service or policy-service.
- Independent scale profile: heartbeat, TTL expiry, typing fanout and
  near-real-time state reads scale differently from durable inbox, message send
  and WebSocket routing.
- Independent failure boundary: presence lag or Redis loss must not block
  message delivery, PullInbox, ACK, push route, policy checks or conversation
  membership facts.
- Security boundary: online state, invisible mode, device presence and typing
  visibility need a dedicated owner that can fail closed without leaking user
  activity.
- Complexity reduction: keeping full presence semantics inside push-gateway
  would mix best-effort route state with product-visible online state and make
  Redis route hardening more fragile.

## Boundary Checks

- push-gateway still owns WebSocket sessions, Redis route, resume buffer and
  online delivery notification.
- delivery-service still owns durable inbox and device ACK cursors.
- conversation-service still owns member facts and conversation visibility.
- policy-service / contacts-service remain the visibility decision sources;
  presence-service must only consume public APIs or explicit ports.
- Redis can be used for hot TTL state, but PostgreSQL remains the durable owner
  for last_seen, subscriptions and outbox. Redis loss must degrade to
  UNKNOWN/OFFLINE + last_seen and must not affect PullInbox / AckDelivery.
- Presence events and metrics must not expose IP, raw user-agent, tokens,
  device secrets, message drafts, typing content or raw identifiers.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `presence-service` from `future`
  to `product-active` with local process / debug / observability metadata.
- `api/proto/nexusim/presence/v1/presence_service.proto`.
- `migrations/postgres/presence/000001_presence_core.sql`.
- `services/presence-service` six-layer skeleton and `cmd/presence-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full
`check-local` before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
UpdatePresence
GetPresence
UpdateTyping
```

Use PostgreSQL for durable state and a local in-memory Redis-like adapter or
real Redis adapter boundary for the first slice. `SubscribePresence`,
push-gateway session event consumer, stale scanner and outbox relay can follow
after the first gRPC smoke unless they are needed to prove the first path.

## Focused Acceptance For First Smoke

- `UpdatePresence(ONLINE)` is idempotent by tenant + user + device + session +
  idempotency key.
- `GetPresence` returns only visibility-filtered state and masks invisible or
  unauthorized targets as OFFLINE/UNKNOWN without leaking real online state.
- `UpdateTyping(STARTED/STOPPED)` stores no draft text and emits only low-sensitive
  typing refs.
- PostgreSQL session/user state/outbox writes are in one transaction.
- Redis loss or unavailable visibility dependency fails closed or returns
  UNKNOWN; it must not affect delivery-service PullInbox / AckDelivery.
- metrics and outbox payloads contain no raw tenant/user/device/session IDs, IP,
  raw user-agent, token, draft text or provider/internal error body.
