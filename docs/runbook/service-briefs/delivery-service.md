# delivery-service

## 当前状态

- 已有 timeline projection、durable `user_inbox`、`PullInbox`、`AckDelivery`、`HideInboxItem` 用户视图隐藏、`delivery_outbox` relay。
- 是 push-gateway 的可靠事实源，不要求 push-gateway 持久化消息或 ACK cursor。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`、Prometheus text `/metrics`、本地 alert rules / Grafana dashboard 原型；默认 scrape target 为 `host.docker.internal:11912`，覆盖 durable read model、delivery outbox、projection blocker、worker/relay、PG pool 和 OTel trace config 聚合指标；这是本地开发 / 面试演示观测，不是生产 SLO。
- 已补 `outbox-audit` / `outbox-repair` / `outbox-repair-audit` / `outbox-repair-cleanup`，可审计、redrive `delivery_outbox` DLQ 并清理 repair audit 历史；`outbox-repair-audit` 可通过 `NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果，便于 operator 留存证据。
- 已补 `projection-checkpoint-repair` / audit / cleanup，当前只允许带审计地回调 checkpoint 做 replay，不允许前跳跳过事件；`projection-checkpoint-repair-audit` 可选 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果，便于 CI / operator 留存结构化证据。
- 已补 projection fail-closed 持久审计：timeline consumer 在 malformed / projection failure 停下前会写低敏失败记录，`last_error` 只保存稳定分类文案，且仍不会提交 checkpoint。
- 已补 projection failure resolved 标记：同一 offset 成功重放后会保留失败记录但标记 `resolved`，`/debug/metrics` 只聚合未解决 blocker。
- 已补按 unresolved failure 定点回调 checkpoint：repair 可直接指定 failure offset，先锁定未解决 failure，再带审计回调到该 offset。
- 已补按最早 unresolved failure 自动回调 checkpoint：repair 不再必须手填 offset，也能安全 rewind 到当前最早 blocker。
- 已补 `projection-failure-audit` 只读模式：可直接列出 unresolved projection failure，并支持按 offset / event / failure class 过滤，减少手写 SQL 排障；可选 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OUTPUT` 会写低敏 JSON 结果，便于 CI / operator 留存结构化证据。
- 已补 `projection-failure-cleanup` operator：只删除超过保留期的 resolved failure 审计行，不会碰 unresolved blocker，并支持按 consumer/topic/partition/class 缩小范围。
- `timeline-consumer` 现已对运行时 `Fetch` / `Commit` 错误做退避重试，并在 worker 模式通过 `/debug/metrics` 暴露低敏 retry 快照；malformed event、projection failure、failure recorder 异常仍保持持久审计 + fail-closed，不会自动越过 blocker。
- `outbox-relay` 现已对非取消运行时错误做退避重试，并在 relay 模式通过 `/debug/metrics` 暴露低敏 retry 快照；publisher 错误写入稳定低敏 `last_error`，malformed payload / unsupported event 仍保持 fail-closed，交给 outbox retry / DLQ 语义处理。
- Kafka writer 已显式固定 `acks=all`、禁自动建 topic、bounded attempts/backoff，并由本地门禁和 package 单测防漂移；真正 idempotent / transactional producer 仍属后续客户端选型。
- 已补 delivery outbox audit / repair audit 错误脱敏：`last_error`、`before_last_error`、`after_last_error` 对外只返回稳定低敏分类，不暴露 broker body、账号、token 或 provider 原文。
- 已补 `delivery.inbox_item.hidden.v1`：`HideInboxItem` 首次隐藏时同事务写 delivery outbox，push-gateway 可向同 user 在线设备发送 `delivery.hide` 轻量提示；重复隐藏不重复写 outbox。
- 已补 trusted metadata 启动门禁：当 `NEXUSIM_DELIVERY_AUTH_MODE=metadata|verified-metadata` 时，如果 gRPC 监听地址不是 loopback / RFC1918 私网，且服务端未启用 mTLS client cert 校验，则启动前直接失败；私网 / loopback 仍保留第一阶段 trusted metadata 直连。
- 已补 first-stage OpenTelemetry gRPC server span；gRPC access log 只记录低敏 `trace_id/request_id`，并对白名单外入口 metadata 直接丢弃。
- `loadtest/delivery` summary 已新增 `capacity_summary`，统一输出 actual duration、poll/item、pull p95/p99、ACK、inbox/outbox 和 checkpoint 关键计数；这是容量基线口径，不等于已完成生产容量压测。
- `loadtest/capacityseed` 已能准备 `tenant-capacity-delivery` 下的 `user_inbox` fixture；`capacity-baseline-seeded-20260616` 本地 seeded 短基线中 `PullInbox + AckDelivery` 成功，`items_per_second=10.49`，报告见 `loadtest/distributed/loadtest-report-20260616-seeded-capacity-baseline.md`。

## 后续
- Projection DLQ / repair 深化、更多 delivery event 消费方、长时间容量曲线；OTel collector / 生产级 alerting / SLO dashboard 仍属于后续统一观测治理。
