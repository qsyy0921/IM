# Message Service EditMessage Smoke

日期：2026-06-10

原始结果：

```text
H:\NexusIM\loadtest-results\message-edit-smoke-20260610-030109\message-edit-summary.json
```

Commit：`cb2f07d`，`git_dirty=false`。

## 结论

`EditMessage` 已跑通最小真实进程链路：

```text
CreateMemberChange(JOIN)
-> SendMessage
-> message outbox relay
-> delivery timeline consumer
-> PullInbox(message.persisted.v1)
-> EditMessage
-> message outbox relay
-> delivery edit projection
-> PullInbox(message.edited.v1)
-> AckDelivery
```

这轮 smoke 证明编辑消息不是只改 `message_log.payload_json`，而是进入共享 conversation timeline，并由 delivery-service 投影成 receiver 可见的 `message.edited.v1` inbox item。它是单会话、单消息、单 receiver 的功能 smoke，不是容量压测。

## 关键结果

| 检查项 | 结果 |
| --- | --- |
| `CreateMemberChange(JOIN)` | `boundary_seq=1` |
| `SendMessage` | `conversation_seq=2` |
| 第一次 `PullInbox` | 拉到 `message.persisted.v1`，`seq=2` |
| `EditMessage` | `conversation_seq=3`，`change_version=1` |
| 第二次 `PullInbox` | 拉到 `message.edited.v1`，`seq=3`，同一 `message_id` |
| `AckDelivery` | `last_received_seq=3` |
| `message_log.status` | `EDITED` |
| `message_log.payload_json` | `{"text": "message edit smoke updated"}` |
| `message_log.edited_at` | 非空，样本为 `2026-06-09T19:01:26.795866Z` |
| `message_change_history` | 1 行，`EDIT / NORMAL -> EDITED` |
| history before payload | `{"text": "message edit smoke original"}` |
| history after payload | `{"text": "message edit smoke updated"}` |
| `conversation_timeline_events` | `conversation.member.joined.v1=1`、`message.persisted.v1=1`、`message.edited.v1=1` |
| `message_outbox` | `PUBLISHED=3` |
| `user_inbox` | `message.persisted.v1=1`、`message.edited.v1=1` |
| `delivery_outbox` | `PUBLISHED=3` |
| delivery checkpoint | `offset_value=3` |

样本消息：

```text
message_id=msg_42ddaba4-b536-4ec6-9e0e-465b12900677
persisted_event_id=feb38c2e-a230-4396-8a9f-f0010ff32620
edited_event_id=45f861d9-a47c-489b-91a5-ec40f858bf7b
```

## 实现边界

本轮覆盖：

- `message-service` gRPC `EditMessage` handler。
- app use case + domain command hash / edit record。
- PostgreSQL 同事务更新 `message_log.payload_json/status/edited_at`、写 `message_change_history`、写 `conversation_timeline_events`、写 `message_outbox`。
- message outbox relay 构造 `message.edited.v1` Kafka payload。
- delivery timeline consumer / repository 将 edit 事件投影为 `user_inbox` 中新的 changed item。
- `PullInbox` 可见 edited event，`AckDelivery` 可推进到 edit seq。
- delivery edit 可见性 hardening：只投给已经在 `user_inbox` 收到原 `message.persisted.v1` 的用户；原消息缺失时 projection fail-closed，不提交 checkpoint。

本轮不覆盖：

- `DeleteMessage`。
- 管理员编辑、多人协同编辑、expected change version / CAS 冲突控制。
- 真实 policy-service；当前仍使用阶段性 policy mock。
- push-gateway 在线通知 edited item。
- 多设备、多会话、并发编辑、编辑后搜索 / 回执 / 会话摘要修正。

## 排查过程

这轮 smoke runner 由 `RevokeMessage` runner 改造而来。最初复制后存在两个典型机械替换问题：

- 事件名被替换成了错误的 `message.editd.v1`，会导致 `PullInbox` 永远等不到 `message.edited.v1`。
- `EditMessageRequest` 未携带 `Payload`，只能证明调用到接口，不能证明真实 payload 编辑。

修复后，runner 明确发送原始 payload：

```json
{"text":"message edit smoke original"}
```

再通过 `EditMessage` 写入编辑 payload：

```json
{"text":"message edit smoke updated"}
```

为避免弱证据，runner 不只检查 `message_change_history=1`，还会读取并校验：

```text
message_log.status = EDITED
message_log.payload_json = updated payload
message_log.edited_at IS NOT NULL
message_change_history.change_type = EDIT
message_change_history.before_status = NORMAL
message_change_history.after_status = EDITED
before_payload / after_payload 分别等于 original / updated
```

本地脚本仍会在启动 relay 前清理测试租户和本 smoke 租户的 PENDING/DLQ outbox rows。这只用于本地 smoke 隔离，避免历史集成测试残留被全局 message outbox relay 发布到本轮临时 topic，不是生产逻辑。

## 面试讲法

这轮可以这样讲：

```text
编辑消息被建模为一条新的时间线事实，而不是原地覆盖。
message-service 在同一个 PostgreSQL 事务里更新 message_log 当前态、写 message_change_history before/after payload、分配新的 conversation_seq、写 timeline event 和 outbox；
outbox relay 发布 message.edited.v1；
delivery-service 消费后只给已经收到原消息的用户写入 edited item；
客户端 PullInbox 看到 seq 更高的编辑事件，再 ACK 到这个 seq。
这样编辑、撤回、投递和断线补拉都共享同一条 conversation_seq 顺序轴。
```

注意不要说这是完整编辑系统。当前只是单会话、单消息、单 receiver 的最小编辑闭环，尚未覆盖并发编辑冲突、管理员权限、push 在线通知和搜索/回执修正。
