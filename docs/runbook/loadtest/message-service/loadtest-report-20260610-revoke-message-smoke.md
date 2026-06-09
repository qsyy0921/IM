# Message Service RevokeMessage Smoke

日期：2026-06-10

原始结果：

```text
H:\NexusIM\loadtest-results\message-revoke-smoke-20260610-021639\message-revoke-summary.json
```

Commit：`9a1a776`，`git_dirty=false`。

## 结论

`RevokeMessage` 已跑通最小真实进程链路：

```text
CreateMemberChange(JOIN)
-> SendMessage
-> message outbox relay
-> delivery timeline consumer
-> PullInbox(message.persisted.v1)
-> RevokeMessage
-> message outbox relay
-> delivery tombstone projection
-> PullInbox(message.revoked.v1)
-> AckDelivery
```

这轮 smoke 证明撤回不是只改 `message_log` 状态，而是已经进入共享 conversation timeline，并被 delivery-service 投影成用户可见 tombstone item。它仍是小规模功能 smoke，不是容量压测。

## 关键结果

| 检查项 | 结果 |
| --- | --- |
| `CreateMemberChange(JOIN)` | `boundary_seq=1` |
| `SendMessage` | `conversation_seq=2` |
| 第一次 `PullInbox` | 拉到 `message.persisted.v1`，`seq=2` |
| `RevokeMessage` | `conversation_seq=3`，`change_version=1` |
| 第二次 `PullInbox` | 拉到 `message.revoked.v1`，`seq=3`，同一 `message_id` |
| `AckDelivery` | `last_received_seq=3` |
| `message_log.status` | `REVOKED` |
| `message_change_history` | 1 行 |
| `conversation_timeline_events` | `conversation.member.joined.v1=1`、`message.persisted.v1=1`、`message.revoked.v1=1` |
| `message_outbox` | `PUBLISHED=3` |
| `user_inbox` | `message.persisted.v1=1`、`message.revoked.v1=1` |
| `delivery_outbox` | `PUBLISHED=3` |
| delivery checkpoint | `offset_value=3` |

样本消息：

```text
message_id=msg_a5771f10-e1f7-40be-ac8d-c687b57c1f03
persisted_event_id=8ec64e94-daac-41d2-82b0-526d273683c2
revoked_event_id=b8b3f8c6-ab80-499d-81ad-40a1881f5e29
```

## 实现边界

本轮覆盖：

- `message-service` gRPC `RevokeMessage` handler。
- app use case + domain command hash / change record。
- PostgreSQL 同事务更新 `message_log`、写 `message_change_history`、写 `conversation_timeline_events`、写 `message_outbox`。
- message outbox relay 构造 `message.revoked.v1` Kafka payload。
- delivery timeline consumer / repository 将 revoke 事件投影为 `user_inbox` tombstone。
- `PullInbox` 可见 tombstone，`AckDelivery` 可推进到 revoke seq。

本轮不覆盖：

- `EditMessage` / `DeleteMessage`。
- 撤回权限的真实 policy-service 规则；当前仍使用阶段性 policy mock。
- push-gateway 在线通知 revoke tombstone。
- 多设备、多会话、并发撤回、撤回后搜索/回执修正。

## 排查过程

第一次 smoke 没有失败在业务链路，而是暴露出 runner 清理逻辑耦合了 receipt-service 的表；当前本地库没有 `receipt_events`，导致清理阶段报错。修复方式是把 `loadtest/messagerevoke` 的清理范围收窄到 message / conversation / delivery 三条链路相关表，不把 receipt 表纳入 RevokeMessage smoke。

第二次 smoke 主链路通过，但立即采样时 `delivery_outbox` 仍有 2 条 `PENDING`，原因是 runner 在 `AckDelivery` 后立刻读 DB，没有等 delivery outbox relay 追平。随后 runner 增加了 `waitDeliveryOutboxDrained`，最终结果为 `delivery_outbox PUBLISHED=3`、无 `PENDING/DLQ`。

## 面试讲法

这轮可以这样讲：

```text
撤回消息不是简单更新一条消息状态。我把它做成了和发消息同一条时间线上的事实事件：
message-service 在一个 PostgreSQL 事务里更新 message_log、写 change_history、分配新的 conversation_seq、写 timeline event 和 outbox；
outbox relay 再把 message.revoked.v1 发布到 Kafka；
delivery-service 消费 timeline 后给用户 inbox 写一条 tombstone；
客户端 PullInbox 看到 seq 更高的撤回事件，再 ACK 到该 seq。
这样撤回、投递、断线补拉和 outbox 至少一次语义能保持一致。
```

注意不要说这是生产级消息变更完整能力。当前只证明了单会话、单消息、单 receiver 的最小撤回闭环。
