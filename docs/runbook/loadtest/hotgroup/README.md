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

## 最新压测记录

| 报告 | 结论 |
| --- | --- |
| `loadtest-report-20260628-hotgroup-relay-bottleneck.md` | 记录 delivery outbox relay 优化前后对比：旧 20 人群在 50/100/150 QPS 卡在 `delivery_outbox` drain；修正为 conversation-sharded relay 后，100 人群 50/100/150 QPS 均能在等待窗口内完成 `user_inbox` 和 `delivery_outbox` drain；200 QPS 暴露下一瓶颈已转移到 delivery timeline projection / `user_inbox` fanout。 |
| 2026-06-30 pre-commit diagnostic | 200 人 / 500 消息 / 16 sender 中等规模诊断曾暴露 `SEQUENCER_BLOCK` 后成员 JOIN 未接 timeline-service 的缺口；修复后 dirty-run 可完成 `BROADCAST_SIGNAL`，`delivery_outbox_pending=0`、Kafka lag=0。正式报告必须用 clean commit 重跑。 |

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
| medium group fanout | 100 成员验证小群 `WRITE_FANOUT`；1,000 成员验证自动 promotion 到 `HYBRID_FANOUT` 和 end-to-end latency。 |
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

## 可视化要求

热点群聊压测必须配套趋势图。仓库已提供 first-stage Grafana dashboard：

```text
deploy/local/grafana/dashboards/hotgroup-observability.json
```

Grafana 通过 `deploy/local/docker-compose.grafana.yml` 自动加载该 dashboard，标题为
`NexusIM Hot Group Loadtest`。正式压测报告至少要记录：

当前三机本地压测约定端口：

```text
Kafka UI:    http://172.31.50.2:19090
Prometheus:  http://172.31.50.2:19091
Grafana:     http://172.31.50.2:13000
OTel gRPC:   172.31.50.2:14317
OTel HTTP:   http://172.31.50.2:14318
OTel health: http://172.31.50.2:14333
```

`19090` 固定留给 Kafka UI；Prometheus 使用 `19091`，避免压测时把两个入口混淆。

```text
dashboard_name
prometheus_time_range
run_name
commit
group_size
fanout_mode / expected_fanout_mode
SendMessage p95 / p99 趋势
message_outbox / delivery_outbox pending 趋势
delivery projection failure / worker error 趋势
user_inbox / membership projection 增长趋势
PullInbox / AckDelivery gRPC 请求和延迟趋势
push session / slow eviction 趋势
PostgreSQL pool 趋势
```

当前 dashboard 只使用已有 `/metrics` 指标，不假装拥有尚未实现的 Kafka consumer lag
exporter 或 fanout-mode distribution exporter。`fanout_mode / expected_fanout_mode`
在 runner summary 和 PostgreSQL 统计中校验；后续再沉淀成 Prometheus exporter。
缺口仍需后续补：

```text
conversation fanout mode distribution
message timeline topic lag
delivery timeline consumer lag by topic / partition / group
delivery_timeline_items count / insert rate
user_inbox rows per message
PostgreSQL lock / WAL / dead tuple time-series exporter
```

没有 Grafana / Prometheus 趋势图的运行，只能作为功能 smoke、dry-run 或一次性
diagnostics，不能写成热点群聊容量证明。

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
  --expect-fanout-mode WRITE_FANOUT `
  --cleanup
```

业务写入路径必须走公开 gRPC。PostgreSQL 只用于清理测试租户、等待异步投影和读取统计。

conversation-service 会在成员变更事务内按 ACTIVE 成员数做单调 promotion：

```text
<=500      WRITE_FANOUT
501-5000   HYBRID_FANOUT
5001-50000 READ_FANOUT
>50000     BROADCAST_SIGNAL / SEQUENCER_BLOCK active first-stage
```

promotion 只向更高版本推进，不因为成员离开自动降级，避免压测中策略反复震荡。
当前热点 `SEQUENCER_BLOCK` 已不是 contract-only：消息写入和成员边界事件都必须通过
timeline-service `AllocateSeqBlock` 获取 valid lease。未配置 sequencer、lease 无效或
lease 过期时 fail-closed，不能回退到本地 row lock，否则压测结果会把热点路径误测成小群路径。
hotgroup runner 的结果必须记录实际 `fanout_mode`，否则不能解释不同规模下的
写放大和延迟曲线。
如果传入 `--expect-fanout-mode`，runner 会在发送前校验 conversation 当前策略；
不匹配时 fail-closed，避免把小群 `WRITE_FANOUT` 结果误写成中大群 fanout 结论。

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
