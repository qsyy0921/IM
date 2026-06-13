# receipt-service

## 当前状态

- 已有 `MarkRead`、`GetReceiptState`、`ListReceiptStates`、`ListConversations`。
- 已支持 unread、archive / pin / mute 的最小会话列表能力。
- 复用 delivery events 和 receipt projection，不跨服务读 delivery 内部表。
- 已补只读 `outbox-audit` 运维模式，可按 outbox_id / event_id / tenant_id / conversation_id / status / event_type 直接审计 `receipt_outbox`。

## 后续

- 送达回执扩展、批量接口优化、会话列表产品化。
