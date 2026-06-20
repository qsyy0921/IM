# NexusIM 压测报告索引

压测报告按微服务归档。每个微服务一个目录，目录内保留该服务所有小规模压测报告、正式矩阵报告和阶段总报告。

| 微服务 | 入口 |
| --- | --- |
| `message-service` | `message-service/loadtest-report-20260609-message-service-consolidated.md` |
| `conversation-service` | `conversation-service/loadtest-report-20260609-conversation-service-consolidated.md` |
| `identity-service` | `identity-service/README.md` |
| `notification-service` | `notification-service/README.md` |

## 归档规则

- 每个微服务使用 `docs/runbook/loadtest/<service>/`。
- 阶段性小报告和正式矩阵报告都保留，不覆盖旧文件。
- 每个微服务至少维护一份 consolidated 总报告，说明压测如何做、如何排查瓶颈、最终结论是什么。
- 原始结果、趋势图和大文件继续放在 `H:\NexusIM\loadtest-results`，默认不提交；仓库只保存报告和必要摘要。

推荐命名：

```text
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<stage>.md
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<service>-consolidated.md
```
