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

- 基于已通过的小规模 smoke 和本轮 sequencer repair readiness 改动，下一步重建最新
  Docker 镜像、三机 redeploy，并做热点群小规模复验。
- 后续再扩大三机热点群压测规模，并把 Prometheus / Grafana 趋势、projection lag、
  push signal 和 PostgreSQL bottleneck 曲线归档到低敏报告；正式生产级运维 UI、
  provider-grade 长周期平台仍后置。
