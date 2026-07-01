# NexusIM Agent Lab Development Process

这份文档回答 Agent Lab 应该按什么过程开发。它不再描述完整 IM 后端的阶段路线；当前
workspace 的目标是重新设计并验证 Agent 层。

## 0. 总原则

NexusIM Agent 层的开发顺序不是先接真实聊天数据，也不是先写生产 runtime。当前推荐路线是：

```text
广泛研究
-> 平台级 SDD
-> 当前设计评审；如有 P0/P1 则打回重做
-> 详细 SDD 包
-> open dataset eval harness
-> synthetic IM-like fixture
-> fixture-only AgentRun trace prototype
-> ADR / service SDD promotion
-> 最小生产切片
```

核心原则：

1. 先证明 Agent 能力边界，再接真实 IM 数据。
2. 先用公开数据集和 synthetic fixture 建 eval gate，再决定 proto / schema / runtime。
3. Memory、RAG、tool、workflow、approval、executor、audit 和 replay 必须一起设计。
4. Agent 不能直接写业务事实，不能绕过 policy / approval / action-executor / audit。
5. Python AI Worker 只做候选，不拥有最终状态。
6. MCP provider、tool description、tool output 和 peer-agent response 都是不可信输入。
7. Workflow-service 拥有长等待和审批状态；Agent Runtime 只拥有认知运行状态。
8. 每个可发布 AgentDefinition / SkillPackage 后续都必须有 eval gate 和 rollback path。

## 1. Phase 0：研究和问题定义

目标：

- 重新理解 2026 年完整 Agent 系统应该包含什么。
- 借鉴 OpenClaw、Hermes、Claude Code、OpenAI Agents SDK、LangGraph、MCP、A2A、Microsoft
  Agent Framework、Google agentic architecture、Databricks 2026 agent report 和相关论文。
- 明确 NexusIM 自己的企业 IM 约束：tenant、conversation、group memory、approval、audit、
  compliance、tool risk、message visibility。

产出：

- `docs/research/agent-plane-redesign-20260701.md`
- `docs/research/agent-ecosystem-research-20260701.md`
- `docs/research/agent-system-complete-scope-20260701.md`

完成条件：

- 有多套候选架构。
- 明确不冻结契约。
- 明确第一阶段不用真实 IM 数据。

## 2. Phase 1：平台级 SDD

目标：

- 将 Agent 层拆成可评审的组成部分。
- 明确每个组成部分拥有什么状态、不能拥有什么状态。
- 明确 Runtime、Workflow、Memory、Tool/MCP、Action Executor、Eval 的边界。
- 评审平台级设计是否足够进入实现；P0/P1 问题必须打回重做。

产出：

- `docs/sdd/agent-platform.md`
- `docs/research/agent-runtime-workflow-ownership-20260701.md`
- `docs/research/agent-current-design-review-20260701.md`
- `docs/research/agent-current-to-target-matrix-20260701.md`
- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-memory-admission.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/sdd/agent-tool-mcp-boundary.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/agent-governance-agentops.md`
- SDD index 和 architecture index 链接。

完成条件：

- 覆盖 Agent Gateway、identity/policy/budget、AgentDefinition、SkillPackage、Model Gateway、
  Runtime/Harness、Context/RAG、Memory、Tool/MCP、A2A、Workflow/HITL、Action Executor、
  Multi-agent、Python AI Worker、Eval/Replay、Observability/Audit、Security。
- 没有 proto、OpenAPI、Kafka schema、migration、production service directory。

## 3. Phase 2：Open Dataset Eval Harness

目标：

- 在不使用真实 IM 数据的情况下，先测试 Agent 关键能力。
- 为后续模型、prompt、retrieval、memory、tool 和 workflow 改动建立 regression gate。

数据集方向：

| 能力 | 数据集 / fixture |
| --- | --- |
| Grounded RAG | BEIR、NQ、HotpotQA、Qasper、MS MARCO |
| Tool / workflow | tau-bench、ToolSandbox、BFCL、MCP-Bench |
| Policy | JourneyBench |
| State diff | Agent-Diff |
| Memory | STATE-Bench、LoCoMo、LongMemEval、EverMemBench、GroupMemBench |
| Multi-agent | MultiAgentBench、MARBLE |
| Security | MCPSecBench、MCP poisoning、tool-selection attack fixture |

产出：

- dataset adapter 草案。
- EvalCase / EvalRun / EvalResult / report 草案。
- synthetic IM-like fixture 规则。
- failure taxonomy。
- 隔离式 Python harness：`ai/python/nexusim_ai_eval/`。
- public-dataset-style adapter skeleton。
- AgentRun / AgentStep trace skeleton。
- fixture-only cancel / resume / replay runtime-control skeleton。
- fixture-only MCP security skeleton。
- fixture-only ContextPackage / EvidencePack skeleton。
- fixture-only ContextPackage / EvidencePack hardening skeleton。
- fixture-only richer MemoryCandidate / memory admission skeleton。
- fixture-only StateDiffReport / action outcome skeleton。
- CLI：`ai/python/scripts/run_agent_eval_fixture.py`。
- Adapter batch CLI：`ai/python/scripts/run_agent_dataset_adapter.py`。
- Regression CLI：`ai/python/scripts/run_agent_eval_regression.py`。
- 单元测试、集成测试和边界测试：`ai/python/tests/test_agent_eval_*.py`。

完成条件：

- 每类能力至少有一个可执行或可模拟的 fixture 入口。
- report 能比较 baseline 和 regression。
- 不接生产启动路径。
- 测试能证明 harness 不需要后端服务、真实 IM 数据、模型 provider 或外部 MCP provider。

## 4. Phase 3：Fixture-only AgentRun Trace Prototype

目标：

- 用 fake/mock/fixture 验证 AgentRun 的过程可追踪、可暂停、可重放。
- 验证 ContextPackage、MemoryCandidate、ToolIntent、ApprovalWorkflowRef、ExecutionRef、
  ReplayBundle 的概念边界。

必须覆盖：

- 只读问答。
- 群组 memory admission。
- 需要审批的业务动作。
- 长时间等待用户审批。
- 外部 tool provider 超时。
- repair / redrive。
- cancel / resume / replay。
- multi-agent handoff。

完成条件：

- 每个流程都有 step trace。
- 失败能落到 failure taxonomy。
- replay 不重新执行外部副作用。
- fake/mock/fixture 不进入生产 fallback。

## 5. Phase 4：ADR / Service SDD Promotion

只有 Phase 2 和 Phase 3 给出足够证据后，才进入 promotion。

候选 ADR：

- Agent Runtime 是否独立为 `agent-runtime-service`。
- ContextPackage / EvidencePack 契约。
- MemoryCandidate / MemoryRecord admission 契约。
- Tool prepare / action execution lineage 契约。
- ReplayBundle / EvalResult 契约。
- Python AI Worker request / candidate contract。

反对条件：

- eval gate 仍无法复现。
- memory pollution 无法拦截。
- workflow 与 runtime 状态边界仍重叠。
- action-executor 无法验证 approval / prepare lineage。
- replay 无法解释失败。

## 6. Phase 5：最小生产切片

推荐顺序：

1. Read-only grounded QA。
2. Memory candidate extraction，不直接 ACTIVE。
3. Human-reviewed memory admission。
4. Proposal-only agent，不执行。
5. Approval -> action-executor 最小闭环。
6. Bounded multi-agent handoff。
7. Controlled external MCP provider。

每一步都必须有：

- policy gate。
- evidence / source refs。
- audit refs。
- eval regression。
- rollback / disable switch。

## 7. 质量门禁

任何进入实现阶段的 Agent 能力都至少需要：

- Citation verifier。
- Permission leakage detector。
- Memory pollution detector。
- Tool poisoning detector。
- Approval bypass detector。
- State-diff checker。
- Replay completeness checker。
- Cost / budget guard。

## 8. 工作区边界

- Agent Lab 只做 Agent / RAG / memory / Python AI Worker / EvidencePack / eval gate。
- 不修改 hotgroup 压测、Docker runtime profile 或后端性能实验路径。
- 完整模块完成后提交并推送 `origin/codex/agent-lab`。
- Handoff 给主集成线程时提供 branch、commit、changed files、checks、风险和下一步建议。
