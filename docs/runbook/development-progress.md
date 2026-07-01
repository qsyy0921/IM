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
下一阶段建议进入 Open Dataset Eval Harness / synthetic IM-like fixture。

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

## 当前未完成项

| 优先级 | 工作 | 输出 |
| --- | --- | --- |
| P0 | open dataset eval harness | 基于现有 eval plan 落 fixture-only adapter / EvalCase / EvalRun / EvalResult |
| P0 | fixture-only AgentRun trace | read-only QA、memory admission、approval wait、timeout、cancel/replay、handoff 流程 |
| P1 | ContextPackage / EvidencePack experiment | citation、source coverage、temporal version、conflict marker、permission abstain |
| P1 | Memory admission eval | scope、speaker attribution、supersedes、revocation、overgeneralization、pollution |
| P1 | Tool / MCP security eval | poisoned tool description、unsafe output、tool-selection attack、provider provenance |
| P1 | State-diff eval | action outcome 与预期状态变化比对 |
| P2 | ADR promotion decision | 是否提升 Agent Runtime / Harness、memory admission、ReplayBundle 等契约 |

## 验证状态

本阶段是文档和探索设计阶段，验证以文档检查和引用一致性为主：

- `git diff --check`
- heading / reference scan
- SDD index / research index / architecture index link check
- 不触碰 proto、schema、migration、production service directory

后续进入 fixture-only prototype 后，再增加单元测试、dataset adapter validation、replay consistency
和 security fixture gate。

## 历史资料路由

- 旧后端 / 客户端 / loadtest 历史不在本文件展开。
- 如果需要全系统历史，查主集成工作区或已有 `archive/`、`loadtest/`、service brief。
- Agent Lab 后续只在本文件维护 Agent / RAG / memory / AI Worker / EvidencePack / eval gate
  相关进度。
