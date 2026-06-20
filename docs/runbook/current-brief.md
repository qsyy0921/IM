# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。
不要在这里维护长历史或完整待办。

## 按需读取

- 具体执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应服务 brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端。
9 个后端服务已进入真实链路：`api-gateway`、`identity-service`、`message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`receipt-service`、`contacts-service`、`policy-service`。

当前 active slice 已切到 `future platform / product services promotion`：

```text
future services -> SDD v0.1 -> stage-switch plan -> service-by-service skeleton
```

AI foundation-active 服务：`search-service`、`memory-service`、`retrieval-gateway`、`rag-service`、`summary-service`、`agent-service`、`skill-registry`、`mcp-gateway`、`action-executor`、`ai-eval-service`。

`ai-eval-service` catalog、gate、negative / action / memory evals、
action-executor provider failure worker / redrive safety eval、memory-service
runtime `at_conversation_seq` query semantics 和 source-ref / validity /
supersession PG coverage / smoke checks、retrieval-gateway EvidencePack
current-only memory query、RAG / Summary / Agent API 显式 at seq 透传、
RAG / Summary / Agent current-memory consumption CI-safe regression、memory
extraction confidence / review eval、current-memory service-stack live smoke、
cross-group / temporal retrieval smoke、RAG / Summary / Agent stack consumption smoke 和 optional stack gate 已落；
只保存低敏 run refs / counters / metadata。

Go 侧服务底座、EvidencePack、proposal / approval / audit 和 eval 持久化
已足够支撑算法切片；后续 Go 工作围绕候选接入、边界校验和状态流转。

当前下一步：

```text
media / notification / audit / control-plane / admin / presence / model-gateway / knowledge-ingestion SDD draft 已存在；下一步推进 workflow-service SDD
```

完整系统测试、生产级 HA、长压和 sizing 继续后置为 hardening backlog。
文档职责：进度总览见 `development-progress.md`，未完成工作见
`remaining-goals.md`，单服务状态见 `service-briefs/<service>.md`。

- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做模型 / 算法 / eval 候选层，Go 负责控制面、状态和审计。
- future 服务 promotion 期间不得一次性创建全部服务目录。
- 媒体、通知、审计、控制面、presence、model、workflow、ingestion、vector 等边界必须继续通过公开 API、事件或明确 port 串联。
- 不回滚用户已有修改。
- 不为了“了解项目”全文读取长历史文档。
- 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
