# Hot Group Loadtest Plan

本目录用于规划热点群聊业务压测。当前已有 `loadtest/hotgroup` runner，可生成
用户模型 dry-run，并在完整本地服务栈可用时执行：

```text
CreateConversation -> batch CreateMemberChange(JOIN)
-> SendMessage
-> delivery membership projection / user_inbox fanout 或 conversation signal
-> 可选 WebSocket conversation subscriber
-> sampled PullInbox / AckDelivery
```

本文继续冻结场景、指标和面试口径，避免只用单接口 QPS 代替真实 IM 业务压测。

## 最新压测记录

| 报告 | 结论 |
| --- | --- |
| `loadtest-report-20260628-hotgroup-relay-bottleneck.md` | 记录 delivery outbox relay 优化前后对比：旧 20 人群在 50/100/150 QPS 卡在 `delivery_outbox` drain；修正为 conversation-sharded relay 后，100 人群 50/100/150 QPS 均能在等待窗口内完成 `user_inbox` 和 `delivery_outbox` drain；200 QPS 暴露下一瓶颈已转移到 delivery timeline projection / `user_inbox` fanout。 |
| 2026-06-30 pre-commit diagnostic | 200 人 / 500 消息 / 16 sender 中等规模诊断曾暴露 `SEQUENCER_BLOCK` 后成员 JOIN 未接 timeline-service 的缺口；修复后 dirty-run 可完成 `BROADCAST_SIGNAL`，`delivery_outbox_pending=0`、Kafka lag=0。正式报告必须用 clean commit 重跑。 |
| `loadtest-report-20260630-hotgroup-clean-redeploy.md` | clean commit `d13bff6c` 重建 / redeploy 后，61 人 / 20 消息、200 人 / 500 消息、500 人 / 1000 消息三档均通过；最大档产生 50000 条 conversation signal，`user_inbox_rows=0`、`delivery_outbox_pending=0`、Kafka lag=0。 |
| `loadtest-report-20260630-hotgroup-message-outbox-relay.md` | message-service outbox relay 已支持 conversation-sharded multi-worker batch publish；1000 人 / 4000 消息 / 800 msg/s 通过，message / delivery outbox 均无积压；随后 clean commit `01b2a70` 完成 READ_FANOUT 6000 人 / 100 subscriber 阶梯复压，最高目标 8000 msg/s、500000 条 conversation signal，outbox / Kafka 均无积压。 |
| `hotgroup-analysis-20260630-readfanout-clean.md` | 由 `tools/analyze-hotgroup-loadtest.ps1` 自动汇总 clean commit `01b2a70` 的 6 档 READ_FANOUT 结果；当前分类为 `online-signal-drain`，证据是 outbox / Kafka 已追平但 500000 条 signal 最慢读完约 176s。 |
| `hotgroup-metrics-window-20260630-readfanout-clean-8000qps.md` | 由 `tools/record-hotgroup-metrics-window.ps1` 采集最高档 Prometheus 时间窗口；核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，`delivery_outbox_pending` 峰值 2258 后归零，push writer / Redis route 指标有数据，slow eviction 为 0。 |
| `hotgroup-analysis-20260701-readfanout-subscriber-step.md` | clean commit `7bff4f3` 的 200 subscriber 阶梯与上一轮 100 subscriber 对比：同为 6000 人 / 5000 消息 / 8000 msg/s，signal 从 500000 增至 1000000，最慢 drain 从 176.554s 增至 349.903s，drain rate 约 2.86k signals/s，继续分类为 `online-signal-drain`。 |
| `hotgroup-metrics-window-20260701-readfanout-200sub.md` | 200 subscriber run 的 Prometheus 低敏窗口：核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，`delivery_outbox_pending` 峰值 2233 后归零，push connected sessions 达到 200，slow eviction 为 0。 |
| `hotgroup-metrics-window-20260701-readfanout-400sub.md` | clean commit `233d695` 的 400 subscriber run 继续通过：6000 人 / 5000 消息 / 8000 msg/s 产生 2000000 条 signal，最慢 drain 704.631s，drain rate 约 2.84k signals/s；Prometheus 窗口内核心 target up、`delivery_outbox_pending` 峰值 2284 后归零、push connected sessions 达到 400、slow eviction 为 0。 |
| `hotgroup-metrics-window-20260701-readfanout-400sub.md` push attribution update | 同一窗口已补 WebSocket writer / Redis route per-event 归因：整窗口 `frame_write_success` 约 200.97 万、`delivery_notify_write_success` 约 200.89 万、`redis subscriber_enqueued` 约 200.89 万，writer / delivery notify / Redis subscriber error 与 eviction 均为 0；下一步瓶颈定位应聚焦写出 / 读取 drain 能力，而不是 Redis 路由失败或 WebSocket 写失败。 |
| `hotgroup-multirunner-analysis-20260701-400sub.md` | clean commit `9e7d4f9` 的 4 runner shard 对照：coordinator 发送 6000 人 / 1000 消息 / 8000 msg/s，4 个 `subscriber-only` shard 共 400 subscriber 读取 400000 条 signal；按首帧到末帧计算总 drain rate 约 2852 signals/s，与单 runner 400 subscriber baseline 约 2840 signals/s 基本一致，说明瓶颈不只是单个 runner JSON decode / accounting。 |
| `hotgroup-push-fanout-optimization-20260701.md` | 第一轮 push-gateway online signal drain 代码级优化记录：memory registry fanout 改为锁内快照、锁外写出，queue full 时精确回锁驱逐仍注册 session；clean commit `4bc4a30` redeploy 后 400 subscriber + 4 shard 复压显示 drain rate 约 2891.8 signals/s，仅比单 runner baseline 约 2839.888 signals/s 高约 1.8%，瓶颈未迁移。 |
| `hotgroup-multirunner-analysis-20260701-pushfanout-400sub.md` | clean commit `4bc4a30` 的 registry fanout 快照优化复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，message / delivery outbox pending=0，400000 条 signal 全部读完，当前瓶颈仍是 `online-signal-drain`。 |
| `hotgroup-metrics-window-20260701-pushfanout-clean-400sub.md` | registry fanout 快照优化复压的 Prometheus 窗口：核心 target up，`delivery_outbox_pending` 峰值 140 后归零，push connected sessions 达到 400，writer / Redis subscriber error 和 eviction 均为 0。 |

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

## 自动分析工具

每轮正式压测后先用离线分析器把多个 `hotgroup-summary.json` 汇总成低敏报告：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\analyze-hotgroup-loadtest.ps1 `
  -RunNamePattern hotgroup-readfanout-6000-*clean-01b2a70e-* `
  -OutputPath docs\runbook\loadtest\hotgroup\hotgroup-analysis-20260630-readfanout-clean.md `
  -RequireCleanCommit
```

分析器只读取 `H:\NexusIM\loadtest-results` 下的原始 summary，不改原始数据。
它会输出 run matrix、clean / dirty 状态、SendMessage / PullInbox 延迟、outbox pending、
conversation signal drain、瓶颈分类和下一步策略。

多 runner 对照需要用专用分析器把一个 coordinator 和多个 subscriber shard 合成同一轮报告：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\analyze-hotgroup-multirunner.ps1 `
  -CoordinatorRunName hotgroup-multirunner-400sub-coordinator-20260701-013557 `
  -ShardRunNamePattern 'hotgroup-multirunner-400sub-shard*-20260701-013557' `
  -BaselineRunName hotgroup-readfanout-6000-8000qps-400sub-233d6956-20260701-004948 `
  -OutputPath docs\runbook\loadtest\hotgroup\hotgroup-multirunner-analysis-20260701-400sub.md
```

该报告会用首帧到末帧的 signal span 计算 drain rate，避免把 subscriber-only 提前等待
coordinator 建群 / 加成员的时间误算为在线信号写出瓶颈。

当前瓶颈分类规则保持保守：

```text
send errors / high p99         -> send path
message_outbox pending / DLQ   -> message outbox relay
delivery_outbox pending / DLQ  -> delivery outbox relay
PullInbox / ACK error or slow  -> receiver read / ack
subscriber incomplete / error  -> push subscribe / read path
outbox 追平但 signal drain 长  -> online signal drain
缺少信号或 lag 字段            -> insufficient observability
```

该报告只能作为本地 / 三机压测诊断材料，不能单独替代 Grafana / Prometheus 时间窗口，
也不能写成生产 SLO。

## 多 Runner 读取验证

400 subscriber 阶梯已证明 Redis route / WebSocket writer 没有错误或 eviction，但
`online-signal-drain` 仍稳定在约 2.8k signals/s。随后 4 个 `subscriber-only`
runner shard 的对照 run 也没有把总 drain rate 提升到新的量级，因此当前不能继续把瓶颈
简单归因到单个 runner 的 JSON decode / accounting。

下一步不要继续只增大 `--conversation-subscriber-count`；应进入 push-gateway
conversation signal 写出路径、WebSocket flush cadence、Redis subscriber fanout、
per-connection write scheduling 和有线网络吞吐的代码级定位。
第一轮代码优化已经处理本地 memory registry fanout 的锁持有范围；clean commit
`4bc4a30` 复压后 drain rate 仍在约 2.89k signals/s，说明 registry global mutex
不是主瓶颈。当前第二轮优化改为在 registry fanout 时预编码 delivery /
conversation notify JSON，让 WebSocket writer 优先写 cached payload，避免同一条
热点 signal 在每个 connection 写出前重复 marshal。该优化仍需 clean commit
镜像 redeploy 后用 coordinator + subscriber shard 复压确认。

`loadtest/hotgroup` 支持两个运行模式：

```text
--runner-mode full             # 默认模式：建群、加成员、发消息、等待投影、可选订阅、Pull/Ack 抽样
--runner-mode subscriber-only  # 只按 deterministic 用户模型打开 WebSocket subscriber 并等待 signal
```

多 runner 运行时，先启动多个 `subscriber-only` 进程，使用相同 tenant /
conversation / group / sender / message_count，并通过 shard 参数拆分订阅者：

```powershell
# shard 0/4 示例；其它机器改 --subscriber-shard-index 为 1、2、3
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --runner-mode subscriber-only `
  --run-name hotgroup-readfanout-6000-8000qps-400sub-shard0 `
  --tenant-id tenant-shared `
  --conversation-id conv-shared `
  --group-size 6000 `
  --sender-count 256 `
  --message-count 5000 `
  --conversation-subscriber-count 400 `
  --subscriber-shard-count 4 `
  --subscriber-shard-index 0 `
  --push-url ws://172.31.50.2:10498/ws `
  --require-conversation-notify `
  --wait-timeout 25m
```

所有 shard 都连上后，再启动一个 coordinator，不打开本地 subscriber，只负责建群和发消息：

```powershell
go run .\loadtest\hotgroup `
  --runner-mode full `
  --run-name hotgroup-readfanout-6000-8000qps-coordinator `
  --conversation-target 172.31.50.2:10496 `
  --message-target 172.31.50.2:10495 `
  --delivery-target 172.31.50.2:10497 `
  --pg-dsn "postgres://nexusim:nexusim@172.31.50.2:5432/nexusim?sslmode=disable" `
  --tenant-id tenant-shared `
  --conversation-id conv-shared `
  --group-size 6000 `
  --sender-count 256 `
  --message-rate 8000 `
  --message-count 5000 `
  --conversation-subscriber-count 0 `
  --receiver-sample-count 20 `
  --expect-fanout-mode READ_FANOUT `
  --require-delivery-outbox-drain `
  --cleanup
```

多 runner 的每个 shard 都会输出自己的 `hotgroup-summary.json`，其中
`push.subscriber_total_count` 是总订阅目标，`push.subscriber_count` 是本 shard
实际连接数，`push.subscriber_shard_index/count` 记录分片身份。正式报告需要把所有 shard
的 signal 总数、最慢 `last_signal_after_ms`、error / eviction 与 coordinator 的
SendMessage / PullInbox / outbox 指标一起记录。

每轮正式压测还必须记录至少一个 Prometheus / Grafana 或 debug metrics 时间窗口。
如果使用 Prometheus，可用窗口记录工具从 H 盘原始目录生成低敏报告：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\record-hotgroup-metrics-window.ps1 `
  -ResultDir H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336 `
  -MarkdownPath docs\runbook\loadtest\hotgroup\hotgroup-metrics-window-20260630-readfanout-clean-8000qps.md
```

工具会把原始 Prometheus JSON 写回对应 `H:\NexusIM\loadtest-results\<run>`，
仓库只保存低敏 Markdown 摘要。窗口报告仍不是生产 SLO，只用于解释该轮压测
是否有核心 target、outbox、projection、push writer、Redis route 和 PostgreSQL pool 指标。
`_5m` 指标表示移动五分钟压力窗口，`_window` 指标表示整个捕获窗口内的近似累计值；
后续定位 online signal drain 时必须同时记录 writer success/error、Redis subscriber enqueue/error、
session eviction 和 runner 侧 subscriber 完成时间。

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
push signal writer flush / client observed gap 的趋势化 dashboard
```

2026-06-30 后续代码已补齐第一阶段 push signal 观测字段：

```text
push-gateway /metrics:
  nexusim_push_gateway_ws_writer_events_total
  nexusim_push_gateway_ws_writer_last_event_unix_milliseconds

loadtest/hotgroup summary:
  push.subscriber_signals[].signal_count
  push.subscriber_signals[].max_conversation_seq
  push.subscriber_signals[].first_signal_after_ms
  push.subscriber_signals[].last_signal_after_ms
  push.subscriber_signals[].completed
  push.subscriber_signals[].error
```

下一轮三机压测必须使用包含这些字段的最新镜像 / runner；否则无法区分 WebSocket writer
未写出、客户端读取慢、session queue 压力或 runner accounting 问题。

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
