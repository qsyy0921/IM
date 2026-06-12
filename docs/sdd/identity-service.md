# identity-service SDD

## Status

`identity-service` is the dedicated identity boundary for NexusIM login, gateway tokens and device/session lifecycle. The first implementation intentionally stays small:

- verify existing user credentials for first-stage password login;
- issue short-lived push gateway tokens;
- rotate opaque refresh tokens;
- persist users, devices and sessions;
- revoke devices and sessions;
- keep push-gateway verification local to avoid synchronous auth RPC on every WebSocket handshake.

It is not yet a full OAuth/OIDC identity platform. It does not implement user registration, MFA, external IdP federation, account recovery or production-grade asymmetric key management.

## Boundary

Owns:

- `identity_users`
- `identity_devices`
- `identity_sessions`
- `identity_refresh_tokens`
- password hash verification for existing users
- gateway token issuance
- device/session revoke state

Does not own:

- message facts;
- conversation membership;
- delivery inbox / ACK cursors;
- push-gateway online session registry;
- contacts or policy decisions.

## Admin Auth

The default admin RPC mode is request-body compatible for local smoke and legacy scripts. Production-like local runs can set:

```text
NEXUSIM_IDENTITY_ADMIN_AUTH_MODE=metadata
```

In metadata mode, admin/read-state RPCs derive the trusted tenant/operator from gateway-verified gRPC metadata instead of trusting request-body `AdminContext`:

- `x-nexusim-tenant-id`
- `x-nexusim-user-id`
- `x-nexusim-trace-id` (optional)
- `x-nexusim-request-id` (optional)

This mode applies to `RevokeDevice`, `RevokeSession` and `GetDeviceState`. `Login`, `RefreshGatewayToken` and `IssueGatewayToken` intentionally remain outside this admin gate. `Login` verifies user credentials; `RefreshGatewayToken` verifies an opaque refresh token; `IssueGatewayToken` is kept as an internal / compatibility signing path for local smoke and gateway-token workflows.

## Login / Refresh

First-stage login expects the user row and `identity_users.password_hash` to already exist. Registration, password reset, MFA, rate limiting and external IdP federation are separate future flows.

```text
Login
-> verify password_hash
-> identity_devices / identity_sessions
-> identity_refresh_tokens ACTIVE
-> short-lived gateway token + opaque refresh token
```

Password hashes use service-local PBKDF2-SHA256 encoding:

```text
pbkdf2-sha256$iterations$base64url(salt)$base64url(key)
```

This keeps the implementation dependency-light for the local Go baseline. A production deployment can migrate to Argon2id / bcrypt by adding verifier support for a new hash prefix without changing service boundaries.

Refresh token rules:

- raw refresh tokens are returned only to the client and are never stored in PostgreSQL;
- `identity_refresh_tokens.token_hash` stores a SHA-256 hash of the token secret;
- a successful refresh marks the presented token `USED`, inserts one new `ACTIVE` refresh token and returns a new short-lived gateway token;
- expired refresh tokens are marked `REVOKED` and rejected;
- reuse of a `USED` or `REVOKED` refresh token is treated as credential compromise: the session is marked `REVOKED`, active refresh tokens for that session are revoked, and `identity.session.revoked.v1` is written through `identity_outbox`.

## Gateway Token

Gateway tokens are compatible with push-gateway HMAC mode. The legacy token format is:

```text
base64url(json claims) + "." + base64url(hmac_sha256(payload, secret))
```

The current implementation also supports standard three-part JWT HS256 gateway tokens:

```text
base64url(header) + "." + base64url(claims) + "." + base64url(signature)
```

Claims:

- `tenant_id`
- `user_id`
- `device_id`
- `session_id`
- `aud`
- `iss` / `sub` / `iat` for JWT mode
- `exp`
- `trace_id`

The default audience is `push-gateway`. Tokens are short-lived. Revocation is enforced at issuance / refresh time and asynchronously projected to push-gateway deny-lists through `im.identity.events`.

The identity debug server can expose `/.well-known/jwks.json` / `/jwks.json` for the current HS256 key. This is internal debug compatibility only because it exposes a symmetric `oct` key; production-grade JWKS should use an asymmetric key ring so gateways only receive public keys.

## Revoke Events

Device and session revoke operations write identity events through the local outbox in the same PostgreSQL transaction:

```text
RevokeDevice / RevokeSession
-> identity_devices / identity_sessions
-> identity_outbox
-> identity-service outbox-relay
-> Kafka im.identity.events
```

Event types:

- `identity.device.revoked.v1`
- `identity.session.revoked.v1`

These events are designed for push-gateway local deny-list projection and audit consumers. Push-gateway should consume the event stream asynchronously; it should not synchronously call identity-service on each WebSocket handshake.

## First Smoke Target

```text
identity-service IssueGatewayToken or Login
-> push-gateway WebSocket HMAC auth
-> delivery.notify
-> PullInbox
-> delivery.ack.ok
```

This proves identity-service can replace runner-side local token signing without adding a synchronous dependency to push-gateway's hot path. The latest smoke also covers `Login` with an existing password hash: `Login` creates a session and refresh token, returns a JWT gateway token, and push-gateway verifies that token locally before completing `delivery.notify -> PullInbox -> delivery.ack.ok`.

The current Login / Refresh implementation is validated by app tests, PostgreSQL integration tests, and a push-gateway Login + JWT smoke:

```text
Login -> ACTIVE refresh token
RefreshGatewayToken -> old token USED + new token ACTIVE
Reuse old refresh token -> session REVOKED + identity.session.revoked.v1 outbox
Expired refresh token -> token REVOKED + stable invalid refresh error
```

## Observability

`identity-service` exposes first-stage local diagnostics when `NEXUSIM_IDENTITY_DEBUG_ADDR` or shared `NEXUSIM_DEBUG_ADDR` is set:

- `GET /healthz`: process liveness, no dependency check.
- `GET /readyz`: PostgreSQL ping readiness.
- `GET /debug/metrics`: pgx pool counters, identity user/device/session counts, and gRPC method/code/latency counters.

The gRPC server also emits one JSON request log per unary RPC with stable fields: `service`, `event`, `method`, `code`, and `latency_ms`.

This is intentionally a lightweight local/debug endpoint. Production tracing, alerting, mTLS, gateway verified metadata and revoke projection/deny-list remain future hardening items.
