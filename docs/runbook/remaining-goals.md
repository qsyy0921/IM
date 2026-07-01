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

1. State-diff hardening：基础 action outcome report、expected-vs-actual state
   change、execution refs、audit refs、incomplete report 和 unauthorized mutation
   fixture 已落地；后续覆盖 repair/redrive、partial execution、idempotency 和
   compensating action cases。
2. Tool / MCP security hardening：基础 malicious tool description、unsafe output、
   MCP provider provenance 和 sandbox-only provider fixture 已落地；后续覆盖 tool
   argument schema mismatch、tool-selection attack、prepare expiry 和多候选 provider
   selection。
3. Runtime-control negative fixtures：覆盖 missing checkpoint、cancel propagation
   incomplete、replay event incomplete，作为已落地正向 cancel/resume/replay fixture 的
   hardening。
4. Current-report generation / baseline refresh review：把当前 EvalReport 生成和 baseline
   更新评审脚本化，避免手工复制导致 baseline 漂移。
5. Memory admission deeper hardening：基础 group/project/profile、supersedes、
   revocation、stale facts、speaker attribution、audience scope、overgeneralization、
   duplicate dedupe、low-confidence rejection、procedural skill binding、
   policy-like memory rejection 和 review timeout fixture 已落地；后续覆盖
   multi-source duplicate clustering、confidence calibration、procedural memory
   migration / invalidation、governed policy source allowlist / revocation 和
   review retry / escalation / redrive cases。
6. ContextPackage / EvidencePack deeper hardening：基础 source coverage、temporal
   version、conflict marker、permission abstain、memory-vs-current-source precedence、
   unsafe tool output quarantine、context-budget retention 和 retrieval lane unavailable
   fixture 已落地；后续覆盖 source ranking、lane redrive、snippet-level citation
   repair、cross-tenant denied-lane 和 taint propagation cases。
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
