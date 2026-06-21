# NexusIM 压测报告索引

压测 / smoke 报告按服务或平台能力归档。每个目录保留该能力的小规模 smoke、
正式矩阵报告、阶段总报告或 evidence manifest。

| 分类 | 入口 |
| --- | --- |
| Core IM services | `message-service/`、`conversation-service/`、`delivery-service/`、`push-gateway/`、`receipt-service/`、`contacts-service/`、`identity-service/`、`policy-service/`、`api-gateway/` |
| Client platform | `client-platform/` |
| AI / Agent foundation | `search-service/`、`memory-service/`、`retrieval-gateway/`、`rag-service/`、`summary-service/`、`agent-service/`、`ai-eval-service/` |
| Product / platform services | `media-service/`、`notification-service/`、`audit-service/`、`admin-service/`、`control-plane-service/`、`presence-service/`、`model-gateway/`、`vector-index-service/`、`knowledge-vector-handoff/` |
| Distributed / local platform | `distributed/`、`demo/` |

## 归档规则

- 每个服务或平台能力使用 `docs/runbook/loadtest/<service-or-platform>/`。
- 阶段性小报告和正式矩阵报告都保留，不覆盖旧文件。
- 每个活跃服务至少维护一份 consolidated 总报告或 README，说明 smoke / 压测如何做、
  如何排查瓶颈、最终结论是什么。
- 原始结果、趋势图和大文件继续放在 `H:\NexusIM\loadtest-results`，默认不提交；仓库只保存报告和必要摘要。

推荐命名：

```text
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<stage>.md
docs/runbook/loadtest/<service>/loadtest-report-YYYYMMDD-<service>-consolidated.md
```
