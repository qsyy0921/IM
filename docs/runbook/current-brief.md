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

已进入真实链路的 9 个后端服务：

```text
api-gateway
identity-service
message-service
conversation-service
delivery-service
push-gateway
receipt-service
contacts-service
policy-service
```

当前唯一默认主线是 AI 大模型应用底座：

```text
group memory -> EvidencePack -> RAG -> summary -> multi-agent
-> skill-registry -> mcp-gateway/tool-gateway -> action-executor
-> proposal / approval / audit -> ai-eval
```

当前已落的 AI foundation-active 服务：

```text
search-service
memory-service
retrieval-gateway
rag-service
summary-service
agent-service
skill-registry
mcp-gateway
action-executor
ai-eval-service
```

`ai-eval-service` first persistent eval run catalog 已落，第一版只保存低敏
run summary / refs / metadata，不运行 eval，不保存 raw prompt / EvidencePack /
model output。

当前下一步：

```text
wire existing AI eval scripts to ai-eval-service RecordEvalRun smoke
```

完整系统测试、生产级 HA、长压和 sizing 继续后置为 hardening backlog。

## 文档职责

- 当前进度总览：`docs/runbook/development-progress.md`
- 当前未完成工作：`docs/runbook/remaining-goals.md`
- 单服务状态：`docs/runbook/service-briefs/<service>.md`
- 开发过程讲述线：`docs/runbook/development-process.md`
- 面试讲述文档：`docs/interview/project-progress.md`

## 硬约束

- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做模型 / 算法 / eval 候选层，Go 负责控制面、状态、
  审计和持久化。
- 不回滚用户已有修改。
- 不为了“了解项目”全文读取长历史文档。
- 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
- 门禁按风险分层：小改跑相关测试 / 文档脚本；跨服务、生成代码、
  migration、registry、Docker/compose、安全边界或提交推送前才跑完整
  `.\tools\check-local.ps1`。
