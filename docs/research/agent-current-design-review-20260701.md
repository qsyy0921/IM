# Agent Current Design Review

Date: 2026-07-01
Mode: Agent Exploration Mode
Status: Review result and rework record. This is not an ADR, proto, OpenAPI,
Kafka schema, migration, production service directory or runtime contract.

## 1. Review Question

This review answers whether the current NexusIM Agent design can be promoted
from exploratory architecture into implementation-oriented SDD work.

Reviewed inputs:

- `docs/sdd/agent-platform.md`
- `docs/architecture/agent-plane-initial-design.md`
- `docs/research/agent-system-complete-scope-20260701.md`
- `docs/research/agent-runtime-workflow-ownership-20260701.md`
- Current service SDDs for `agent-service`, `memory-service`,
  `retrieval-gateway`, `rag-service`, `mcp-gateway`, `action-executor`,
  `ai-eval-service`, `model-gateway` and `skill-registry`
- Public 2026 agent signals and benchmark inputs listed in section 8

## 2. Verdict

The current direction is architecturally correct, but the current
`agent-platform.md` alone is not sufficient for implementation promotion.

Formal result:

```text
REJECTED_FOR_IMPLEMENTATION_PROMOTION
Severity: P1
Disposition: rework required, not a rejection of the overall direction
```

The design should not be expanded directly into proto, schema, runtime service
or production implementation until the missing ownership, eval, memory,
context, tool-security and governance details are repaired.

After this review, the rework path is to add a detailed SDD package rather than
continue making one platform document larger:

- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-memory-admission.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/sdd/agent-tool-mcp-boundary.md`
- `docs/sdd/agent-eval-replay-harness.md`
- `docs/sdd/agent-governance-agentops.md`
- `docs/research/agent-current-to-target-matrix-20260701.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`

## 3. What Is Correct

The current design has the right high-level invariants:

- Agent does not enter IM message delivery hot path.
- Read paths go through retrieval / EvidencePack / ContextPackage.
- Write paths go through proposal / approval / action-executor / audit.
- Python AI Worker is candidate-only.
- Memory is source-backed, scoped, reviewed and revocable.
- MCP and tool providers are untrusted inputs, not authorization boundaries.
- Eval and replay are architecture components, not post-launch scripts.
- Agent Runtime and workflow-service are separated by cognitive run state vs
  long-wait human/operational state.

These should remain target invariants.

## 4. Blocking Findings

| Severity | Finding | Impact | Required Rework |
| --- | --- | --- | --- |
| P1 | Platform SDD is too broad to implement safely | Teams may interpret a capability map as service contract | Split into focused SDDs with owner, state, failure, eval and promotion gates |
| P1 | Current-to-target service mapping is incomplete | Existing services may drift or duplicate responsibilities | Add explicit migration matrix for agent-service, memory, retrieval, RAG, MCP, executor, eval and model gateway |
| P1 | Open dataset plan is a dataset list, not an eval spec | Agent Lab cannot prove capability before real IM data | Define EvalCase, EvalRun, EvalResult, adapters, synthetic IM-like fixtures and pass/fail dimensions |
| P1 | Memory admission lacks operational rules | Durable memory pollution and cross-scope leakage can persist | Add candidate extraction, dedupe, conflict, supersedes, review, revoke, expiry and retrieval rules |
| P1 | Tool/MCP security is under-specified | Tool poisoning or malicious output can affect planning | Add provenance, risk levels, prepare envelope, output tainting, sandbox and approval boundaries |
| P1 | Runtime pause/resume/replay lacks idempotency detail | Two engines may emerge: runtime and workflow | Define checkpoint, resume, replay and side-effect rules in a Runtime SDD |
| P1 | AgentOps is mostly metric naming | Release, canary, rollback and kill switch are not governed | Add governance SDD for AgentDefinition, SkillPackage and release gates |

## 5. Non-Blocking Findings

| Severity | Finding | Impact | Follow-up |
| --- | --- | --- | --- |
| P2 | A2A / peer-agent boundary is still conceptual | Future peer-agent integration may be confused with tools | Keep A2A out of first production slice; model only bounded delegation |
| P2 | Multi-agent taxonomy is intentionally not frozen | Some examples remain abstract | Acceptable in Exploration Mode; evaluate bounded handoff first |
| P2 | SDD does not choose `agent-runtime-service` vs module | Service promotion is unresolved | Keep as runtime module plus workflow-service until fixture evidence justifies ADR |
| P2 | Synthetic IM-like fixture rules are not yet concrete | Dataset adapters may diverge | Fix in open dataset eval plan |
| P3 | References are spread across research docs | Reviewers need a clearer source map | Add links from indexes and platform SDD |

## 6. Rework Acceptance Criteria

The reworked package is acceptable only if it satisfies all conditions below:

- Every Agent component states what it owns and what it cannot own.
- Memory admission has source, scope, speaker, audience, version, supersedes,
  revocation and review semantics.
- EvidencePack / ContextPackage design includes citation coverage, abstain,
  conflict markers, temporal validity and permission leakage checks.
- Tool/MCP design treats description, schema examples and output as untrusted.
- Runtime design distinguishes short cognitive run state from workflow-owned
  long waits.
- Eval design starts from public datasets and synthetic fixtures, not real
  NexusIM IM data.
- Governance design includes owner, risk tier, eval gate, canary, rollback and
  kill switch.
- The design remains non-contractual: no proto, OpenAPI, Kafka schema,
  migration or production directory is created.

## 7. Self-Review After Rework

| Review Dimension | Result After SDD Package | Notes |
| --- | --- | --- |
| Correctness | Conditional pass | Core state boundaries are explicit; no production contract frozen |
| Sufficiency | Conditional pass | Enough detail for fixture/eval work; not yet enough for production ADR |
| Security | Conditional pass | MCP/tool/memory/prompt-injection risks are modeled as first-class gates |
| Ownership | Pass for exploration | Runtime/workflow/executor/memory ownership is separated |
| Operability | Conditional pass | AgentOps design exists; no real dashboard or metrics contract yet |
| Evalability | Pass for next phase | Public dataset and synthetic fixture path is defined |
| Implementation risk | Conditional pass | Recommended next step remains offline harness, not service promotion |

Remaining residual risks are P2/P3 and should not block documentation handoff,
but they block production implementation.

## 8. Source Signals Used

The review used the following external signals as pressure tests, not as
contracts:

- OpenClaw: typed gateway, handshake, idempotency and workspace-aware agent
  runtime boundaries.
- Hermes: memory provider lifecycle, prefetch/sync/write hooks and fenced
  memory context handling.
- Claude Code: lifecycle hooks, subagents, permission modes, checkpoints and
  scoped MCP/tool controls.
- OpenAI Agents SDK: agent, handoff, guardrail and tracing model.
- LangGraph, AutoGen, CrewAI, Google ADK and Microsoft Agent Framework:
  explicit runtime, state, HITL, checkpoint, tracing and workflow signals.
- Databricks 2026 State of AI Agents:
  <https://www.databricks.com/resources/ebook/state-of-ai-agents>
- STATE-Bench:
  <https://opensource.microsoft.com/blog/2026/05/19/introducing-state-bench-a-benchmark-for-ai-agent-memory/>
- Agent-Diff:
  <https://arxiv.org/abs/2602.11224>
- MCPSecBench:
  <https://arxiv.org/abs/2508.13220>
- ToolSandbox:
  <https://aclanthology.org/2025.findings-naacl.65/>
- GroupMemBench:
  <https://www.microsoft.com/en-us/research/publication/groupmembench-benchmarking-llm-agent-memory-in-multi-party-conversations/>

## 9. Decision

Do not promote the current platform-level SDD directly to implementation.

Proceed with the reworked SDD package and open-dataset-first eval plan. If the
reworked package introduces new P0/P1 ownership or security gaps, reject it and
redo the affected SDD before any ADR or runtime prototype is started.
