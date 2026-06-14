# NexusIM 文档索引

本文只做短入口索引，具体设计以各文档正文为准。

| 目录 / 文档 | 说明 |
| --- | --- |
| `architecture/` | 总架构和架构补充文档。入口是 `architecture/target-architecture.md`，细节分卷在 `target-architecture-*.md`。 |
| `sdd/` | 服务级软件设计文档。当前已落地 `message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`identity-service`、`policy-service`、`receipt-service`、`contacts-service` 和第一版 `api-gateway` 的设计入口。 |
| `runbook/` | 本地运行、压测、故障处理和演练说明。压测报告按微服务归档到 `runbook/loadtest/<service>/`。 |
| `interview/project-progress.md` | 面试用项目进度说明：已完成能力、未完成能力、讲述线和后续路线。 |

## 当前阅读路径

Codex 目标框只放根目录 `../prompt.md` 里的短 Prompt。每轮 Codex 工作先读仓库根目录 `prompt.md`，再按它的路由读取 `runbook/current-brief.md` 和必要短文档。需要找文档时先读 `runbook/README.md`，再按需进入短索引或具体服务文档。

1. `../prompt.md`：Codex 目标 prompt 的唯一维护源。
2. `runbook/current-brief.md`：低 token 当前入口，确认当前优先级、硬边界和下一步。
3. `runbook/README.md`：runbook 短路由页。
4. `runbook/current-goal.md`：长期目标摘要；只按需查询。
5. `architecture/target-architecture.md`：总架构短入口，按需跳转到 foundation / timeline / platform 分卷。
6. `sdd/api-gateway.md`：确认统一 user-facing gRPC 入口、gateway token 验证和 trusted metadata 传播边界。
7. `sdd/receipt-service.md`：确认当前第三层产品能力切片，送达 / 已读回执边界。
8. `sdd/delivery-service.md`：确认 durable inbox、AckDelivery 和 delivery event 边界。
9. `sdd/push-gateway.md`：确认在线通知、Redis route、resume buffer 和 ACK 转发边界。
10. `sdd/conversation-service.md`：确认当前 `GetSendContext` read path 和成员事实边界。
11. `sdd/conversation-service-member-change-saga.md`：确认成员变更 Saga、边界事件和 ACL 投影失败窗口。
12. `sdd/message-service.md`：确认已完成的 `SendMessage` 普通会话写入链路和对 conversation port 的依赖。
13. `architecture/tadd.md`：确认工程目录、六层 DDD、Docker Compose 和编码门禁。
14. `runbook/distributed-local.md`：确认当前本地多进程分布式 smoke 拓扑和运行入口。

## 写文档规则

- 架构、SDD、Runbook 均使用中文。
- 总架构不要继续堆服务级细节；服务级细节写入 `sdd/`。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
- 文档中提到的路径必须和仓库真实路径一致。
