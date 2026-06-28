# timeline-service Brief

状态：core runtime / noop。六层目录、Docker runtime、Prometheus / Grafana 观测和
`noop` runtime 已进入本地运行链路，用于承接热点会话 sequencer、conversation
timeline 分区、seq block、epoch fencing 和 gap marker 的后续实现。

## 当前边界

- 当前只提供 `NEXUSIM_TIMELINE_SERVICE_MODE=noop`，可启动 debug health / metrics，并作为
  `timeline-service-noop` 纳入本地 Docker 运行链路。
- 不分配 `conversation_seq`，不写 timeline，不发布 Kafka，不修改 message /
  conversation / delivery 事实源。
- message-service 对 `SEQUENCER_BLOCK` 仍 fail-closed 返回 sequencer unavailable；
  delivery-service 对 `HYBRID_FANOUT` / `READ_FANOUT` / `BROADCAST_SIGNAL` 仍
  fail-closed，不把未实现 fanout 策略伪装成 write fanout。

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

- 冻结 timeline-service SDD：gRPC contract、PostgreSQL tables、lease / fencing、
  gap marker event、control-plane rollout 和 repair operator。
- 实现第一版 `seq-block-allocator`，并让 message-service 只在显式
  `SEQUENCER_BLOCK` + valid block lease 下取号。
- 补热点群压测中的 seq block / gap marker 观测指标。
