# receipt-service ArchiveConversation Smoke - 2026-06-10

## Scope

本报告记录 `receipt-service` 会话归档 / 取消归档的真实进程 smoke。它不是容量压测，目标是验证 `ArchiveConversation` 作为用户侧列表过滤偏好，不影响消息事实、delivery、receipt projection、unread 或 outbox 发布。

原始 summary：

```text
H:\NexusIM\loadtest-results\receipt-service-archive-smoke-clean-20260610\receipt-summary.json
```

代码基线：

```text
commit=f8ab746
commit_full=f8ab74600ec8a0acf316a4f66b5d73a754b66249
git_dirty=false
```

## Method

本轮复用现有 receipt smoke runner，启动真实本地进程：

```text
conversation-service
message-service
delivery-service grpc / timeline-consumer / outbox-relay
receipt-service grpc / delivery-consumer / outbox-relay
Kafka topics: conversation.timeline.*, im.delivery.events, im.receipt.events
PostgreSQL: NexusIM local schema
```

执行链路：

```text
CreateMemberChange(JOIN)
-> SendMessage(seq=2)
-> PullInbox
-> AckDelivery
-> receipt projection
-> MarkRead(seq=2)
-> ListConversations
-> ArchiveConversation(true)
-> ListConversations(default / include_archived)
-> SendMessage while archived(seq=3)
-> PullInbox + AckDelivery
-> ListConversations(default / include_archived)
-> ArchiveConversation(false)
-> ListConversations(default)
```

## Key Results

| Check | Result |
| --- | --- |
| 首条消息 | `conversation_seq=2`, `message_id=msg_f949b6dd-0b1e-40e6-881d-9ce12de71c81` |
| 投递后列表 | `item_count=1`, `unread_count=1`, `archived=false` |
| MarkRead 后列表 | `item_count=1`, `unread_count=0`, `last_read_seq=2`, `archived=false` |
| ArchiveConversation(true) | response `archived=true` |
| 默认列表 after archive | `item_count=0` |
| include_archived after archive | `item_count=1`, `last_visible_seq=2`, `archived=true` |
| 归档期间第二条消息 | `conversation_seq=3`, `message_id=msg_e8303d0c-4be1-4a00-b253-4ac9fe1c968a` |
| 默认列表 after archived new message | `item_count=0` |
| include_archived after archived new message | `item_count=1`, `last_visible_seq=3`, `unread_count=1`, `last_read_seq=2`, `archived=true` |
| ArchiveConversation(false) | response `archived=false` |
| 默认列表 after unarchive | `item_count=1`, `last_visible_seq=3`, `unread_count=1`, `last_read_seq=2`, `archived=false` |
| receipt_outbox | `total=3`, `PUBLISHED=3`, `PENDING=0`, `DLQ=0` |
| delivery_outbox | `total=4`, `PUBLISHED=4`, `PENDING=0`, `DLQ=0` |

## Conclusion

本轮证明 `ArchiveConversation` 的 v0.1 语义成立：

- 归档只影响当前用户默认会话列表过滤，不删除 message / delivery / receipt 事实。
- `include_archived=true` 能查回归档会话，并保留 `last_visible_seq`、`unread_count`、`last_read_seq`。
- 归档期间新消息仍会推进 receipt projection 和 unread，但不会自动取消归档。
- 取消归档后默认列表恢复可见，且保留最新 seq、未读数和读游标。

## Limits

- 本轮只覆盖单 tenant、单 conversation、单 receiver device、两条 TEXT message。
- 本轮不覆盖 pin、mute、draft、会话分类、搜索排序或多会话矩阵。
- 本轮不验证真实 policy；当前 receipt gRPC 仍使用本地访问控制实现。
- `ArchiveConversation` 是用户列表偏好，不是消息删除、隐藏历史或合规擦除。
