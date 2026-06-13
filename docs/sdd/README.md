# NexusIM SDD Index

这是服务级 SDD 的短索引。不要把服务状态、历史评审或 smoke 结果写到这里。

## 读取规则

- 当前工作入口先读 `docs/runbook/current-brief.md`。
- 只在修改某个服务设计或契约时读取对应 SDD。
- 需要历史 SDD 状态时按关键词查 `docs/sdd/archive/README-20260614-long.md`。

## 服务 SDD

- `message-service.md`
- `conversation-service.md`
- `conversation-service-member-change-saga.md`
- `delivery-service.md`
- `push-gateway.md`
- `receipt-service.md`
- `receipt-service-conversation-list.md`
- `contacts-service.md`
- `identity-service.md`
- `policy-service.md`
- `api-gateway.md`
- `search-service.md`

## 通用约束

- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布。
- 不跨服务读取内部表；服务间状态传播优先 Kafka facts + 本服务 projection。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
