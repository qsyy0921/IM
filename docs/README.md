# NexusIM 文档索引

本文只做文档入口索引，具体设计以各文档正文为准。

| 目录 / 文档 | 说明 |
| --- | --- |
| `architecture/` | 总架构和架构补充文档。主文档是 `architecture/target-architecture.md`。 |
| `sdd/` | 服务级软件设计文档。第一阶段主文档是 `sdd/message-service.md`。 |
| `runbook/` | 本地运行、压测、故障处理和演练说明。 |

## 当前阅读路径

1. `architecture/target-architecture.md`：确认目标态、技术栈和不可退让项。
2. `sdd/message-service.md`：确认第一条可编码切片。
3. `architecture/tadd.md`：确认工程目录、六层 DDD、Docker Compose 和编码门禁。
4. `runbook/current-goal.md`：确认当前阶段、评审要求、风险和下一步。
5. `runbook/local-loadtest.md`：确认本地双机压测端口和执行方式。

## 写文档规则

- 架构、SDD、Runbook 均使用中文。
- 总架构不要继续堆服务级细节；服务级细节写入 `sdd/`。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
- 文档中提到的路径必须和仓库真实路径一致。
