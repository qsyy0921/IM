# receipt-service

## 当前状态

- 已有 `MarkRead`、`GetReceiptState`、`ListReceiptStates`、`ListConversations`。
- 已支持 unread、archive / pin / mute 的最小会话列表能力。
- 复用 delivery events 和 receipt projection，不跨服务读 delivery 内部表。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏 gRPC、PG pool、receipt projection、conversation summary 和 `receipt_outbox` 聚合状态。
- 已补只读 `outbox-audit`，以及 `outbox-repair` / 只读 `outbox-repair-audit` / `outbox-repair-cleanup` 运维模式，可直接审计、redrive 和清理 `receipt_outbox` repair 历史。

## 后续

- 送达回执扩展、批量接口优化、会话列表产品化。
