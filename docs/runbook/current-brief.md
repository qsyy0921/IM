# NexusIM Current Brief

本文件只做每轮入口摘要；长历史、smoke 证据和设计细节放到 SDD、service
brief、loadtest report、development-progress 或 archive。

## 当前主线

- 客户端 Web / PC 已达到演示 MVP；除阻塞演示的问题外，不继续追完整产品级客户端。
- 后端主线按用户指令临时切到热点群聊链路收口：conversation fanout policy、
  timeline-service seq block、message-service sequencer active path、delivery projection
  和 push conversation signal。
- 当前 active module：hot group fanout / sequencer / projection hardening。

## 最近收口

- Agent demo path 已能演示 EvidencePack -> RAG -> Agent proposal -> approval
  -> action-executor -> conversation-service public API，覆盖 conversation note
  和 conversation profile mutation。
- action-executor 已覆盖 provider failure metrics、batch handoff、provider replay
  operator UI、admin / workflow handoff、review / readiness / invocation manifest、
  controlled redrive execution、redrive result manifest、redrive audit append
  manifest handoff、external audit append operator path 和 audit append result
  manifest。
- workflow-service 已覆盖 provider replay queue、approval timeout、external
  approval binding、operator queues、external callback wait、external callback
  delivery plan、delivery status / redrive plan、external callback delivery
  persistent worker first path、external callback delivery redrive operator path、
  external callback delivery review page / dashboard / batch redrive invocation
  manifest / runner / result manifest / audit append handoff / audit append result manifest、
  approval queue review page / batch decision manifest /
  runner / result review page / audit append handoff / audit append result manifest、
  compensation review bundle / page、instruction approval page、execution readiness /
  invocation manifest、execution result visibility、audit append manifest handoff、
  audit append result manifest，以及 workflow outbox relay first path。
- group memory / retrieval / eval 持续保留 source refs、conversation scope、
  member visibility、time/version boundary、citations 和 no-citation refusal。
- Conversation scale policy 已在 conversation-service domain 层落地：direct / small group
  使用 active `WRITE_FANOUT`；medium group 使用 active first-stage `HYBRID_FANOUT`；
  large group 使用 active first-stage `READ_FANOUT`；hot group 的
  `BROADCAST_SIGNAL + SEQUENCER_BLOCK` 已有 timeline seq-block allocator 和 push
  conversation subscription / signal 广播服务端 first path；message-service 已接入
  第一阶段 active `SEQUENCER_BLOCK` 写路径，发送时通过 timeline-service
  `AllocateSeqBlock` 获取 valid lease 后才写 message facts。本轮已补 message-service
  本地 seq block cache、lease safety margin、lease metadata 写入 / 校验；timeline-service
  已补 lease status、显式 gap marker 表和 `seq-lease-expire` / `gap-marker-create` /
  `gap-marker-close` / `gap-marker-audit` operator first path。epoch fencing 当前以
  lease epoch / lease_id / expires_at 校验进入写路径，leader ownership audit 和 virtual
  partition mapping 仍后续深化。
- conversation-service 已补热点成员边界 seq 分配：`SEQUENCER_BLOCK` 会话中的成员
  JOIN / LEAVE / REMOVE / owner transfer 通过 timeline-service `AllocateSeqBlock`
  获取单 seq lease，并以当前 conversation timeline 最大 seq 作为 floor。未配置 sequencer、
  lease 无效或 lease 过期时 fail-closed，不回退到本地 row lock。
- delivery-service 已补 outbox relay 吞吐 hardening：ready SQL 避免积压下反复扫描历史
  `PUBLISHED` 行，新增 pending-ready / blocking aggregate indexes，relay 支持
  conversation-sharded workers 和 delivery 专用 Kafka batch 参数；2026-06-28
  hotgroup QPS step 复测显示旧瓶颈 `delivery_outbox -> Kafka im.delivery.events`
  已解除到 100 人群 150 QPS 可追平，200 QPS 的下一瓶颈转移到 delivery timeline
  projection / `user_inbox` fanout。
- delivery-service 已补 `delivery.conversation.signal.v1` first path：`READ_FANOUT` /
  `BROADCAST_SIGNAL` 不再按成员数写 delivery outbox，而是写会话级 signal；push-gateway
  已支持 `conversation.subscribe / unsubscribe` 和 conversation signal fanout，receipt-service
  当前仍只校验并 checkpoint，不伪造 user-level 回执。hotgroup runner 已支持
  `--expect-fanout-mode` 并按 WRITE / HYBRID / READ / BROADCAST 区分验证 `user_inbox`
  或 `delivery_timeline_items`。
- delivery-service materialized `user_inbox` projection 已将 per-recipient inbox insert
  合并为批量 insert，保持 per-recipient delivery outbox 事件语义不变，减少小 / 中群写扩散
  的 PostgreSQL roundtrip。
- delivery-service `timeline-consumer` 已支持 `NEXUSIM_DELIVERY_TIMELINE_CONSUMER_WORKERS`
  多 worker runtime：同一 consumer group 内启动多个 Kafka reader，由 Kafka 做 partition
  assignment；这能并行多个 conversation / partition 的 projection，但仍保持单 partition 顺序，
  不把单热点会话伪装成可无序并行。
- 2026-06-29 已在 Ubuntu Docker 核心链路跑通热点群小规模 smoke：
  `group_size=61`、`sender_count=4`、`message_count=20`、`fanout_mode=BROADCAST_SIGNAL`、
  `conversation_mode=SEQUENCER_BLOCK`、3 个 WebSocket conversation subscriber 共收到
  60 条 `delivery.notify` conversation signal；`send_p95_ms=19.03`、
  `user_inbox_rows=0`、`delivery_timeline_rows=20`、`delivery_outbox_pending=0`、
  `message_outbox_dlq=0`、`delivery_outbox_dlq=0`。原始结果在
  `H:\NexusIM\loadtest-results\hotgroup-broadcast-push-smoke-20260629-2135`。
- 2026-06-30 已用 clean commit `d13bff6c` 重建 / redeploy conversation-service，并完成
  三档热点群复验：61 人 / 20 消息 / 3 subscriber、200 人 / 500 消息 / 20 subscriber、
  500 人 / 1000 消息 / 50 subscriber。最大一档 `send_p95_ms=10.633`、
  `send_p99_ms=13.013`、conversation signal=50000、`user_inbox_rows=0`、
  `delivery_outbox_pending=0`、Kafka lag=0。报告见
  `docs/runbook/loadtest/hotgroup/loadtest-report-20260630-hotgroup-clean-redeploy.md`。
- 2026-06-30 已用 clean commit `0a1395c` 收口 message-service outbox relay：
  relay 现在支持 4 worker conversation-sharded batch publish、批量 mark published 和
  message outbox ready query conversation/version indexes。1000 人 / 4000 消息 /
  800 msg/s / 100 subscriber 档位通过，`message_outbox_pending=0`、
  `delivery_outbox_pending=0`、Kafka lag=0。1200 / 1500 msg/s 更高档位已证明
  SendMessage、message outbox、delivery projection、delivery outbox 和 Kafka consumer 都能追平；
  新瓶颈迁移到 push-gateway conversation signal 写出 / runner 读取观测。报告见
  `docs/runbook/loadtest/hotgroup/loadtest-report-20260630-hotgroup-message-outbox-relay.md`。
- push-gateway / hotgroup 已补下一轮定位所需指标：push-gateway WebSocket writer 现在暴露
  outbound dequeued、frame write success/error、delivery.notify write success/error、
  resume_hint write success/error 和 last event timestamp；`loadtest/hotgroup` 现在在
  summary/report 中记录每个 conversation subscriber 的 signal 数、max seq、首帧/末帧耗时、
  completed 和 read error。push-gateway 镜像已按 clean commit redeploy，READ_FANOUT
  6000 人 / 1000 消息 / 400 msg/s / 100 subscriber 诊断 run 已通过，100000 条
  conversation signal 均被 subscriber 读到，`delivery_outbox_pending=0`、Kafka lag=0。
- 2026-06-30 已用 clean commit `01b2a70` 重建 / redeploy delivery-service 后完成
  READ_FANOUT 阶梯复压：6000 人、100 subscriber 下目标 400 / 800 / 1200 /
  2000 / 4000 / 8000 msg/s 全部通过；最高档发送 5000 条消息并产生 500000 条
  conversation signal，所有 subscriber 读完，`send_p95_ms=18.54`、`send_p99_ms=22.41`、
  `PullInbox p95=26.93ms`、`message_outbox_pending=0`、`delivery_outbox_pending=0`、
  Kafka lag=0。当前瓶颈不在 SendMessage、message outbox、delivery projection、
  delivery outbox 或 Kafka consumer；下一步看 online signal drain / reader 侧。
- 2026-06-30 已新增 hotgroup 离线分析器 `tools/analyze-hotgroup-loadtest.ps1`，
  能从 H 盘 `hotgroup-summary.json` 自动生成 run matrix、瓶颈分类和下一步策略。
  已生成 `docs/runbook/loadtest/hotgroup/hotgroup-analysis-20260630-readfanout-clean.md`：
  clean commit `01b2a70` 6 档 READ_FANOUT 结果被分类为 `online-signal-drain`，证据是
  outbox / Kafka 已追平，但 500000 条 conversation signal 最慢读完约 176s。
- 2026-07-01 已新增 hotgroup Prometheus 时间窗口记录工具
  `tools/record-hotgroup-metrics-window.ps1`，并为最高档
  `hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336` 写出低敏窗口报告：
  `docs/runbook/loadtest/hotgroup/hotgroup-metrics-window-20260630-readfanout-clean-8000qps.md`。
  该窗口内核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，
  `delivery_outbox_pending` 峰值 2258 后归零，push writer / Redis route 指标有数据，
  slow eviction 为 0。
- 2026-07-01 已跑 200 subscriber 阶梯：
  `hotgroup-readfanout-6000-8000qps-200sub-7bff4f38-20260701-002833`，
  clean commit `7bff4f3`、6000 人、5000 消息、目标 8000 msg/s、256 sender、
  200 subscriber，产生 1000000 条 conversation signal；`send_p95_ms=18.315`、
  `send_p99_ms=21.808`、`PullInbox p95=26.326ms`，outbox / Kafka 追平。
  与上一轮 100 subscriber / 500000 signal 对比，drain rate 仍约 2.85k signals/s，
  当前瓶颈继续指向 online signal drain / reader 侧。
- 2026-07-01 已跑 400 subscriber 阶梯：
  `hotgroup-readfanout-6000-8000qps-400sub-233d6956-20260701-004948`，
  clean commit `233d695`、6000 人、5000 消息、目标 8000 msg/s、256 sender、
  400 subscriber，产生 2000000 条 conversation signal；`send_p95_ms=19.724`、
  `send_p99_ms=25.668`、`PullInbox p95=25.341ms`，outbox / Kafka 追平。
  100 / 200 / 400 subscriber 三档 drain rate 分别约 2832 / 2858 / 2838 signals/s，
  瓶颈继续线性落在 online signal drain。
- 2026-07-01 已把 hotgroup Prometheus 窗口工具升级为 push attribution 版本：
  `hotgroup-metrics-window-20260701-readfanout-400sub.md` 现在同时记录 WebSocket
  writer 与 Redis route 的 per-event 五分钟峰值和整窗口计数。400 subscriber 窗口内
  `frame_write_success` 约 200.97 万、`delivery_notify_write_success` 约 200.89 万、
  `redis subscriber_enqueued` 约 200.89 万，writer / delivery notify / Redis subscriber
  error 与 eviction 均为 0，说明下一步应定位写出 / 读取 drain 能力，而不是继续调
  message / delivery outbox 或 Kafka。
- 2026-07-01 已为 `loadtest/hotgroup` 增加多 runner 读取验证：`full` coordinator
  继续建群 / 加成员 / 发消息 / PullInbox / ACK；`subscriber-only` runner 只打开
  WebSocket conversation subscribers 并通过 `subscriber-shard-count/index` 读取
  deterministic receiver 子集。下一轮可以把 400 subscriber 拆到多个进程 / 机器上，
  先判断 2.8k signals/s 是否由单 runner 读取 / JSON decode / accounting 限制。
- 2026-07-01 已跑 4 runner shard 对照：
  `hotgroup-multirunner-400sub-coordinator-20260701-013557` + 4 个
  `subscriber-only` shard，clean commit `9e7d4f9`，6000 人、1000 消息、目标
  8000 msg/s、400 subscriber。coordinator send / PullInbox / ACK 成功，message /
  delivery outbox pending=0；4 个 shard 共读完 400000 条 signal。按首帧到末帧计算
  drain rate 约 2852 signals/s，与单 runner 400 subscriber baseline 约 2840 signals/s
  基本一致，说明当前瓶颈不只是单 runner JSON decode / accounting。
- 2026-07-01 已完成第一轮 push-gateway online signal drain 代码级优化：本地 memory
  registry 的 user / conversation fanout 改为锁内快照、锁外写出，queue full 时再
  精确回锁驱逐仍然注册的同一 session；focused tests / build / diff check 已通过。
  clean commit `4bc4a30` 镜像 redeploy 后，400 subscriber coordinator + 4 shard
  复压显示 drain rate 从单 runner baseline 约 2839.888 signals/s 到约
  2891.8 signals/s，仅约 1.8% 提升，瓶颈仍是 online signal drain。
- 2026-07-01 已完成第二轮 push-gateway online signal drain 代码优化并复压：
  clean commit `d8d78fd` 的 delivery / conversation notify 预编码 payload 版本已
  重建 / redeploy。400 subscriber coordinator + 4 shard run
  `hotgroup-pushpreenc-clean-400sub-coordinator-20260701-024044` 共读完
  400000 条 signal，drain rate 约 `2863.092 signals/s`，相比单 runner baseline
  `2839.888 signals/s` 仅约 `0.8%` 提升，也没有超过上一轮 registry lock 优化后的
  `2891.8 signals/s`。结论：重复 JSON marshal 不是主瓶颈，online signal drain
  仍需从 WebSocket 写调度 / flush / 连接读取背压 / 网络吞吐继续定位。
- 2026-07-01 已补 push-gateway WebSocket writer 写耗时观测：`frame_write` 和
  `delivery_notify` 现在暴露 histogram / sum / count / max，hotgroup Prometheus
  时间窗口脚本会输出 delivery notify p95 / p99 / avg / max。该改动是下一轮定位
  `conn.Write` 长尾、flush / scheduling 或读端背压的前置证据，不改变消息协议或
  fanout 语义；focused push-gateway tests 和 hotgroup build 已通过。
- 2026-07-01 已用 clean commit `4f45519` 重建 / redeploy push-gateway，并用同一
  400 subscriber coordinator + 4 shard 场景复压。run
  `hotgroup-writerdur-clean-400sub-coordinator-20260701-031058` 共读完
  400000 条 signal，drain rate 约 `2876.698 signals/s`，相比旧 baseline
  `2839.888 signals/s` 只高约 `1.3%`。Prometheus 窗口显示 `delivery_notify`
  write p95 / p99 约 `0.345ms / 0.499ms`，avg 约 `0.125ms`，max 约 `10.056ms`，
  writer / Redis subscriber error 与 eviction 为 0。结论：单次 WebSocket
  `conn.Write` 长尾不是当前主瓶颈，下一步应定位 Redis subscriber 本地 fanout /
  enqueue 调度、writer goroutine 节奏、runner 读取背压和网络吞吐。
- 2026-07-01 已用 clean commit `6099ecd` 重建 / redeploy push-gateway，并完成
  Redis subscriber fanout duration 复压：
  `hotgroup-redisfanout-clean-400sub-coordinator-20260701-033606`。同一 400
  subscriber coordinator + 4 shard 场景下，400000 条 signal 全部读完，drain rate
  约 `2883.976 signals/s`；WebSocket `delivery_notify` write p95 / p99 约
  `0.406ms / 0.63ms`，Redis subscriber conversation signal fanout/enqueue 整窗口
  p95 / p99 约 `56.14ms / 91.228ms`，5m last p95 / p99 约
  `60.263ms / 92.053ms`，avg 约 `16.485ms`。结论：瓶颈已进一步收窄到
  Redis subscriber 收到 conversation signal 后的本地 fanout/enqueue 调度。
- 2026-07-01 已实现 push-gateway Redis subscriber conversation signal worker /
  shard queue：conversation signal 按 `tenant_id + conversation_id` 入 bounded queue
  并由 worker fanout，delivery notify 路径不变；新增 worker / queue size env、
  queued / queue_full / worker_error / queue_depth / queue_wait 指标，并同步
  hotgroup Prometheus 时间窗口脚本。clean commit `93654117` 已重建 / 归档 /
  redeploy，并完成 400 subscriber coordinator + 4 shard 复压：
  `hotgroup-signalqueue-clean-400sub-coordinator-20260701-041641`。本轮 400000 条
  signal 全部读完，drain rate 约 `2876.076 signals/s`，queue full / worker error 为 0，
  queue wait p95 / p99 约 `0.095ms / 0.099ms`；但 worker fanout p95 / p99 仍约
  `38.636ms / 87.5ms`，总 drain 没突破旧曲线。下一步不要继续调 Redis subscriber
  handoff，而要分析 worker 本地 fanout、per-session writer 调度、flush / batching
  和 runner 读取侧。
- 2026-07-01 已补 push-gateway WebSocket writer queue latency 和 batch drain：
  `ServerFrame.EnqueuedAtMS` 是 transport-only 元数据，writer 会记录 outbound queue
  duration histogram / max，`NEXUSIM_PUSH_WS_WRITER_BATCH_SIZE` 默认 16 用于单连接内
  小批量 drain 已排队 frame。该改动保持单连接顺序和现有 durable PullInbox 边界；
  focused push-gateway tests、hotgroup tests 和 build 已通过。下一步需要 clean commit
  镜像重建 / redeploy 后复压，确认 queue latency 是否解释 online signal drain 曲线。
- 2026-06-30 HYBRID 1000 人 / 1000 消息 / 400 msg/s 诊断 run 暴露 per-user outbox
  写扩散下的 delivery outbox ready query 退化：旧 anti-join blocker 查询在约 100 万
  pending row 下每批 500 行约 24s。delivery-service 已改为 per-conversation frontier
  ready query，本地 relay worker 提高到 8；这是 first-stage 查询优化，后续仍需通过
  clean commit 镜像复压并决定 HYBRID 策略是否提前切 READ_FANOUT 或引入显式
  frontier / progress 表。

## 已成型底座

- 10 个核心运行链路服务：api-gateway、identity-service、message-service、
  conversation-service、delivery-service、push-gateway、receipt-service、
  contacts-service、policy-service，以及已进入本地 Docker / 观测链路的
  timeline-service seq-block allocator。
- AI foundation：search-service、memory-service、retrieval-gateway、rag-service、
  summary-service、agent-service、skill-registry、mcp-gateway、action-executor、
  ai-eval-service、Python AI Worker。
- Product-active first paths：admin-service、audit-service、control-plane-service、
  knowledge-ingestion-service、media-service、model-gateway、notification-service、
  presence-service、vector-index-service、workflow-service。
- Distributed timeline planning：timeline-service 已建立六层边界、PostgreSQL
  seq state / block lease / gap marker 表、`AllocateSeqBlock` gRPC API、Docker runtime
  和 Prometheus / Grafana 观测；message-service 已在 active `SEQUENCER_BLOCK` 写路径消费
  lease，并支持本地 seq block cache。
- Observability platform：当前 first-stage 指标和 trace 继续按 Prometheus / Grafana /
  OpenTelemetry 分工维护；它们只提供观测，不参与业务判定或隐藏降级。

## 工作规则

- 每轮先读 `prompt.md`、`agent.md`、`docs/runbook/current-goal.md`。
- 一个 goal 必须是可感知功能模块；不要把小字段、小测试、小文档句子当目标。
- 不写隐藏 alternate path；不确定时 fail-closed，或显式 repair / retry / redrive。
- 文档只在阶段、公开能力、架构边界、新服务 / 中间件 / provider 变化时同步。

## 下一个方向

- 下一步围绕 push-gateway worker 本地 fanout / session writer 调度继续优化：
  queue handoff 已不是瓶颈；当前已补 writer queue latency / batch drain，下一步
  先用同一 400 subscriber coordinator + shard 场景复压，再决定是否继续调整
  per-conversation worker 数、session outbound queue drain、writer goroutine 调度、
  WebSocket flush 策略或 runner 读取背压。
  正式生产级运维 UI、provider-grade 长周期平台仍后置。
