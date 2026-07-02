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

## 存放位置

- Zotero collection：`NexusIM`
- 本地 PDF：`H:\NexusIM\papers\im-ai-agent-rag-2026`
- 仓库内只放分类规则、引用说明和读论文后形成的设计结论。
- Agent Lab 的当前研究结论需要回链到 `docs/sdd/agent-platform.md`，不要只停留在
  原始资料摘录。
