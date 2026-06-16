# receipt-service

## 当前状态

- 已有 `MarkRead`、`GetReceiptState`、`ListReceiptStates`、`ListConversations`。
- `ListReceiptStates` 已从应用层逐条循环查询收敛为 repository 级批量查询，单次最多 50 个 item，保持输入顺序和原有 not-found 语义。
- 已支持 unread、pinned、muted 过滤，`UPDATED_AT` / `PINNED_UPDATED_AT` / `UNREAD_UPDATED_AT` 排序，以及 archive / pin / mute 的最小会话列表能力；分页 cursor 会绑定 sort / archived / unread / pinned / muted 过滤条件和排序边界，避免串页。
- 复用 delivery events 和 receipt projection，不跨服务读 delivery 内部表。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 和 first-stage Prometheus text `/metrics`；可观察低敏 gRPC、PG pool、receipt projection、conversation summary、`receipt_outbox`、worker / relay retry 和 OTel trace config 聚合状态；本地 scrape 目标为 `host.docker.internal:11914`，并已补 Prometheus alert rules 和 Grafana dashboard 原型；这些只用于本地开发 / 面试展示，不代表生产 SLO。
- debug HTTP 监听默认只允许 loopback / RFC1918 私网，公网或 unspecified 地址必须显式 `NEXUSIM_RECEIPT_DEBUG_ALLOW_PUBLIC=true`。
- `delivery-consumer` 对非取消错误已改为退避重试，并在 worker 模式通过 `/debug/metrics` 暴露 projection worker retry 快照。
- `outbox-relay` 对非取消运行时错误已改为退避重试，并在 relay 模式通过 `/debug/metrics` 暴露 low-sensitive outbox relay retry 快照；publisher 错误写入稳定低敏 `last_error`。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁和 package 单测防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 已补只读 `outbox-audit`，以及 `outbox-repair` / 只读 `outbox-repair-audit` / `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `receipt_outbox` repair 历史。
- 已补 receipt outbox audit / repair audit 错误脱敏：`last_error`、`previous_last_error` 对外和新写 repair 历史只保留稳定低敏分类，不暴露 broker body、账号、token 或 provider 原文。
- 已补 trusted metadata 启动门禁：当 `NEXUSIM_RECEIPT_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端未启用 mTLS client cert 校验，则启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- 已补 first-stage OpenTelemetry gRPC server span；gRPC access log 只记录低敏 `trace_id/request_id`，并对白名单外入口 metadata 直接丢弃。
- receipt full-smoke runner 已按 config / model / auth / util 同 package 拆分，避免后续回执和会话列表验证继续堆进单个 `main.go`。
- receipt full-smoke summary 已新增 `capacity_summary`，统一输出 actual duration、message/pull/ACK/mark-read/list/mutation/event 计数、outbox 计数和每秒速率；这是容量基线口径，不等于已完成生产容量压测。
- `capacity-baseline-receipt-stack-20260616` 本地 stack 短基线已跑通：覆盖 message/delivery/receipt relay-consumer 链路、`MarkRead`、receipt state、会话列表 archive/pin/mute，`receipt_outbox PUBLISHED=3/PENDING=0/DLQ=0`、`delivery_outbox PUBLISHED=4/PENDING=0/DLQ=0`，Kafka 读回 3 条 receipt event；报告见 `loadtest/distributed/loadtest-report-20260616-receipt-stack-capacity-baseline.md`。

## 后续

- 送达回执扩展、会话列表更多产品化能力（草稿、标签、更多摘要策略等）、长时间容量曲线；生产级 OTel collector、Alertmanager、SLO dashboard 和容量验证仍属于后续统一观测治理。
