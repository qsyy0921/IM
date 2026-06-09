# push-gateway HMAC Auth Smoke

## 目标

验证 `push-gateway` 在 `NEXUSIM_PUSH_AUTH_MODE=hmac` 下可以拒绝裸 query 身份依赖，并使用短期 signed gateway token 完成真实在线通知链路。

本轮不是完整 identity-service 验收，只证明 gateway 入口能校验第一版 token：

- HMAC 签名；
- `aud=push-gateway`；
- `exp` 未过期；
- `device_id` 与 WebSocket 连接设备一致；
- token 通过 `Authorization: Bearer` 传输，WebSocket URL 不再携带裸 `tenant_id/user_id`。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `8aa414c` |
| git_dirty | `false` |
| runner | `loadtest/pushgateway` |
| raw summary | `H:\NexusIM\loadtest-results\push-gateway-hmac-auth-smoke-clean-20260610-052736\pushgateway-summary.json` |
| scenario | `full` |
| route_backend | `memory` |
| push_auth_mode | `hmac` |
| token transport | `authorization_header` |
| token ttl | `600s` |
| query identity sent | `false` |

## 命令

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -PushAuthHmacSecret local-push-smoke-secret `
  -RunName push-gateway-hmac-auth-smoke-clean-20260610-052736 `
  -SkipBuild
```

## 链路

```text
client signs short gateway token
-> WebSocket Authorization: Bearer <token>
-> push-gateway validates HMAC / aud / exp / device
-> server.hello
-> CreateMemberChange(JOIN)
-> SendMessage
-> delivery_outbox -> im.delivery.events
-> delivery.notify
-> PullInbox
-> delivery.ack
-> delivery-service AckDelivery
-> delivery.ack.ok
```

## 结果

| 指标 | 值 |
| --- | --- |
| success | `true` |
| server_hello.session_id | `sess_d22b95860b66da5eb35963694333108e` |
| delivery_notify.source_event_type | `message.persisted.v1` |
| delivery_notify.conversation_seq | `2` |
| PullInbox item_count | `1` |
| PullInbox max_seq | `2` |
| delivery.ack.ok last_received_seq | `2` |
| cursor_last_received_seq | `2` |
| delivery_outbox_total | `2` |
| delivery_outbox_published | `2` |
| delivery_outbox_pending | `0` |
| delivery_outbox_dlq | `0` |
| create_member_join latency | `75.592ms` |
| send_message latency | `48.011ms` |

## 判断

本轮证明：`push-gateway` 的 WebSocket 入口可以从本地 mock auth 推进到第一版 signed gateway token。HMAC 模式下 smoke runner 没有发送裸 `tenant_id/user_id` query，而是用 `Authorization` header 传短期 token。

这使 `push-gateway` 的 ACK 和 online notify 链路仍然保持低耦合：

- 不新增 identity-service；
- 不跨服务读取用户或设备表；
- 不改变 delivery-service 的 durable inbox / cursor 权威边界；
- 不把 WebSocket notify 当作可靠消息事实。

## 限制

- 这不是完整登录系统。
- 未覆盖 refresh token、device revoke、session revoke、key rotation、多 issuer、JWT/JWK。
- 本轮只跑单实例 memory route full smoke；Redis route / Win-Mac / Sentinel 场景仍沿用 mock auth，后续如要演示更接近真实客户端，可按需重跑 HMAC 版本。
- HMAC secret 是本地 smoke secret，不能作为生产密钥管理方案。
