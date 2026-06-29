# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

Hot group pressure step-up and bottleneck curve：在 clean commit Docker redeploy 和
`SEQUENCER_BLOCK + BROADCAST_SIGNAL` 三档复验通过基础上，继续扩大热点群压测规模，
补 Prometheus / Grafana 趋势和下一瓶颈曲线。

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

## 目标

- 继续做热点群 step run，逐步提高 message rate、subscriber count、online ratio 和慢连接比例，
  找出下一瓶颈。
- 对每轮正式压测记录 run name、commit、dashboard 时间窗口、Kafka lag、delivery projection lag、
  push signal、PullInbox / ACK 追平和 PostgreSQL 关键指标。
- 本轮只写本地 / 三机实验结论，不写生产容量上限。

## 本轮完成条件

- clean commit step run 完成，并明确记录是否存在新瓶颈。
- 至少补一轮 Prometheus / Grafana 时间窗口信息；若缺 exporter，必须写清楚缺口，不把
  一次性 CLI 统计冒充趋势图。
- 文档同步本轮公开能力或瓶颈变化。
- 提交并推送到 GitHub。

## 非目标

- 不做长时间正式容量压测或生产 sizing。
- 不把 first-stage block cache / repair operator 说成完整 sequencer 分区系统。
- 不新增隐藏 fallback，不用本地 row lock 冒充热点 sequencer。
- 不继续扩完整产品级客户端。
- 不展开无关 Agent action boundary / repair cases。

## 后续优先级

1. 扩大三机热点群 step run，补趋势图 / 瓶颈曲线。
2. delivery projection lag / inbox rows per message / push notify storm 指标深化。
3. timeline virtual partition mapping、leader ownership audit 和更完整 repair workflow。
4. 压测报告与面试叙事维护。
