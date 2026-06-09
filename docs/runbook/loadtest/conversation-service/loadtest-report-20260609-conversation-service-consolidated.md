# conversation-service 阶段总报告

## 阶段定位

`conversation-service` 当前阶段已经完成两条最小链路：

```text
GetSendContext
-> conversations / conversation_members
-> 返回发送上下文
-> message-service SendMessage 使用真实 gRPC 依赖
```

```text
CreateMemberChange
-> member_change_saga / conversation_members / conversations version
-> conversation_seq
-> conversation_timeline_events
-> message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
```

以及一个低耦合的当前成员 roster 读接口：

```text
ListConversationMembers
-> conversations / conversation_members
-> 返回当前 ACTIVE 成员列表
```

它仍不是完整的会话服务。`GetMemberChange` 和成员变更 Saga completion worker 已经完成最小闭环；DLQ repair、ACL 投影和完整角色变更流程后续单独实现。

## 当前能力

| 能力 | 状态 |
| --- | --- |
| 独立六层 DDD 目录 | 已完成 |
| gRPC proto | 已完成，`api/proto/nexusim/conversation/v1/conversation_service.proto` |
| PostgreSQL migration | 已完成，`migrations/postgres/conversation/000001_conversation_core.sql` |
| `GetSendContext` app/domain/repository | 已完成 |
| `CreateMemberChange` app/domain/repository | 已完成最小 `JOIN` 写路径 |
| `ListConversationMembers` app/repository/gRPC | 已完成当前 ACTIVE 成员列表读路径 |
| gRPC handler | 已完成 |
| message-service gRPC client | 已完成，可通过 `NEXUSIM_CONVERSATION_SERVICE_ADDR` 启用 |
| 真实 PostgreSQL repository 集成测试 | 已通过 |
| message-service -> conversation-service smoke | 已通过 |
| CreateMemberChange -> outbox relay -> Kafka smoke | 已通过 |
| GetMemberChange | 已完成，并在完整 smoke 中返回 `DONE` |
| member-change-worker | 已完成，能观察 outbox `PUBLISHED` 并推进 saga `DONE` |

## 核心结论

第一轮 smoke 证明：

- `message-service` 不再只能依赖 conversation strict mock。
- 会话发送上下文已经由真实 `conversation-service` 读取。
- `message-service` 仍保持边界：只读 conversation context，不写成员事实。
- `conversation-service` 已经能独立写成员事实，并把成员边界事件放入共享 timeline/outbox。
- 统一 outbox relay 可以发布 `conversation.member.*` 到 Kafka，并把 outbox 标记为 `PUBLISHED`。
- `conversation-service` 的 member-change-worker 可以观察已发布 outbox，并把本地 saga 推进到 `DONE`。
- `GetMemberChange` 可以通过 gRPC 返回成员变更完成态。
- 当前链路足以支撑后续继续开发 `delivery-service` / `push-gateway`，不需要继续在 `message-service` 上做大规模压测。

## 验证记录

| 报告 | 结论 |
| --- | --- |
| `loadtest-report-20260609-send-context-smoke.md` | 725 / 725 成功，p99 13.26ms，跨服务 read path 打通 |
| `loadtest-report-20260609-member-change-smoke.md` | 279 / 279 成功，p99 24.95ms，outbox 279 条全部 PUBLISHED |
| `loadtest-report-20260609-member-change-full-smoke.md` | 350 / 350 成功，p99 40.90ms，saga 350 条全部 DONE，GetMemberChange 返回 DONE |
| `loadtest-report-20260610-list-conversation-members-smoke.md` | 3 / 3 JOIN 成功，随后 `ListConversationMembers` 返回 4 个 ACTIVE 成员 |
| `loadtest-report-20260610-list-conversation-members-remove-smoke.md` | 1 / 1 REMOVE 成功，目标成员状态变为 `LEFT`，随后 `ListConversationMembers` 不再返回目标成员 |

## 面试讲法

可以这样讲：

> 第一阶段我先把 `message-service` 的 SendMessage 主链路做完整。后面没有继续只刷压测数字，而是把 mock 依赖拆出来，落了第二个真实微服务 `conversation-service`。它拥有 conversations 和 conversation_members 两张核心事实表，提供 `GetSendContext` gRPC 接口。message-service 通过端口读取 member_version、permission_version、conversation_mode 和 fanout_mode，再进入本地事务写消息。这样服务边界更真实，也能解释为什么 message-service 不能直接写成员事实。

成员变更可以这样讲：

> `conversation-service` 负责成员事实变更。`CreateMemberChange` 会在一个 PostgreSQL 事务里推进 member_change_saga、conversation_members、conversation 版本号、conversation_seq、conversation_timeline_events 和 message_outbox。成员边界事件和消息事件共享同一条 conversation timeline，所以 delivery、retrieval 和 audit 后续只需要消费统一的 `conversation.timeline.events`。本地完整 smoke 里 350 条成员加入全部成功，outbox relay 发布后 350 条都变成 PUBLISHED，member-change-worker 又把 350 条 saga 推进到 DONE，GetMemberChange 能读到 DONE。这个阶段证明第二个微服务已经不是文档服务，而是有真实写路径、事件出口和后台任务闭环。当前只验证保守权限矩阵下的 JOIN，不能把它说成完整成员管理系统。

成员列表可以这样讲：

> 做群管理时我没有让 message-service、delivery-service 或 push-gateway 直接读 conversation_members。conversation-service 作为成员事实源提供 `ListConversationMembers`，第一版只列当前 ACTIVE 成员，并要求调用者本身是 ACTIVE 成员。这样 roster 查询走正式服务边界，后续再补 admin-only 历史成员、owner transfer、邀请审批等能力，不把普通列表接口一次性做复杂。

REMOVE 后 roster 过滤可以这样讲：

> `REMOVE` 不做物理删除，而是把目标成员状态写成 `LEFT` 并记录 `leave_seq`。普通 `ListConversationMembers` 只返回 ACTIVE 成员，所以被移除用户不会再出现在普通 roster；历史成员和审计视图后续通过 admin-only 查询单独设计，避免把普通列表接口做复杂。

## 下一步

1. 补 `LEAVE / ROLE_CHANGED` 的真实进程 smoke 或至少 repository / gRPC 负例覆盖，并验证 `ListConversationMembers` 在成员离开 / 角色变化后的行为。
2. 补 DLQ repair 设计和最小实现。
3. 进入 `delivery-service` 或 `push-gateway` 前，先明确它们的 SDD 和最小可运行链路。
