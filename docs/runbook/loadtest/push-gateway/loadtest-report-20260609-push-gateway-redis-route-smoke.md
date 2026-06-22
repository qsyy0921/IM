# push-gateway Redis Route Smoke - 2026-06-09

## 结论

本轮验证了 `push-gateway` 的最小 Redis route / cross-instance route：

```text
push-gateway-ws holds WebSocket session
push-gateway-consumer consumes im.delivery.events
-> Redis route lookup / PubSub
-> push-gateway-ws local fanout
-> WebSocket delivery.notify
-> PullInbox
-> delivery.ack
-> delivery-service AckDelivery
-> delivery.ack.ok
```

结果通过。它证明在线连接不在消费到 Kafka event 的同一个 push-gateway 进程内时，仍可通过 Redis route 收到 `delivery.notify`。

这不是容量压测，也不是完整生产多实例方案。当前仍未覆盖 Redis route TTL 续期、route cleanup ticker、Redis 故障降级、跨实例 resume buffer 和多实例慢连接。

## 运行环境

| 项 | 值 |
| --- | --- |
| Commit | `903f205b787579bfcf454837ca6971941202ac8b` |
| Git dirty | `false` |
| PostgreSQL | `localhost:5432` |
| Kafka | `localhost:9092` |
| Redis | `127.0.0.1:6379` |
| Result dir | `H:\NexusIM\loadtest-results\push-gateway-redis-route-smoke-20260609-185046` |
| Summary | `H:\NexusIM\loadtest-results\push-gateway-redis-route-smoke-20260609-185046\pushgateway-summary.json` |

## 启动方式

使用脚本：

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -RouteBackend redis `
  -RunName push-gateway-redis-route-smoke-20260609-185046 `
  -SkipBuild
```

脚本启动两个 push-gateway 进程：

| 进程 | 模式 | Gateway ID |
| --- | --- | --- |
| WebSocket gateway | `NEXUSIM_PUSH_GATEWAY_MODE=ws` | `push-ws-push-gateway-redis-route-smoke-20260609-185046` |
| Delivery consumer gateway | `NEXUSIM_PUSH_GATEWAY_MODE=delivery-consumer` | `push-consumer-push-gateway-redis-route-smoke-20260609-185046` |

Redis route 配置：

```text
NEXUSIM_PUSH_ROUTE_BACKEND=redis
NEXUSIM_PUSH_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_PUSH_REDIS_KEY_PREFIX=nexusim:push:push-gateway-redis-route-smoke-20260609-185046
NEXUSIM_PUSH_ROUTE_TTL=90s
```

## 关键证据

| 指标 | 结果 |
| --- | --- |
| `server.hello` | 成功，session `sess_5692b8bf0cc23d5f0227a5573796c76d` |
| JOIN boundary seq | `1` |
| SendMessage seq | `2` |
| WebSocket notify seq | `2` |
| WebSocket notify message_id | `msg_587da28d-c1fc-4d2a-9765-536ef8f4b25a` |
| PullInbox item_count | `1` |
| PullInbox max_seq | `2` |
| delivery.ack.ok | `last_received_seq=2` |
| device cursor | `2` |
| delivery_outbox | `PUBLISHED=2 / PENDING=0 / DLQ=0` |

## 排查逻辑

本轮不是只验证单进程 `all` 模式。脚本把 WebSocket 和 delivery consumer 拆到两个不同进程：

1. WebSocket client 只连接 `push-gateway-ws`。
2. `im.delivery.events` 只由 `push-gateway-consumer` 消费。
3. 如果 Redis route 没生效，consumer 进程本地没有 WebSocket session，客户端不会收到 `delivery.notify`。
4. 实际结果中客户端收到 seq=2 的 `delivery.notify`，随后 `PullInbox` 和 ACK 都成功，因此 Redis route / PubSub / 远端本机 fanout 链路成立。

## 已知限制

- 只覆盖单用户、单设备、单消息。
- 只证明 Redis route 在线通知转发，不证明跨实例 resume buffer。
- Redis route TTL 当前只在 connect 写入时设置，尚未做 heartbeat 续期。
- Redis 故障时的 fail-open / fail-closed 策略尚未做真实故障 smoke。
- `/debug/metrics` 仍是单实例 registry 调试指标，不是生产 Prometheus 指标。

## 下一步

1. 补 Redis route TTL 续期 / cleanup。
2. 补 Redis route 故障语义测试。
3. 设计跨实例 resume buffer 或明确跨实例只使用 `PullInbox` recovery。
4. 后续再做 Redis route 多设备 / 慢连接组合 smoke。
