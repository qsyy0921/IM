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
- 2026-07-01 已用 clean commit `fedb5f43` 重建 / 归档 / redeploy push-gateway，并完成
  WebSocket writer queue latency / batch drain 复压：
  `hotgroup-writerqueue-clean-400sub-coordinator-20260701-045022`。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT；coordinator `send_p95_ms=18.928`、`send_p99_ms=21.741`、
  `PullInbox p95=115.432ms`，message / delivery outbox pending=0；4 个 shard
  共读完 400000 条 signal，drain rate 约 `2884.066 signals/s`。Prometheus 窗口显示
  WebSocket `delivery_notify` queue p95 / p99 约 `4.665ms / 4.942ms`，
  write p95 / p99 约 `0.383ms / 0.587ms`，但 worker fanout p95 / p99 仍约
  `57.759ms / 92.241ms`。结论：writer queue wait 和单次 `conn.Write`
  不是主瓶颈，下一步应评估 conversation-local fanout buckets，让同一 conversation
  的在线 subscriber 按稳定 bucket 并行 fanout，同时保持每个 session 内信号顺序。
- 2026-07-01 已实现 conversation-local fanout buckets：push-gateway memory registry
  在 `EnqueueConversationSignal` 中保持锁内快照 / seen / resume buffer 语义，锁外按
  stable `session_id` bucket 并行写 session outbound queue；外层 Redis subscriber
  仍保持同 conversation 顺序，queue full 仍显式 slow-session eviction。新增
  `NEXUSIM_PUSH_CONVERSATION_FANOUT_BUCKETS`，本地 Docker `push-gateway-ws` 设为 8。
  focused push-gateway / hotgroup tests 和 build 已通过；clean commit 复压结果见下一条。
- 2026-07-01 已用 clean commit `a15e0ad` 重建 / 归档 / redeploy push-gateway，并完成
  conversation-local fanout buckets 复压：
  `hotgroup-fanoutbuckets-clean-400sub-coordinator-20260701-053403`。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、400 subscriber、READ_FANOUT；
  coordinator `send_p95_ms=19.288`、`send_p99_ms=22.026`、`PullInbox p95=128.099ms`，
  message / delivery outbox pending=0；4 个 shard 共读完 400000 条 signal，
  drain rate 约 `2874.378 signals/s`。Prometheus 窗口显示 `delivery_notify`
  queue p95 / p99 约 `4.616ms / 4.931ms`，write p95 / p99 约
  `0.383ms / 0.574ms`，Redis subscriber conversation-signal fanout p95 / p99
  约 `54.133ms / 90.827ms`，queue wait p95 / p99 约 `0.095ms / 0.099ms`，
  writer / Redis subscriber error、queue-full 和 slow eviction 均为 0。结论：
  bucket 并行没有突破 `2.85k-2.89k signals/s` 区间；下一步不要继续在同一调用路径
  堆并发，而应评估持久 per-conversation / per-bucket worker、跨 push 实例分摊订阅，
  或对超大房间采用更激进的 pull-first 策略来减少在线 signal 总量。
- 2026-07-01 已用 clean commit `4be4b2d` 增加 4 个本地 push-gateway ws 实例拓扑，
  每个实例使用独立 `NEXUSIM_PUSH_GATEWAY_ID`、host ws/debug 端口，并让 Prometheus
  同时 scrape 4 个 push target。复压
  `hotgroup-multiws-clean-400sub-coordinator-20260701-055706` 将 400 个
  conversation subscriber 按 100 / 100 / 100 / 100 分配到 4 个 ws 端口。该 run 为
  6000 人、1000 消息、目标 8000 msg/s、256 sender、READ_FANOUT；coordinator
  `send_p95_ms=19.386`、`send_p99_ms=22.528`、`PullInbox p95=19.133ms`，
  message / delivery outbox pending=0；4 个 shard 共读完 400000 条 signal，
  drain rate 约 `2822.479 signals/s`，低于单 ws fanout-buckets baseline
  `2874.378 signals/s`。Prometheus 窗口显示 4 个 push target 均 up，
  `delivery_notify` queue p95 / p99 约 `3.703ms / 4.742ms`，write p95 / p99
  约 `0.425ms / 0.769ms`，Redis subscriber fanout p95 / p99 约
  `69.014ms / 93.803ms`，writer / Redis error、queue-full 和 slow eviction 均为 0。
  结论：简单把同一会话订阅者分散到多个 push-gateway ws 进程没有突破在线 drain
  曲线，当前不应把“多开容器”当成热点群主要优化；下一步需要改变 fanout 模型或减少
  超大房间在线 signal 总量。
- 2026-07-01 已用 clean commit `bac71c65` 完成显式 pull-first sampled online signal
  复压：push-gateway 支持 `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY`，默认 `1`
  保持全量 signal；本轮将 delivery-consumer 和 4 个 ws 实例统一配置为 `10`，
  并用 `loadtest/hotgroup --conversation-signal-sample-every 10` 校验。400 subscriber
  coordinator + 4 shard run
  `hotgroup-sample10-clean-400sub-coordinator-20260701-070655` 为 6000 人、
  1000 消息、目标 8000 msg/s、256 sender、READ_FANOUT；coordinator
  `send_p95_ms=17.835`、`send_p99_ms=22.003`、`PullInbox p95=122.69ms`，
  message / delivery outbox pending=0；4 个 shard 共读完 40000 条 signal，
  signal span 25.243s。对比 full-signal multi-ws baseline 的 400000 条 signal /
  141.719s，在线帧总量显著下降，durable PullInbox / ACK 仍成立。
- 2026-07-01 继续扩大 sample=10 的 message_count：run
  `hotgroup-sample10-400sub-5000msg-coordinator-20260701-072206` 使用 clean commit
  `f5bc0199`、服务镜像仍为 `bac71c65` sampled push-gateway，6000 人、5000 消息、
  目标 8000 msg/s、256 sender、400 subscriber、READ_FANOUT。coordinator
  `send_p95_ms=18.103`、`send_p99_ms=20.914`、`PullInbox p95=23.874ms`，
  message / delivery outbox pending=0；4 个 shard 共读完 200000 条 signal，
  span 138.555s，drain rate 约 `1443.474 signals/s`。Prometheus 窗口显示
  `delivery_outbox_pending` 峰值 1763 后归零，writer / Redis error、queue-full 和
  eviction 均为 0；`delivery_notify` write p95 / p99 约 `0.458ms / 0.87ms`，
  Redis subscriber conversation fanout p95 / p99 约 `54.541ms / 90.908ms`。
  结论：sample=10 能降低在线 frame 总量，但当消息数扩大到 5000 时仍呈
  online-signal-drain，且本地 Redis subscriber fanout/enqueue 继续是主要证据点。
- 2026-07-01 已实现 fanout-mode conversation signal policy 模块：push-gateway
  内部 `DeliveryNotification` 现在携带 `fanout_mode`，delivery consumer 从
  `delivery.conversation.signal.v1` 传入该字段，memory registry 和 Redis route
  registry 共用 `ConversationSignalPolicy`。新增显式 mode-specific env：
  `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_WRITE_FANOUT`、
  `..._HYBRID_FANOUT`、`..._READ_FANOUT`、`..._BROADCAST_SIGNAL`。未设置时使用
  `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY` 作为默认 policy；无效配置会启动失败。
  该模块不新增中间件、不改变 durable PullInbox / ACK 边界。
- 2026-07-01 已用 clean commit `37b575e5` 重建 / 归档 / redeploy push-gateway：
  `H:\NexusIM\docker-images\archives\nexusim-push-gateway-37b575e5-20260701-083327.tar`。
  delivery-consumer 和 4 个 ws 实例均以 default=`1`、READ_FANOUT=`10`、
  BROADCAST_SIGNAL=`10` 启动。复压
  `hotgroup-fanoutpolicy-clean-400sub-5000msg-coordinator-20260701-084638`
  使用 6000 人、5000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT；4 个 shard 共读完 200000 条 sampled signal，span `141.504s`，
  `send_p95_ms=21.055`、`send_p99_ms=26.145`、`PullInbox p95=68.047ms`，
  message / delivery outbox pending=0，writer / Redis subscriber error、queue-full
  和 eviction 均为 0。Prometheus 窗口显示 Redis subscriber conversation fanout
  window p95 / p99 约 `39.944ms / 84.614ms`，5m last p95 / p99 约
  `62.5ms / 92.5ms`。结论：fanout-mode policy 已把全局 sample knob 收敛成
  room policy；在 READ_FANOUT=10 的等价行为下性能与上一轮 global sample=10
  基本同一量级，瓶颈仍是 online-signal-drain。
- 2026-07-01 已实现并复验 subscriber-aware conversation signal cadence 模块：在
  fanout-mode policy 之上，新增
  `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SUBSCRIBER_POLICY_<MODE>`，格式为
  `min_subscribers:sample_every` 的逗号列表。memory registry 按本机
  conversation subscriber 数调整 sample cadence；Redis route 按每个远端
  gateway 的订阅数决定是否 publish，且被采样丢弃的 remote signal 不写 Redis
  resume。没有配置 subscriber threshold 时，Redis route 保留旧的采样前置快速路径，
  不为 sampled-out signal 做 route lookup。clean commit `9bdf21c5` 已完成镜像
  重建、归档、Ubuntu redeploy 和同场景复压。run
  `hotgroup-subscriberpolicy-400sub-5000msg-coordinator-20260701-095457`
  使用 6000 人、5000 消息、目标 8000 msg/s、256 sender、400 subscriber、
  READ_FANOUT、4 个 subscriber shard、READ_FANOUT threshold `100:20`；4 个 shard
  共读完 100000 条 signal，message / delivery outbox pending=0，writer /
  Redis subscriber error、queue-full 和 eviction 均为 0。但 signal span 为
  `289.249s`，span rate 约 `345.723 signals/s`，明显低于 `37b575e5`
  baseline 的 `200000 signal / 141.504s / 1413.391 signals/s`。结论：该
  first-stage threshold 策略能降低 emitted signal 数，但当前配置没有改善 drain，
  不能作为热点群吞吐优化闭环；下一步应转向消息速率感知的动态 cadence、持久
  per-conversation / per-bucket worker，或更强 pull-first 策略。
- 2026-07-01 已补 push-gateway Redis conversation route cache 代码模块：当
  subscriber-aware cadence 必须先计算每个远端 gateway 的 conversation subscriber
  数时，Redis route registry 会对同一 `tenant_id + conversation_id` 的 route lookup
  使用短 TTL 进程内缓存，默认 `NEXUSIM_PUSH_CONVERSATION_ROUTE_CACHE_TTL=250ms`。
  该缓存只减少重复 Redis `SMembers + GET` 查询，不保存业务事实、不替代 Redis route
  权威状态；订阅 / 退订 / unregister 会显式失效本机缓存，Redis 失败仍按原逻辑计
  `lookup_error`，不假装成功。新增 Prometheus / debug 指标：
  `conversation_route_cache_hit`、`conversation_route_cache_miss`、
  `conversation_route_cache_invalidated`，并已接入 hotgroup metrics-window 脚本。
  clean commit `304383ea` 已完成镜像重建 / 归档 / Ubuntu Docker redeploy，并用
  同场景 subscriber-aware 400 subscriber / 5000 message 复压。run
  `hotgroup-routecache-400sub-5000msg-coordinator-20260701-104942` 使用 6000 人、
  5000 消息、目标 8000 msg/s、256 sender、400 subscriber、READ_FANOUT、
  4 个 subscriber shard、READ_FANOUT threshold `100:20`；4 个 shard 共读完
  100000 条 signal，message / delivery outbox pending=0，writer / Redis subscriber
  error、queue-full 和 eviction 均为 0。signal span 从上一轮 subscriber-aware
  baseline 的 `289.249s` 降至 `146.62s`，span rate 从约 `345.723 signals/s`
  提升到约 `682.034 signals/s`，约 `1.97x`。Prometheus 窗口显示
  Redis subscriber fanout p95 / p99 降到约 `1.96ms / 6.25ms`，WebSocket
  delivery notify write p95 / p99 约 `0.241ms / 0.433ms`。注意：当前 Prometheus
  只 scrape 4 个 ws debug target，delivery-consumer 未暴露 debug 端口，因此
  route cache hit / miss 在窗口中仍为 0；这是观测缺口，不代表 cache 未生效。
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
- `sample_every=10` 的 1000 消息和 5000 消息两档复压均已完成：1000 消息档将
  emitted signal 从 400000 降至 40000，span 从 141.719s 降至 25.243s；5000
  消息档产生 200000 条 sampled signal，span 138.555s。两档 SendMessage、
  PullInbox、ACK 和 outbox drain 均成立，但 5000 消息档继续显示 online-signal-drain
  和 Redis subscriber 本地 fanout/enqueue 长尾。
- fanout-mode conversation signal policy 已完成 clean commit、Docker 镜像重建 /
  归档 / redeploy 和 mode-specific 可比复压。该复压证明策略边界生效，但没有改变
  READ_FANOUT=10 下的核心瓶颈。
- subscriber-aware conversation signal cadence 已完成代码、focused 验证、clean
  Docker redeploy 和可比复压。复压确认 outbox、writer、Redis route 和 subscriber
  错误路径均正常，但 READ_FANOUT `100:20` 没有改善 drain span。
- Redis conversation route cache 代码、配置、指标和 hotgroup metrics-window 查询已完成
  clean commit 镜像重建 / 归档 / redeploy / 可比复压。该模块在 subscriber-aware
  threshold 场景下把 signal drain rate 提升约 1.97x，但仍未回到 fanout-mode
  policy baseline 的约 1.4k signals/s，且 delivery-consumer cache hit / miss 仍缺
  debug scrape 证据。
- 文档同步本轮公开能力或瓶颈变化。
- 提交并推送到 GitHub。

## 非目标

- 不做长时间正式容量压测或生产 sizing。
- 不把 first-stage block cache / repair operator 说成完整 sequencer 分区系统。
- 不新增隐藏 fallback，不用本地 row lock 冒充热点 sequencer。
- 不继续扩完整产品级客户端。
- 不展开无关 Agent action boundary / repair cases。

## 后续优先级

1. subscriber-aware threshold + Redis conversation route cache 已完成 clean Docker
   复压，证明重复 route lookup 曾是该策略的明显成本来源，但 route cache 后仍未突破
   fanout-mode policy baseline。下一步先补 delivery-consumer debug/metrics scrape 或
   低敏 sidecar 观测，使 route cache hit / miss 可见；随后不要继续只调静态 sample
   knob，转向消息速率 / 在线人数感知的 dynamic cadence、持久 per-conversation /
   per-bucket fanout worker，或更强 pull-first 策略。
2. 继续为每轮优化保留 clean commit、Docker 镜像归档、三机部署版本和 Prometheus
   时间窗口，保证压测曲线可复现。
3. 若 HYBRID 仍要支持千人级 per-user materialized outbox，优先评估显式 frontier /
   progress 表或把策略提前切到 READ_FANOUT；不要把 Kafka / Redis 当成替代 fanout 策略。
4. delivery projection lag / inbox rows per message / push notify storm 指标深化。
5. timeline virtual partition mapping、leader ownership audit 和更完整 repair workflow。
6. 压测报告与面试叙事维护。
