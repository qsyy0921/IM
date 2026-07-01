# NexusIM Current Brief

本文件只做每轮入口摘要。当前 workspace 是 Agent Lab，主线是 Agent / RAG /
memory / AI worker / EvidencePack / eval gate 的探索和设计，不承接后端热点群压测。

## 当前主线

- Agent Lab 已从探索稿推进到详细 SDD 包。
- 当前工作不使用 NexusIM 真实 IM 数据；第一阶段能力验证使用公开数据集和
  synthetic IM-like fixture。
- 当前 active module：`Open Dataset Eval Harness / synthetic IM-like fixture`，已进入
  backend-isolated Python skeleton 增量实现。

## 最近收口

- 已完成 Agent Plane 初步设计：
  `docs/architecture/agent-plane-initial-design.md`。
- 已完成 runtime / workflow ownership matrix：
  `docs/research/agent-runtime-workflow-ownership-20260701.md`。
- 已完成 2026 Agent 生态研究附录：
  `docs/research/agent-ecosystem-research-20260701.md`。
- 已完成完整 Agent 系统能力范围探索：
  `docs/research/agent-system-complete-scope-20260701.md`。
- 已完成当前设计评审：
  `docs/research/agent-current-design-review-20260701.md`。结论是方向正确，但
  `docs/sdd/agent-platform.md` v0.1 不能单独推广实现。
- 已完成 current-to-target matrix：
  `docs/research/agent-current-to-target-matrix-20260701.md`。
- 已完成 open dataset eval plan：
  `docs/research/agent-open-dataset-eval-plan-20260701.md`。
- 已完成六份详细 Agent SDD：
  `docs/sdd/agent-runtime.md`、`docs/sdd/agent-memory-admission.md`、
  `docs/sdd/agent-context-evidencepack.md`、`docs/sdd/agent-tool-mcp-boundary.md`、
  `docs/sdd/agent-eval-replay-harness.md`、`docs/sdd/agent-governance-agentops.md`。
- 设计边界保持为探索 / SDD 草案：不冻结 proto、schema、migration、runtime、
  agent taxonomy、skill taxonomy、EvidencePack shape 或 memory event shape。

## 当前设计方向

NexusIM Agent 层按以下能力平面组织：

```text
Agent Gateway / UX
-> Agent identity / policy / budget
-> AgentDefinition / SkillPackage governance
-> Model gateway / Python candidate worker
-> Agent Runtime / Harness
-> Context / EvidencePack / RAG
-> Memory system
-> Tool / MCP boundary
-> Workflow / human-in-the-loop
-> Action executor handoff
-> Eval / replay / open dataset harness
-> Observability / audit / AgentOps
```

核心不变量：

- IM 业务事实仍由 IM 服务拥有。
- Agent 不进入消息投递热路径。
- Agent 读路径必须经过 retrieval-gateway / EvidencePack。
- Agent 写路径必须经过 proposal / approval / action-executor / audit。
- Python AI Worker 只返回候选。
- Memory 必须 source-backed、scoped、versioned、reviewed、可撤销。
- MCP server 不是权限边界；tool description 和 output 均按不可信输入处理。
- Eval / replay 是架构组成部分，不是上线后脚本。

## 当前输出

- 进度入口已指向 Agent Lab 和完整 Agent SDD 包。
- `docs/sdd/agent-platform.md` 保留为平台总览，但已标注不能单独推广实现。
- 详细设计以 runtime、memory admission、context、tool/MCP、eval/replay、
  governance 六份 SDD 为准。
- 下一阶段应先做公开数据集和 synthetic fixture，不接真实 IM 数据。
- 已开始第一段隔离式编码实验：`ai/python/nexusim_ai_eval/`，只运行 synthetic
  fixture 和低敏 EvalReport / ReplayBundle，不接后端服务。
- 当前骨架已包含 adapter skeleton、AgentRun / AgentStep trace skeleton、
  `synthetic_core_scenarios.json` 和对应 unit / integration / boundary tests。
- 已补本地 public-dataset-style adapter sample payload、批量转换 / 运行 CLI、
  EvalReport baseline fixture 和 regression comparison CLI。
- 已补 fixture-only runtime-control coverage：cancel propagation、approval resume
  from checkpoint、replay without side-effect reexecution。
- 已补 fixture-only MCP security coverage：poisoned tool description、unsafe MCP
  output instruction、provider provenance mismatch、sandbox-only provider。
- 已补 fixture-only MCP security hardening：tool argument schema mismatch、
  tool-selection attack blocking、prepare expiry detection、多候选 provider selection。
- 已补 fixture-only ContextPackage / EvidencePack coverage：source coverage、
  conflict marker、stale evidence avoidance、permission abstain。
- 已补 fixture-only ContextPackage / EvidencePack hardening：memory-vs-current-source
  precedence、unsafe tool output quarantine、context-budget retention、
  unavailable retrieval lane gap reporting。
- 已补 fixture-only ContextPackage / EvidencePack deeper hardening：source ranking
  / tie-break、retrieval lane redrive、snippet-level citation repair、cross-tenant
  denied-lane reporting、provider/tool/peer-agent taint propagation。
- 已补 Qasper / HotpotQA / BEIR 风格 ContextPackage / EvidencePack adapter
  alignment：public RAG adapter 可保留 rerank confidence threshold refs、
  rerank explanation refs、denied-lane audit refs 和 taint vocabulary refs，
  evaluator / trace / CLI 均保持 fixture-only。
- 已补 fixture-only richer memory admission coverage：group speaker/audience、
  project supersedes、profile aggregate review、revoked/stale memory blocking、
  overgeneralization prevention。
- 已补 fixture-only memory admission hardening：duplicate dedupe、
  low-confidence rejection、procedural skill binding、policy-like memory rejection、
  review timeout metadata。
- 已补 fixture-only memory admission deeper hardening：multi-source duplicate
  clustering、confidence calibration、procedural memory migration/invalidation、
  governed policy source allowlist/revocation、review retry/escalation/redrive。
- 已补 STATE-Bench / LoCoMO 风格 memory adapter alignment：duplicate cluster
  representative selection / tie-break refs、confidence threshold refs、
  governed policy revocation window refs，并保持 fixture-only。
- 已补 fixture-only memory admission calibration：基于
  STATE-Bench / LoCoMO / LongMemEval / EverMemBench / GroupMemBench 风格本地样本，
  对 confidence threshold、governed policy revocation-window retention 和
  review backoff/operator queue policy 输出推荐 refs 或 blocked reasons。
- 已补 ToolSandbox / MCP-Bench 风格 Tool / MCP adapter alignment：capability
  lease refs、capability scope refs、provider attestation refs，并保持
  fixture-only。
- 已补 fixture-only state-diff report coverage：approved action outcome refs、
  expected-vs-actual state changes、missing execution refs、incomplete report、
  unauthorized mutation detection。
- 已补 fixture-only state-diff hardening：repair/redrive lineage、partial execution
  detection、idempotency-preserved replay、compensating action refs。
- 已补 fixture-only state-diff deeper hardening：state dependency graph、
  cross-action compensation chain、operator redrive review refs。
- 已补 fixture-only runtime-control negative coverage：missing checkpoint、
  cancel propagation incomplete、replay event incomplete。
- 已补 fixture-only runtime-control deeper hardening：checkpoint version drift
  detection、workflow wakeup race dedupe、ReplayBundle lineage completeness。
- 已补 fixture-only ReplayBundle observability skeleton：低敏
  observability refs、hash refs、version metadata refs、failure taxonomy refs 和
  trace linkage refs，并接入 EvalReport / ReplayBundle / AgentRunTrace。
- 已补 current EvalReport generation / baseline refresh review CLI，生成当前报告和
  baseline refresh review artifact，默认不覆盖 baseline。
- 已补 current-report / baseline lifecycle deeper hardening：多 suite report
  matrix、baseline refresh approval manifest 和 report retention metadata，
  支持 synthetic fixture 与 public-dataset-style adapter sample 混合进矩阵。
- 下一段优先做 memory calibration data expansion 或 ReplayBundle observability
  review 后续 hardening；仍只允许公开数据集导出和 synthetic fixture。

## 工作规则

- 每轮先读 `prompt.md`、`agent.md`、`docs/runbook/current-goal.md`。
- Agent Lab 不修改 hotgroup 压测、Docker runtime profile 或后端性能实验路径。
- 文档和实验可以使用 fake / mock / fixture，但不能接生产启动路径或作为真实服务失败 fallback。
- 完整模块完成后提交并推 `origin/codex/agent-lab`，再 handoff 给主集成线程。
