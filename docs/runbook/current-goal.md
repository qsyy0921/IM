# NexusIM Current Goal

本文件只写当前 Agent Lab 可执行目标。完整架构见
`docs/architecture/agent-plane-initial-design.md`，Agent 研究输入见
`docs/research/`，服务级设计见 `docs/sdd/`。

## Active Module

Agent Platform SDD package 已完成文档重做：`agent-platform.md` v0.1 方向保留，但被评审为
不能单独推广实现；当前可评审输入是平台总览 + runtime、memory admission、context、
tool/MCP、eval/replay、governance 六份详细 SDD，以及 current-to-target matrix 和
open-dataset eval plan。

当前编码模块：Open Dataset Eval Harness / synthetic IM-like fixture。仍然只做
Agent / RAG / memory / Python AI Worker / EvidencePack / eval gate，不使用真实 IM 数据。

## 当前边界

- 本工作区当前只负责 Agent / RAG / memory / Python AI Worker / EvidencePack /
  eval gate 相关设计与实验。
- 不使用 NexusIM 真实 IM 数据做第一阶段 Agent 能力验证；先使用公开数据集和
  synthetic IM-like fixture。
- 不写 proto、OpenAPI、Kafka schema、migration、生产服务目录或生产启动路径。
- 不冻结 agent taxonomy、skill taxonomy、EvidencePack shape、memory event shape、
  workflow shape、tool shape、MCP shape 或 A2A peer-agent contract。
- Python AI Worker 只产出候选；Go 服务仍拥有 auth、policy、audit、persistence、
  final proposal、execution 和 memory admission。
- fake / mock / fixture 只能用于研究或测试隔离，不能接入生产路径，也不能作为真实服务失败时的 fallback。

## 已完成探索输入

- `docs/research/agent-plane-redesign-20260701.md`：Agent Plane 重新设计问题定义和候选路线。
- `docs/architecture/agent-plane-initial-design.md`：Agent 层初步设计报告。
- `docs/research/agent-runtime-workflow-ownership-20260701.md`：Agent Runtime 与
  workflow-service ownership matrix。
- `docs/research/agent-ecosystem-research-20260701.md`：OpenClaw / Hermes /
  Claude Code / OpenAI Agents SDK / LangGraph / AutoGen / CrewAI / Google ADK /
  A2A / MCP / benchmark / 企业报告输入。
- `docs/research/agent-system-complete-scope-20260701.md`：2026 完整 Agent 系统
  能力平面和 open-dataset-first 开发流程。
- `docs/research/agent-current-design-review-20260701.md`：当前设计 P1 评审结论；
  方向正确，但平台级 SDD v0.1 不能单独推广实现。
- `docs/research/agent-current-to-target-matrix-20260701.md`：现有 AI / Agent foundation
  服务到目标 Agent 平台的迁移矩阵。
- `docs/research/agent-open-dataset-eval-plan-20260701.md`：公开数据集优先 eval 计划和
  synthetic IM-like fixture 草案。
- `docs/research/agent-coding-experiment-path-20260701.md`：隔离式 Agent 编码实验路径。
- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-memory-admission.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/sdd/agent-tool-mcp-boundary.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/agent-governance-agentops.md`

## 当前目标

1. 保持 Agent SDD 包为当前设计事实源，不从旧后端压测或单一平台总览继续推进。
2. 当前实验只能先做公开数据集 adapter、EvalCase / EvalRun / EvalResult、
   ReplayBundle 和 synthetic IM-like fixture。
3. 任何 production proto、schema、migration、service directory、runtime implementation
   都必须等 eval/fixture 证据和 ADR。
4. 完整模块完成后提交并推送到 `origin/codex/agent-lab`，再 handoff 给主集成线程。

## 完成条件

- `docs/runbook/current-goal.md`、`docs/runbook/current-brief.md`、
  `docs/runbook/remaining-goals.md` 均明确指向 Agent Lab。
- `docs/runbook/codex-sessions.md` 记录当前协作边界。
- `docs/sdd/agent-platform.md` 存在，并明确标注不能单独推广实现。
- 六份详细 Agent SDD 存在，并能作为后续 ADR / fixture 实验前置输入。
- `docs/sdd/README.md`、`docs/research/README.md`、`docs/architecture/README.md` 和
  `docs/architecture/agent-plane-initial-design.md` 已链接新 SDD / 研究文档。
- `git diff --check` 和 heading / reference scan 通过。
- 工作区 clean，commit 已推送。

## 非目标

- 不继续 hotgroup 压测、Docker runtime profile、后端性能实验或容量曲线。
- 不新增生产服务、生产目录、proto、schema、migration 或 runtime implementation。
- 不接入真实 IM 数据。
- 不把任何外部框架原样照搬为 NexusIM 终局。
- 不让 Agent 直接写业务事实、直接执行工具或绕过 approval / executor / audit。

## 后续优先级

1. 扩展 `ai/python/nexusim_ai_eval/`，加入公开数据集 adapter skeleton。
2. 做 fixture-only AgentRun trace、ContextPackage、MemoryCandidate、ToolIntent 和
   ReplayBundle 输出格式实验。
3. 做 MCP poisoning、memory pollution、state-diff、approval pause/resume、cancel/replay
   等离线 eval fixture。
4. 在上述实验通过后，再决定是否把 Agent Runtime / Harness 提升为 ADR、服务级 SDD
   或实际 runtime module。
