# push-gateway Loadtest / Smoke Index

本文是 `push-gateway` 验证报告入口。当前已完成六层骨架、WebSocket frame codec、in-memory session registry、delivery event consumer、`server.pong`、`delivery.notify`、`delivery.ack.ok`、queue-full broad `server.resume_hint` active close、单实例 in-memory resume buffer 和 Redis route 最小 adapter；真实进程 full smoke、同 user 多 device notify smoke、slow-client 负向 smoke 和跨进程 Redis route smoke 已通过。

## 当前验证目标

第一阶段只验证在线通知链路，不做 WebSocket 容量极限：

```text
delivery_outbox
-> delivery-service outbox-relay
-> Kafka im.delivery.events
-> push-gateway delivery event consumer
-> online WebSocket client receives delivery.notify
-> client PullInbox reads durable user_inbox
-> client sends delivery.ack frame
-> push-gateway calls delivery-service AckDelivery
-> client receives delivery.ack.ok
```

必须证明：

- push-gateway 消费的是 `im.delivery.events`，不是 `conversation.timeline.events`。
- `delivery.notify` 是轻量唤醒信号，不是 message 事实源。
- 客户端展示和本地持久化以 `PullInbox` 返回为准。
- ACK 仍由 `delivery-service AckDelivery` 推进 cursor。
- `delivery.ack` 成功必须有 `delivery.ack.ok`，失败必须返回稳定 error frame。
- push-gateway 不直接读写 `message_log`、`conversation_members`、`user_inbox`、`device_delivery_cursors`。

当前最小运行模式：

```text
NEXUSIM_PUSH_GATEWAY_MODE=all
NEXUSIM_PUSH_WS_ADDR=0.0.0.0:10496
NEXUSIM_DELIVERY_GRPC_ADDR=127.0.0.1:10497
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_DELIVERY_EVENTS_TOPIC=im.delivery.events
NEXUSIM_PUSH_CONSUMER_GROUP=nexusim-push-gateway-smoke
```

`all` 模式只用于第一阶段本地 smoke：WebSocket handler 和 `im.delivery.events` consumer 共享同一个进程内 session registry。默认 route backend 仍是 memory；跨实例在线路由需要启用 Redis route。

本地分布式模拟使用两个独立 `push-gateway` 进程：

```text
push-gateway-ws       -> 只负责 WebSocket 连接和本机 session registry
push-gateway-consumer -> 只消费 Kafka im.delivery.events
Redis route / PubSub  -> 把 consumer 进程收到的 delivery.notify 转发到 ws 进程
```

这能在一台机器上证明 push-gateway 已经从单进程 `all` 模式推进到最小分布式在线路由模型。它仍不是生产多实例结论：Redis Pub/Sub 是 best-effort online wakeup，可靠恢复仍依赖 `delivery-service PullInbox`。

Redis route 最小参数：

```text
NEXUSIM_PUSH_ROUTE_BACKEND=redis
NEXUSIM_PUSH_GATEWAY_ID=push-gateway-a
NEXUSIM_PUSH_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_PUSH_REDIS_PASSWORD=
NEXUSIM_PUSH_REDIS_DB=0
NEXUSIM_PUSH_REDIS_KEY_PREFIX=nexusim:push
NEXUSIM_PUSH_ROUTE_TTL=90s
NEXUSIM_PUSH_ROUTE_CLEANUP_INTERVAL=30s
```

## 报告位置

当前报告：

| 报告 | 说明 |
| --- | --- |
| `loadtest-report-20260609-push-gateway-full-smoke.md` | `delivery_outbox -> im.delivery.events -> push-gateway -> WebSocket notify -> PullInbox -> AckDelivery` 真实进程 smoke |
| `loadtest-report-20260609-push-gateway-multidevice-smoke.md` | 同一 user 两个在线 device 均收到同一条 `delivery.notify`，并分别 ACK 到各自 cursor |
| `loadtest-report-20260609-push-gateway-slow-client-smoke.md` | 慢客户端触发 queue full / active close 后，通过 durable `PullInbox` 补拉并 ACK |
| `loadtest-report-20260609-push-gateway-redis-route-smoke.md` | WebSocket gateway 与 delivery consumer gateway 分离后，通过 Redis route / PubSub 完成跨进程在线通知 |
| `loadtest-report-20260609-push-gateway-redis-route-ttl-smoke.md` | Redis route 增加 TTL 续期后，clean commit 上再次验证跨进程在线通知 |

报告 Markdown 保存在仓库内：

```text
E:\development\IM\docs\runbook\loadtest\push-gateway\
```

推荐命名：

```text
loadtest-report-YYYYMMDD-push-gateway-smoke.md
```

中大型原始数据、长日志和趋势图保存到机械盘：

```text
H:\NexusIM\loadtest-results
```

小规模 smoke 的轻量 summary 可以临时放在：

```text
E:\development\IM\loadtest\results
```

## 第一阶段不做

- 不做十万级 WebSocket 长连接压测。
- 不打满 Win-Mac 2.5Gbps 链路。
- 不重新做 message-service 硬件矩阵。
- 不把短时 resume buffer 当作 durable inbox。
- 不把单实例 in-memory resume buffer 表述为跨实例 resume；当前 Redis route 只负责在线 session 路由，不负责跨实例 resume buffer；未知客户端 `resume_token` 必须返回 `buffer_miss` 并由服务端签发新 token。
- 不把 push smoke 表述为生产容量结论。
- 不把 queue-full active close 表述为完整慢连接治理；当前 `server.resume_hint` 只是 broad pull fallback，客户端必须用本地 durable cursor 决定 `PullInbox` 起点。已完成单实例 slow-client 真实进程负向 smoke，但它验证的是 durable `PullInbox` fallback，不验证 resume buffer replay；后续还没有多实例慢连接验证。
- `/debug/metrics` 目前只暴露单实例 in-memory registry 调试指标，用于 smoke 排障；不是生产级 Prometheus 指标。
- `NEXUSIM_PUSH_TEST_WRITE_DELAY` 只允许本地 smoke 使用，生产环境必须 unset 或保持 `0`。
- Redis route 当前对在线通知采用 fail-open：lookup / publish 错误不会阻塞 delivery consumer 提交当前 Kafka event；该次在线唤醒可以丢，客户端靠 durable `PullInbox` 恢复。connect 写 route 失败仍 fail-closed，避免把无法跨实例路由的 session 注册成在线。后台 cleanup loop 已能清理 missing / malformed / mismatched stale route；Redis unavailable / stale route cleanup 目前仍主要由单元测试覆盖，尚未做真实 Redis 故障 smoke。

## 面试可讲点

`push-gateway` 的价值不是“直接把消息正文推给客户端”，而是把在线通道放在 durable delivery 之后：

```text
message-service 写消息事实
-> delivery-service 写 user_inbox / delivery_outbox
-> push-gateway 只做在线唤醒
-> 客户端 PullInbox + AckDelivery 完成可靠投递
```

这样断线、重连、成员边界、ACK 丢失和服务重启都可以由 `delivery-service` 的 durable inbox / cursor 兜底。

分布式可讲点：

```text
同一个用户的 WebSocket 连接可能落在 gateway A
Kafka delivery event 可能被 gateway B 消费
gateway B 通过 Redis route 找到 gateway A
gateway A 只做在线 notify
客户端最终仍通过 PullInbox / AckDelivery 完成可靠投递
```

这体现了在线唤醒层和可靠投递层解耦：Redis route 可以丢，WebSocket 可以断，但 message fact、user_inbox 和 ACK cursor 不丢。
