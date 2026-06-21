# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。
不要在这里维护长历史或完整待办。

## 按需读取

- 当前执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应 service brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端。

当前 active slice：

```text
future platform / product services promotion
```

Core services：api-gateway、contacts-service、conversation-service、delivery-service、
identity-service、message-service、policy-service、push-gateway、receipt-service。

AI foundation：action-executor、agent-service、ai-eval-service、mcp-gateway、
memory-service、rag-service、retrieval-gateway、search-service、skill-registry、
summary-service。

future platform / product services 的 10 个目标服务已经进入 product-active
first-stage implementation：admin-service、audit-service、control-plane-service、
knowledge-ingestion-service、media-service、model-gateway、notification-service、
presence-service、vector-index-service、workflow-service。

当前短线重点：

- admin / audit / workflow：继续补公开 API handoff、operator workflow、低敏审批
  review artifact 和补偿边界。
- vector-index：继续 provider backend、pgvector / Milvus / OpenSearch 相关 focused smoke。
- notification：可继续 provider adapter、bounce / suppression。

## 不变量

- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做模型 / 算法 / eval 候选层；Go 负责控制面、状态和审计。
- future 服务之间不得读私有表，必须通过公开 API、事件或明确 port 串联。
- 新发现待完成工作写入 `docs/runbook/remaining-goals.md`。
