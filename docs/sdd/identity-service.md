# identity-service SDD

## Status

`identity-service` is the dedicated identity boundary for NexusIM login, gateway tokens and device/session lifecycle. The first implementation intentionally stays small:

- create first-stage local user credentials;
- verify user credentials for first-stage password login;
- apply a small persistent failed-login counter and temporary account lockout;
- issue short-lived push gateway tokens;
- rotate opaque refresh tokens;
- issue and confirm email / phone verification challenges;
- issue and confirm password reset challenges;
- enroll, confirm and disable first-stage TOTP MFA factors;
- issue, regenerate and revoke one-time MFA recovery codes;
- persist users, devices and sessions;
- revoke devices and sessions;
- keep push-gateway verification local to avoid synchronous auth RPC on every WebSocket handshake.

It is not yet a full OAuth/OIDC identity platform. It does not implement email / SMS sending, WebAuthn/passkeys, external IdP federation, production-grade account-risk workflows or production-grade asymmetric key management.

## Boundary

Owns:

- `identity_users`
- `identity_devices`
- `identity_sessions`
- `identity_refresh_tokens`
- `identity_challenges`
- `identity_mfa_factors`
- `identity_mfa_recovery_codes`
- password hash verification for existing users
- email / phone verification and password reset challenge state
- TOTP MFA factor secret lifecycle
- MFA recovery-code hash lifecycle
- gateway token issuance
- device/session revoke state

Does not own:

- message facts;
- conversation membership;
- delivery inbox / ACK cursors;
- push-gateway online session registry;
- email / SMS provider delivery;
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

This mode applies to `RevokeDevice`, `RevokeSession` and `GetDeviceState`. `RegisterUser`, `Login`, `RefreshGatewayToken`, `BeginMFAEnrollment`, `ConfirmMFAEnrollment`, `DisableMFAFactor`, `RegenerateMFARecoveryCodes`, `RevokeMFARecoveryCodes` and `IssueGatewayToken` intentionally remain outside this admin gate. `RegisterUser` creates a first-stage local credential; `Login` verifies user credentials; `RefreshGatewayToken` verifies an opaque refresh token; MFA factor RPCs are protected by current password and/or one-time TOTP proof; `IssueGatewayToken` is kept as an internal / compatibility signing path for local smoke and gateway-token workflows.

## Register / Login / Refresh

First-stage registration creates an ACTIVE `identity_users` credential with a service-local password hash. It is a strict create path: an existing `tenant_id + user_id` returns `ALREADY_EXISTS`, and account claiming / recovery for pre-created users is a later workflow. It does not create contacts, conversation membership, profile state, or any cross-service user projection. Tenant-level rate limiting and external IdP federation are separate future flows.

```text
RegisterUser
-> validate tenant/user/password
-> hash password
-> identity_users ACTIVE

Login
-> verify password_hash
-> if ACTIVE TOTP factors exist, require and verify mfa_code or one-time mfa_recovery_code
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
- If the user has one or more ACTIVE TOTP MFA factors, `Login` must verify either `mfa_code` or a one-time `mfa_recovery_code` before generating refresh token, session or gateway token state. Missing proof returns stable `MFA_REQUIRED`; invalid proof returns stable `INVALID_MFA`.
- Invalid TOTP codes are counted on the selected MFA factor, independent from password failure counters. Reaching `NEXUSIM_IDENTITY_MFA_MAX_FAILED_ATTEMPTS` within `NEXUSIM_IDENTITY_MFA_FAILURE_WINDOW` locks that factor for `NEXUSIM_IDENTITY_MFA_LOCK_DURATION` and public Login returns stable `mfa temporarily locked`.
- A valid MFA recovery code is consumed in the same PostgreSQL transaction that writes session / refresh-token state. Recovery-code proof does not update TOTP `last_used_at` and does not count as a TOTP failed-code attempt.
- Defaults are `NEXUSIM_IDENTITY_LOGIN_MAX_FAILED_ATTEMPTS=5`, `NEXUSIM_IDENTITY_LOGIN_FAILURE_WINDOW=15m` and `NEXUSIM_IDENTITY_LOGIN_LOCK_DURATION=15m`; deployments may tune them.
- This is not a complete fraud/risk engine. IP/device reputation, CAPTCHA, geo-anomaly, tenant policy, alert routing, adaptive throttling and tenant-level rate limits remain future hardening.

## MFA TOTP Factors

First-stage MFA support is limited to TOTP factors, password Login enforcement and one-time recovery codes generated during TOTP confirmation or regeneration. It does not yet implement refresh-token step-up, WebAuthn/passkeys or tenant-specific factor policy.

```text
BeginMFAEnrollment
-> validate tenant/user/type/password
-> verify current password
-> generate TOTP secret
-> store encrypted secret in identity_mfa_factors as PENDING
-> return raw secret and otpauth_uri once

ConfirmMFAEnrollment
-> load encrypted PENDING factor
-> verify 6-digit TOTP code
-> mark factor ACTIVE
-> replace ACTIVE recovery-code hashes
-> return plaintext recovery codes once

DisableMFAFactor
-> verify current password
-> mark PENDING / ACTIVE factor DISABLED

RegenerateMFARecoveryCodes
-> verify current password
-> verify ACTIVE TOTP factor code
-> disable previous ACTIVE recovery-code hashes
-> insert new ACTIVE recovery-code hashes
-> return plaintext recovery codes once

RevokeMFARecoveryCodes
-> verify current password
-> disable all ACTIVE recovery-code hashes

Login with ACTIVE MFA
-> verify password_hash
-> list ACTIVE TOTP factors
-> require mfa_code or mfa_recovery_code when any ACTIVE factor exists
-> verify selected factor or consume recovery code before session / refresh-token write
```

MFA factor rules:

- only `TOTP` is supported in this slice;
- `identity_mfa_factors.secret_ciphertext`, `secret_nonce` and `secret_key_version` store encrypted secret material;
- raw TOTP secret is returned only by `BeginMFAEnrollment` and is never persisted as plaintext;
- `ConfirmMFAEnrollment` accepts only a pending TOTP factor and a six-digit code;
- `ConfirmMFAEnrollment` returns plaintext recovery codes once; PostgreSQL stores only `identity_mfa_recovery_codes.code_hash`, never raw recovery codes;
- `DisableMFAFactor` requires the current password and does not delete historical rows;
- `RegenerateMFARecoveryCodes` requires the current password plus an ACTIVE TOTP factor's six-digit code; the repository re-checks and locks that ACTIVE TOTP factor in the replace transaction, disables previous ACTIVE recovery-code hashes and returns new plaintext recovery codes once;
- `RevokeMFARecoveryCodes` requires the current password and idempotently disables all ACTIVE recovery-code hashes for that user;
- `LoginRequest.mfa_factor_id` selects a factor; if omitted and exactly one ACTIVE factor exists, that factor is used;
- `LoginRequest.mfa_code` must be a six-digit TOTP code; session and refresh-token state are written only after MFA succeeds;
- `LoginRequest.mfa_recovery_code` is mutually exclusive with `mfa_code` and `mfa_factor_id`; it is normalized, hashed, matched against ACTIVE recovery-code hashes and marked `USED` in the same transaction as session / refresh-token creation;
- `identity_mfa_factors.login_failed_count`, `login_failed_last_at` and `login_locked_until` are factor-level Login risk state, not global account lock state;
- successful MFA Login clears the selected factor's Login failure state and updates `last_used_at` in the same PostgreSQL transaction that writes session / refresh-token state;
- `NEXUSIM_IDENTITY_MFA_SECRET_KEY` is the local AES-GCM encryption key input; local smoke may fall back to the existing gateway token secret, but production profiles should use a dedicated secret managed by KMS/HSM. If no MFA key is configured, the service still starts and existing Login/JWKS flows are unaffected, but MFA factor RPCs and MFA-protected Login return stable `MFA_UNAVAILABLE` / `mfa temporarily unavailable` until the key is configured.
- `NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET` is the preferred HMAC secret for recovery-code hashes. Local smoke may fall back to `NEXUSIM_IDENTITY_MFA_SECRET_KEY`; it does not fall back to gateway / push token secrets.

Known MFA hardening still pending:

- Refresh-token step-up enforcement;
- backup factor handling beyond TOTP recovery codes;
- WebAuthn / passkeys;
- richer MFA risk policy beyond the first factor-level failed-code counter and short lockout;
- secret key rotation and KMS/HSM-backed envelope encryption;
- per-factor audit events and risk-based challenge policy.

## Verification / Password Reset

First-stage email / phone verification and password reset use service-owned one-time challenges:

```text
RequestVerificationChallenge
-> verify current password
-> upsert pending email / phone destination
-> identity_challenges ACTIVE

ConfirmVerificationChallenge
-> lock challenge FOR UPDATE
-> verify token hash, expiry and max attempts
-> mark challenge CONSUMED
-> mark email_verified_at / phone_verified_at

RequestPasswordReset
-> require already verified email / phone destination
-> identity_challenges ACTIVE
-> invalid destination or active-challenge limit returns neutral accepted shape

ConfirmPasswordReset
-> lock challenge FOR UPDATE
-> verify token hash, expiry and max attempts
-> mark challenge CONSUMED
-> update password_hash
-> revoke active refresh tokens and sessions
-> identity.session.revoked.v1 outbox for revoked sessions
```

Challenge token rules:

- raw verification challenge tokens are returned only to the caller when `NEXUSIM_IDENTITY_DEV_RETURN_CHALLENGE_TOKEN=true`; this is a local smoke / development aid and must stay disabled in production profiles.
- password reset requests never return raw challenge tokens, even when the development token flag is enabled.
- PostgreSQL stores only `identity_challenges.token_hash`, never the raw token.
- Invalid token attempts increment `attempt_count`; reaching `max_attempts` expires the challenge.
- Password reset requires an already verified destination for the same `tenant_id + user_id`.
- `RequestPasswordReset` hides invalid credentials and active-challenge throttling behind the same accepted response shape; it does not return raw tokens in that path.
- Verification challenge creation requires the current password to avoid unauthenticated email / phone takeover.
- A first-stage durable cap limits active challenges per `tenant_id + user_id + challenge_type + channel + destination`.
- `identity-service` does not send email or SMS directly in this slice. A sender adapter or notification service can consume an explicit future command/event without changing the challenge table ownership.

Known hardening still pending:

- timing- and sender-side account-enumeration resistance;
- tenant / IP / device rate limits for challenge creation and confirmation;
- email / SMS provider integration and audit;
- WebAuthn and OIDC federation;
- production alerting for repeated challenge failures.

## Gateway Token

Gateway tokens are compatible with push-gateway HMAC mode. The legacy token format is:

```text
base64url(json claims) + "." + base64url(hmac_sha256(payload, secret))
```

The current implementation also supports standard three-part JWT gateway tokens:

```text
base64url(header) + "." + base64url(claims) + "." + base64url(signature)
```

Supported signing modes:

- `legacy` / `hmac`: local HMAC compatibility token for old smoke runners.
- `jwt` / `jwt-hs256`: standard JWT with HS256. This is still local / internal debug compatibility because push-gateway must know the symmetric signing secret.
- `jwt-rs256` / `rs256`: standard JWT with RS256. `identity-service` loads an RSA private key from `NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_PEM` or `NEXUSIM_IDENTITY_GATEWAY_TOKEN_RSA_PRIVATE_KEY_FILE`; its debug server exposes public RSA JWK material through `/.well-known/jwks.json` / `/jwks.json`. During a manual rotation overlap window, `NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_JSON` or `NEXUSIM_IDENTITY_GATEWAY_TOKEN_ADDITIONAL_JWKS_FILE` may add old public keys to that JWKS response; the current signing key wins duplicate `kid` values. `push-gateway` verifies locally with `NEXUSIM_PUSH_AUTH_MODE=jwt` plus static `NEXUSIM_PUSH_AUTH_JWKS_JSON` / `NEXUSIM_PUSH_AUTH_JWKS_FILE`, or remote `NEXUSIM_PUSH_AUTH_JWKS_URL` with `NEXUSIM_PUSH_AUTH_JWKS_REFRESH_INTERVAL`; it may restrict `iss` with `NEXUSIM_PUSH_AUTH_TRUSTED_ISSUERS`.

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

The first RS256 implementation now supports a static local key-ring slice, a remote JWKS URL cache with periodic refresh, and a manual old-public-key overlap window through additional JWKS env/file configuration. It is still not a complete production key management system. Production hardening still needs automatic key rotation workflows, KMS / HSM backed private keys, stronger issuer governance, trace / alert coverage and operational runbooks.

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
RequestVerificationChallenge -> identity_challenges ACTIVE with token_hash only
ConfirmVerificationChallenge -> email_verified_at / phone_verified_at
ConfirmPasswordReset -> password hash updated + active session / refresh token revoke
```

## Observability

`identity-service` exposes first-stage local diagnostics when `NEXUSIM_IDENTITY_DEBUG_ADDR` or shared `NEXUSIM_DEBUG_ADDR` is set:

- `GET /healthz`: process liveness, no dependency check.
- `GET /readyz`: PostgreSQL ping readiness.
- `GET /debug/metrics`: pgx pool counters, identity user/device/session counts, failed password-login user counts, currently password-login-locked user counts, MFA factor counts, MFA factor failed-login counts, currently MFA-login-locked ACTIVE factor counts, and gRPC method/code/latency counters.

The gRPC server also emits one JSON request log per unary RPC with stable fields: `service`, `event`, `method`, `code`, and `latency_ms`.

This is intentionally a lightweight local/debug endpoint. Production tracing, alerting, mTLS, external SIEM / audit sinks and adaptive risk analytics remain future hardening items.
