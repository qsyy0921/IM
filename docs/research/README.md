# NexusIM Research Index

这个目录只维护研究资料的短索引和分类规则，不存放论文 PDF。

## 入口

- `paper-organization.md`：Zotero 与本地 PDF 的分类规则。
- `agent-plane-redesign-20260701.md`：Agent Plane 重新设计研究入口。
- `agent-runtime-workflow-ownership-20260701.md`：Agent Runtime 与 workflow-service
  ownership matrix。
- `agent-ecosystem-research-20260701.md`：OpenClaw、Hermes、Claude Code、OpenAI
  Agents SDK、LangGraph、MCP、A2A、benchmark、安全论文和企业报告输入。
- `agent-system-complete-scope-20260701.md`：2026 完整 Agent 系统能力范围和
  open-dataset-first 开发流程。
- `agent-current-design-review-20260701.md`：当前 Agent 平台设计评审结论；方向正确，
  但 `agent-platform.md` v0.1 被 P1 打回，不能单独推广实现。
- `agent-current-to-target-matrix-20260701.md`：当前 AI / Agent foundation 服务到目标
  Agent 平台的迁移和 ownership matrix。
- `agent-open-dataset-eval-plan-20260701.md`：公开数据集优先的 Agent eval 计划，
  包含 EvalCase / EvalRun / EvalResult / synthetic IM-like fixture 草案。
- `agent-coding-experiment-path-20260701.md`：隔离式 Agent 编码实验路径，记录
  `ai/python/nexusim_ai_eval`、fixture、CLI、单元测试、集成测试和后续切片顺序。
- `agent-adr-promotion-readiness-20260702.md`：隔离式 Agent eval / replay /
  memory calibration skeleton 的 ADR 候选 readiness review；不冻结生产契约。
- `agent-skeleton-completion-audit-20260702.md`：不可变 Agent Lab goal 对当前
  backend-isolated skeleton 的完成度审计，确认 Phase 1 骨架可作为当前可执行基线，
  但不提升生产契约。
- `agent-architecture-gap-closure-20260702.md`：资深架构评审后的缺口关闭包，
  给出 ADR candidate map、版本策略、集成 preservation matrix 和生产提升阻断条件。
- `agent-production-object-model-20260702.md`：Agent 生产级对象模型草案，
  收敛 decision、version、runtime、evidence、memory、tool、workflow、AgentOps、
  dataset/eval 和 operator UX 对象；不冻结生产 schema。
- `adr-candidates/`：Agent ADR candidate package，包含 Eval / Replay、
  Runtime / Workflow、Context / EvidencePack、Memory Admission、Tool / MCP、
  AgentOps / Governance 六份候选 ADR 和 cross-service versioning / replay /
  governance appendix；仍不是正式 ADR。
- `agent-architecture-review-loop-20260702.md`：首轮 Agent 架构
  review-and-closure ledger，记录打回原因、P0/P1/P2 缺口、补齐项和复审结论。
- `agent-controlled-implementation-entry-audit-20260702.md`：受控实现入口审计，
  将架构通过标准映射到当前 SDD / ADR candidate / fixture evidence，并明确
  Agent Lab 条件通过但真实受控实现仍被 accepted ADR、full-package entry
  decision、owner review 和 real-service preservation smoke 阻断。
- `agent-full-package-entry-review-request-20260702.md`：面向主集成的完整包准入
  review request，列出请求裁决、acceptance questions、非请求项、接受 / 打回
  后动作；只请求 ADR acceptance review，不请求生产实现授权。
- `agent-adr-acceptance-review-playbook-20260702.md`：ADR acceptance review
  playbook，定义 L0-L4 准入层级、候选 ADR signoff matrix、auto-reject /
  deferral rules 和 reviewer result template；不授权生产实现。
- `agent-l1-package-closure-audit-20260702.md`：六个 Agent ADR candidates
  均被主集成接受为 L1 reviewability only 后的闭环审计，主集成已接受为
  closure record；下一步只能进入 L2 scoped design 或 fixture-only hardening。
- `agent-eval-replay-l2-scoped-implementation-design-20260702.md`：Eval /
  Replay promotion gate 的第一份 L2 scoped implementation design；只定义 owner、
  L3 smoke、operator / audit / replay gates，不授权生产实现。
- `agent-runtime-workflow-l2-scoped-implementation-design-20260702.md`：Runtime /
  Workflow boundary 的第二份 L2 scoped implementation design；只定义
  cognitive runtime、durable workflow wait、checkpoint / wakeup、operator /
  audit gates 和 L3 smoke，不授权生产实现。
- `agent-context-evidencepack-l2-scoped-implementation-design-20260702.md`：
  Context / EvidencePack / RAG boundary 的第三份 L2 scoped implementation
  design；只定义 AI read boundary、source visibility、denied lanes、citation
  verifier、taint / redaction / replay-reader、operator gates 和 L3 smoke，不授权生产实现。
- `agent-eval-replay-l1-acceptance-review-20260702.md`：Eval / Replay
  candidate 的 L1 acceptance review 自评包，建议进入主集成 ADR review，
  但继续 defer production implementation。
- `agent-runtime-workflow-l1-acceptance-review-20260702.md`：Runtime /
  Workflow candidate 的 L1 acceptance review 自评包，建议作为第二个候选
  进入主集成 ADR review；不授权 runtime service 或 workflow 改动。
- `agent-context-evidencepack-l1-acceptance-review-20260702.md`：Context /
  EvidencePack candidate 的 L1 acceptance review 自评包，建议作为第三个候选
  进入主集成 ADR review；不冻结 EvidencePack / ContextPackage schema。
- `agent-memory-admission-l1-acceptance-review-20260702.md`：Memory Admission
  candidate 的 L1 acceptance review 自评包，建议作为第四个候选进入主集成
  ADR review；不冻结 MemoryCandidate / MemoryClaim / memory event schema。
- `agent-tool-mcp-l1-acceptance-review-20260702.md`：Tool / MCP candidate 的
  L1 acceptance review 自评包，建议作为第五个候选进入主集成 ADR review；
  不授权生产 MCP provider、tool schema 或 side-effect execution path。
- `agentops-governance-l1-acceptance-review-20260702.md`：AgentOps /
  Governance candidate 的 L1 acceptance review 自评包，建议作为第六个候选进入
  主集成 ADR review；不授权生产 release pipeline、admin console 或
  control-plane API。
- `agent-eval-replay-adr-review-20260702.md`：Eval / Replay ADR candidate
  focused review，补齐 failure-class lifecycle、baseline approval、retention /
  redaction 和 contract-version bump rehearsal。
- `agent-runtime-workflow-adr-review-20260702.md`：Runtime / Workflow ADR
  candidate focused review，补齐 checkpoint storage、wakeup dedupe、operator
  controls 和 budget ledger 边界。
- `agent-context-evidencepack-adr-review-20260702.md`：Context / EvidencePack
  ADR candidate focused review，补齐 source visibility、denied-lane、citation
  verifier、taint vocabulary 和 operator evidence inspection。
- `agent-memory-admission-adr-review-20260702.md`：Memory Admission ADR
  candidate focused review，补齐 lifecycle states、category thresholds、
  revocation / dependency invalidation、audit explanation 和 operator UX。
- `agent-tool-mcp-adr-review-20260702.md`：Tool / MCP ADR candidate focused
  review，补齐 capability lease、provider attestation、prepare expiry /
  re-prepare、sandbox onboarding 和 output reuse。
- `agentops-governance-adr-review-20260702.md`：AgentOps / Governance ADR
  candidate focused review，补齐 kill switch、release pinning、baseline
  approval、failure-class owner workflow 和 canary/shadow comparison。
- `agent-fixture-evidence-hardening-20260702.md`：fixture-only evidence 更新，
  记录 Eval / Replay version-bump rehearsal 的代码、fixture、测试和剩余条件。
- `agent-runtime-workflow-fixture-evidence-20260702.md`：fixture-only evidence
  更新，记录 Runtime / Workflow ownership rehearsal 的 wakeup dedupe、stale
  checkpoint、resume/cancel、operator control 和 BudgetLedger 证据。
- `agent-context-evidence-fixture-evidence-20260702.md`：fixture-only evidence
  更新，记录 Context / EvidencePack preservation rehearsal 的 denied-lane、
  source-ref、citation verifier、taint 和 operator inspect 证据。
- `agent-memory-fixture-evidence-20260702.md`：fixture-only evidence 更新，
  记录 Memory Admission governance rehearsal 的 category threshold、
  revocation、retrieval eligibility、ACTIVE explanation 和 operator UX 证据。
- `agent-tool-mcp-fixture-evidence-20260702.md`：fixture-only evidence 更新，
  记录 Tool / MCP governance rehearsal 的 capability lease denial、
  provider attestation downgrade、sandbox onboarding、prepare re-prepare、
  tool-output taint 和 executor stale-prepare rejection 证据。
- `agentops-governance-fixture-evidence-20260702.md`：fixture-only evidence
  更新，记录 AgentOps governance rehearsal 的 release blocking、baseline
  approval、kill-switch propagation、failure owner、canary / shadow comparability
  和 operator controls 证据。
- `agent-dataset-reproducibility-fixture-evidence-20260702.md`：fixture-only
  evidence 更新，记录 open dataset / synthetic fixture manifest、snapshot、
  split、import、adapter version 和 deterministic report 证据。
- `agent-cross-service-preservation-fixture-evidence-20260702.md`：
  fixture-only evidence 更新，记录 retrieval、memory、MCP、workflow、executor
  和 audit 边界的 refs / scope / version / taint / audit-lineage preservation
  证据。
- `agent-object-completeness-fixture-evidence-20260702.md`：fixture-only
  evidence 更新，记录当前生产对象目录的 owner / lifecycle / version /
  permission / audit / replay / operator / evidence / rejection 覆盖证据。
- `agent-multi-agent-handoff-fixture-evidence-20260702.md`：fixture-only
  evidence 更新，记录 internal specialist、future peer-agent 和
  multi-specialist bounded delegation 的 primary responsibility、candidate-only
  output、scope、budget / deadline、taint、audit、replay、verifier 和 rejection
  覆盖证据；不冻结生产 A2A contract。
- `agent-operator-governance-fixture-evidence-20260702.md`：fixture-only
  evidence 更新，记录 memory、evidence、replay、approval、release、
  failure-class、kill-switch、rollback operator surfaces 的 inspect-and-act、
  owner、auth-policy、audit、redaction、replay-reader、failure-class、evidence
  和 rejection 覆盖证据。
- `agent-operational-readiness-fixture-evidence-20260702.md`：fixture-only
  evidence 更新，记录 runtime step、model spend、tool timeout、retrieval
  latency、eval retention、canary telemetry 和 incident escalation budget 的
  owner、limit、measurement、operator view、audit、release gate 和 rejection
  覆盖证据；不授权生产 SLO。
- `agent-controlled-implementation-readiness-fixture-evidence-20260702.md`：
  fixture-only evidence 更新，记录 fixture-only hardening 可以继续、但
  controlled implementation / production contract 在缺少 accepted ADR、main
  review、owner review、preservation evidence 或出现真实服务 / Python final
  owner 越界时必须阻断。
- `agent-architecture-coverage-fixture-evidence-20260702.md`：fixture-only
  evidence 更新，记录 13 个必需 Agent 架构面的 owner、SDD、research、ADR、
  fixture evidence、lifecycle、version、replay、preservation、audit、operator、
  eval gate 和 rejection refs 覆盖证据。
- `agent-contract-version-compatibility-fixture-evidence-20260702.md`：
  fixture-only evidence 更新，记录 EvidencePack、ContextPackage、
  MemoryCandidate、MemoryClaim、ToolIntent、PreparedToolRef、ApprovalDecision、
  ExecutionReceipt、EvalReport 和 ReplayBundle 的兼容窗口、replay-reader、
  redaction、deprecation、migration、preservation、audit、operator、eval gate
  和 rejection refs 覆盖证据。

## 存放位置

- Zotero collection：`NexusIM`
- 本地 PDF：`H:\NexusIM\papers\im-ai-agent-rag-2026`
- 仓库内只放分类规则、引用说明和读论文后形成的设计结论。
- Agent Lab 的当前研究结论需要回链到 `docs/sdd/agent-platform.md`，不要只停留在
  原始资料摘录。
