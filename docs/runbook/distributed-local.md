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

当前系统级分布式 smoke 原始结果：

```text
H:\NexusIM\loadtest-results\nexusim-distributed-smoke-redis-cleanup-20260609-193331\pushgateway-summary.json
```

该 run 使用 clean commit `29b8cc6`，`git_dirty=false`，WebSocket gateway 与 delivery consumer gateway 分离，通过 Redis route 收到 `delivery.notify`，随后 `PullInbox` 返回 1 条 inbox，`delivery.ack.ok` 推进 cursor 到 seq `2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

当前 Redis route 故障 smoke 原始结果：

```text
H:\NexusIM\loadtest-results\push-gateway-redis-fault-smoke-20260609-195200\pushgateway-summary.json
```

该 run 使用 clean commit `074902b`，`git_dirty=false`。runner 在 WebSocket session 已注册后停止 `nexusim-redis`，随后发送一条消息；在线 `delivery.notify` 不作为成功条件，客户端通过 `PullInbox` 拉到 seq `2`，恢复 Redis 后重连并通过 `delivery.ack.ok` 推进 cursor 到 seq `2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

## 5. Win/Mac 双机分布式模拟计划

当前直连网络规划：

```text
Windows wired: 172.31.50.1/24
Mac wired:     172.31.50.2/24
Wi-Fi:         两端继续用于上网和下载依赖
```

后续双机 smoke / 小压测优先使用 `172.31.50.*`，避免走随身 Wi-Fi 的 `192.168.0.*` 管理网段。

用户当前希望用两台机器模拟多节点，而不是继续做重型单机硬件矩阵。建议资源切分：

| 机器 | 模拟节点 | 建议资源上限 | 初始用途 |
| --- | --- | --- | --- |
| Windows | `win-node-a` | 4 CPU / 8GB | PostgreSQL、Kafka、Redis、message/conversation 核心写链路 |
| Windows | `win-node-b` | 4 CPU / 8GB | delivery-service、push-gateway consumer、局部压测器 |
| Mac | `mac-node-a` | 4 CPU / 4GB | push-gateway WebSocket 实例、Redis route 跨机验证 |
| Mac | `mac-node-b` | 4 CPU / 4GB | 轻量 load generator / 第二 gateway 实例 |

第一阶段不需要真的把每个服务都拆进独立容器。更稳的顺序是：

1. 先在 Mac 配好 Docker / Go / repo 路径，确认 `docker version`、`go test ./...` 或必要 smoke runner 可用。
2. 在 Windows 保留 PostgreSQL / Kafka / Redis，Mac 运行一个或两个 `push-gateway` 实例，使用 `172.31.50.1` 访问 Windows 基础设施。
3. 跑跨机器 route smoke：Windows consumer gateway -> Redis route -> Mac WebSocket gateway，验证 `delivery.notify -> PullInbox -> ACK`。
4. 再考虑用 Docker Compose profiles / resource limits 把服务按 `win-node-a/win-node-b/mac-node-a/mac-node-b` 分组。

当前 Mac 准备状态：

```text
172.31.50.2:22 reachable
192.168.0.182:22 reachable
Windows -> Mac SSH key auth: not accepted yet
```

下一步需要在 Mac 的 `/Users/qsyy0921/.ssh/authorized_keys` 加入 Windows 当前公钥。Windows 侧可用：

```powershell
Get-Content $env:USERPROFILE\.ssh\id_ed25519.pub
```

免密恢复后再在 Mac 上验证：

```bash
docker version
docker info
```

压测原始结果继续放机械盘：

```text
H:\NexusIM\loadtest-results
```

仓库内只保存 Markdown 报告、脚本和小体积 summary 链接。

## 6. 面试讲法

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

## 7. 已知缺口

- Redis route 已做一次真实 stop/start fault smoke，证明 online notify 可丢但 `PullInbox + AckDelivery` 可恢复；这仍不是 Redis HA、Sentinel、Cluster 或网络分区结论。
- Redis route 已有 TTL 续期和后台 stale route cleanup；异常进程退出后 session route 仍依赖 TTL 过期，user route set 中的 stale 成员由 lookup / cleanup loop 移除。
- `push-gateway` 跨实例 resume buffer 尚未实现；跨实例恢复仍应 fallback `PullInbox`。
- `push-gateway` `/debug/metrics` 仍是本地 smoke 调试端点，不是正式 Prometheus 指标。
- 真实生产部署还未接入 Kubernetes / service discovery / mTLS / OTel。
- Mac Docker / 双机 Docker Compose profile 尚未完成配置和验证；当前阻塞是 Windows -> Mac SSH 免密未恢复，两个地址的 22 端口均可达。
