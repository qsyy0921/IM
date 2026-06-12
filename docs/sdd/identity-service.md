# identity-service SDD

## Status

`identity-service` is the dedicated identity boundary for NexusIM login, gateway tokens and device/session lifecycle. The first implementation intentionally stays small:

- create first-stage local user credentials;
- verify user credentials for first-stage password login;
- apply a small persistent failed-login counter and temporary account lockout;
- issue short-lived push gateway tokens;
- rotate opaque refresh tokens;
- persist users, devices and sessions;
- revoke devices and sessions;
- keep push-gateway verification local to avoid synchronous auth RPC on every WebSocket handshake.

It is not yet a full OAuth/OIDC identity platform. It does not implement email / phone verification, MFA, external IdP federation, account recovery or production-grade asymmetric key management.

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

This mode applies to `RevokeDevice`, `RevokeSession` and `GetDeviceState`. `RegisterUser`, `Login`, `RefreshGatewayToken` and `IssueGatewayToken` intentionally remain outside this admin gate. `RegisterUser` creates a first-stage local credential; `Login` verifies user credentials; `RefreshGatewayToken` verifies an opaque refresh token; `IssueGatewayToken` is kept as an internal / compatibility signing path for local smoke and gateway-token workflows.

## Register / Login / Refresh

First-stage registration creates an ACTIVE `identity_users` credential with a service-local password hash. It is a strict create path: an existing `tenant_id + user_id` returns `ALREADY_EXISTS`, and account claiming / recovery for pre-created users is a later workflow. It does not create contacts, conversation membership, profile state, or any cross-service user projection. Email / phone verification, password reset, MFA, rate limiting and external IdP federation are separate future flows.

```text
RegisterUser
-> validate tenant/user/password
-> hash password
-> identity_users ACTIVE

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

Login risk first-stage rules:

- `identity_users.failed_login_count`, `failed_login_last_at` and `locked_until` are owned by identity-service.
- A failed password login records one durable failure for the user.
- Failures are counted within `NEXUSIM_IDENTITY_LOGIN_FAILURE_WINDOW`; older failures do not keep accumulating forever.
- When the configured threshold is reached, password `Login` is temporarily locked and public Login returns stable `account temporarily locked`.
- A successful Login clears the failure counter and lock fields in the same PostgreSQL transaction that writes session / refresh-token state.
- The first-stage lock applies only to password Login. A valid refresh token can still rotate through `RefreshGatewayToken`; refresh token theft / reuse is handled by the separate rotation and session-revoke logic. This avoids letting an external password brute-force attempt break an already authenticated client session.
- Defaults are `NEXUSIM_IDENTITY_LOGIN_MAX_FAILED_ATTEMPTS=5`, `NEXUSIM_IDENTITY_LOGIN_FAILURE_WINDOW=15m` and `NEXUSIM_IDENTITY_LOGIN_LOCK_DURATION=15m`; deployments may tune them.
- This is not a complete fraud/risk engine. IP/device reputation, CAPTCHA, geo-anomaly, tenant policy, alert routing, adaptive throttling and tenant-level rate limits remain future hardening.

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
identity-service IssueGatewayToken or RegisterUser + Login
-> push-gateway WebSocket HMAC auth
-> delivery.notify
-> PullInbox
-> delivery.ack.ok
```

This proves identity-service can replace runner-side local token signing without adding a synchronous dependency to push-gateway's hot path. The latest smoke also covers `RegisterUser + Login`: `RegisterUser` creates an ACTIVE user credential, `Login` verifies that password hash, creates a session and refresh token, returns a JWT gateway token, and push-gateway verifies that token locally before completing `delivery.notify -> PullInbox -> delivery.ack.ok`.

The current Register / Login / Refresh implementation is validated by app tests, PostgreSQL integration tests, and push-gateway JWT smokes:

```text
RegisterUser -> ACTIVE user credential
Login -> ACTIVE refresh token
Repeated failed Login -> durable failed_login_count + temporary account lockout
RefreshGatewayToken -> old token USED + new token ACTIVE
Reuse old refresh token -> session REVOKED + identity.session.revoked.v1 outbox
Expired refresh token -> token REVOKED + stable invalid refresh error
```

## Observability

`identity-service` exposes first-stage local diagnostics when `NEXUSIM_IDENTITY_DEBUG_ADDR` or shared `NEXUSIM_DEBUG_ADDR` is set:

- `GET /healthz`: process liveness, no dependency check.
- `GET /readyz`: PostgreSQL ping readiness.
- `GET /debug/metrics`: pgx pool counters, identity user/device/session counts, failed password-login user counts, currently password-login-locked user counts, and gRPC method/code/latency counters.

The gRPC server also emits one JSON request log per unary RPC with stable fields: `service`, `event`, `method`, `code`, and `latency_ms`.

This is intentionally a lightweight local/debug endpoint. Production tracing, alerting, mTLS, external SIEM / audit sinks and adaptive risk analytics remain future hardening items.
