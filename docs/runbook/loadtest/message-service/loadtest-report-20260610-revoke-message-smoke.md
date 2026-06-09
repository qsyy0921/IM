# Message Service RevokeMessage Smoke

日期：2026-06-10

原始结果：

```text
H:\NexusIM\loadtest-results\message-revoke-smoke-20260610-023321\message-revoke-summary.json
```

Commit：`8d008de`，`git_dirty=false`。

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
message_id=msg_95c439de-50c5-4bf5-bdc2-ce5079e6695d
persisted_event_id=548b5c9a-b0e1-4489-9fb9-ee892a57a38f
revoked_event_id=8c73751e-0bae-449b-a1a8-c76d86c15a5a
```

## 实现边界

本轮覆盖：

- `message-service` gRPC `RevokeMessage` handler。
- app use case + domain command hash / change record。
- PostgreSQL 同事务更新 `message_log`、写 `message_change_history`、写 `conversation_timeline_events`、写 `message_outbox`。
- message outbox relay 构造 `message.revoked.v1` Kafka payload。
- delivery timeline consumer / repository 将 revoke 事件投影为 `user_inbox` tombstone。
- `PullInbox` 可见 tombstone，`AckDelivery` 可推进到 revoke seq。
- delivery tombstone 可见性 hardening：撤回只投给已经在 `user_inbox` 收到原 `message.persisted.v1` 的用户；原消息发送后才加入的成员，即使撤回时是 ACTIVE，也不会收到该消息的 tombstone。
- Revoke 权限 hardening：第一阶段只允许原消息发送者撤回自己的消息；`message.revoke.any` 这类管理员能力留给真实 policy-service 后续扩展。
- Projection fail-closed：delivery-service 如果先收到 `message.revoked.v1`，但本地还没有原 `message.persisted.v1` inbox 投影，会返回 projection dependency error，不提交 Kafka checkpoint，避免静默丢 tombstone。

本轮不覆盖：

- `EditMessage` / `DeleteMessage`。
- 撤回权限的真实 policy-service 规则；当前仍使用阶段性 policy mock。
- push-gateway 在线通知 revoke tombstone。
- 多设备、多会话、并发撤回、撤回后搜索/回执修正。

## 排查过程

第一次 smoke 没有失败在业务链路，而是暴露出 runner 清理逻辑耦合了 receipt-service 的表；当前本地库没有 `receipt_events`，导致清理阶段报错。修复方式是把 `loadtest/messagerevoke` 的清理范围收窄到 message / conversation / delivery 三条链路相关表，不把 receipt 表纳入 RevokeMessage smoke。

第二次 smoke 主链路通过，但立即采样时 `delivery_outbox` 仍有 2 条 `PENDING`，原因是 runner 在 `AckDelivery` 后立刻读 DB，没有等 delivery outbox relay 追平。随后 runner 增加了 `waitDeliveryOutboxDrained`，最终结果为 `delivery_outbox PUBLISHED=3`、无 `PENDING/DLQ`。

阶段复核时发现一个可见性风险：如果 revoke 按撤回时的 ACTIVE 成员窗口投影，原消息发送后才加入的成员也可能收到 tombstone。修复后 delivery-service 不再用当前成员窗口决定 revoke fanout，而是从自己的 `user_inbox` 查询已收到原消息的用户作为 tombstone 目标；新增真实 PostgreSQL 集成测试覆盖 `user-2` 在原消息后加入、撤回发生时仍不收到该 tombstone。

阶段复核还发现两个边界问题：第一，policy check 在 app 层发生时还不知道原消息 sender，容易把“任意活跃成员撤回任意消息”放过；修复后 repository 在锁住原消息后校验 actor 必须等于 sender。第二，delivery projection 不能只依赖上游顺序；现在 revoke 找不到本地原消息投影时会 fail-closed，不写 checkpoint，等待重试或 repair。

本地 smoke 还暴露了一个测试隔离问题：message outbox relay 会全局扫描 `message_outbox`，历史 PostgreSQL 集成测试残留的 `tenant-it-*` PENDING rows 会污染本次临时 Kafka topic。`loadtest/messagerevoke/run-local-smoke.ps1` 已在启动 relay 前清理测试租户和本 smoke 租户的 PENDING/DLQ outbox rows，只用于本地 smoke 隔离，不是生产逻辑。

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
