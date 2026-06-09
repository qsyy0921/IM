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
2. 当前第三层产品能力已切到送达 / 已读回执：receipt-service 已落 proto / Kafka schema / migration / 六层骨架、PostgreSQL repository、delivery event consumer 和 `MarkRead` 事务。
3. 下一步做 receipt-service 真实进程 smoke：`im.delivery.events -> receipt projection -> MarkRead -> GetReceiptState`；随后补 receipt outbox relay / `im.receipt.events` 发布链路。不要直接读取 delivery-service 内部表。
4. 后续第三层候选：消息编辑/撤回/删除、会话列表/未读数、真实鉴权。
5. RAG / Agent / 智能总结属于第四层，必须等消息事实、权限边界、撤回删除语义更稳定后再做。
6. Kafka HA、PostgreSQL failover、Redis quorum / 网络分区可作为后续生产化项，不作为当前主线阻塞。

## 硬边界

- 项目名统一为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 `delivery-service user_inbox`。
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
