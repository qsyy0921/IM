# delivery-service

## 当前状态

- 已有 timeline projection、durable `user_inbox`、`PullInbox`、`AckDelivery`、`delivery_outbox` relay。
- 是 push-gateway 的可靠事实源。
- 不要求 push-gateway 持久化消息或 ACK cursor。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 基础观测入口，可在 gRPC / timeline-consumer / outbox-relay 模式下独立挂载。
- 已补 `outbox-audit` 只读模式：可直接列出当前 `delivery_outbox` 行，并按 outbox/event/tenant/conversation/status/event_type 缩小排障范围。
- 已补 `outbox-repair` 运维模式，支持 `audit` 和 `redrive-dlq-pending`，并持久记录 repair audit。
- 已补 `outbox-repair-audit` 只读模式：可直接列出 outbox repair audit 历史，并按 outbox/event/tenant/conversation/mode/outcome 缩小排障范围。
- 已补 `outbox-repair-cleanup` operator：只删除超过保留期的 outbox repair audit 历史，并支持按 outbox/event/tenant/conversation/mode/outcome 缩小范围。
- 已补 `projection-checkpoint-repair` 运维模式，当前只允许带审计地回调 checkpoint 做 replay，不允许前跳跳过事件。
- 已补 `projection-checkpoint-repair-audit` 只读模式：可直接列出 repair audit 历史，并按 mode/outcome 缩小排障范围。
- 已补 `projection-checkpoint-repair-cleanup` operator：只删除超过保留期的 repair audit 历史，并支持按 consumer/topic/partition/mode/outcome 缩小范围。
- 已补 projection fail-closed 持久审计：timeline consumer 在 malformed / projection failure 停下前会写低敏失败记录，但仍不会提交 checkpoint。
- 已补 projection failure resolved 标记：同一 offset 成功重放后会保留失败记录但标记 `resolved`，`/debug/metrics` 只聚合未解决 blocker。
- 已补按 unresolved failure 定点回调 checkpoint：repair 可直接指定 failure offset，先锁定未解决 failure，再带审计回调到该 offset。
- 已补按最早 unresolved failure 自动回调 checkpoint：repair 不再必须手填 offset，也能安全 rewind 到当前最早 blocker。
- 已补 `projection-failure-audit` 只读模式：可直接列出 unresolved projection failure，并支持按 offset / event / failure class 过滤，减少手写 SQL 排障。
- 已补 `projection-failure-cleanup` operator：只删除超过保留期的 resolved failure 审计行，不会碰 unresolved blocker，并支持按 consumer/topic/partition/class 缩小范围。
- `timeline-consumer` 现已对运行时 `Fetch` / `Commit` 错误做退避重试，并在 worker 模式通过 `/debug/metrics` 暴露低敏 retry 快照；malformed event、projection failure、failure recorder 异常仍保持持久审计 + fail-closed，不会自动越过 blocker。
- `outbox-relay` 现已对非取消运行时错误做退避重试，并在 relay 模式通过 `/debug/metrics` 暴露低敏 retry 快照；malformed payload / unsupported event 仍保持 fail-closed，交给 outbox retry / DLQ 语义处理。

## 后续

- Projection DLQ / repair、更多 delivery event 消费方。
