# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。不要在这里维护长历史或完整待办。

## 按需读取

- 具体执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应服务 brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/` 或 `docs/runbook/history/`。

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

当前重点调整为：9 个现有服务做必要收口，不再以短期生产级测试为阻塞；向 AI 大模型应用底座转进。

```text
9 服务必要收口：mutation / visibility / contacts privacy / policy / audit / security
-> search-service v0.1
-> memory-service / retrieval-gateway
-> RAG / summary-service / agent-service
-> skill-registry / mcp-gateway / action-executor
```

当前必要收口已推进到 policy-service tool policy precheck / low-sensitive audit。`search-service v0.1` 已跑通真实 projection smoke；`memory-service v0.1` 已跑通 source-backed group memory projection smoke；`retrieval-gateway` / EvidencePack 已完成第一轮真实本地 smoke，并已补 policy precheck、`rerank_score`、`dedupe_reason`、`source_coverage`；AI eval harness first pass 已新增低敏 case schema / validator；`rag-service` 已完成 first read-only answer path、真实本地 RAG adapter smoke、first-stage provider boundary 和 citation verifier；`summary-service` 已完成 first read-only EvidencePack summary path 和真实本地 `retrieval-gateway -> summary-service` adapter smoke；`agent-service` 已完成 first proposal-only path、旧真实本地 `retrieval-gateway -> policy-service -> agent-service` adapter smoke、新的 `retrieval-gateway -> agent-service -> mcp-gateway` adapter smoke、proposal store、approval workflow first path、approval outbox relay first path 和 proposal approval operator first path，proposal 前调用 `mcp-gateway.PrepareToolCall`，返回 `skill_id` / `prepared_audit_id`，审批后可由 executor 公开校验，并可发布低敏 `im.agent.events` approval event；`skill-registry` 已完成 first catalog path；`mcp-gateway` 已完成 first prepare path，把 skill catalog、policy precheck 和低敏 audit 串起来但不执行外部工具；`action-executor` 已完成 first execution audit path、approved proposal / approval / prepare audit 校验、low-sensitive result projection、本地安全 `nexusim.local.echo` adapter、外部 MCP fallback 稳定错误分类和 tool output safety first path，仍不连接外部 MCP/provider，也不执行业务写动作。Agent execution eval adapter 已覆盖 proposal -> approve -> action-executor execution audit / result projection，并新增 safe local tool output case。ADR-036 已固定 Python AI Worker 边界：Python 只做 LLM / embedding / rerank / memory extraction / planner / eval 候选层，Go 继续负责权限、审批、审计、outbox 和持久化。Python AI Worker foundation 已落：`ai/python` 目录、`IM` conda toolchain、candidate contract helpers、低敏 safety guard、contract validator 和 `tools/check-python-ai-worker-boundary.ps1`。RAG/Summary guarded external HTTP LLM boundary 已落：prompt 只能来自 EvidencePack，provider failure 回退 extractive，unsafe / malformed output fail closed，HTTP 明文 endpoint 只允许 loopback / private。当前下一步补 Python worker malformed / unsafe output eval coverage 或第一条 worker smoke。完整系统测试、生产级 HA、长压和 sizing 后置为 hardening backlog。

## 文档职责

- 具体执行目标：`docs/runbook/current-goal.md`
- 当前进度总览：`docs/runbook/development-progress.md`
- 当前未完成工作：`docs/runbook/remaining-goals.md`
- 单服务状态：`docs/runbook/service-briefs/<service>.md`
- 开发过程讲述线：`docs/runbook/development-process.md`
- 面试讲述文档：`docs/interview/project-progress.md`

## 硬约束

- 项目统一命名为 NexusIM，不再引入旧项目名。
- 不回滚用户已有修改。
- 不为了“了解项目”全文读取长历史文档。
- 压测原始数据放 `H:\NexusIM\loadtest-results`，E 盘仓库只放报告和文档；`ResultRoot` / `OutputRoot` 类入口必须拒绝仓库内路径。
- 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
- 门禁按风险分层：小改跑相关测试 / 文档脚本；跨服务、生成代码、migration、registry、Docker/compose、安全边界或提交推送前才跑完整 `.\tools\check-local.ps1`。
- 安全启动门禁索引见 `docs/runbook/security-gate-catalog.json`，api-gateway legacy 迁移证据索引见 `docs/runbook/api-gateway-legacy-evidence.json`，分布式 smoke 证据索引见 `docs/runbook/distributed-smoke-evidence.json`，短容量基线证据索引见 `docs/runbook/capacity-baseline-evidence.json`，长压 campaign 证据索引见 `docs/runbook/capacity-longrun-campaign-evidence.json`，资源快照证据索引见 `docs/runbook/resource-snapshot-evidence.json`，观测 smoke 证据索引见 `docs/runbook/observability-evidence.json`。
