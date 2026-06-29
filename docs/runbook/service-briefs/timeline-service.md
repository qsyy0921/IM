# timeline-service Brief

状态：core runtime / first-stage seq block allocator。六层目录、Docker runtime、
Prometheus / Grafana 观测和 `seq-block-allocator` runtime 已进入本地运行链路，
用于承接热点会话 sequencer、conversation timeline 分区、epoch fencing 和 gap
marker 的后续实现。

## 当前边界

- 当前提供 `NEXUSIM_TIMELINE_SERVICE_MODE=seq-block-allocator`，通过
  `AllocateSeqBlock` gRPC API 按 `tenant_id + conversation_id` 原子分配 seq block，
  并将 lease / idempotency 记录写入 timeline-service 自有 PostgreSQL 表。
- `noop` 模式仍保留为显式空运行模式，但本地 Docker 运行链路默认使用
  `timeline-service-seq-block-allocator`。
- timeline-service 不拥有消息正文、不写 message facts、不发布 Kafka，也不修改
  conversation / delivery 事实源。
- message-service 已接入第一阶段 `SEQUENCER_BLOCK` active 写路径：发送时调用
  `AllocateSeqBlock(block_size=1)` 获得 valid lease 后才写 message facts；未配置
  timeline client 时 fail-closed，不回退到本地 row lock。当前仍未实现本地 block cache、
  gap marker 或 epoch fencing 深化。

## 目标职责

- 热点会话识别和 seq mode control-plane。
- seq block 分配、sequencer epoch fencing、leader ownership audit。
- gap marker 生成与修复，保证允许有解释的 gap，不允许乱序。
- 与 control-plane 协同管理 conversation timeline virtual partition / physical
  partition mapping。

## 非职责

- 不拥有消息正文或 message facts。
- 不拥有成员事实，成员仍由 conversation-service 管理。
- 不拥有 durable inbox，投递仍由 delivery-service 管理。
- 不绕过 outbox / Kafka / projection / audit 边界。

## 下一步

- 将单条 seq block 调用推进为本地 block cache、gap marker event、epoch fencing、
  control-plane rollout 和 repair operator。
- 补热点群压测中的 seq block allocation、lease replay / conflict、gap marker 和
  projection lag 观测指标。
