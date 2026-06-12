# identity-service SDD

## Status

`identity-service` is the dedicated identity boundary for NexusIM gateway tokens and device/session lifecycle. The first implementation intentionally stays small:

- issue short-lived push gateway HMAC tokens;
- persist users, devices and sessions;
- revoke devices and sessions;
- keep push-gateway verification local to avoid synchronous auth RPC on every WebSocket handshake.

It is not yet a full OAuth/JWK identity platform.

## Boundary

Owns:

- `identity_users`
- `identity_devices`
- `identity_sessions`
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

This mode applies to `RevokeDevice`, `RevokeSession` and `GetDeviceState`. `IssueGatewayToken` intentionally remains outside this admin gate, because token issuance is the identity boundary itself and will later be replaced by a real login / identity provider flow.

## Gateway Token

The first token format is compatible with push-gateway HMAC mode:

```text
base64url(json claims) + "." + base64url(hmac_sha256(payload, secret))
```

Claims:

- `tenant_id`
- `user_id`
- `device_id`
- `session_id`
- `aud`
- `exp`
- `trace_id`

The default audience is `push-gateway`. Tokens are short-lived. Revocation is enforced at issuance time; already-issued tokens are bounded by TTL until push-gateway supports an async revoke feed or local deny-list projection.

## First Smoke Target

```text
identity-service IssueGatewayToken
-> push-gateway WebSocket HMAC auth
-> delivery.notify
-> PullInbox
-> delivery.ack.ok
```

This proves identity-service can replace runner-side local token signing without adding a synchronous dependency to push-gateway's hot path.

## Observability

`identity-service` exposes first-stage local diagnostics when `NEXUSIM_IDENTITY_DEBUG_ADDR` or shared `NEXUSIM_DEBUG_ADDR` is set:

- `GET /healthz`: process liveness, no dependency check.
- `GET /readyz`: PostgreSQL ping readiness.
- `GET /debug/metrics`: pgx pool counters, identity user/device/session counts, and gRPC method/code/latency counters.

The gRPC server also emits one JSON request log per unary RPC with stable fields: `service`, `event`, `method`, `code`, and `latency_ms`.

This is intentionally a lightweight local/debug endpoint. Production tracing, alerting, mTLS, gateway verified metadata and revoke projection/deny-list remain future hardening items.
