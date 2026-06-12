# push-gateway identity revoke smoke

日期：2026-06-12

## 目标

验证 `identity-service` 的设备吊销事件能通过异步链路投影到 `push-gateway`，并让旧 gateway token 在 WebSocket 建连时被拒绝。

链路：

```text
IssueGatewayToken
-> push-gateway HMAC auth connect ok
-> RevokeDevice
-> identity_outbox
-> identity-service outbox-relay
-> Kafka im.identity.events
-> push-gateway identity-consumer
-> Redis deny-list
-> old token reconnect returns PERMISSION_DENIED
```

本轮验证的是 revoke projection / deny-list，不验证完整登录、refresh token、JWT/JWK、mTLS 或生产级 identity federation。

## 环境

- Commit: `148d959746e7ce6477b3d1f88d0dff5be9a33128`
- Git dirty: `false`
- Raw summary: `H:\NexusIM\loadtest-results\push-gateway-identity-revoke-redis-clean-20260612-181807\pushgateway-summary.json`
- Route backend: `redis`
- Push auth mode: `hmac`
- Token source: `identity_service`
- Identity event topic: `im.identity.events`
- Push WebSocket gateway: `127.0.0.1:11598`
- Push identity consumer group: `nexusim-push-gateway-identity-smoke-20260612181807`

运行命令：

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario identity-revoke `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -RouteBackend redis `
  -RunName push-gateway-identity-revoke-redis-clean-20260612-181807
```

## 结果

```text
success=true
initial server.hello ok
revoked_device_id=push-device-1
denied_frame.op=error
denied_frame.code=PERMISSION_DENIED
denied_frame.message=permission denied
reconnect_attempts=2
git_dirty=false
```

关键证据：

- Revoke 前同一个 identity token 可以完成 WebSocket `server.hello`。
- `RevokeDevice` 后，identity outbox relay 发布 `identity.device.revoked.v1`。
- push-gateway `identity-consumer` 消费 `im.identity.events`，把设备写入 Redis deny-list。
- WebSocket gateway 再次使用旧 token 建连时返回稳定错误帧 `PERMISSION_DENIED`，不是 `SERVER_BUSY` 或内部错误。

## 结论

当前 `push-gateway` 已补齐第一版低耦合 revoke projection：

- WebSocket 热路径仍是本地 HMAC 验签 + 本地/Redis deny-list 查询，不同步 RPC 调 identity-service。
- 单进程 `all` 模式可用 in-memory deny-list。
- 分进程 / Redis route 模式下，`identity-consumer` 与 `ws` 进程通过 Redis deny-list 共享 revoke 状态。
- device revoke 会拒绝同设备所有旧 gateway token；session revoke 已有事件 consumer 和 session_id 解析，但本轮 smoke 只覆盖 device revoke。

## 限制

- 这不是完整登录系统；`Login / refresh token / JWT JWK / 多 issuer` 仍未实现。
- deny-list 当前无 TTL / compact / repair UI；设备吊销按持久黑名单处理。
- Redis 故障时 revoke checker fail-closed，会拒绝建连；这是安全优先策略，但后续需要 metrics 和故障 smoke。
- 本轮不主动踢掉已经在线的旧 WebSocket 连接，只验证新建连拒绝；在线连接主动关闭可作为后续 hardening。
