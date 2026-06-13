# receipt-service verified metadata auth smoke

日期：2026-06-13

## 结论

通过。

本轮验证 `loadtest/receipt/run-local-smoke.ps1 -VerifiedAuthMetadata` 可以在真实本地进程中跑通 receipt-service 的核心用户链路：

```text
CreateMemberChange(metadata auth)
-> SendMessage(metadata auth)
-> PullInbox(metadata auth)
-> AckDelivery(metadata auth)
-> receipt projection
-> GetReceiptState(metadata auth)
-> ListConversations(metadata auth)
-> MarkRead(metadata auth)
-> receipt_outbox
-> im.receipt.events
-> ArchiveConversation / PinConversation / MuteConversation(metadata auth)
```

这证明 conversation / message / delivery / receipt 四个 user-facing gRPC server 在 metadata auth 模式下，可以完成投递、回执、会话列表、未读、归档、置顶和静音的最小真实进程 smoke。

这不是完整 API gateway、真实生产鉴权、证书治理或全服务 mTLS 结论。

## 命令

```powershell
.\loadtest\receipt\run-local-smoke.ps1 `
  -VerifiedAuthMetadata `
  -RunName receipt-verified-metadata-smoke-20260613-184324
```

## 原始结果

```text
H:\NexusIM\loadtest-results\receipt-verified-metadata-smoke-20260613-184324\receipt-summary.json
```

关键元数据：

```text
commit=4cd165e
git_dirty=false
verified_auth_metadata=true
conversation_tls_enabled=false
message_tls_enabled=false
delivery_tls_enabled=false
receipt_tls_enabled=false
```

## 关键证据

基础投递 / 回执：

```text
member_join boundary_seq=1 member_version=2 permission_version=2
SendMessage message_id=msg_a810560c-0d85-4459-82c9-b53ab66af248 conversation_seq=2
PullInbox item_count=1 max_seq=2
AckDelivery last_received_seq=2
GetReceiptState before read: received_user_count=1 read_user_count=0
MarkRead last_read_seq=2
GetReceiptState after read: received_user_count=1 read_user_count=1
GetReceiptState by message_id: read_user_count=1
```

会话列表 / 未读：

```text
ListConversations before read: item_count=1 unread_count=1 last_read_seq=0
ListConversations(unread_only=true) before read: item_count=1
ListConversations after read: item_count=1 unread_count=0 last_read_seq=2
ListConversations(unread_only=true) after read: item_count=0
```

归档 / 置顶 / 静音：

```text
ArchiveConversation archived=true
default list after archive item_count=0
include_archived list item_count=1 archived=true
new message while archived conversation_seq=3
include_archived after new message last_visible_seq=3 unread_count=1
UnarchiveConversation archived=false
PinConversation pinned=true
UnpinConversation pinned=false
MuteConversation muted=true
UnmuteConversation muted=false
```

outbox / Kafka：

```text
receipt_outbox total=3 published=3 pending=0 dlq=0
receipt.message.received.v1=2
receipt.message.read.v1=1
delivery_outbox total=4 published=4 pending=0 dlq=0
```

## Preflight Isolation

本轮同时给 `loadtest/receipt/run-local-smoke.ps1` 增加本地 preflight cleanup：启动 message relay 前，只清理测试 / smoke tenant 前缀下 `status <> 'PUBLISHED'` 的 `message_outbox` 残留，避免旧测试事件进入本次临时 timeline topic。该修复和业务服务逻辑无关，只用于提高本地 smoke 可复现性。

## 边界

- 本轮只验证单机本地多进程、小规模 happy path 和少量偏好操作。
- `-VerifiedAuthMetadata` 验证的是 gateway verified metadata 接口形态，不代表完整 API gateway。
- 本轮不启用 TLS / mTLS，不代表证书签发、轮换、分发或动态服务身份治理。
- receipt-service 仍只消费 `im.delivery.events` 并维护自己的 read model，不读取 delivery-service 内部表。
- `receipt_outbox -> im.receipt.events` 已发布，但本轮不验证下游真实消费者。
