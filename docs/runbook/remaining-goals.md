# NexusIM Remaining Goals

本文件只记录 Agent Lab 未完成工作。当前进度见
`docs/runbook/current-goal.md` 和 `docs/runbook/current-brief.md`。

## 维护规则

- 只记录 Agent / RAG / memory / Python AI Worker / EvidencePack / eval gate
  相关 backlog。
- 不记录后端 hotgroup 压测、Docker runtime profile 或性能实验任务；这些由其它工作区负责。
- 新发现待办追加到本文件；完成后删除或改写为下一阶段 hardening。
- 不写长历史、完成证据、SDD / ADR 正文或 loadtest report。
- 不新增生产契约、schema、migration 或 service directory，除非用户明确要求进入实现阶段。

## 当前优先顺序

1. Public dataset adapter skeleton：在已落地的 `ai/python/nexusim_ai_eval/` 隔离 harness
   上增加 Qasper/HotpotQA-like RAG、ToolSandbox/tau-bench-like tool、STATE-Bench/LoCoMo-like
   memory adapter skeleton。
2. Fixture-only AgentRun trace：建模 read-only QA、group memory admission、
   approval wait、provider timeout redrive、cancel/resume/replay 和 bounded
   multi-agent handoff。
3. ContextPackage / EvidencePack 实验：验证 citation、source coverage、temporal
   version、conflict marker 和 permission abstain。
4. Memory admission eval：覆盖 group memory、project memory、profile aggregate、
   supersedes、revocation、stale facts、speaker attribution、audience scope 和
   overgeneralization。
5. Tool / MCP security eval：覆盖 malicious tool description、unsafe tool output、
   prompt injection、tool-selection attack、MCP server provenance 和 sandbox-only
   high-risk provider。
6. State-diff eval：基于 Agent-Diff 思路验证 action outcome，而不是只比较 tool call trace。
7. ReplayBundle / observability：定义低敏 refs、hashes、version metadata、failure
   taxonomy 和 trace linkage。
8. ADR promotion review：只有前述 fixture/eval 有证据后，才评估 Agent Runtime、
   ContextPackage、MemoryCandidate、Tool prepare、ReplayBundle 是否提升为 ADR / 契约。

## Dataset Backlog

- Grounded RAG：BEIR、Natural Questions、HotpotQA、Qasper、MS MARCO。
- Tool / workflow：tau-bench、ToolSandbox、BFCL、MCP-Bench。
- Policy adherence：JourneyBench。
- Enterprise state diff：Agent-Diff。
- Memory：STATE-Bench、LoCoMo、LongMemEval、EverMemBench、GroupMemBench。
- Multi-agent：MultiAgentBench / MARBLE。
- Security：MCPSecBench、MCP security papers、tool-selection attack fixtures。

## Hard Boundaries

- 不使用真实 NexusIM IM 数据作为第一阶段 eval 数据。
- 不把公开 benchmark 改写成产品事实；benchmark ground truth 与 synthetic IM fixture
  必须分离。
- 不让 Python AI Worker 持久化 final state、ACTIVE memory、approval、execution
  或 audit archive。
- 不让 workflow-service 理解 raw prompt、EvidencePack body、planner state 或模型输出。
- 不让 action-executor 执行未审批、未 prepare 或无法审计的动作。
- 不把 MCP server、tool description 或 provider output 当可信输入。
