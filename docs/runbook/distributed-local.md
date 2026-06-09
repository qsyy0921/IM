# NexusIM Local Distributed Runbook

本文说明当前本地可运行的 NexusIM 分布式拓扑。它不是生产部署手册，而是面向开发、演示和面试复盘的最小分布式 smoke 入口。

## 1. 当前分布式边界

当前已落地的真实微服务 / 网关：

| 组件 | 当前职责 | 本地进程模式 |
| --- | --- | --- |
| `conversation-service` | 会话、成员、成员变更 saga、`GetSendContext` | `grpc` |
| `message-service` | `SendMessage` 本地事务、message outbox relay | `grpc`、`outbox-relay` |
| `delivery-service` | timeline projection、`PullInbox`、`AckDelivery`、delivery outbox relay | `grpc`、`timeline-consumer`、`outbox-relay` |
| `push-gateway` | WebSocket 在线连接、`im.delivery.events` 消费、Redis route 在线路由 | `ws`、`delivery-consumer` |

基础设施：

```text
PostgreSQL -> 交易事实源 / inbox / outbox / cursor
Kafka      -> conversation.timeline.events / im.delivery.events
Redis      -> push-gateway online route
```

## 2. 最小分布式 smoke 链路

```text
client WebSocket
-> push-gateway-ws

SendMessage
-> message-service grpc
-> PostgreSQL local transaction
-> message_outbox
-> message-service outbox-relay
-> Kafka conversation.timeline.events
-> delivery-service timeline-consumer
-> user_inbox + delivery_outbox
-> delivery-service outbox-relay
-> Kafka im.delivery.events
-> push-gateway delivery-consumer
-> Redis route / PubSub
-> push-gateway-ws local fanout
-> WebSocket delivery.notify
-> delivery-service PullInbox
-> delivery.ack frame
-> delivery-service AckDelivery
-> delivery.ack.ok
```

关键点：

- `push-gateway-ws` 和 `push-gateway-consumer` 是两个不同进程。
- WebSocket 连接只在 `push-gateway-ws`。
- Kafka `im.delivery.events` 只由 `push-gateway-consumer` 消费。
- 如果 Redis route / PubSub 不工作，客户端收不到 `delivery.notify`。
- 可靠投递不依赖 Redis：客户端最终以 `PullInbox` 和 `AckDelivery` 为准。

## 3. 运行命令

先启动本地基础设施：

```powershell
.\tools\local-up.ps1
```

运行分布式 smoke：

```powershell
. .\tools\go-env.ps1
.\tools\local-distributed-smoke.ps1
```

常用参数：

```powershell
.\tools\local-distributed-smoke.ps1 `
  -RunName nexusim-distributed-smoke-YYYYMMDD-HHMMSS `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RedisAddr 127.0.0.1:6379
```

如果二进制已构建：

```powershell
.\tools\local-distributed-smoke.ps1 -SkipBuild
```

## 4. 结果位置

大结果文件保存到机械盘：

```text
H:\NexusIM\loadtest-results\<run-name>\pushgateway-summary.json
```

报告归档在仓库：

```text
docs/runbook/loadtest/push-gateway/
```

当前最接近系统级分布式证据的报告：

```text
docs/runbook/loadtest/push-gateway/loadtest-report-20260609-push-gateway-redis-route-ttl-smoke.md
```

## 5. 面试讲法

可以说：

```text
我把 IM 系统拆成 message / conversation / delivery / push-gateway 四个真实进程边界。
消息写入只提交 PostgreSQL 本地事务和 outbox，不直接发 Kafka。
delivery-service 从 timeline 事件构建 user_inbox，再写 delivery_outbox。
push-gateway 只消费 delivery 事件做在线唤醒，WebSocket 连接和 Kafka consumer 可以落在不同 gateway 实例，通过 Redis route 找到目标连接。
可靠恢复由 PullInbox 和 AckDelivery 保证，Redis/WebSocket 只负责在线提示。
```

不要说：

```text
已经完成生产级多机部署和完整容量验证。
```

更准确的限制：

```text
当前是本地多进程分布式 smoke。它证明服务边界、outbox、Kafka、durable inbox、Redis route、WebSocket notify 和 ACK 能串起来。
生产级还需要真实鉴权、Kubernetes 部署、Redis 故障治理、跨实例 resume、正式 metrics、容量和故障演练。
```

## 6. 已知缺口

- Redis route 故障目前只有单元测试覆盖，尚未做真实故障 smoke；当前策略是 connect 写 route 失败 fail-closed，online notify lookup / publish 失败 fail-open，并依赖 `PullInbox` 兜底。
- Redis route 已有 TTL 续期和后台 stale route cleanup；异常进程退出后 session route 仍依赖 TTL 过期，user route set 中的 stale 成员由 lookup / cleanup loop 移除。
- `push-gateway` 跨实例 resume buffer 尚未实现；跨实例恢复仍应 fallback `PullInbox`。
- `push-gateway` `/debug/metrics` 仍是本地 smoke 调试端点，不是正式 Prometheus 指标。
- 真实生产部署还未接入 Kubernetes / service discovery / mTLS / OTel。
