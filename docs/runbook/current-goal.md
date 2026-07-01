# NexusIM Current Goal

本文件只写当前 Agent Lab 可执行目标。完整架构见
`docs/architecture/agent-plane-initial-design.md`，Agent 研究输入见
`docs/research/`，服务级设计见 `docs/sdd/`。

## Active Module

Agent Platform SDD package：在 Agent Exploration Mode 的研究基础上，把 NexusIM
Agent 层推进为一份可评审的详细 SDD 草案，并把本工作区的进度入口从后端热点群压测
切回 Agent / RAG / memory / AI worker / eval gate。

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

## 本轮目标

1. 重新核对 2026 Agent 系统公开输入：runtime、memory、tool/MCP、A2A、workflow、
   observability、eval、governance、安全和公开 benchmark。
2. 把本工作区进度报告改为 Agent Lab 当前事实，避免后续线程继续沿后端热点群压测主线推进。
3. 新增详细 `docs/sdd/agent-platform.md`，覆盖 Agent 各组成部分：
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
4. 更新 SDD index 和架构参考入口。
5. 做文档级检查、提交并推送到 `origin/codex/agent-lab`，再 handoff 给主集成线程。

## 完成条件

- `docs/runbook/current-goal.md`、`docs/runbook/current-brief.md`、
  `docs/runbook/remaining-goals.md` 均明确指向 Agent Lab。
- `docs/runbook/codex-sessions.md` 记录当前协作边界。
- `docs/sdd/agent-platform.md` 存在，并能作为后续 ADR / SDD / fixture 实验前置输入。
- `docs/sdd/README.md` 和 `docs/architecture/agent-plane-initial-design.md` 已链接新 SDD。
- `git diff --check` 和 heading / reference scan 通过。
- 工作区 clean，commit 已推送。

## 非目标

- 不继续 hotgroup 压测、Docker runtime profile、后端性能实验或容量曲线。
- 不新增生产服务、生产目录、proto、schema、migration 或 runtime implementation。
- 不接入真实 IM 数据。
- 不把任何外部框架原样照搬为 NexusIM 终局。
- 不让 Agent 直接写业务事实、直接执行工具或绕过 approval / executor / audit。

## 后续优先级

1. 基于 `agent-platform.md` 拆出 open-dataset eval 计划，选择首批 RAG / tool-workflow /
   memory 数据集。
2. 做 fixture-only AgentRun trace、ContextPackage、MemoryCandidate、ToolIntent 和
   ReplayBundle 输出格式实验。
3. 做 MCP poisoning、memory pollution、state-diff、approval pause/resume、cancel/replay
   等离线 eval fixture。
4. 在上述实验通过后，再决定是否把 Agent Runtime / Harness 提升为 ADR、服务级 SDD
   或实际 runtime module。
