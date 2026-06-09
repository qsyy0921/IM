# receipt-service PinConversation Smoke - 2026-06-10

## Scope

本报告记录 `receipt-service` 会话置顶 / 取消置顶的真实进程 smoke。它不是容量压测，目标是验证 `PinConversation` 作为当前用户会话列表偏好，可以和 archive / unread / receipt projection 共存。

原始 summary：

```text
H:\NexusIM\loadtest-results\receipt-service-pin-smoke-clean-20260610\receipt-summary.json
```

代码基线：

```text
commit=bad4dda
commit_full=bad4dda4ac52c6102cc00b2752e6f8657498740c
git_dirty=false
```

## Method

本轮复用 `loadtest/receipt/run-local-smoke.ps1` 启动真实本地进程：

```text
conversation-service
message-service
delivery-service grpc / timeline-consumer / outbox-relay
receipt-service grpc / delivery-consumer / outbox-relay
Kafka topics: conversation.timeline.*, im.delivery.events, im.receipt.events
PostgreSQL: NexusIM local schema
```

执行链路在 archive smoke 基础上增加：

```text
ArchiveConversation(false)
-> ListConversations(default)
-> PinConversation(true)
-> ListConversations(default)
-> PinConversation(false)
-> ListConversations(default)
```

## Key Results

| Check | Result |
| --- | --- |
| clean commit | `bad4dda`, `git_dirty=false` |
| unarchive 后列表 | `item_count=1`, `last_visible_seq=3`, `unread_count=1`, `archived=false`, `pinned=false` |
| PinConversation(true) | response `pinned=true` |
| pin 后默认列表 | `item_count=1`, `last_visible_seq=3`, `unread_count=1`, `archived=false`, `pinned=true` |
| PinConversation(false) | response `pinned=false` |
| unpin 后默认列表 | `item_count=1`, `last_visible_seq=3`, `unread_count=1`, `archived=false`, `pinned=false` |
| receipt_outbox | `total=3`, `PUBLISHED=3`, `PENDING=0`, `DLQ=0` |
| delivery_outbox | `total=4`, `PUBLISHED=4`, `PENDING=0`, `DLQ=0` |

PostgreSQL 集成测试额外覆盖：

- 默认 `ListConversations` 使用 `pinned DESC, sort_updated_at DESC, conversation_id ASC`。
- 显式 `UPDATED_AT_DESC` 仍按纯 `sort_updated_at DESC, conversation_id ASC` 排序。
- pinned-first cursor 能跨 pinned / unpinned 边界分页。
- unknown summary 返回 `CONVERSATION_NOT_FOUND`。

## Conclusion

本轮证明 `PinConversation` 的 v0.1 语义成立：

- 置顶只影响当前用户会话列表排序，不修改 message、delivery、push 或 receipt event 事实。
- 默认列表是 pinned-first；显式 `UPDATED_AT_DESC` 可保留纯更新时间排序。
- pin / archive 是两个独立用户偏好：archive 控制默认列表是否隐藏，pin 控制可见列表中的排序。
- cursor 已升级并绑定 pinned 维度，避免置顶排序分页跨组错页。

## Limits

- 本轮 smoke 只有一个 conversation，真实排序顺序由 PostgreSQL integration test 覆盖。
- 本轮不实现 mute、draft、会话分类、pin 上限或多端偏好同步策略。
- 当前 receipt gRPC 访问控制仍使用本地访问控制实现，不代表完整 policy-service。
