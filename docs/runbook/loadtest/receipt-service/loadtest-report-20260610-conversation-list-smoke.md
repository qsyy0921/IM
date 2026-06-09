# receipt-service Conversation List Smoke

日期：2026-06-10

原始结果：

```text
H:\NexusIM\loadtest-results\receipt-service-conversation-list-smoke-20260610-013109\receipt-summary.json
```

## 目标

验证 `receipt-service` 在不新增独立 conversation-list 服务、不读取 `delivery-service` 内部表的前提下，基于自己的 receipt projection 生成最小会话列表和未读数：

```text
im.delivery.events
-> receipt_inbox_projection
-> user_conversation_summaries
-> ListConversations
-> MarkRead
-> ListConversations
```

本轮不是容量压测，只验证链路和不变量。

## 环境

- commit：`503ff25dd537128a49ffe946285ae15f674aa872`
- git_dirty：`false`
- 原始数据目录：`H:\NexusIM\loadtest-results`
- PostgreSQL / Kafka：本地 Docker
- 参与服务：`conversation-service`、`message-service`、`delivery-service`、`receipt-service`

## 方法

1. 通过 `conversation-service CreateMemberChange(JOIN)` 把 receiver 加入会话。
2. 通过 `message-service SendMessage` 写入一条文本消息。
3. 等待 `delivery-service` 投影到 `user_inbox`，并执行 `AckDelivery`。
4. 等待 `receipt-service` 消费 `im.delivery.events`，写入 `receipt_inbox_projection`、`message_receipt_states` 和 `user_conversation_summaries`。
5. 调用 `ListConversations`，断言会话列表只有一条，`unread_count=1`、`last_read_seq=0`。
6. 调用 `MarkRead(seq=2)`。
7. 再次调用 `ListConversations`，断言 `unread_count=0`、`last_read_seq=2`。
8. 验证 `receipt_outbox` 已发布 `receipt.message.received.v1` 和 `receipt.message.read.v1`。

## 关键结果

| 指标 | 结果 |
| --- | --- |
| SendMessage conversation_seq | `2` |
| PullInbox item_count / max_seq | `1 / 2` |
| AckDelivery last_received_seq | `2` |
| ListConversations before read | `item_count=1, unread_count=1, last_read_seq=0` |
| MarkRead last_read_seq | `2` |
| ListConversations after read | `item_count=1, unread_count=0, last_read_seq=2` |
| receipt_outbox | `total=2, PUBLISHED=2, PENDING=0, DLQ=0` |
| Kafka im.receipt.events readback | `received=1, read=1` |

## 结论

`receipt-service` 已具备最小会话列表 / 未读数 read model：

- 未读数来自 `receipt_inbox_projection` 中 `conversation_seq > last_read_seq` 的可见消息行数，不是简单 seq 差值。
- `delivery.ack.recorded.v1` 只表示 received，不清空 unread。
- `MarkRead` 在同一事务内推进 read cursor 并更新 `user_conversation_summaries`。
- inbox projection 与 `MarkRead` 更新 summary 时使用同一 tenant/user/conversation advisory lock，避免并发投影用旧 read cursor 覆盖新 summary。
- `ListConversations` 返回 projection watermark，便于客户端和排障判断投影进度。

## 边界

- 当前只验证单会话、单 receiver、单条文本消息。
- 还未覆盖多会话分页、mute / pin / archive、草稿、会话置顶、真实权限服务。
- `ListConversations` 不返回消息正文；消息展示仍应由后续消息查询 / PullInbox 路径提供。
- 当前访问控制仍是本地 `StaticAllowAccess`，真实鉴权和 policy-service 后置。
