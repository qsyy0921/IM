# NexusIM E2E demo smoke

本报告记录一次本地多进程端到端演示 smoke。它验证一条面向产品的最小用户路径，而不是容量或 HA。

## 链路

```text
conversation-service CreateMemberChange(JOIN)
-> message-service SendMessage
-> message outbox relay
-> Kafka conversation.timeline.*
-> delivery-service timeline projection
-> user_inbox + delivery_outbox
-> delivery outbox relay
-> Kafka im.delivery.events
-> push-gateway delivery.notify
-> delivery-service PullInbox
-> push-gateway delivery.ack
-> delivery-service AckDelivery
-> receipt-service MarkRead
-> receipt-service ListConversations
```

## 运行结果

| 项 | 值 |
| --- | --- |
| Result dir | `H:\NexusIM\loadtest-results\e2e-demo-smoke-20260612-013408` |
| Summary | `H:\NexusIM\loadtest-results\e2e-demo-smoke-20260612-013408\e2e-demo-summary.json` |
| Commit | `ffd00e0` |
| Dirty | `true`，因为本轮同时包含未提交 Docker 文档和 demo runner 修复 |
| Success | `true` |

关键事实：

| Check | Result |
| --- | --- |
| receiver JOIN | `boundary_seq=1` |
| SendMessage | `conversation_seq=2` |
| WebSocket notify | `delivery.notify`, `source_event_type=message.persisted.v1`, `conversation_seq=2` |
| PullInbox | `item_count=1`, `max_seq=2` |
| WebSocket ACK | `delivery.ack.ok last_received_seq=2` |
| MarkRead | `last_read_seq=2` |
| List before read | `unread_count=1`, `last_read_seq=0` |
| List after read | `unread_count=0`, `last_read_seq=2` |
| PostgreSQL evidence | `user_inbox_count=1`, `device_delivery_cursor_seq=2`, `user_read_cursor_seq=2`, `user_conversation_summaries=1` |

## 排查与修复

第一次运行失败在 `MarkRead`，错误为 `read out of visible range`。原因是 `PullInbox` 已经能读到 delivery read model，但 receipt-service 的 delivery-event projection 还没追上，runner 立即调用 `MarkRead` 会早于 receipt read model。

修复：`loadtest/demo` 先通过公开 `ListConversations` 等待目标会话出现在 receipt read model，且 `last_visible_seq >= message_seq`、`unread_count=1`，再调用 `MarkRead`。

第二次运行暴露 receipt consumer 读取公共 `im.delivery.events` 历史开发事件时可能先遇到旧 payload。demo smoke 保持 `im.delivery.events` 作为 push-gateway 的固定 topic，但在启动本轮新 consumer group 前将 receipt / push consumer group reset 到 latest，避免把历史开发事件纳入本轮演示证据。

第三次运行失败在 `MarkRead`，错误为 `read out of received range`。原因是 WebSocket ACK 已经返回，但 receipt-service 还没消费到 `delivery.ack.recorded.v1`，即 received projection 尚未追上。

修复：`loadtest/demo` 对 `MarkRead` 的 `FailedPrecondition` 做短重试，直到 ACK projection 追上或超时。

这些等待点都只依赖公开 API，不让 demo runner 通过内部表决定业务成功。

## 边界

- 本报告证明本地多进程产品主链路可运行。
- 它不证明生产级容量、Redis HA、Kafka HA 或 PostgreSQL failover。
- `push-gateway` 仍只是在线唤醒层，展示事实以 `PullInbox` 为准。
- `receipt-service` 的会话列表是异步 projection，runner 必须等待 projection 追上。
