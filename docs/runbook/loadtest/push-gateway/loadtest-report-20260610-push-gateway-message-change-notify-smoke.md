# push-gateway Message Change Notify Smoke - 2026-06-10

## 结论

本轮在 clean commit `81fe92c` 跑通 `message-change-notify` 三组真实进程 smoke：

```text
SendMessage(message.persisted.v1 notify)
-> EditMessage / RevokeMessage / DeleteMessage
-> message outbox relay
-> delivery-service timeline projection
-> delivery_outbox
-> im.delivery.events
-> push-gateway delivery.notify(source_event_type=message.*.v1)
-> client PullInbox 校验 durable inbox event_type
-> delivery.ack
-> delivery-service AckDelivery
-> delivery.ack.ok
```

这证明 push-gateway 能透传消息变更唤醒的 `source_event_type`，但不改变边界：`delivery.notify` 仍只是轻量在线唤醒，客户端展示事实以 `PullInbox` 返回的 durable `user_inbox` item 为准。

## 本轮范围

验证内容：

- 单实例 `NEXUSIM_PUSH_GATEWAY_MODE=all`，route backend 为 `memory`。
- 单 user、单 device、单条 TEXT message。
- 初始 `message.persisted.v1` notify 先到达。
- `edit / revoke / delete` 三类变更 notify 的 `source_event_type` 正确。
- `PullInbox` 中 durable item 的 `event_type + message_id + conversation_seq` 与变更事件一致。
- ACK 推进到变更 seq，`delivery_outbox` drain 到 `PENDING=0 / DLQ=0`。

不验证：

- WebSocket 容量、长连接规模、生产鉴权。
- Redis route / Sentinel / Win-Mac 组合。
- slow-client、resume buffer 与 message-change notify 的组合。
- push-gateway 直接展示消息事实。

## 执行方式

原始数据写入 H 盘：

```text
H:\NexusIM\loadtest-results
```

本轮先修复 `run-local-smoke.ps1`，让每个 `RunName` 默认派生独立 `tenant_id / conversation_id`，避免多组 smoke 复用固定测试数据造成幂等键和消息状态互相污染。

执行命令：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 -Scenario message-change-notify -MessageChangeAction edit -RunName push-gateway-message-change-edit-20260610-r2 -ResultRoot H:\NexusIM\loadtest-results
.\loadtest\pushgateway\run-local-smoke.ps1 -Scenario message-change-notify -MessageChangeAction revoke -RunName push-gateway-message-change-revoke-20260610-r2 -ResultRoot H:\NexusIM\loadtest-results
.\loadtest\pushgateway\run-local-smoke.ps1 -Scenario message-change-notify -MessageChangeAction delete -RunName push-gateway-message-change-delete-20260610-r2 -ResultRoot H:\NexusIM\loadtest-results
```

## 结果

| action | expected source_event_type | persisted notify | change notify | PullInbox event_type | send seq | change seq | ACK seq | user_inbox_count | delivery_outbox P/Pending/DLQ | summary |
| --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | --- | --- |
| edit | `message.edited.v1` | `message.persisted.v1` | `message.edited.v1` | `message.edited.v1` | 2 | 3 | 3 | 2 | `3/0/0` | `H:\NexusIM\loadtest-results\push-gateway-message-change-edit-20260610-r2\pushgateway-summary.json` |
| revoke | `message.revoked.v1` | `message.persisted.v1` | `message.revoked.v1` | `message.revoked.v1` | 2 | 3 | 3 | 2 | `3/0/0` | `H:\NexusIM\loadtest-results\push-gateway-message-change-revoke-20260610-r2\pushgateway-summary.json` |
| delete | `message.deleted.v1` | `message.persisted.v1` | `message.deleted.v1` | `message.deleted.v1` | 2 | 3 | 3 | 2 | `3/0/0` | `H:\NexusIM\loadtest-results\push-gateway-message-change-delete-20260610-r2\pushgateway-summary.json` |

三组 summary 均满足：

- `success=true`
- `commit=81fe92c`
- `git_dirty=false`
- `change_delivery_notify.message_id == send_message.message_id`
- `change_delivery_notify.conversation_seq == message_change.conversation_seq`
- `change_pull_inbox.items[0].event_type == message_change.source_event_type`
- `delivery_ack_ok.last_received_seq == message_change.conversation_seq`
- `cursor_last_received_seq == message_change.conversation_seq`
- `delivery_outbox_pending=0`
- `delivery_outbox_dlq=0`

## 解释

本轮补齐的是第三层 IM 产品能力和 push-gateway 在线唤醒之间的衔接：

```text
message-service 负责消息事实和变更事实
delivery-service 负责 durable inbox 投影
push-gateway 负责在线唤醒和 ACK 转发
```

`source_event_type` 的意义是让客户端在收到在线唤醒时知道该回源读取哪类变更。它不是消息正文，也不是事实源。真正展示的消息变更仍来自 `PullInbox`。

这个切片没有新增服务，也没有让 push-gateway 读取 message-service 或 delivery-service 的内部表；它复用已有 `conversation.timeline.events -> delivery projection -> im.delivery.events -> push-gateway` 事实流，保持微服务低耦合。

## 剩余风险

- 本轮是单实例、单设备、单消息功能 smoke，不是容量结论。
- 生产鉴权、Redis route、Sentinel、Win-Mac、slow-client、resume buffer 与 message-change notify 的组合仍未覆盖。
- 客户端仍必须按 `event_id / conversation_id + seq` 做幂等，且以本地 durable cursor 决定 `PullInbox` 起点。

