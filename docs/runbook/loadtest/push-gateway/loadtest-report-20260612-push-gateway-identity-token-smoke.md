# push-gateway identity-service token smoke

本报告记录 `identity-service` 作为 push-gateway HMAC token 签发方的最小真实进程 smoke。

这不是完整 OAuth / JWK / identity 平台验收；它只证明 push-gateway 可以不再依赖 runner 本地签名，而由独立 identity-service 签发短期 gateway token，同时 push-gateway 握手仍保持本地验签，不同步 RPC 依赖 identity-service。

## Chain

```text
identity-service IssueGatewayToken
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

```powershell
.\tools\local-up.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -UseIdentityServiceToken `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName push-gateway-identity-token-smoke-20260612-identity-v4
```

## Result

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

Remaining production work:

- real login / credential proof;
- JWT/JWK or gateway-token standardization;
- device revoke projection or short-TTL-only revoke policy clarification;
- session revoke propagation to online gateways;
- mTLS / gateway verified metadata for other gRPC services.
