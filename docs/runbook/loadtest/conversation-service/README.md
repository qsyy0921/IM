# Conversation Service 压测报告入口

本目录保存 `conversation-service` 的小规模验证报告和阶段总报告。

当前阶段重点不是做容量压测，而是证明第二个真实微服务已经接入：

```text
message-service SendMessage
-> ConversationQueryPort
-> conversation-service gRPC GetSendContext
-> PostgreSQL conversations / conversation_members
-> message-service PostgreSQL local transaction
```

## 当前结论

- `conversation-service` 已有独立六层 DDD 骨架。
- `GetSendContext` 已经能从 PostgreSQL 读取会话状态、成员状态、版本号、会话模式和 fanout 策略。
- `message-service` 已支持通过 `NEXUSIM_CONVERSATION_SERVICE_ADDR` 切到真实 conversation-service，不再只能依赖 strict mock。
- 第一轮真实进程 smoke 结果：`725 / 725` 成功，p95 `10.36ms`，p99 `13.26ms`。
- 本轮 smoke 没启动 outbox relay，因此 summary 中 `outbox_pending_count=725` 是预期现象；测试结束后已删除本次 tenant 数据，避免污染后续压测。

## 报告列表

| 类型 | 文件 |
| --- | --- |
| 阶段总报告 | `loadtest-report-20260609-conversation-service-consolidated.md` |
| 第一轮跨服务 smoke | `loadtest-report-20260609-send-context-smoke.md` |

## 面试可讲点

- 这一步把系统从“一个 message-service + mock”推进到“两个真实微服务之间的 gRPC 依赖”。
- `conversation-service` 是会话和成员事实源，`message-service` 只读取发送上下文，不写成员事实。
- 版本号通过 `member_version` / `permission_version` 返回给 `message-service`，用于发送前的一致性校验。
- 本轮只验证 read path，成员变更 Saga、群主/管理员规则、ACL 投影后续单独冻结 SDD。
