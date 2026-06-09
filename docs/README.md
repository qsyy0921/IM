# NexusIM 文档索引

本文只做文档入口索引，具体设计以各文档正文为准。

| 目录 / 文档 | 说明 |
| --- | --- |
| `architecture/` | 总架构和架构补充文档。主文档是 `architecture/target-architecture.md`。 |
| `sdd/` | 服务级软件设计文档。`message-service` 已完成第一阶段主链路，当前推进 `conversation-service`。 |
| `runbook/` | 本地运行、压测、故障处理和演练说明。压测报告按微服务归档到 `runbook/loadtest/<service>/`。 |

## 当前阅读路径

每轮 Codex 工作必须先读 `runbook/current-goal.md`，再按当前目标进入其他文档。

1. `runbook/current-goal.md`：确认当前阶段、评审要求、风险和下一步。
2. `architecture/target-architecture.md`：确认目标态、技术栈和不可退让项。
3. `sdd/conversation-service.md`：确认当前 `GetSendContext` read path 和成员事实边界。
4. `sdd/conversation-service-member-change-saga.md`：确认下一步成员变更 Saga、边界事件和 ACL 投影失败窗口。
5. `sdd/message-service.md`：确认已完成的 `SendMessage` 普通会话写入链路和对 conversation port 的依赖。
6. `architecture/tadd.md`：确认工程目录、六层 DDD、Docker Compose 和编码门禁。
7. `runbook/conversation-service-local.md`：确认当前 conversation-service 本地启动和跨服务 smoke 方式。
8. `runbook/local-loadtest.md`：确认本地双机压测端口和执行方式。
9. `runbook/loadtest/message-service/loadtest-report-20260609-message-service-consolidated.md`：查看 `message-service` 第一阶段压测总报告、瓶颈排查过程和面试可讲结论。
10. `runbook/loadtest/message-service/README.md`：查看 `message-service` 所有小规模压测报告和矩阵报告索引。

## 写文档规则

- 架构、SDD、Runbook 均使用中文。
- 总架构不要继续堆服务级细节；服务级细节写入 `sdd/`。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
- 文档中提到的路径必须和仓库真实路径一致。
