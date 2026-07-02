# NexusIM Research Index

这个目录只维护研究资料的短索引和分类规则，不存放论文 PDF。

## 入口

- `paper-organization.md`：Zotero 与本地 PDF 的分类规则。
- `agent-plane-redesign-20260701.md`：Agent Plane 重新设计研究入口。
- `agent-runtime-workflow-ownership-20260701.md`：Agent Runtime 与 workflow-service
  ownership matrix。
- `agent-ecosystem-research-20260701.md`：OpenClaw、Hermes、Claude Code、OpenAI
  Agents SDK、LangGraph、MCP、A2A、benchmark、安全论文和企业报告输入。
- `agent-system-complete-scope-20260701.md`：2026 完整 Agent 系统能力范围和
  open-dataset-first 开发流程。
- `agent-current-design-review-20260701.md`：当前 Agent 平台设计评审结论；方向正确，
  但 `agent-platform.md` v0.1 被 P1 打回，不能单独推广实现。
- `agent-current-to-target-matrix-20260701.md`：当前 AI / Agent foundation 服务到目标
  Agent 平台的迁移和 ownership matrix。
- `agent-open-dataset-eval-plan-20260701.md`：公开数据集优先的 Agent eval 计划，
  包含 EvalCase / EvalRun / EvalResult / synthetic IM-like fixture 草案。
- `agent-coding-experiment-path-20260701.md`：隔离式 Agent 编码实验路径，记录
  `ai/python/nexusim_ai_eval`、fixture、CLI、单元测试、集成测试和后续切片顺序。
- `agent-adr-promotion-readiness-20260702.md`：隔离式 Agent eval / replay /
  memory calibration skeleton 的 ADR 候选 readiness review；不冻结生产契约。
- `agent-skeleton-completion-audit-20260702.md`：不可变 Agent Lab goal 对当前
  backend-isolated skeleton 的完成度审计，确认 Phase 1 骨架可作为当前可执行基线，
  但不提升生产契约。
- `agent-architecture-gap-closure-20260702.md`：资深架构评审后的缺口关闭包，
  给出 ADR candidate map、版本策略、集成 preservation matrix 和生产提升阻断条件。
- `agent-production-object-model-20260702.md`：Agent 生产级对象模型草案，
  收敛 decision、version、runtime、evidence、memory、tool、workflow、AgentOps、
  dataset/eval 和 operator UX 对象；不冻结生产 schema。
- `adr-candidates/`：Agent ADR candidate package，包含 Eval / Replay、
  Runtime / Workflow、Context / EvidencePack、Memory Admission、Tool / MCP、
  AgentOps / Governance 六份候选 ADR 和 cross-service versioning / replay /
  governance appendix；仍不是正式 ADR。
- `agent-architecture-review-loop-20260702.md`：首轮 Agent 架构
  review-and-closure ledger，记录打回原因、P0/P1/P2 缺口、补齐项和复审结论。
- `agent-eval-replay-adr-review-20260702.md`：Eval / Replay ADR candidate
  focused review，补齐 failure-class lifecycle、baseline approval、retention /
  redaction 和 contract-version bump rehearsal。
- `agent-runtime-workflow-adr-review-20260702.md`：Runtime / Workflow ADR
  candidate focused review，补齐 checkpoint storage、wakeup dedupe、operator
  controls 和 budget ledger 边界。
- `agent-context-evidencepack-adr-review-20260702.md`：Context / EvidencePack
  ADR candidate focused review，补齐 source visibility、denied-lane、citation
  verifier、taint vocabulary 和 operator evidence inspection。
- `agent-memory-admission-adr-review-20260702.md`：Memory Admission ADR
  candidate focused review，补齐 lifecycle states、category thresholds、
  revocation / dependency invalidation、audit explanation 和 operator UX。
- `agent-tool-mcp-adr-review-20260702.md`：Tool / MCP ADR candidate focused
  review，补齐 capability lease、provider attestation、prepare expiry /
  re-prepare、sandbox onboarding 和 output reuse。
- `agentops-governance-adr-review-20260702.md`：AgentOps / Governance ADR
  candidate focused review，补齐 kill switch、release pinning、baseline
  approval、failure-class owner workflow 和 canary/shadow comparison。
- `agent-fixture-evidence-hardening-20260702.md`：fixture-only evidence 更新，
  记录 Eval / Replay version-bump rehearsal 的代码、fixture、测试和剩余条件。

## 存放位置

- Zotero collection：`NexusIM`
- 本地 PDF：`H:\NexusIM\papers\im-ai-agent-rag-2026`
- 仓库内只放分类规则、引用说明和读论文后形成的设计结论。
- Agent Lab 的当前研究结论需要回链到 `docs/sdd/agent-platform.md`，不要只停留在
  原始资料摘录。
