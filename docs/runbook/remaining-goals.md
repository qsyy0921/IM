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

1. Memory admission adapter alignment：基础 group/project/profile、supersedes、
   revocation、stale facts、speaker attribution、audience scope、overgeneralization、
   duplicate dedupe、low-confidence rejection、procedural skill binding、
   policy-like memory rejection、review timeout、multi-source duplicate clustering、
   confidence calibration、procedural memory migration / invalidation、governed policy
   source allowlist / revocation 和 review retry / escalation / redrive fixture 已落地；
   后续只保留 STATE-Bench / LoCoMo 风格 adapter sample 对齐、
   duplicate cluster representative tie-break strategy、confidence calibration thresholds
   和 governed policy revocation window metadata。
2. Tool / MCP security deeper hardening：基础 malicious tool description、unsafe
   output、MCP provider provenance、sandbox-only provider、tool argument schema
   mismatch、tool-selection attack、prepare expiry 和多候选 provider selection fixture
   已落地；后续只保留 MCPSecBench / MCP-Bench adapter 对齐、capability lease matrix 和
   provider attestation metadata。
3. ContextPackage / EvidencePack adapter alignment：基础 source coverage、temporal
   version、conflict marker、permission abstain、memory-vs-current-source precedence、
   unsafe context quarantine、context-budget retention、retrieval lane unavailable、
   source ranking、lane redrive、snippet-level citation repair、cross-tenant denied-lane
   和 taint propagation fixture 已落地；后续只保留公开 RAG adapter alignment、
   rerank confidence thresholds、denied-lane audit metadata 和 taint vocabulary 对齐。
4. Current-report / baseline lifecycle deeper hardening：当前 EvalReport 生成和 baseline
   refresh review CLI 已落地；后续只保留多 suite report matrix、baseline refresh approval
   manifest 和 report retention metadata。
5. Runtime-control deeper hardening：基础 cancel/resume/replay 正向 fixture 和 missing
   checkpoint、cancel propagation incomplete、replay event incomplete 负向 fixture 已落地；
   后续只保留 checkpoint version drift、workflow wakeup race 和 replay bundle lineage
   completeness cases。
6. State-diff deeper hardening：基础 action outcome report、expected-vs-actual state
   change、execution refs、audit refs、incomplete report、unauthorized mutation、
   repair/redrive、partial execution、idempotency 和 compensating action fixture 已落地；
   后续只保留更深的 state dependency graph、cross-action compensation chain 和 operator
   redrive review cases。
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
