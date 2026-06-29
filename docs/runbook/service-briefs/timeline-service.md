# timeline-service Brief

状态：core runtime / first-stage seq block allocator + repair readiness。六层目录、
Docker runtime、Prometheus / Grafana 观测和 `seq-block-allocator` runtime 已进入本地
运行链路；本轮新增 lease status、gap marker 和 operator repair first path，用于承接热点
会话 sequencer、conversation timeline 分区和后续更完整 fencing / repair workflow。

## 当前边界

- 当前提供 `NEXUSIM_TIMELINE_SERVICE_MODE=seq-block-allocator`，通过
  `AllocateSeqBlock` gRPC API 按 `tenant_id + conversation_id` 原子分配 seq block，
  并将 lease / idempotency 记录写入 timeline-service 自有 PostgreSQL 表。
- 当前提供显式 repair operator modes：`seq-lease-expire`、`gap-marker-create`、
  `gap-marker-close`、`gap-marker-audit`。这些 operator 只操作 timeline-service 自有
  lease / gap marker 表，不读取 message / conversation / delivery 私有表。
- `noop` 模式仍保留为显式空运行模式，但本地 Docker 运行链路默认使用
  `timeline-service-seq-block-allocator`。
- timeline-service 不拥有消息正文、不写 message facts、不发布 Kafka，也不修改
  conversation / delivery 事实源。
- message-service 已接入第一阶段 `SEQUENCER_BLOCK` active 写路径：发送时调用
  `AllocateSeqBlock` 获得 valid lease 后才写 message facts；未配置 timeline client、
  lease 过期或 epoch / lease_id 缺失时 fail-closed，不回退到本地 row lock。message-service
  已支持本地 seq block cache 和 lease safety margin。
- conversation-service 已接入热点成员边界 first path：当 conversation 已提升到
  `SEQUENCER_BLOCK` 时，成员 JOIN / LEAVE / REMOVE / owner transfer 使用
  `AllocateSeqBlock` 获取单 seq lease，并以现有 conversation timeline 最大 seq 作为
  floor；未配置 timeline client 或 lease 无效时 fail-closed，不回退到本地 row lock。

## 目标职责

- 热点会话识别和 seq mode control-plane。
- seq block 分配、lease status 管理、sequencer epoch fencing、leader ownership audit。
- gap marker 生成、关闭与审计，保证允许有解释的 gap，不允许无解释乱序。
- 与 control-plane 协同管理 conversation timeline virtual partition / physical
  partition mapping。

## 非职责

- 不拥有消息正文或 message facts。
- 不拥有成员事实，成员仍由 conversation-service 管理。
- 不拥有 durable inbox，投递仍由 delivery-service 管理。
- 不绕过 outbox / Kafka / projection / audit 边界。

## 下一步

- 重建镜像后跑热点群 clean commit 小 / 中规模复验。
- 补热点群压测中的 seq block allocation、lease replay / conflict、gap marker、
  projection lag 和 operator repair 指标。
- 后续继续做 virtual partition mapping、leader ownership audit、operator UI 和更完整
  gap repair workflow。
