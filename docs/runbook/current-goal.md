# NexusIM Current Goal

本文件只写当前 Agent Lab 可执行目标。完整历史放在
`docs/runbook/development-progress.md`，剩余 backlog 放在
`docs/runbook/remaining-goals.md`。

## Active Module

Document-driven Agent Lab execution。

Immutable Codex goal 不变：在完全隔离后端层面的前提下，先写出完整的 NexusIM
Agent 层骨架，再基于文档和测试逐步优化。具体阶段、优先级、实验内容和完成条件由
runbook / SDD / research 文档维护。

当前 Phase 1 backend-isolated skeleton 已完成到可执行基线，见
`docs/research/agent-skeleton-completion-audit-20260702.md`。

## 当前事实源

- 架构总览：`docs/architecture/agent-plane-initial-design.md`。
- 当前编码路径：`docs/research/agent-coding-experiment-path-20260701.md`。
- 完成度审计：`docs/research/agent-skeleton-completion-audit-20260702.md`。
- ADR readiness：`docs/research/agent-adr-promotion-readiness-20260702.md`。
- 缺口关闭包：`docs/research/agent-architecture-gap-closure-20260702.md`。
- 对象模型：`docs/research/agent-production-object-model-20260702.md`。
- ADR candidates：`docs/research/adr-candidates/`。
- Review loop：`docs/research/agent-architecture-review-loop-20260702.md`。
- Eval / Replay review：`docs/research/agent-eval-replay-adr-review-20260702.md`。
- Runtime / Workflow review：`docs/research/agent-runtime-workflow-adr-review-20260702.md`。
- Context / EvidencePack review：`docs/research/agent-context-evidencepack-adr-review-20260702.md`。
- Memory / Tool / AgentOps reviews：`docs/research/agent-memory-admission-adr-review-20260702.md`、
  `docs/research/agent-tool-mcp-adr-review-20260702.md`、`docs/research/agentops-governance-adr-review-20260702.md`。
- Fixture evidence：`docs/research/agent-*-fixture-evidence-20260702.md`。
- Operator governance surface evidence：`docs/research/agent-operator-governance-fixture-evidence-20260702.md`。
- 详细 SDD：`docs/sdd/agent-runtime.md`、`agent-memory-admission.md`、
  `agent-context-evidencepack.md`、`agent-tool-mcp-boundary.md`、
  `agent-eval-replay-harness.md`、`agent-governance-agentops.md`。

## 当前边界

- 只做 Agent / RAG / memory / Python AI Worker / EvidencePack / eval gate。
- 第一阶段不使用真实 NexusIM IM 数据，只用公开数据集风格样本和 synthetic
  IM-like fixture。
- 不接生产后端、PostgreSQL、Kafka、Redis、OpenSearch、真实 MCP provider、
  真实 model provider、workflow-service、memory-service 或 action-executor。
- 不写 proto、OpenAPI、Kafka schema、migration、production service directory
  或 production startup path。
- 不冻结 agent taxonomy、skill taxonomy、EvidencePack shape、memory event shape、
  workflow shape、tool shape、MCP shape 或 A2A peer-agent contract。
- Python AI Worker 只产出候选；Go 服务拥有 auth、policy、audit、persistence、
  final proposal、execution 和 ACTIVE memory admission。

## 当前目标

1. 保持 Phase 1 skeleton 作为当前可执行基线，不重新做大范围路线研究。
2. 用 runbook / SDD / research 文档维护阶段推进，不把可变计划写回 immutable goal。
3. 若进入编码，只做 backend-isolated fixture / adapter / eval / report hardening。
4. 若进入架构推进，只起草 ADR candidates，不直接提升生产契约。
5. 完整模块完成后提交并推送到 `origin/codex/agent-lab`，再 handoff 给主集成线程。

## 完成条件

- 变更只落在允许的 Agent Lab 路径。
- 相关 SDD / research / runbook 已更新。
- `python -m pytest ai/python/tests -q`、ruff、mypy、Python AI worker boundary
  和 `git diff --check` 通过，或明确记录无法通过原因。
- 不新增生产 schema、migration、service directory、runtime implementation 或真实
  backend / model / MCP 接入。
- commit 已推送，并向主集成线程 handoff branch、commit hash、changed files、
  checks、风险和下一步建议。

## 后续优先级

1. 六个 ADR candidates 已完成首轮 focused review；未接受前不提升生产契约。
2. Eval / Replay、Runtime / Workflow、Context / Evidence、Memory、Tool、AgentOps、
   dataset reproducibility、cross-service preservation、object completeness 和
   operator governance surface fixture evidence 已落地；未接受前不提升生产契约。
3. 若继续编码，优先主集成 review 指出的 P0/P1 或 focused contract/version hardening。
