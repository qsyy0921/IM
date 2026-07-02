# NexusIM Current Brief

本文件只做每轮入口摘要。当前 workspace 是 Agent Lab，主线是 Agent / RAG /
memory / AI worker / EvidencePack / eval gate，不承接后端热点群压测。

## 当前状态

- Immutable Codex goal 保持稳定：先完成 backend-isolated Agent skeleton，再用
  runbook / SDD / research 文档维护阶段、优先级和验收条件。
- Phase 1 isolated Agent-layer skeleton 已可作为当前可执行基线：
  `docs/research/agent-skeleton-completion-audit-20260702.md`。
- ADR readiness 已完成，但只能进入候选起草，不能直接推广生产契约：
  `docs/research/agent-adr-promotion-readiness-20260702.md`。
- 架构缺口、生产对象模型、ADR candidate package 和首轮 review ledger 已完成到
  research 级事实源；下一步只做评审或 fixture-only hardening，不直接写生产契约。
- 完整进度历史在 `docs/runbook/development-progress.md`，不要把长历史塞回本文件。

## 当前设计事实源

- 初步架构：`docs/architecture/agent-plane-initial-design.md`。
- 详细 SDD：`docs/sdd/agent-runtime.md`、`agent-memory-admission.md`、
  `agent-context-evidencepack.md`、`agent-tool-mcp-boundary.md`、
  `agent-eval-replay-harness.md`、`agent-governance-agentops.md`。
- 编码路径：`docs/research/agent-coding-experiment-path-20260701.md`。
- 剩余工作：`docs/runbook/remaining-goals.md`。

## 当前可执行基线

- Code: `ai/python/nexusim_ai_eval/`。
- Fixtures: `ai/python/fixtures/agent_eval/`。
- CLIs: `ai/python/scripts/run_agent_eval_*.py` and
  `ai/python/scripts/run_agent_dataset_adapter.py`。
- Tests: `ai/python/tests/test_agent_eval_*.py` plus worker / memory boundary tests。

覆盖能力：EvalCase / EvalRun / EvalResult / EvalReport、ReplayBundle、dataset
adapter skeleton、synthetic IM-like fixtures、AgentRun / AgentStep trace、
ContextPackage / EvidencePack、MemoryCandidate、ToolIntent / MCP security、
runtime control、state-diff、bounded multi-agent handoff、report / regression /
baseline lifecycle 和 memory calibration。

## 不变量

- 第一阶段不使用真实 NexusIM IM 数据。
- 不接 PostgreSQL、Kafka、Redis、OpenSearch、真实 MCP provider、真实 model
  provider、workflow-service、memory-service 或 action-executor。
- 不写 proto、OpenAPI、Kafka schema、migration、production service directory
  或 production startup path。
- Python AI Worker 只产出候选；Go 服务拥有 auth、policy、audit、persistence、
  final proposal、execution 和 ACTIVE memory admission。
- MCP server、tool description 和 provider output 都按不可信输入处理。

## 下一步

若继续架构推进，按 `docs/research/agent-architecture-review-loop-20260702.md`
优先评审 Context / EvidencePack；否则只做 fixture-only hardening。若继续编码，
优先 Eval / Replay version-bump rehearsal 或 Runtime wakeup dedupe fixture。
完整模块完成后 commit、push `origin/codex/agent-lab` 并 handoff。
