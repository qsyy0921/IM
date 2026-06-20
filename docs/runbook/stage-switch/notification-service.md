# notification-service Stage-Switch Review

Date: 2026-06-20

## Result

`notification-service` SDD v0.1 is ready to enter the implementation stage. No
P0/P1 blocker was found in the stage-switch review.

This review does not create `services/notification-service` yet. The
implementation slice must switch `notification-service` out of `future` in
`service-registry.json` and create the service directory in the same coherent
change.

## Why Promotion Is Justified

- Independent data model: notification requests, templates, delivery attempts,
  provider routes, suppressions and notification outbox are not identity,
  message, delivery or push facts.
- Independent scale profile: email / SMS / APNs / FCM retries, provider latency
  and bounce processing scale differently from login, message send or online
  WebSocket fanout.
- Independent failure boundary: provider outages should not corrupt identity
  challenge facts, IM message facts, delivery inbox or push-gateway session
  state.
- Security boundary: raw challenge codes, password reset tokens, provider
  credentials, provider bodies and destination PII require a dedicated
  low-sensitive delivery owner.
- Complexity reduction: identity-service currently has local challenge webhook /
  SMTP paths, but that local sender should not become a full provider routing,
  retry, bounce and template platform inside identity-service.

## Boundary Checks

- identity-service remains owner of verification challenges, reset tokens, MFA
  and credential facts.
- notification-service may receive encrypted short-lived secret payloads, but it
  must not persist raw challenge code, reset token, provider credential, provider
  response body, SMTP transcript, raw destination or push token.
- business services create notifications through public API / request port or
  future outbox adapter, not by writing provider tables directly.
- tenant channel policy, quota and template allowlist come from policy /
  control-plane public ports; notification-service must not hard-code global
  policy as business truth.
- low-sensitive `im.notification.events` must be enough for audit/admin
  projection without leaking secret payloads or destination plaintext.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `notification-service` from
  `future` to `product-active` with local process / debug / observability
  metadata.
- `api/proto/nexusim/notification/v1/notification_service.proto`.
- `migrations/postgres/notification/000001_notification_core.sql`.
- `services/notification-service` six-layer skeleton and
  `cmd/notification-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
CreateNotificationRequest
GetNotificationStatus
CancelNotificationRequest
```

Use a fake/noop provider first. Provider-grade SMTP/SMS/APNs/FCM, bounce
handling, encrypted secret payload KMS/HSM and identity challenge migration are
later hardening.

## Focused Acceptance For First Smoke

- create request is idempotent by tenant + requester + idempotency key.
- request payload and outbox payload do not contain raw destination, raw secret,
  provider body or provider credential.
- status read returns stable public state and masked destination only.
- cancel is fail-closed after delivered / DLQ.
- fake provider delivery worker can mark a request delivered and write a
  low-sensitive delivery event.
