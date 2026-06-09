# Message Service DeleteMessage Smoke

日期：2026-06-10

原始结果：

```text
H:\NexusIM\loadtest-results\message-delete-smoke-20260610-032242\message-delete-summary.json
```

Commit：`b001eb1`，`git_dirty=false`。

## 结论

`DeleteMessage` 已跑通最小真实进程链路：

```text
CreateMemberChange(JOIN)
-> SendMessage
-> message outbox relay
-> delivery timeline consumer
-> PullInbox(message.persisted.v1)
-> DeleteMessage
-> message outbox relay
-> delivery delete projection
-> PullInbox(message.deleted.v1)
-> AckDelivery
```

这轮 smoke 证明删除消息不是本地假更新，而是进入共享 conversation timeline，并由 delivery-service 投影成 receiver 可见的 `message.deleted.v1` inbox item。它是单会话、单消息、单 receiver 的功能 smoke，不是容量压测。

## 关键结果

| 检查项 | 结果 |
| --- | --- |
| `CreateMemberChange(JOIN)` | `boundary_seq=1` |
| `SendMessage` | `conversation_seq=2` |
| 第一次 `PullInbox` | 拉到 `message.persisted.v1`，`seq=2` |
| `DeleteMessage` | `conversation_seq=3`，`change_version=1` |
| 第二次 `PullInbox` | 拉到 `message.deleted.v1`，`seq=3`，同一 `message_id` |
| `AckDelivery` | `last_received_seq=3` |
| `message_log.status` | `DELETED` |
| `message_log.payload_json` | 仍保留原始 payload：`{"text": "message delete smoke original"}` |
| `message_log.deleted_at` | 非空，样本为 `2026-06-09T19:22:59.093757Z` |
| `message_change_history` | 1 行，`DELETE / NORMAL -> DELETED` |
| history before payload | `{"text": "message delete smoke original"}` |
| history after payload | `NULL` |
| `conversation_timeline_events` | `conversation.member.joined.v1=1`、`message.persisted.v1=1`、`message.deleted.v1=1` |
| `message_outbox` | `PUBLISHED=3` |
| `user_inbox` | `message.persisted.v1=1`、`message.deleted.v1=1` |
| `delivery_outbox` | `PUBLISHED=3` |
| delivery checkpoint | `offset_value=3` |

样本消息：

```text
message_id=msg_2c3631b5-d0c1-4c30-92f9-e91dba8e2000
persisted_event_id=92bcd897-80cf-4863-bdf9-1142e29290e3
deleted_event_id=be1e1ff4-71f2-4389-a87c-9400807d7330
```

## 语义边界

第一阶段 `DeleteMessage` 是全局 conversation-view tombstone：

```text
DeleteScope = CONVERSATION_VIEW
message_log.status = DELETED
conversation.timeline.events 追加 message.deleted.v1
delivery-service 给已收到原消息的用户写入 deleted item
```

这不是用户私有的“只为我删除”，也不是合规删除 / 物理清除：

- 不按 user 维护私有 delete marker。
- 不删除历史 `message_log.payload_json`。
- 不做对象存储、搜索索引、审计日志的合规擦除。
- 不代表管理员删除、群主删除或风控删除已经上线。

这样处理是为了把第一阶段复杂度控制在合理范围内：复用现有 message change 事务、outbox、timeline 和 delivery projection，不新增独立 delete 服务或跨服务内部表读取。

## 实现边界

本轮覆盖：

- `message-service` gRPC `DeleteMessage` handler。
- app use case + domain command hash / delete record。
- PostgreSQL 同事务更新 `message_log.status/deleted_at`、写 `message_change_history`、写 `conversation_timeline_events`、写 `message_outbox`。
- message outbox relay 构造 `message.deleted.v1` Kafka payload。
- delivery timeline consumer / repository 将 delete 事件投影为 `user_inbox` 中新的 tombstone item。
- `PullInbox` 可见 deleted event，`AckDelivery` 可推进到 delete seq。
- delivery delete 可见性 hardening：只投给已经在 `user_inbox` 收到原 `message.persisted.v1` 的用户；原消息缺失时 projection fail-closed，不提交 checkpoint。

本轮不覆盖：

- 用户私有删除 / delete for me。
- 合规删除 / 物理擦除 / retention purge。
- 管理员删除、群主删除、风控删除。
- 多设备、多会话、并发删除。
- push-gateway 在线通知 deleted item。
- 搜索索引、receipt、conversation summary 的删除修正。

## 排查过程

这轮 smoke runner 由 `EditMessage` runner 改造而来。初版机械替换后存在几个会让证据失真的问题：

- 常量被替换成 `eventMessageDeleteed`，事件名读起来容易误判。
- `DeleteMessageRequest` 仍携带 edit payload，但真实 delete API 不应该修改 payload。
- PostgreSQL 断言仍按 edit 语义检查 `after_payload` 和更新后的 `message_log.payload_json`。

修复后，runner 明确验证 delete 语义：

```text
message_log.status = DELETED
message_log.deleted_at IS NOT NULL
message_log.payload_json 仍等于 original payload
message_change_history.change_type = DELETE
message_change_history.before_status = NORMAL
message_change_history.after_status = DELETED
message_change_history.after_payload_json IS NULL
```

本地脚本仍会在启动 relay 前清理测试租户和本 smoke 租户的 PENDING/DLQ outbox rows。这只用于本地 smoke 隔离，避免历史集成测试残留被全局 message outbox relay 发布到本轮临时 topic，不是生产逻辑。

## 面试讲法

这轮可以这样讲：

```text
删除消息没有被做成直接从数据库里物理删除，而是建模为一条新的时间线事实。
message-service 在同一个 PostgreSQL 事务里更新 message_log 当前态、写 message_change_history、分配新的 conversation_seq、写 timeline event 和 outbox；
outbox relay 发布 message.deleted.v1；
delivery-service 消费后只给已经收到原消息的用户写入 deleted tombstone；
客户端 PullInbox 看到 seq 更高的删除事件，再 ACK 到这个 seq。
这样发送、编辑、撤回、删除都共享同一条 conversation_seq 顺序轴，断线补拉也能恢复状态变化。
```

注意不要说这是完整删除系统。当前只是 `CONVERSATION_VIEW` 的全局 tombstone 最小闭环，尚未覆盖用户私有删除、合规擦除、管理员权限、push 在线通知和搜索/回执修正。
