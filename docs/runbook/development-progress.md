# NexusIM Agent Lab Development Progress

这份文档只做 Agent Lab 当前开发进度总览。它不再维护 NexusIM 后端、客户端、热点群压测
或 Docker/runtime profile 的长历史；这些由主集成或对应工作区维护。每轮默认入口仍是
`current-brief.md` 和 `current-goal.md`。

## 当前快照

当前 workspace 是 Agent Lab，主线是重新设计 NexusIM Agent 层：

```text
Agent / RAG / memory / Python AI Worker / EvidencePack / eval gate
```

当前阶段：Agent Exploration Mode -> Agent Platform SDD package 已完成文档重做；
Open Dataset Eval Harness / synthetic IM-like fixture 已开始第一段隔离式编码。

当前原则：

- 现有 IM 系统设计只作为参考，不作为 Agent 终局约束。
- 第一阶段不使用真实 IM 数据；先使用公开数据集和 synthetic IM-like fixture。
- 不写 proto、OpenAPI、Kafka schema、migration、生产服务目录或生产启动路径。
- 不冻结 agent taxonomy、skill taxonomy、EvidencePack shape、memory event shape、workflow
  shape、tool shape、MCP shape 或 A2A peer contract。
- Python AI Worker 只做 candidate-only intelligence plane；Go 服务继续拥有 auth、policy、
  audit、persistence、final proposal、execution 和 memory admission。

## 已完成探索

| 文档 | 状态 | 作用 |
| --- | --- | --- |
| `docs/research/agent-plane-redesign-20260701.md` | 已完成 | Agent Plane 重新设计的问题定义和候选路线 |
| `docs/architecture/agent-plane-initial-design.md` | 已完成并持续引用 | 初步 Agent 层设计报告，不冻结契约 |
| `docs/research/agent-runtime-workflow-ownership-20260701.md` | 已完成 | Candidate B runtime / harness 与 workflow-service ownership matrix |
| `docs/research/agent-ecosystem-research-20260701.md` | 已完成 | OpenClaw、Hermes、Claude Code、OpenAI Agents SDK、LangGraph、A2A、MCP、benchmark 和企业报告输入 |
| `docs/research/agent-system-complete-scope-20260701.md` | 已完成 | 2026 完整 Agent 系统能力范围和 open-dataset-first 流程 |
| `docs/sdd/agent-platform.md` | 已完成但不能单独推广实现 | 平台级 SDD 总览；v0.1 已被 P1 评审打回为实现前需重做 |
| `docs/research/agent-current-design-review-20260701.md` | 已完成 | 当前设计评审：方向正确，但平台总览不能单独进入实现 |
| `docs/research/agent-current-to-target-matrix-20260701.md` | 已完成 | 当前服务到目标 Agent 平台的迁移矩阵 |
| `docs/research/agent-open-dataset-eval-plan-20260701.md` | 已完成 | 公开数据集优先 eval 计划和 synthetic fixture 草案 |
| `docs/research/agent-coding-experiment-path-20260701.md` | 已完成 | 隔离式 Agent 编码实验路径、测试矩阵和后续切片顺序 |
| `docs/research/agent-adr-promotion-readiness-20260702.md` | 已完成 | ADR 候选 readiness review；可进入候选起草但不能直接推广生产契约 |
| `docs/research/agent-skeleton-completion-audit-20260702.md` | 已完成 | 不可变 Agent Lab goal 对 Phase 1 backend-isolated skeleton 的完成度审计 |
| `docs/research/agent-architecture-gap-closure-20260702.md` | 已完成 | 架构缺口关闭包：ADR candidate map、版本策略、集成 preservation matrix 和生产提升阻断条件 |
| `docs/sdd/agent-runtime.md` | 已完成 | Runtime / Harness 详细 SDD |
| `docs/sdd/agent-memory-admission.md` | 已完成 | Memory admission 详细 SDD |
| `docs/sdd/agent-context-evidencepack.md` | 已完成 | Context / EvidencePack 详细 SDD |
| `docs/sdd/agent-tool-mcp-boundary.md` | 已完成 | Tool / MCP boundary 详细 SDD |
| `docs/sdd/agent-eval-replay-harness.md` | 已完成 | Eval / Replay harness 详细 SDD |
| `docs/sdd/agent-governance-agentops.md` | 已完成 | Governance / AgentOps 详细 SDD |

## 当前设计范围

Agent Platform SDD 覆盖以下能力平面：

- Agent Gateway / UX
- Agent identity / policy / budget
- AgentDefinition / SkillPackage / release governance
- Model gateway and provider boundary
- Agent Runtime / Harness
- Context / EvidencePack / RAG
- Memory system
- Tool / MCP gateway
- A2A / peer-agent boundary
- Workflow / human-in-the-loop
- Action executor handoff
- Multi-agent bounded delegation
- Python AI Worker candidate boundary
- Eval / replay / open dataset harness
- Observability / audit / AgentOps
- Security / privacy / compliance

核心架构判断：

- Agent 不进入 IM 消息投递 hot path。
- Agent 读路径必须通过 retrieval-gateway / EvidencePack / ContextPackage。
- Agent 写路径必须通过 proposal / approval / action-executor / audit。
- Memory 是一等系统，不是 prompt cache 或向量库附属品。
- MCP / tool provider 是不可信边界。
- Eval / replay 是平台组成部分，不是上线后的脚本。

## Open Dataset-first 进度

已明确第一阶段数据策略：

| 能力 | 候选数据集 / fixture |
| --- | --- |
| Grounded RAG | BEIR、Natural Questions、HotpotQA、Qasper、MS MARCO |
| Tool / workflow | tau-bench、ToolSandbox、BFCL、MCP-Bench |
| Policy adherence | JourneyBench |
| State diff | Agent-Diff + synthetic enterprise state |
| Memory | STATE-Bench、LoCoMo、LongMemEval、EverMemBench、GroupMemBench |
| Multi-agent | MultiAgentBench / MARBLE + bounded handoff fixture |
| Security | MCPSecBench、MCP poisoning、tool-selection attack fixtures |

下一步不是接真实 IM 数据，而是为这些能力建立 dataset adapter、EvalCase、AgentRun trace、
EvalResult 和低敏 report 输出。

已落地第一段可运行代码：

```text
ai/python/nexusim_ai_eval/
ai/python/fixtures/agent_eval/adapter_samples/
ai/python/fixtures/agent_eval/baselines/synthetic_core_scenarios_baseline.json
ai/python/fixtures/agent_eval/synthetic_first_trio.json
ai/python/fixtures/agent_eval/synthetic_core_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
ai/python/fixtures/agent_eval/synthetic_runtime_control_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_scenarios.json
ai/python/fixtures/agent_eval/synthetic_mcp_security_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_context_evidence_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_memory_admission_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/memory_calibration_sample.json
ai/python/fixtures/agent_eval/memory_calibration_public_export.json
ai/python/fixtures/agent_eval/synthetic_state_diff_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_state_diff_deeper_hardening_scenarios.json
ai/python/fixtures/agent_eval/synthetic_replay_observability_scenarios.json
ai/python/fixtures/agent_eval/report_matrix_sample.json
ai/python/scripts/run_agent_eval_fixture.py
ai/python/scripts/run_agent_eval_current_report.py
ai/python/scripts/run_agent_eval_report_matrix.py
ai/python/scripts/run_agent_memory_calibration.py
ai/python/scripts/run_agent_dataset_adapter.py
ai/python/scripts/run_agent_eval_regression.py
ai/python/tests/test_agent_eval_*.py
```

该切片只读 synthetic fixture，输出低敏 EvalReport / ReplayBundle，不接后端服务、
生产数据库、Kafka、Redis、OpenSearch、真实 MCP provider 或模型 provider。

当前骨架还包含：

- Qasper/HotpotQA-like RAG adapter skeleton。
- ToolSandbox/tau-bench-like tool adapter skeleton。
- STATE-Bench/LoCoMo-like memory adapter skeleton。
- AgentRun / AgentStep trace skeleton。
- EvidencePack、ContextPackage、MemoryCandidate、ToolIntent fixture refs。
- 本地 adapter sample payloads 和批量转换 / 运行 CLI。
- EvalReport baseline comparison、regression delta 和 blocked promotion reasons。
- Current EvalReport generation、baseline refresh review artifact 和 baseline overwrite guard。
- Current-report lifecycle hardening：多 suite report matrix、baseline refresh
  approval manifest、report retention metadata，且支持 synthetic fixture 与
  public-dataset-style adapter sample 混合进同一矩阵。
- RuntimeControlFixture、checkpoint refs、cancel/resume/replay runtime events 和
  对应 synthetic fixture。
- Runtime-control negative fixture：missing checkpoint、cancel propagation incomplete、
  replay event incomplete。
- Runtime-control deeper hardening fixture：checkpoint version drift detection、
  workflow wakeup race dedupe、ReplayBundle lineage completeness。
- MCP security fixture：poisoned tool description、unsafe output instruction、
  provider provenance mismatch、sandbox-only provider。
- MCP security hardening fixture：tool argument schema mismatch、
  tool-selection attack blocking、prepare expiry detection、多候选 provider selection。
- ContextPackage / EvidencePack fixture：source coverage、conflict marker、
  stale evidence avoidance、permission abstain 和低敏 trace metadata。
- ContextPackage / EvidencePack hardening fixture：memory-vs-current-source
  precedence、unsafe tool output quarantine、context-budget retention、
  unavailable retrieval lane gap reporting。
- ContextPackage / EvidencePack deeper hardening fixture：source ranking、
  lane redrive、snippet-level citation repair、cross-tenant denied-lane、
  provider/tool/peer-agent taint propagation。
- ContextPackage / EvidencePack adapter alignment：Qasper / HotpotQA / BEIR
  风格 sample 已覆盖 public RAG adapter alignment、rerank confidence threshold
  refs、rerank explanation refs、denied-lane audit refs、taint vocabulary refs，
  evaluator / trace / adapter runner 均保持低敏 fixture-only。
- Memory admission fixture：group speaker/audience、project supersedes、profile
  aggregate review、revoked/stale memory blocking、overgeneralization prevention。
- Memory admission hardening fixture：duplicate dedupe、low-confidence rejection、
  procedural skill binding、policy-like memory rejection、review timeout metadata。
- Memory admission deeper hardening fixture：multi-source duplicate clustering、
  confidence calibration、procedural memory migration/invalidation、governed policy
  source allowlist/revocation、review retry/escalation/redrive。
- Memory admission adapter alignment：STATE-Bench / LoCoMO 风格 sample 已覆盖
  duplicate cluster representative / tie-break、confidence threshold、
  governed policy revocation window，且 adapter 会保留这些低敏 metadata。
- Memory admission calibration：STATE-Bench / LoCoMO / LongMemEval /
  EverMemBench / GroupMemBench 风格本地样本已覆盖 confidence threshold、
  governed policy revocation-window retention、review backoff/operator queue
  policy recommendation，并输出 blocked promotion reasons。
- Memory calibration public export：扩展到 5 类 dataset-source refs、15 个
  gate case、8 个 policy-window case、12 个 review-backoff case，并输出
  per-dataset case counts。
- Tool / MCP adapter alignment：ToolSandbox / MCP-Bench 风格 sample 已覆盖
  capability lease refs、capability scope refs、provider attestation refs，
  evaluator 会输出对应低敏 aggregate score，且仍保持 fixture-only。
- State-diff fixture：approved action outcome refs、expected-vs-actual state
  changes、execution/audit refs、incomplete report、unauthorized mutation detection。
- State-diff hardening fixture：repair/redrive lineage、partial execution detection、
  idempotency-preserved replay、compensating action refs。
- State-diff deeper hardening fixture：state dependency graph、cross-action
  compensation chain、operator redrive review refs。
- ReplayBundle observability fixture：low-sensitive observability refs、hash refs、
  version metadata refs、failure taxonomy refs、trace linkage refs，并在
  EvalReport / ReplayBundle / AgentRunTrace 中保持 fixture-only 输出。
- Skeleton completion audit：已把 immutable goal 的 17 个骨架要求映射到当前
  代码、fixture、CLI、tests 和 runbook evidence；结论是 Phase 1 isolated
  Agent-layer skeleton 可作为当前可执行基线，但不能直接推广生产契约。
- Architecture gap closure：已把资深架构评审发现的 ADR 缺失、版本策略、真实服务
  integration proof、Runtime/Workflow ownership、Governance、Memory、EvidencePack、
  MCP、dataset pipeline 和 operator UX 缺口收敛为六个 ADR candidate 和生产阻断条件。

## 当前未完成项

| 优先级 | 工作 | 输出 |
| --- | --- | --- |
| P1 | ADR candidate drafting decision | 主集成 / 用户确认是否进入 ADR candidate drafting |
| P1 | Document-driven process | immutable goal 保持稳定，具体阶段与验收条件只维护在 runbook / SDD / research 文档 |
| P2 | ReplayBundle observability hardening review | 如评审要求，继续补 fixture-only taxonomy / trace evidence |
| P2 | Memory calibration hardening | 仅在需要时继续追加公开数据集导出或 adapter metadata |

## 验证状态

本阶段已进入 backend-isolated Python skeleton。验证以 Python 单元 / 集成 / 边界测试和
文档一致性为主：

- `python -m pytest ai/python/tests -q`
- `python -m ruff check ai/python`
- `python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts`
- `.\tools\check-python-ai-worker-boundary.ps1`
- `git diff --check`
- SDD index / research index / architecture index link check
- 不触碰 proto、schema、migration、production service directory

后续等待主集成 / 用户确认是否进入 ADR candidate drafting；ReplayBundle
observability 和 memory calibration 只在评审要求时继续 fixture-only hardening。

## 历史资料路由

- 旧后端 / 客户端 / loadtest 历史不在本文件展开。
- 如果需要全系统历史，查主集成工作区或已有 `archive/`、`loadtest/`、service brief。
- Agent Lab 后续只在本文件维护 Agent / RAG / memory / AI Worker / EvidencePack / eval gate
  相关进度。
