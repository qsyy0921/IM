# Agent Current-to-Target Matrix

Date: 2026-07-01
Mode: Agent Exploration Mode
Status: Research matrix for SDD alignment. This is not a service contract,
schema, migration or implementation plan.

## 1. Purpose

This matrix maps the current NexusIM AI / Agent foundation services to the
target Agent platform design. It prevents two common failure modes:

- current first-path services continue growing without a target ownership model;
- target Agent architecture ignores the safe proposal-only baseline that already
  exists.

The matrix is intentionally non-contractual. It does not add proto, OpenAPI,
Kafka schema, database migration, service directory or production startup path.

## 2. Target Invariant

The target Agent platform is a governed execution system:

```text
Agent Gateway
  -> policy / budget / release gate
  -> Agent Runtime / Harness
  -> EvidencePack / ContextPackage / Memory / Tool Prepare
  -> proposal / workflow approval
  -> action-executor
  -> audit
  -> eval / replay / AgentOps
```

Current services should be evolved toward this shape, but implementation should
start with open datasets and synthetic fixtures before touching real IM data.

## 3. Service Matrix

| Service / Component | Current Role | Target Role | Keep | Change | Must Not Own | Promotion Trigger |
| --- | --- | --- | --- | --- | --- | --- |
| `agent-service` | Safe proposal-only baseline; verifies approved agent proposals; integrates EvidencePack, MCP prepare and action-executor preflight | Public Agent API, proposal facade, AgentDefinition metadata and possibly a short-lived runtime module | Proposal/approval safety chain | Do not become giant runtime/workflow/tool/memory/eval monolith | Workflow timers, provider redrive queues, ACTIVE memory, raw tool execution, audit archive | Keep module form until run/step/checkpoint/replay pressure justifies ADR |
| Agent Runtime / Harness | Conceptual only | Cognitive run state: AgentRun, AgentStep, checkpoint, ContextPackage refs, candidate outputs, ReplayBundle | None yet | Start as offline harness / module, not production service | Long approval waits, execution final state, business facts, ACTIVE memory | Promote to `agent-runtime-service` only after fixture evidence shows independent state/scaling/failure boundary |
| `memory-service` | Structured collaborative memory projection with MemoryCandidate review and ACTIVE/REJECTED states | Durable memory owner: admission, scope, version, supersedes, revoke, expiry, retrieval refs | Source-backed candidate/review pattern | Add stronger group/project/procedural rules and memory eval linkage | Business facts, raw model inference as durable fact, workflow timers | Promote memory admission contracts only after pollution/scope eval passes |
| `retrieval-gateway` | Unified EvidencePack boundary over search, memory and vector lanes | AI read boundary and source-coverage gate for ContextPackage construction | EvidencePack as hard read boundary | Add explicit ContextPackage handoff semantics and citation/abstain metrics | Raw source truth, memory admission, agent planning | Promote ContextPackage contract after fixture citation/permission tests pass |
| `rag-service` | Read-only grounded answer service using EvidencePack | Deterministic / constrained grounded answer lane for simple tasks and verifier lane for Agent Runtime | Extractive/grounded default, citation verifier | Use as evaluator/verifier beside runtime, not as general agent | Tool execution, workflow state, memory writes | Keep simple RAG path separate from heavy runtime path |
| `mcp-gateway` | Tool prepare/precheck, skill contract validation, low-sensitive prepare audit | Tool/MCP trust boundary: registry, provenance, risk, schema, prepare, output taint and sandbox policy | Prepare-before-execute boundary | Add MCP poisoning, tool-source provenance and output quarantine rules | Authorization truth, approval wait, external side-effect execution | Promote only after malicious tool description/output fixtures pass |
| `action-executor` | Approved execution boundary; idempotency; provider failure projection; controlled redrive | Sole owner of approved side-effect execution and execution result refs | Proposal/approval/prepare verification chain | Link execution result to state-diff eval and ReplayBundle refs | Agent planning, approval decisions, memory review | Keep as execution owner for all business mutations |
| `ai-eval-service` | Low-sensitive eval run catalog; does not execute evals | Eval run/report owner and AgentOps quality record store | Low-sensitive catalog pattern | Add suite aggregation for RAG, memory, tool, state-diff, security and replay | Production fallback decision, business facts, raw prompt archive | Promote after offline harness produces repeatable EvalRun/EvalResult |
| `ai-eval-harness` | Existing SDD for harness direction | Offline runner for public datasets and synthetic fixtures | Keep outside production path | Add AgentRun trace, ReplayBundle and failure taxonomy validation | Production runtime fallback | First concrete next implementation target after docs |
| `model-gateway` | Provider routing, model policy, usage/cost metadata | Required boundary for all Agent model/embedding/rerank calls | Provider isolation and budget metadata | Add run/eval linkage, model version refs and retention policy hooks | Planner state, memory admission, execution state | No direct Agent code should call raw provider SDK |
| `skill-registry` | Skill catalog: skill/version/tool_name/allowed_actions/schema/risk/approval/owner | SkillPackage governance catalog and eval gate linkage | Registry-only design | Add release/eval/risk metadata before production AgentDefinition release | Execution, approval, MCP provider secret | SkillPackage promotion requires eval suite and rollback policy |
| `workflow-service` | Workflow/HITL service in target architecture | Long-wait owner: approval, timeout, callback, repair/redrive decision, operator queue | Human/operational wait ownership | Add Agent workflow refs only; do not parse prompts | Raw prompt, EvidencePack body, planner state, tool selection, execution | Integrate after runtime can pause on workflow refs |
| `audit-service` | Low-sensitive append/query/export boundary | Cross-plane lineage owner for policy, memory, tool, workflow, execution and eval refs | Low-sensitive audit refs | Add AgentRun lineage views and replay refs | Authorization, workflow transition decisions, raw transcript archive | Required before any high-risk action production slice |
| Python AI Worker | Candidate intelligence plane | Transient candidate generation for rerank, extraction, scoring, memory candidate, eval helper | Candidate-only boundary | Add deterministic metadata: hashes, citations, confidence, failure reason | Final state, ACTIVE memory, approval, execution, audit archive | Never promoted to business state owner |

## 4. Current Safe Baseline

The current safe baseline should be preserved:

```text
EvidencePack
  -> grounded answer or proposal
  -> approval when required
  -> action-executor
  -> public business API
  -> audit
```

This baseline is intentionally narrower than the target Agent Runtime. It is
valuable because it proves the core safety invariant: model output never writes
business facts directly.

## 5. Target Gaps by Capability

| Capability | Current State | Target Gap | First Safe Step |
| --- | --- | --- | --- |
| Read-only QA | RAG / agent proposal baseline exists | ContextPackage and citation/abstain eval are not formalized | Public RAG dataset adapter plus synthetic permission fixture |
| Group memory | Memory candidate/review exists | Speaker/audience scope, conflict, supersedes and revocation are under-specified | Memory admission SDD and GroupMemBench/LoCoMo style fixture |
| Approval actions | Proposal/approval/executor chain exists | Runtime pause/resume and workflow refs are not exercised | Fixture-only approval wait AgentRun trace |
| Tool/MCP | Prepare/precheck exists | Tool poisoning, output taint and MCP provenance need first-class rules | MCP security eval fixtures before external MCP expansion |
| Replay | Audit/eval refs exist in pieces | Unified ReplayBundle and failure taxonomy are missing | Offline ReplayBundle draft generated by eval harness |
| AgentOps | Metrics listed | Release channel, canary, kill switch and regression gates need governance model | Governance SDD and eval gate catalog |

## 6. Service Promotion Recommendation

Recommended order:

1. Keep `agent-service` as safe proposal facade and optional runtime module.
2. Build offline `ai-eval-harness` fixtures for AgentRun, ContextPackage,
   MemoryCandidate, ToolIntent and ReplayBundle.
3. Use `workflow-service` only for long waits in fixtures; do not duplicate
   approval queues in runtime.
4. Promote `agent-runtime-service` only if fixture evidence shows independent
   run/step/checkpoint/replay state, separate scaling, or failure isolation need.

Do not choose `agent-runtime-service` simply because "Agent" sounds like a
service. Choose it only when ownership pressure is real.

## 7. Rejection Conditions

Reject a future implementation plan if it does any of the following:

- adds a production runtime service before public dataset and synthetic fixture
  evidence exists;
- lets `agent-service` own workflow timers, execution redrive, memory ACTIVE
  state or MCP provider adapters;
- lets workflow-service interpret prompt, EvidencePack body or planner state;
- lets Python AI Worker decide durable state;
- treats MCP server descriptions or outputs as trusted;
- uses fake/mock data as production fallback;
- uses real NexusIM IM data in first-stage Agent eval.

## 8. Next Step

Use this matrix to align the detailed SDD package and the open-dataset-first eval
plan. The next implementation-like artifact should be an offline harness or
fixture pack, not a production service directory.
