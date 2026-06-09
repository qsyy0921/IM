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
4. 会话列表 / 未读数最小 `ListConversations` 已在 receipt-service 内落地并跑通真实进程 smoke：`unread_count` 从投递后的 `1` 在 `MarkRead` 后变为 `0`；该能力复用 `im.delivery.events`、`receipt_inbox_projection` 和 `user_read_cursors`，没有新增独立服务，也不读取其它服务内部表。
5. 当前第三层消息变更能力已在 clean commit `8d008de` 跑通 `RevokeMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> RevokeMessage -> message outbox relay -> delivery tombstone projection -> PullInbox(message.revoked.v1) -> AckDelivery`；并已补 hardening：第一阶段只允许原发送者撤回，delivery tombstone 只投给已在 `user_inbox` 收到原消息的用户，revoke 早于 persisted 投影时 fail-closed 不提交 checkpoint。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-revoke-message-smoke.md`。
6. 后续第三层候选：消息编辑/删除、真实鉴权、多会话分页 / 权限校验强化；优先选能补全 IM 产品闭环、且不会显著增加跨服务耦合和代码复杂度的切片。下一步建议先做 RevokeMessage 阶段复核收口，再决定是否扩展 Edit/Delete。
7. RAG / Agent / 智能总结属于第四层，必须等消息事实、权限边界、撤回删除语义更稳定后再做。
8. Kafka HA、PostgreSQL failover、Redis quorum / 网络分区可作为后续生产化项，不作为当前主线阻塞。

## 硬边界

- 项目名统一为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 `delivery-service user_inbox`。
- 开发时优先降低微服务耦合、控制代码复杂度；不要为了“分布式”引入网状依赖、跨服务内部表读取或过度抽象。
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
