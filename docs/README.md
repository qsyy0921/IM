# NexusIM 文档索引

本文只做文档入口索引，具体设计以各文档正文为准。

| 目录 / 文档 | 说明 |
| --- | --- |
| `architecture/` | 总架构和架构补充文档。主文档是 `architecture/target-architecture.md`。 |
| `sdd/` | 服务级软件设计文档。当前已落地 `message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`identity-service`、`policy-service`、`receipt-service`、`contacts-service` 和第一版 `api-gateway` 的设计入口。 |
| `runbook/` | 本地运行、压测、故障处理和演练说明。压测报告按微服务归档到 `runbook/loadtest/<service>/`。 |

## 当前阅读路径

每轮 Codex 工作必须先读 `runbook/current-brief.md`。只有需要细节、历史证据或风险上下文时，再按关键词查询 `runbook/current-goal.md`，不要每轮全文读取长文档。

1. `runbook/current-brief.md`：低 token 当前入口，确认当前优先级、硬边界和下一步。
2. `runbook/current-goal.md`：完整历史、风险、评审要求和报告索引；只按需查询。
3. `architecture/target-architecture.md`：确认目标态、技术栈和不可退让项。
4. `sdd/api-gateway.md`：确认统一 user-facing gRPC 入口、gateway token 验证和 trusted metadata 传播边界。
5. `sdd/receipt-service.md`：确认当前第三层产品能力切片，送达 / 已读回执边界。
6. `sdd/delivery-service.md`：确认 durable inbox、AckDelivery 和 delivery event 边界。
7. `sdd/push-gateway.md`：确认在线通知、Redis route、resume buffer 和 ACK 转发边界。
8. `sdd/conversation-service.md`：确认当前 `GetSendContext` read path 和成员事实边界。
9. `sdd/conversation-service-member-change-saga.md`：确认成员变更 Saga、边界事件和 ACL 投影失败窗口。
10. `sdd/message-service.md`：确认已完成的 `SendMessage` 普通会话写入链路和对 conversation port 的依赖。
11. `architecture/tadd.md`：确认工程目录、六层 DDD、Docker Compose 和编码门禁。
12. `runbook/distributed-local.md`：确认当前本地多进程分布式 smoke 拓扑和运行入口。

## 写文档规则

- 架构、SDD、Runbook 均使用中文。
- 总架构不要继续堆服务级细节；服务级细节写入 `sdd/`。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
- 文档中提到的路径必须和仓库真实路径一致。
