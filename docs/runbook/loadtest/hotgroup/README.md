# Hot Group Loadtest Plan

本目录用于规划热点群聊业务压测。当前已有 `loadtest/hotgroup` v0.1 初版
runner，可生成用户模型 dry-run，并在完整本地服务栈可用时执行：

```text
CreateConversation -> batch CreateMemberChange(JOIN)
-> SendMessage
-> delivery membership projection / user_inbox fanout
-> sampled PullInbox / AckDelivery
```

v0.1 暂不覆盖 WebSocket notify storm；push-gateway 在线通知压力、慢连接和 Redis
route fault 仍是后续阶段。本文继续冻结场景、指标和面试口径，避免只用单接口 QPS
代替真实 IM 业务压测。

## 目标

热点群聊压测要证明端到端链路，而不是单个服务的孤立吞吐：

```text
CreateConversation / member seed
-> 多 sender SendMessage
-> message_outbox
-> Kafka conversation.timeline.events
-> delivery-service membership projection + user_inbox fanout
-> delivery_outbox / im.delivery.events
-> push-gateway online notify
-> PullInbox
-> AckDelivery
```

## 推荐场景

| 场景 | 目的 |
| --- | --- |
| medium group fanout | 100 / 1,000 成员，验证普通群 write fanout 和 end-to-end latency。 |
| hot sender burst | 多 sender 高频发送，观察 conversation seq、Kafka lag 和 outbox pending。 |
| online notify storm | 高在线比例 + 慢客户端，验证 push queue、slow eviction 和 PullInbox 兜底。 |
| member churn during send | 发送期间 join / leave / remove / role change，验证历史可见窗口。 |
| delivery outage recovery | 压测中停止 / 恢复 delivery worker，验证 projection 追平和幂等。 |
| push route fault | Redis / push-gateway 局部故障，验证在线通知可丢但 durable inbox 不丢。 |

## 必需指标

runner summary 至少输出：

```text
group_size
online_ratio
sender_count
message_rate
duration
send_success_count
send_error_count
send_p95_ms
send_p99_ms
timeline_lag_max
delivery_projection_lag_max
inbox_rows_created
inbox_rows_per_message
pull_visible_p95_ms
pull_visible_p99_ms
ack_p95_ms
ack_p99_ms
push_notify_received
push_notify_missed
slow_session_evicted
outbox_pending
dlq_count
postgres_pool_acquire_p95_ms
postgres_lock_wait_count
kafka_lag_max
```

原始大文件继续写入 `H:\NexusIM\loadtest-results`；仓库只保留低敏 summary 和报告。

## 当前 v0.1 runner

Dry-run 只生成用户模型和计划，适合先评审 group/user/device/session 建模：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --dry-run `
  --run-name hotgroup-review-dryrun `
  --group-size 100 `
  --sender-count 5 `
  --message-count 20 `
  --online-ratio 0.2 `
  --slow-client-ratio 0.05
```

输出：

```text
H:\NexusIM\loadtest-results\<run-name>\hotgroup-summary.json
H:\NexusIM\loadtest-results\<run-name>\hotgroup-report.md
H:\NexusIM\loadtest-results\<run-name>\users.jsonl
```

真实执行需要 conversation / message / delivery 主进程、message outbox relay、
delivery timeline consumer、PostgreSQL 和 Kafka 已启动：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --run-name hotgroup-smoke-100 `
  --pg-dsn postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable `
  --conversation-target 127.0.0.1:13096 `
  --message-target 127.0.0.1:13095 `
  --delivery-target 127.0.0.1:13097 `
  --group-size 100 `
  --sender-count 5 `
  --message-rate 10 `
  --duration 60s `
  --receiver-sample-count 10 `
  --cleanup
```

业务写入路径必须走公开 gRPC。PostgreSQL 只用于清理测试租户、等待异步投影和读取统计。

如果本轮要把 `delivery_outbox -> im.delivery.events` 也纳入通过条件，必须同时启动
`delivery-service outbox-relay`，并显式打开：

```powershell
  --require-delivery-outbox-drain
```

该开关会等待当前测试 conversation 的 `delivery_outbox PENDING` 行追平到 `0`。没有打开时，
runner 只证明 durable inbox / PullInbox / AckDelivery，不证明在线通知事件已经全部发布。

## 设计边界

- 不把热点群聊压测结果表述为生产 SLO。
- 不用固定字符串 toy endpoint 代替真实 IM 链路。
- 不用当前成员表回查历史可见性；必须经过 conversation / delivery 的成员窗口模型。
- 不让 push-gateway 拥有 durable inbox；在线通知缺口必须通过 PullInbox 恢复。
- 如果引入新中间件，先在架构文档和 middleware catalog 说明瓶颈、替代方案和 owner。

## 面试口径

热点群聊不是单接口 QPS 问题，而是 fanout、写放大、在线通知风暴和补拉追平问题。
NexusIM 的第一阶段策略是：

```text
消息事实只写一次；
delivery 异步 fanout；
push 只做轻量在线唤醒；
可靠恢复靠 durable inbox；
超大群优先考虑 fanout bucket / lazy inbox，再按瓶颈引入中间件。
```
