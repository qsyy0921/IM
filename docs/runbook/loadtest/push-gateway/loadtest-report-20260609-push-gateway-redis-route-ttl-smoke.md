# push-gateway Redis Route TTL Smoke - 2026-06-09

## 结论

本轮验证了 `push-gateway` Redis route 增加 TTL 续期后的最小分布式在线路由仍然可运行：

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

结果通过。它证明在 WebSocket 连接和 Kafka delivery event 消费不在同一个 `push-gateway` 进程内时，在线通知仍可通过 Redis route 到达目标 gateway。

这可以作为面试中“最小分布式在线路由模拟”的证据，但不能表述为完整生产多实例能力。当前仍未覆盖 Redis 故障、route cleanup ticker、跨实例 resume buffer 和多实例容量。

## 运行环境

| 项 | 值 |
| --- | --- |
| Commit | `a7b1f7ed91f2958ba266ed7960adf19ddd001069` |
| Git dirty | `false` |
| PostgreSQL | `localhost:5432` |
| Kafka | `localhost:9092` |
| Redis | `127.0.0.1:6379` |
| Result dir | `H:\NexusIM\loadtest-results\push-gateway-redis-route-ttl-smoke-20260609-190440` |
| Summary | `H:\NexusIM\loadtest-results\push-gateway-redis-route-ttl-smoke-20260609-190440\pushgateway-summary.json` |

## 启动方式

使用脚本：

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -RouteBackend redis `
  -RunName push-gateway-redis-route-ttl-smoke-20260609-190440
```

脚本启动两个 `push-gateway` 进程：

| 进程 | 模式 | Gateway ID |
| --- | --- | --- |
| WebSocket gateway | `NEXUSIM_PUSH_GATEWAY_MODE=ws` | `push-ws-push-gateway-redis-route-ttl-smoke-20260609-190440` |
| Delivery consumer gateway | `NEXUSIM_PUSH_GATEWAY_MODE=delivery-consumer` | `push-consumer-push-gateway-redis-route-ttl-smoke-20260609-190440` |

Redis route 配置：

```text
NEXUSIM_PUSH_ROUTE_BACKEND=redis
NEXUSIM_PUSH_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_PUSH_REDIS_KEY_PREFIX=nexusim:push:push-gateway-redis-route-ttl-smoke-20260609-190440
NEXUSIM_PUSH_ROUTE_TTL=90s
```

## 关键证据

| 指标 | 结果 |
| --- | --- |
| `server.hello` | 成功，session `sess_668ac001f1f8c90f6ee8d92f1795d0f4` |
| JOIN boundary seq | `1` |
| SendMessage seq | `2` |
| WebSocket notify seq | `2` |
| WebSocket notify message_id | `msg_0790568f-dc11-4727-82c4-9778fdd4452e` |
| PullInbox item_count | `1` |
| PullInbox max_seq | `2` |
| delivery.ack.ok | `last_received_seq=2` |
| device cursor | `2` |
| delivery_outbox | `PUBLISHED=2 / PENDING=0 / DLQ=0` |

## 排查逻辑

本轮不是单进程 `all` 模式：

1. WebSocket client 只连接 `push-gateway-ws`。
2. `im.delivery.events` 只由 `push-gateway-consumer` 消费。
3. 如果 Redis route / PubSub 不工作，consumer 进程本地没有 WebSocket session，客户端不会收到 `delivery.notify`。
4. 实际结果中客户端收到 seq=2 的 `delivery.notify`，随后 `PullInbox` 和 ACK 都成功，因此 Redis route / PubSub / 远端本机 fanout 链路成立。

TTL 续期本轮由单元测试覆盖：`TestRegistryRenewsRouteTTLUntilUnregister` 使用 miniredis 验证 route key 会在 TTL 到期前被刷新，`Unregister` 后会被删除。本 smoke 验证续期改动没有破坏真实跨进程路由。

## 已知限制

- 只覆盖单用户、单设备、单消息。
- 只证明 Redis route 在线通知转发，不证明跨实例 resume buffer。
- Redis route 续期已实现，但进程崩溃后的主动 cleanup ticker 尚未实现，仍依赖 TTL 过期。
- Redis 故障时的 fail-open / fail-closed 策略尚未做真实故障 smoke。
- `/debug/metrics` 仍是单实例 registry 调试指标，不是生产 Prometheus 指标。

## 面试讲法

可以这样表述：

```text
push-gateway 已经从单进程 all mode 推进到最小分布式 route：
WebSocket 连接在一个 gateway 进程，Kafka delivery event 被另一个 gateway 进程消费。
消费进程通过 Redis route 找到持有连接的 gateway，再通过 Pub/Sub 转发轻量 notify。
可靠消息本体仍在 delivery-service 的 user_inbox，客户端收到 notify 后回源 PullInbox 并 AckDelivery。
```

不要这样表述：

```text
已经具备完整生产多实例 push 平台。
```

更准确的限制是：

```text
已验证最小跨进程在线唤醒；生产级多实例还需要 Redis 故障治理、route cleanup、跨实例 resume、正式 metrics 和容量压测。
```

## 下一步

1. 补 Redis route 故障语义 smoke：connect/write route 失败、lookup 失败、publish 失败、subscriber down / stale route。
2. 补 route cleanup ticker 或 stale user set cleanup 指标。
3. 决定跨实例 resume：Redis-backed resume buffer，或明确跨实例只支持 `PullInbox` recovery。
4. 后续再做 Redis route 多设备 / 慢连接组合 smoke。
