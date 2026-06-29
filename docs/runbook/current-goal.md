# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

Hot group Docker redeploy and pressure validation：在已通过的
`SEQUENCER_BLOCK + BROADCAST_SIGNAL` 小规模 smoke 和 sequencer repair readiness
基础上，收口 conversation-service 热点成员边界 seq 分配，重建最新镜像，三机 redeploy，
再跑 clean commit 小 / 中规模热点群复验。

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

## 目标

- 将当前 conversation member-boundary sequencer 改动收口提交，确保 focused tests、
  Docker / compose 相关检查通过。
- 用 clean commit 重建最新 Docker 镜像并 redeploy Ubuntu Docker 核心链路。
- 先跑小规模热点群复验，再跑中等规模诊断压测；记录 Kafka lag、delivery projection lag、
  push signal、PullInbox / ACK 追平和 PostgreSQL 关键指标。
- 本轮只写本地 / 三机实验结论，不写生产容量上限。

## 本轮完成条件

- 当前 uncommitted conversation member-boundary sequencer 改动收口，focused tests 和
  Docker / compose 相关检查通过。
- 最新 Docker 镜像已备份到 `H:\NexusIM\docker-images\archives`，Ubuntu Docker 核心链路
  已使用新镜像 redeploy。
- clean commit 小 / 中规模热点群复验完成，并明确记录是否存在新瓶颈。
- 文档同步当前公开能力：message-service 已接 timeline-service seq block cache；
  conversation-service 热点成员边界已接 timeline-service；timeline-service 已有 lease
  expire / gap marker repair operator first path。
- 提交并推送到 GitHub。

## 非目标

- 不做长时间正式容量压测或生产 sizing。
- 不把 first-stage block cache / repair operator 说成完整 sequencer 分区系统。
- 不新增隐藏 fallback，不用本地 row lock 冒充热点 sequencer。
- 不继续扩完整产品级客户端。
- 不展开无关 Agent action boundary / repair cases。

## 后续优先级

1. clean commit 中等规模热点群复验和报告归档。
2. 扩大三机热点群压测规模，补趋势图 / 瓶颈曲线。
3. delivery projection lag / inbox rows per message / push notify storm 指标深化。
4. timeline virtual partition mapping、leader ownership audit 和更完整 repair workflow。
5. 压测报告与面试叙事维护。
