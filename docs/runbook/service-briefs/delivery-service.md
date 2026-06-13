# delivery-service

## 当前状态

- 已有 timeline projection、durable `user_inbox`、`PullInbox`、`AckDelivery`、`delivery_outbox` relay。
- 是 push-gateway 的可靠事实源。
- 不要求 push-gateway 持久化消息或 ACK cursor。
- 已补 `/healthz`、`/readyz`、`/debug/metrics` 基础观测入口，可在 gRPC / timeline-consumer / outbox-relay 模式下独立挂载。
- 已补 `outbox-repair` 运维模式，支持 `audit` 和 `redrive-dlq-pending`，并持久记录 repair audit。

## 后续

- Projection DLQ / repair、更多 delivery event 消费方。
