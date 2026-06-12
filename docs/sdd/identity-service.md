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
