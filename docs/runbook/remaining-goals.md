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

1. Document-driven process：immutable Codex goal 保持稳定；具体阶段、优先级、
   acceptance criteria 和剩余工作只维护在 runbook / SDD / research 文档中。
   `docs/research/agent-skeleton-completion-audit-20260702.md` 已确认 Phase 1
   backend-isolated skeleton 可作为当前可执行基线。
2. ADR candidate package：`docs/research/adr-candidates/` 已起草六个候选 ADR
   和 cross-service versioning / replay / governance appendix；首轮 review ledger
   在 `docs/research/agent-architecture-review-loop-20260702.md`，Eval / Replay
   focused review 在 `docs/research/agent-eval-replay-adr-review-20260702.md`，
   Runtime / Workflow focused review 在
   `docs/research/agent-runtime-workflow-adr-review-20260702.md`，Context /
   EvidencePack focused review 在
   `docs/research/agent-context-evidencepack-adr-review-20260702.md`，Memory /
   Tool / AgentOps focused reviews 也已补齐。Full package 已进入 ADR review；
   Eval / Replay、Runtime / Workflow、Context / EvidencePack 与 Memory Admission L1 已接受，Tool / MCP L1 自评包已起草，未接受前不提升生产契约。
3. Fixture-only evidence hardening：Eval / Replay、Runtime / Workflow、Context /
   Evidence、Memory、Tool、AgentOps、dataset reproducibility、cross-service
   preservation、multi-agent handoff、object completeness 和 operator
   governance、operational readiness、controlled implementation readiness、
   architecture coverage 和 contract version compatibility rehearsal 已落地；
   下一步优先主集成 review 或 focused review-requested hardening。
   Controlled implementation entry audit 已确认 Agent Lab 内部证据条件通过，
   但实际受控实现仍需 accepted ADR、owner review 和 real-service smoke。
   Memory Admission L1 已被主集成接受；下一步等待 Tool / MCP L1 verdict 或 owner / smoke 证据要求。
4. ReplayBundle observability hardening review：当前 skeleton 已覆盖低敏
   observability refs、hash refs、version metadata refs、failure taxonomy refs 和
   trace linkage refs；如评审要求继续，只增加 fixture-only taxonomy / trace 证据。
5. Memory calibration hardening：当前已补 public-dataset-style export 和
   reproducibility rehearsal；如需继续，只追加低敏 adapter metadata 或 review
   要求的 reproducibility case，不接真实 IM 数据。

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
