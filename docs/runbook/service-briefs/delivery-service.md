# delivery-service

## 当前状态

- 已有 timeline projection、durable `user_inbox`、`PullInbox`、`AckDelivery`、`delivery_outbox` relay。
- 是 push-gateway 的可靠事实源。
- 不要求 push-gateway 持久化消息或 ACK cursor。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 基础观测入口，可在 gRPC / timeline-consumer / outbox-relay 模式下独立挂载。
- 已补 `outbox-repair` 运维模式，支持 `audit` 和 `redrive-dlq-pending`，并持久记录 repair audit。
- 已补 `projection-checkpoint-repair` 运维模式，当前只允许带审计地回调 checkpoint 做 replay，不允许前跳跳过事件。
- 已补 projection fail-closed 持久审计：timeline consumer 在 malformed / projection failure 停下前会写低敏失败记录，但仍不会提交 checkpoint。
- 已补 projection failure resolved 标记：同一 offset 成功重放后会保留失败记录但标记 `resolved`，`/debug/metrics` 只聚合未解决 blocker。
- 已补按 unresolved failure 定点回调 checkpoint：repair 可直接指定 failure offset，先锁定未解决 failure，再带审计回调到该 offset。

## 后续

- Projection DLQ / repair、更多 delivery event 消费方。
