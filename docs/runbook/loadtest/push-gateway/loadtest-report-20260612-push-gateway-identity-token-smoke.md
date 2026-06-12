# push-gateway identity-service token smoke

本报告记录 `identity-service` 作为 push-gateway gateway token 签发方的最小真实进程 smoke。

这不是完整 OAuth / identity 平台验收；它证明 push-gateway 可以不再依赖 runner 本地签名，而由独立 identity-service 签发短期 gateway token，同时 push-gateway 握手仍保持本地验签，不同步 RPC 依赖 identity-service。2026-06-12 已补充标准三段 JWT HS256 兼容 smoke、Login 签发 JWT gateway token smoke，以及 RegisterUser -> Login -> JWT gateway token smoke；当前 JWKS 是 identity debug server 上的内部对称 key 发现入口，不应作为公网生产 JWKS。

## Chain

```text
identity-service IssueGatewayToken / RegisterUser + Login
-> push-gateway WebSocket HMAC auth
-> conversation-service CreateMemberChange(JOIN)
-> message-service SendMessage
-> delivery-service projection / delivery_outbox
-> push-gateway delivery.notify
-> delivery-service PullInbox
-> push-gateway delivery.ack
-> delivery-service AckDelivery
```

## Command

### Legacy gateway token smoke

```powershell
.\tools\local-up.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -UseIdentityServiceToken `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName push-gateway-identity-token-smoke-20260612-identity-v4
```

### JWT gateway token smoke

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -IdentityGatewayTokenFormat jwt `
  -RunName push-gateway-identity-jwt-token-20260612-192547
```

### Login + JWT gateway token smoke

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -IdentityGatewayTokenFormat jwt `
  -IdentityTokenMethod login `
  -RunName push-gateway-identity-login-jwt-20260612-195407
```

### Register + Login + JWT gateway token smoke

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -IdentityGatewayTokenFormat jwt `
  -IdentityTokenMethod register_login `
  -RunName push-gateway-identity-register-login-jwt-20260612-201330
```

## Result

### Legacy gateway token smoke

| Item | Value |
| --- | --- |
| Result dir | `H:\NexusIM\loadtest-results\push-gateway-identity-token-smoke-20260612-identity-v4` |
| Summary | `H:\NexusIM\loadtest-results\push-gateway-identity-token-smoke-20260612-identity-v4\pushgateway-summary.json` |
| Commit | `8f7724f` |
| Dirty | `true`, because identity-service code and docs were uncommitted during the run |
| Success | `true` |
| `identity_target` | `127.0.0.1:11610` |
| `push_auth_mode` | `hmac` |
| `push_auth_token_source` | `identity_service` |
| `push_auth_query_identity_sent` | `false` |

### JWT gateway token smoke

| Item | Value |
| --- | --- |
| Result dir | `H:\NexusIM\loadtest-results\push-gateway-identity-jwt-token-20260612-192547` |
| Summary | `H:\NexusIM\loadtest-results\push-gateway-identity-jwt-token-20260612-192547\pushgateway-summary.json` |
| Commit | `dbca0e66628432b58a2517da23ea9972d34900e2` |
| Dirty | `false` |
| Success | `true` |
| `identity_target` | `127.0.0.1:11610` |
| `push_auth_mode` | `hmac` |
| `push_auth_token_source` | `identity_service` |
| `identity_gateway_token_format` | `jwt` |
| `push_auth_query_identity_sent` | `false` |
| server hello | `server.hello`, `session_id=sess_78f4bd94584f677328fe0a20d2e68dfe` |
| notify | `delivery.notify`, `source_event_type=message.persisted.v1`, `conversation_seq=2` |
| PullInbox | `item_count=1`, `max_seq=2` |
| ACK | `delivery.ack.ok last_received_seq=2` |
| delivery outbox | `PUBLISHED=2`, `PENDING=0`, `DLQ=0` |

### Login + JWT gateway token smoke

| Item | Value |
| --- | --- |
| Result dir | `H:\NexusIM\loadtest-results\push-gateway-identity-login-jwt-20260612-195407` |
| Summary | `H:\NexusIM\loadtest-results\push-gateway-identity-login-jwt-20260612-195407\pushgateway-summary.json` |
| Commit | `a5386aab34ef1d531a1bea7b87e5c6e465e1afcf` |
| Dirty | `false` |
| Success | `true` |
| `identity_target` | `127.0.0.1:11610` |
| `push_auth_mode` | `hmac` |
| `push_auth_token_source` | `identity_service_login` |
| `identity_gateway_token_format` | `jwt` |
| `identity_token_method` | `login` |
| `push_auth_query_identity_sent` | `false` |
| server hello | `server.hello`, `session_id=sess_a6da46f382f231edcd7ed765463e9637` |
| notify | `delivery.notify`, `source_event_type=message.persisted.v1`, `conversation_seq=2` |
| PullInbox | `item_count=1`, `max_seq=2` |
| ACK | `delivery.ack.ok last_received_seq=2` |
| cursor | `cursor_last_received_seq=2` |
| delivery outbox | `PUBLISHED=2`, `PENDING=0`, `DLQ=0` |

### Register + Login + JWT gateway token smoke

| Item | Value |
| --- | --- |
| Result dir | `H:\NexusIM\loadtest-results\push-gateway-identity-register-login-jwt-20260612-201330` |
| Summary | `H:\NexusIM\loadtest-results\push-gateway-identity-register-login-jwt-20260612-201330\pushgateway-summary.json` |
| Commit | `b2136b66e08f41ac4d165262e351ac5fac586e93` |
| Dirty | `false` |
| Success | `true` |
| `identity_target` | `127.0.0.1:11610` |
| `push_auth_mode` | `hmac` |
| `push_auth_token_source` | `identity_service_register_login` |
| `identity_gateway_token_format` | `jwt` |
| `identity_token_method` | `register_login` |
| `push_auth_query_identity_sent` | `false` |
| server hello | `server.hello`, `session_id=sess_f522ffef1085eee47e9308816454033f` |
| notify | `delivery.notify`, `source_event_type=message.persisted.v1`, `conversation_seq=2` |
| PullInbox | `item_count=1`, `max_seq=2` |
| ACK | `delivery.ack.ok last_received_seq=2` |
| cursor | `cursor_last_received_seq=2` |
| delivery outbox | `PUBLISHED=2`, `PENDING=0`, `DLQ=0` |

Key facts:

| Check | Result |
| --- | --- |
| server hello | `server.hello`, session created |
| member join | `boundary_seq=1` |
| SendMessage | `conversation_seq=2` |
| notify | `delivery.notify`, `source_event_type=message.persisted.v1`, `conversation_seq=2` |
| PullInbox | `item_count=1`, `max_seq=2` |
| ACK | `delivery.ack.ok last_received_seq=2` |
| cursor | `cursor_last_received_seq=2` |
| delivery outbox | `PUBLISHED=2`, `PENDING=0`, `DLQ=0` |

## Notes

The first two attempts failed before WebSocket connection because the local script piped SQL into `psql` through PowerShell, which introduced a BOM before `CREATE TABLE`. The script now copies the identity migration into the PostgreSQL container with `docker cp` and then runs `psql -f`, avoiding PowerShell pipe encoding.

## Interpretation

This smoke supports the following statement:

```text
NexusIM now has a dedicated identity-service that can issue short-lived push gateway tokens.
push-gateway still verifies tokens locally, so identity-service is not on the WebSocket hot path.
```

JWT 兼容 smoke 额外支持：

```text
identity-service can issue a standard three-part JWT gateway token.
push-gateway can verify that JWT locally with the existing shared-secret verifier path.
the smoke still uses Authorization: Bearer and does not trust query tenant/user identity.
```

Login + JWT smoke 额外支持：

```text
identity-service can verify an existing user password hash through Login.
Login creates a session and refresh token, then returns a JWT gateway token.
push-gateway verifies that gateway token locally and completes the online notify / PullInbox / ACK chain.
```

Register + Login + JWT smoke 额外支持：

```text
identity-service can create an ACTIVE user credential through RegisterUser.
Login then verifies that newly registered password hash and issues a refresh token plus JWT gateway token.
push-gateway still verifies the gateway token locally and does not synchronously call identity-service in the WebSocket hot path.
```

Remaining production work:

- email / phone verification, password reset, MFA and OIDC / federation;
- login rate limit, risk control and account lockout policy;
- asymmetric JWT/JWK key ring and multi issuer support;
- deny-list TTL / compact / repair policy;
- mTLS / gateway verified metadata for other gRPC services.
