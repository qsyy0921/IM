# NexusIM Current Brief

本文是每轮 Codex 工作的低 token 入口。需要细节、历史、风险和报告索引时，再查 `docs/runbook/current-goal.md`。

## 当前定位

NexusIM 已完成四个真实微服务的最小链路：

```text
conversation-service
-> message-service
-> delivery-service
-> push-gateway
```

当前系统可以表述为“本地多进程 + Win/Mac 双机 Docker 最小分布式 IM 后端”。已跑通发消息、会话上下文、PostgreSQL outbox、Kafka timeline、durable inbox、PullInbox、AckDelivery、WebSocket notify、Redis route、cross-instance resume、Win/Mac 双机 smoke、Redis Sentinel discovery、手动 failover 和停止当前 master 后的自动切主 recovery smoke。

## 当前优先级

1. 当前分布式证据已经够用于面试讲“最小分布式 IM 后端”，不要继续长期停留在重型基础设施故障矩阵。
2. 当前第三层产品能力已切到送达 / 已读回执：receipt-service 已完成 proto / Kafka schema / migration / 六层骨架、PostgreSQL repository、delivery event consumer、`MarkRead` 事务和 receipt outbox relay。
3. receipt-service 真实进程 smoke 已跑通：`im.delivery.events -> receipt projection -> MarkRead -> GetReceiptState -> receipt_outbox -> im.receipt.events`；不要直接读取 delivery-service 内部表。
4. 会话列表 / 未读数最小 `ListConversations` 已在 receipt-service 内落地并跑通真实进程 smoke：`unread_count` 从投递后的 `1` 在 `MarkRead` 后变为 `0`；当前已补 `source_event_type` 口径和 `updated_at desc` keyset 分页契约，edit/revoke/delete 可推进会话 `last_visible_seq` 和 `last_source_event_type`，但不作为新未读消息计数。该能力复用 `im.delivery.events`、`receipt_inbox_projection` 和 `user_read_cursors`，没有新增独立服务，也不读取其它服务内部表。
5. 当前第三层消息变更能力已在 clean commit `8d008de` 跑通 `RevokeMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> RevokeMessage -> message outbox relay -> delivery tombstone projection -> PullInbox(message.revoked.v1) -> AckDelivery`；并已补 hardening：第一阶段只允许原发送者撤回，delivery tombstone 只投给已在 `user_inbox` 收到原消息的用户，revoke 早于 persisted 投影时 fail-closed 不提交 checkpoint。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-revoke-message-smoke.md`。
6. 当前第三层消息变更能力已在 clean commit `cb2f07d` 跑通 `EditMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> EditMessage -> message outbox relay -> delivery edit projection -> PullInbox(message.edited.v1) -> AckDelivery`；第一阶段限定原发送者编辑自己的 TEXT 消息，采用 last-write-wins + `message_change_history` 保留 before/after payload，不引入新服务或跨服务内部表读取。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-edit-message-smoke.md`。
7. 当前第三层消息变更能力已在 clean commit `b001eb1` 跑通 `DeleteMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> DeleteMessage -> message outbox relay -> delivery delete projection -> PullInbox(message.deleted.v1) -> AckDelivery`；第一阶段语义是全局 `CONVERSATION_VIEW` tombstone，不是用户私有删除或合规物理擦除。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-delete-message-smoke.md`。
8. push-gateway 在线 `delivery.notify` 已保持轻量通知边界并透传 `source_event_type`，让客户端能区分新增 / 编辑 / 撤回 / 删除唤醒；展示事实仍以 `PullInbox` 为准。
9. push-gateway 标准 smoke runner 已在 clean commit `81fe92c` 跑通 `message-change-notify` 三类真实进程 smoke：`edit / revoke / delete` 均证明在线 `delivery.notify.source_event_type` 与 durable `PullInbox.event_type + message_id + conversation_seq` 一致；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260610-push-gateway-message-change-notify-smoke.md`。
10. conversation-service 已补第三层群管理最小读能力：`ListConversationMembers` 只返回当前 ACTIVE 成员，调用者必须是 ACTIVE 成员；clean commit `99aacc6` 的真实进程 smoke 通过，3 条 `JOIN` 后 roster 返回 4 个 ACTIVE 成员；clean commit `be2e039` 的 `REMOVE` roster smoke 通过，目标成员变为 `LEFT` 后不再出现在 ACTIVE roster。它是低耦合 roster API，不让其它服务跨表读取 `conversation_members`，也不把成员历史 / 审计视图塞进普通列表。
11. RAG / Agent / 智能总结属于第四层，必须等消息事实、权限边界、撤回/编辑/删除语义更稳定后再做。
12. Kafka HA、PostgreSQL failover、Redis quorum / 网络分区可作为后续生产化项，不作为当前主线阻塞。

## 硬边界

- 项目名统一为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 `delivery-service user_inbox`。
- 开发时优先降低微服务耦合、控制代码复杂度；不要为了“分布式”引入网状依赖、跨服务内部表读取、不必要的同步 RPC 或过度抽象。实现方案如果明显变复杂，先拆成更小切片，或先补契约 / SDD 再编码。
- 新能力优先复用已有事实流、outbox、projection、read model 和端口；只有能减少重复、稳定边界或支撑真实链路时才新增服务、表、公共包或抽象。
- 单个切片保持小闭环：先补契约 / migration / 本地事务 / consumer 或 relay / smoke，再扩展 hardening；不要一次性横跨多个产品能力。
- 开发中可以主动创建 sub-agent 做设计、实现、测试、文档或风险复核；专项任务结束后及时关闭，不要长期占用线程池。
- 压测原始数据放到 `H:\NexusIM\loadtest-results`，E 盘仓库只放报告和文档。
- Win/Mac 服务间通信优先使用有线 `172.31.50.*`，不要把服务间流量走外网或代理。
- 不回滚用户已有修改。

## 每轮开始

```powershell
git status --short --branch
Get-Content docs\runbook\current-brief.md -Raw
```

如需细节再按需查询：

```powershell
Select-String -Path docs\runbook\current-goal.md -Pattern "关键词" -Context 2,4
```

## 每轮结束

- 更新 `docs/runbook/current-brief.md` 的当前优先级。
- 如果阶段状态、风险或历史证据变化，再同步更新 `docs/runbook/current-goal.md`。
- 有意义的切片完成后运行必要检查、提交；批量推送 GitHub，不为低风险小改动频繁推送。
