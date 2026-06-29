# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

Hot group fanout / sequencer / projection hardening：把热点群聊从普通小群写扩散路径
推进到可演示的 `SEQUENCER_BLOCK + BROADCAST_SIGNAL` 链路，并准备真实三机压测。

## 当前已收口摘要

- conversation-service 已按群规模输出 fanout / conversation mode：direct / small group
  使用 `WRITE_FANOUT`，medium / large 使用 first-stage `HYBRID_FANOUT` /
  `READ_FANOUT`，hot group 使用 `BROADCAST_SIGNAL + SEQUENCER_BLOCK`。
- timeline-service 已进入本地运行链路，提供 `seq-block-allocator` runtime、PostgreSQL
  sequence state / lease 表和 `AllocateSeqBlock` gRPC API。
- message-service 已在 `SEQUENCER_BLOCK` active 写路径调用 timeline-service
  `AllocateSeqBlock(block_size=1)`；拿不到 valid lease 时 fail-closed，不回退到本地
  row lock。
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

- 将当前 hotgroup / sequencer / projection 改动收口提交，确保 focused checks、Docker /
  compose / registry 相关检查通过。
- 保留小规模 smoke 证据，并明确它证明 first-stage 链路可用，不代表容量上限。
- 下一模块进入 timeline-service block cache、gap marker、epoch fencing 和 repair operator；
  随后扩大三机热点群压测规模，记录 Kafka lag、delivery projection lag、push notify、
  PullInbox / ACK 追平和 PostgreSQL 关键指标。

## 本轮完成条件

- 当前 uncommitted hotgroup / sequencer / projection 改动收口，focused tests 和
  Docker / compose / registry 相关检查通过。
- 若本地 / 远端 Docker 未启动，必须明确标注真实压测未执行，不写成完成。
- 文档同步当前公开能力：message-service 已接 timeline-service 单条 seq block；
  block cache / gap marker / epoch fencing / 三机压测仍是后续。
- 提交并推送到 GitHub。

## 非目标

- 不做长时间正式容量压测或生产 sizing。
- 不把单条 seq block 接入说成完整 sequencer 分区系统。
- 不新增隐藏 fallback，不用本地 row lock 冒充热点 sequencer。
- 不继续扩完整产品级客户端。
- 不展开无关 Agent action boundary / repair cases。

## 后续优先级

1. timeline-service block cache、gap marker、epoch fencing 和 repair operator。
2. 扩大三机热点群压测规模，补趋势图 / 瓶颈曲线。
3. delivery projection lag / inbox rows per message / push notify storm 指标深化。
4. 压测报告与面试叙事维护。
