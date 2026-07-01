# NexusIM Current Brief

本文件只做每轮入口摘要。当前 workspace 是 Agent Lab，主线是 Agent / RAG /
memory / AI worker / EvidencePack / eval gate 的探索和设计，不承接后端热点群压测。

## 当前主线

- Agent Lab 正在从探索稿推进到详细 SDD 草案。
- 当前工作不使用 NexusIM 真实 IM 数据；第一阶段能力验证使用公开数据集和
  synthetic IM-like fixture。
- 当前 active module：`Agent Platform SDD package`。

## 最近收口

- 已完成 Agent Plane 初步设计：
  `docs/architecture/agent-plane-initial-design.md`。
- 已完成 runtime / workflow ownership matrix：
  `docs/research/agent-runtime-workflow-ownership-20260701.md`。
- 已完成 2026 Agent 生态研究附录：
  `docs/research/agent-ecosystem-research-20260701.md`。
- 已完成完整 Agent 系统能力范围探索：
  `docs/research/agent-system-complete-scope-20260701.md`。
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

## 本轮输出

- 更新进度入口，使后续线程从 Agent Lab 当前目标开始。
- 新增 `docs/sdd/agent-platform.md`，作为覆盖各 Agent 部件的详细 SDD 草案。
- 更新 SDD index 和 Agent 初步设计报告的参考入口。

## 工作规则

- 每轮先读 `prompt.md`、`agent.md`、`docs/runbook/current-goal.md`。
- Agent Lab 不修改 hotgroup 压测、Docker runtime profile 或后端性能实验路径。
- 文档和实验可以使用 fake / mock / fixture，但不能接生产启动路径或作为真实服务失败 fallback。
- 完整模块完成后提交并推 `origin/codex/agent-lab`，再 handoff 给主集成线程。
