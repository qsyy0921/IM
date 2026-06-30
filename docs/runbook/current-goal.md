# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

Hot group pressure step-up and bottleneck curve：在 clean commit Docker redeploy、
`SEQUENCER_BLOCK + BROADCAST_SIGNAL` 三档复验和 READ_FANOUT 6000 人阶梯复压基础上，
继续扩大 subscriber 数、总 signal 数和在线读取侧瓶颈曲线。

## 当前已收口摘要

- conversation-service 已按群规模输出 fanout / conversation mode：direct / small group
  使用 `WRITE_FANOUT`，medium / large 使用 first-stage `HYBRID_FANOUT` /
  `READ_FANOUT`，hot group 使用 `BROADCAST_SIGNAL + SEQUENCER_BLOCK`。
- conversation-service 在 `SEQUENCER_BLOCK` 会话中已不再把成员 JOIN / LEAVE /
  REMOVE / owner transfer 当成本地 row-lock 边界处理；成员边界 seq 通过
  timeline-service `AllocateSeqBlock` 分配，拿不到 valid lease 时 fail-closed，不回退。
- timeline-service 已进入本地运行链路，提供 `seq-block-allocator` runtime、PostgreSQL
  sequence state / lease 表和 `AllocateSeqBlock` gRPC API。
- message-service 已在 `SEQUENCER_BLOCK` active 写路径调用 timeline-service
  `AllocateSeqBlock`；本轮新增本地 seq block cache、lease safety margin 和 lease
  metadata 写入 / 校验。拿不到 valid lease、lease 过期或 epoch / lease_id 缺失时
  fail-closed，不回退到本地 row lock。
- timeline-service 已新增 lease 状态、显式 gap marker 表和 repair operator modes：
  `seq-lease-expire`、`gap-marker-create`、`gap-marker-close`、`gap-marker-audit`。
  repair 只操作 timeline-service 自有表，不读 message / conversation 私有表。
- delivery-service 已支持 `WRITE_FANOUT` / `HYBRID_FANOUT` / `READ_FANOUT` /
  conversation-level signal；materialized `user_inbox` 已改为批量 insert；
  `timeline-consumer` 支持同 consumer group 多 worker 按 Kafka partition 安全并行。
- push-gateway 已支持 `conversation.subscribe / unsubscribe` 和 conversation signal
  fanout first path。
- 2026-06-29 小规模热点群 smoke 已跑通：61 人群、20 条消息、`SEQUENCER_BLOCK +
  BROADCAST_SIGNAL`，3 个 WebSocket conversation subscriber 收到 60 条 signal，
  `send_p95_ms=19.03`、`user_inbox_rows=0`、`delivery_outbox_pending=0`、无 message /
  delivery DLQ。原始目录：
  `H:\NexusIM\loadtest-results\hotgroup-broadcast-push-smoke-20260629-2135`。
- 2026-06-30 已用 clean commit `d13bff6c` 重建并 redeploy conversation-service 镜像，
  归档到 `H:\NexusIM\docker-images\archives\nexusim-conversation-service-d13bff6c-20260630-002306.tar`。
  三档复验均通过：
  - 61 人 / 20 消息 / 3 subscriber；
  - 200 人 / 500 消息 / 20 subscriber；
  - 500 人 / 1000 消息 / 50 subscriber。
  最大一档产生 50000 条 conversation signal，`send_p95_ms=10.633`、`send_p99_ms=13.013`、
  `user_inbox_rows=0`、`delivery_outbox_pending=0`、Kafka lag=0。低敏报告见
  `docs/runbook/loadtest/hotgroup/loadtest-report-20260630-hotgroup-clean-redeploy.md`。
- 2026-06-30 已用 clean commit `0a1395c` 优化 message-service outbox relay：
  4 worker conversation-sharded batch publish、批量 mark `PUBLISHED`、同 conversation
  失败后续 ready row 保持 pending、`message_outbox` conversation/version ready query indexes。
  镜像已归档到
  `H:\NexusIM\docker-images\archives\nexusim-message-service-0a1395c3-20260630-125317.tar`，
  并已 redeploy 到 Ubuntu Docker。
- message outbox relay 复验结果：
  - 1000 人 / 2000 消息 / 400 msg/s / 100 subscriber：通过，conversation signal=200000，
    message / delivery outbox pending=0；
  - 1000 人 / 4000 消息 / 800 msg/s / 100 subscriber：通过，conversation signal=400000，
    message / delivery outbox pending=0；
  - 1000 人 / 8000 消息 / 1200 msg/s / 150 subscriber、2000 人 / 8000 消息 /
    1500 msg/s / 200 subscriber：SendMessage / message outbox / delivery projection /
    delivery outbox / Kafka consumer lag 均追平，但 runner 等待 conversation signal 超时。
  报告见
  `docs/runbook/loadtest/hotgroup/loadtest-report-20260630-hotgroup-message-outbox-relay.md`。

## 目标

- 本轮已补并 redeploy push-gateway conversation signal 写出 / runner 读取观测代码：
  push-gateway WebSocket writer 暴露 frame write / delivery.notify write /
  resume_hint write 低敏指标；`loadtest/hotgroup` 报告每个 conversation subscriber
  的首帧、末帧、signal 数、max seq 和 read error。
- 2026-06-30 已用 clean commit `01b2a70` 重建 / redeploy delivery-service，并完成
  READ_FANOUT 阶梯复压：6000 人、100 subscriber 下，目标 400 / 800 / 1200 /
  2000 / 4000 / 8000 msg/s 档位均通过；最高档 5000 条消息、500000 条
  conversation signal、`send_p95_ms=18.54`、`send_p99_ms=22.41`、PullInbox p95
  `26.93ms`、`user_inbox_rows=0`、`delivery_outbox_pending=0`、Kafka lag=0。
  这证明当前瓶颈不在 SendMessage、message outbox、delivery projection、delivery outbox
  或 Kafka consumer；需要继续观察 online signal drain 和压测端读取能力。
- 已新增 `tools/analyze-hotgroup-loadtest.ps1`，用于离线汇总 H 盘
  `hotgroup-summary.json`，生成 run matrix、瓶颈分类和下一步策略。当前对 clean commit
  `01b2a70` 的 6 档 READ_FANOUT 结果生成了
  `docs/runbook/loadtest/hotgroup/hotgroup-analysis-20260630-readfanout-clean.md`，
  分类为 `online-signal-drain`：outbox / Kafka 追平，但 500000 条 signal 最慢读完约
  176s。该报告是诊断材料，不替代 Grafana / Prometheus 时间窗口。
- 已新增 `tools/record-hotgroup-metrics-window.ps1`，并对最高档
  `hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336` 采集
  Prometheus 时间窗口。原始 JSON 写入
  `H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336\hotgroup-prometheus-window.json`，
  仓库低敏报告见
  `docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260630-readfanout-clean-8000qps.md`。
  该窗口显示核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，
  `delivery_outbox_pending` 峰值 2258 后归零，push writer / Redis route 指标有数据，
  slow eviction 为 0。
- 2026-07-01 已完成 200 subscriber 阶梯复压：
  `hotgroup-readfanout-6000-8000qps-200sub-7bff4f38-20260701-002833`，
  clean commit `7bff4f3`、6000 人、5000 条消息、目标 8000 msg/s、256 sender、
  200 subscriber、READ_FANOUT。该 run 产生 1000000 条 conversation signal，
  `send_p95_ms=18.315`、`send_p99_ms=21.808`、`PullInbox p95=26.326ms`、
  `message_outbox_pending=0`、`delivery_outbox_pending=0`，所有 subscriber 读完。
  Prometheus 窗口显示核心 4 个 scrape target 全部 up，`delivery_outbox_pending`
  峰值 2233 后归零，push connected sessions 达到 200，slow eviction 为 0。
  与上一轮 100 subscriber / 500000 signal / 176.554s drain 对比，200 subscriber /
  1000000 signal 最慢 drain 为 349.903s，drain rate 约 2857.934 signals/s，
  继续证明当前瓶颈是 `online-signal-drain` 线性放大，而不是 SendMessage、
  outbox、delivery projection 或 Kafka。
- 2026-07-01 继续完成 400 subscriber 阶梯：
  `hotgroup-readfanout-6000-8000qps-400sub-233d6956-20260701-004948`，
  clean commit `233d695`、6000 人、5000 条消息、目标 8000 msg/s、256 sender、
  400 subscriber、READ_FANOUT。该 run 产生 2000000 条 conversation signal，
  `send_p95_ms=19.724`、`send_p99_ms=25.668`、`PullInbox p95=25.341ms`、
  `message_outbox_pending=0`、`delivery_outbox_pending=0`，400 个 subscriber
  全部读完。Prometheus 窗口显示核心 4 个 scrape target 全部 up，
  `delivery_outbox_pending` 峰值 2284 后归零，push connected sessions 达到 400，
  slow eviction 为 0。与 100 / 200 subscriber 档对比，drain rate 分别约
  2831.995 / 2857.934 / 2838.365 signals/s，说明继续呈线性 online signal drain，
  当前不应优先优化 message / delivery outbox 或 Kafka。
- 2026-07-01 已增强 `tools/record-hotgroup-metrics-window.ps1` 的 push attribution：
  同一 400 subscriber 窗口现在记录 WebSocket writer 和 Redis route 的 per-event
  五分钟峰值与整窗口计数。最新报告显示整窗口 `frame_write_success` 约 2009692、
  `delivery_notify_write_success` 约 2008889、`redis subscriber_enqueued` 约 2008889，
  writer error、delivery notify error、subscriber evicted / error 均为 0。该证据把
  当前瓶颈进一步收窄为 online signal 写出 / 客户端读取 drain 速度，而不是 Redis
  路由失败、WebSocket 写失败或 session eviction。
- 2026-07-01 已为 `loadtest/hotgroup` 增加多 runner 读取验证能力：
  `--runner-mode subscriber-only` 只负责打开 WebSocket conversation subscribers 并等待
  signal；`--subscriber-shard-count/index` 将同一 deterministic receiver 列表拆给多个
  runner 进程。后续可用一个 coordinator 负责建群 / 发消息，多个 subscriber-only
  runner 分散在 Windows / Ubuntu / Mac 上读取同一 conversation signal，从而判断单
  runner JSON decode / accounting 是否限制 drain rate。
- 2026-07-01 已完成一轮多 runner 对照验证：
  `hotgroup-multirunner-400sub-coordinator-20260701-013557` + 4 个
  `subscriber-only` shard，clean commit `9e7d4f9`，6000 人、1000 消息、目标
  8000 msg/s、400 subscriber、READ_FANOUT。coordinator `send_p95_ms=19.732`、
  `send_p99_ms=22.501`、`PullInbox p95=124.681ms`，message / delivery outbox
  pending=0；4 个 shard 共读完 400000 条 signal，按首帧到末帧计算
  drain rate 约 2852.227 signals/s。与单 runner 400 subscriber baseline
  约 2839.888 signals/s 基本一致，说明瓶颈不只是单 runner JSON decode /
  accounting。低敏报告见
  `docs/runbook/loadtest/hotgroup/hotgroup-multirunner-analysis-20260701-400sub.md`。
- 2026-07-01 已实现第一轮 push-gateway online signal drain 代码级优化：
  本地 memory registry 的 user / conversation fanout 改成“锁内快照、锁外写出”，
  queue full 时再精确回锁驱逐仍然注册的同一 session。该改动降低热点 signal fanout
  持有全局 registry mutex 的时间，不改变 durable PullInbox 兜底和 slow-session
  fail-closed 语义。focused checks 已通过；下一步需要 clean commit 镜像重建 /
  redeploy 后复跑 400 subscriber coordinator + shard 对照，确认 drain rate 是否突破
  约 2.85k signals/s。记录见
  `docs/runbook/loadtest/hotgroup/hotgroup-push-fanout-optimization-20260701.md`。
- 2026-07-01 已用 clean commit `4bc4a30` 重建 / redeploy push-gateway，并完成
  400 subscriber coordinator + 4 shard 复压：
  `hotgroup-pushfanout-clean-400sub-coordinator-20260701-022043`。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT；coordinator `send_p95_ms=17.929`、`send_p99_ms=19.618`、
  `PullInbox p95=18.825ms`，message / delivery outbox pending=0；4 个 shard
  共读完 400000 条 signal，drain rate 约 2891.8 signals/s。与单 runner
  400 subscriber baseline 约 2839.888 signals/s 相比仅约 1.8% 提升，说明
  registry lock hold time 不是主瓶颈。低敏报告见
  `docs/runbook/loadtest/hotgroup/hotgroup-multirunner-analysis-20260701-pushfanout-400sub.md`
  和
  `docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260701-pushfanout-clean-400sub.md`。
- 2026-07-01 已用 clean commit `d8d78fd` 重建 / redeploy push-gateway，并完成
  第二轮 online signal drain 优化复压：delivery / conversation notify 在 registry
  fanout 时预编码一次 JSON，WebSocket writer 优先写 cached payload，避免同一条
  热点 signal 被 400 个 connection 各自重复 `json.Marshal`。400 subscriber
  coordinator + 4 shard 对照 run 为
  `hotgroup-pushpreenc-clean-400sub-coordinator-20260701-024044`；coordinator
  `send_p95_ms=18.769`、`send_p99_ms=20.644`、`PullInbox p95=113.882ms`，
  message / delivery outbox pending=0；4 个 shard 共读完 400000 条 signal，
  drain rate 约 `2863.092 signals/s`。与单 runner 400 subscriber baseline
  `2839.888 signals/s` 相比仅约 `0.8%` 提升，也低于上一轮 registry lock
  优化复压的 `2891.8 signals/s`。结论：重复 JSON marshal 也不是主瓶颈，瓶颈仍是
  online signal drain。
- 2026-07-01 已补下一轮 WebSocket 写出定位指标：push-gateway WebSocket writer
  现在记录 `frame_write` 和 `delivery_notify` 写耗时 histogram / sum / count / max，
  `tools/record-hotgroup-metrics-window.ps1` 会输出 delivery notify 写耗时 p95 / p99 /
  avg / max。该改动只增加低基数观测，不改变协议、fanout、durable inbox 或
  PullInbox / ACK 语义；focused tests / build 已通过。下一步需要 clean commit 镜像
  重建 / redeploy，并用同一 400 subscriber coordinator + shard 场景复压，确认每帧
  `conn.Write` 长尾是否解释 `online-signal-drain`。
- 2026-07-01 已用 clean commit `4f45519` 重建 / redeploy push-gateway，并完成
  writer duration 复压：
  `hotgroup-writerdur-clean-400sub-coordinator-20260701-031058`。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT；coordinator `send_p95_ms=18.655`、`send_p99_ms=21.62`、
  `PullInbox p95=119.884ms`，message / delivery outbox pending=0；4 个 shard
  共读完 400000 条 signal，drain rate 约 `2876.698 signals/s`，相对旧
  400 subscriber baseline 约 `2839.888 signals/s` 只高约 `1.3%`。Prometheus
  窗口显示 `delivery_notify` write p95 / p99 约 `0.345ms / 0.499ms`，avg 约
  `0.125ms`，max 约 `10.056ms`，writer / Redis subscriber error 和 eviction 均为 0。
  结论：`conn.Write` 单次调用长尾不是当前主瓶颈，下一步应定位 Redis subscriber
  收到 conversation signal 后的本地 fanout / enqueue 调度、per-session writer
  调度节奏、runner 读取背压和链路吞吐。
- 2026-07-01 已用 clean commit `6099ecd` 重建 / redeploy push-gateway，并完成
  Redis subscriber fanout duration 复压：
  `hotgroup-redisfanout-clean-400sub-coordinator-20260701-033606`。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT；coordinator `send_p95_ms=18.325`、`send_p99_ms=21.333`、
  `PullInbox p95=20.016ms`，message / delivery outbox pending=0；4 个 shard
  共读完 400000 条 signal，drain rate 约 `2883.976 signals/s`。Prometheus
  窗口显示 WebSocket `delivery_notify` write p95 / p99 约 `0.406ms / 0.63ms`，
  Redis subscriber conversation signal fanout/enqueue 整窗口 p95 / p99 约
  `56.14ms / 91.228ms`，5m last p95 / p99 约 `60.263ms / 92.053ms`，
  avg 约 `16.485ms`，max 约 `84.305ms`；writer / Redis subscriber error
  和 eviction 均为 0。结论：瓶颈已进一步收窄到 Redis subscriber 收到
  conversation signal 后对本机 400 个 session 的本地 fanout/enqueue 调度，而不是
  SendMessage、outbox、Kafka、registry mutex、重复 JSON marshal 或单次 WebSocket
  `conn.Write`。
- 2026-07-01 已用 clean commit `93654117` 重建 / 归档 / redeploy push-gateway，并完成
  Redis subscriber conversation signal worker / shard queue 复压：
  `hotgroup-signalqueue-clean-400sub-coordinator-20260701-041641`。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT；coordinator `send_p95_ms=18.417`、`send_p99_ms=20.639`、
  `PullInbox p95=115.093ms`，message / delivery outbox pending=0；4 个 shard
  共读完 400000 条 signal，drain rate 约 `2876.076 signals/s`。Prometheus 窗口显示
  queue handoff 正常：`subscriber_signal_fanout_queue_full=0`、`worker_error=0`、
  queue depth 峰值为 0，queue wait p95 / p99 约 `0.095ms / 0.099ms`；但 worker
  侧 conversation signal fanout p95 / p99 仍约 `38.636ms / 87.5ms`，WebSocket
  delivery notify write p95 / p99 约 `0.349ms / 0.495ms`。结论：Redis subscriber
  快速 handoff 已成立，但总 drain 曲线没有突破约 2.85k-2.89k signals/s；瓶颈仍在
  worker 对本机 400 session 的本地 fanout/enqueue、session writer 调度或客户端读取侧。
- HYBRID 诊断档位 1000 人 / 1000 消息 / 400 msg/s 暴露 `delivery_outbox` ready query
  在百万级 per-user outbox 下退化：旧 anti-join blocker 查询每批 500 行约 24s。当前
  delivery outbox relay 已改成 per-conversation frontier ready query，并把本地 worker
  数提高到 8；这是 first-stage 查询优化，不替代后续 fanout 策略 / frontier progress 表设计。
- 对每轮正式压测记录 run name、commit、dashboard 时间窗口、Kafka lag、delivery projection lag、
  push signal、PullInbox / ACK 追平和 PostgreSQL 关键指标。
- 本轮只写本地 / 三机实验结论，不写生产容量上限。

## 本轮完成条件

- push-focused step 的 READ_FANOUT clean commit 阶梯 run 已完成，并明确记录 signal
  写出 / 读取指标；自动分析报告和最高档 Prometheus 低敏时间窗口均已生成。
- 下一轮围绕 push-gateway worker 本地 fanout / session writer 调度做架构分析和优化：
  重点比较 per-conversation worker 数、per-session outbound queue drain、writer goroutine
  调度、WebSocket flush 策略和 runner 读取背压；继续使用 400 subscriber coordinator +
  shard 场景做可比复压。
- 文档同步本轮公开能力或瓶颈变化。
- 提交并推送到 GitHub。

## 非目标

- 不做长时间正式容量压测或生产 sizing。
- 不把 first-stage block cache / repair operator 说成完整 sequencer 分区系统。
- 不新增隐藏 fallback，不用本地 row lock 冒充热点 sequencer。
- 不继续扩完整产品级客户端。
- 不展开无关 Agent action boundary / repair cases。

## 后续优先级

1. 基于 clean commit `93654117` 的复压结果，继续定位 push-gateway worker 本地 fanout /
   session writer 调度：queue handoff 已不是瓶颈，下一轮不要继续调 Redis subscriber
   receive path；应分析 session queue、writer goroutine、flush / batching 和 runner
   读取侧对约 2.85k-2.89k signals/s 曲线的影响。
2. 继续为每轮优化保留 clean commit、Docker 镜像归档、三机部署版本和 Prometheus
   时间窗口，保证压测曲线可复现。
3. 若 HYBRID 仍要支持千人级 per-user materialized outbox，优先评估显式 frontier /
   progress 表或把策略提前切到 READ_FANOUT；不要把 Kafka / Redis 当成替代 fanout 策略。
4. delivery projection lag / inbox rows per message / push notify storm 指标深化。
5. timeline virtual partition mapping、leader ownership audit 和更完整 repair workflow。
6. 压测报告与面试叙事维护。
