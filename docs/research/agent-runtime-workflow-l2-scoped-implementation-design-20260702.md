# Agent Runtime / Workflow L2 Scoped Implementation Design

Date: 2026-07-02

Status: L2 scoped implementation design draft for the Agent Runtime /
workflow-service boundary. This is not an accepted production ADR, proto,
OpenAPI, Kafka schema, migration, service directory, startup path, workflow
change, queue design or runtime implementation.

## Verdict

Conditionally passed as the second L2 scoped design draft.

Rejected for implementation until main integration, Agent Runtime,
workflow-service, action-executor, audit/security, governance/operator and
SRE/incident owners review the design and approve the required L3 real-service
smoke plan.

Reason: L1 accepted the Runtime / Workflow candidate for reviewability only.
The L2 question is narrower: how can a future controlled implementation prove
that cognitive run orchestration, durable workflow waits, side-effect execution
and replay remain owned by separate services without creating a second workflow
engine or a giant agent-service.

## Scope

The scoped slice is the Runtime / Workflow ownership boundary.

It covers:

- AgentRun, AgentStep, AgentCheckpoint, RuntimeWakeup, CancelToken,
  ResumeToken, ReplayIndex and BudgetLedger ownership;
- workflow approval wait, timer, callback, repair and compensation ownership;
- checkpoint and wakeup compatibility rules;
- cancel, resume, replay and redrive behavior;
- action-executor handoff invariants;
- low-sensitive operator inspect-and-act surfaces;
- service-promotion choices and refusal conditions;
- L3 real-service smoke requirements.

It does not cover:

- final AgentRun, checkpoint, wakeup, workflow or replay field shape;
- workflow-service API, timer, queue, schema or callback changes;
- production agent-runtime-service creation;
- production worker deployment or capacity model;
- production audit table, release automation or admin UI;
- real NexusIM IM data;
- real PostgreSQL, Kafka, Redis, OpenSearch, model, MCP, workflow, memory,
  action-executor or audit integration.

## Boundary Thesis

Split by state type, not by product feature.

```text
Agent Runtime owns cognitive run state:
  context refs, planning, model/provider call metadata, verifier refs, tool
  intent, memory candidates, checkpoints, replay refs, run-local budget,
  cancel/resume intent and bounded handoff candidates.

workflow-service owns durable wait state:
  approval waits, repair waits, external callbacks, workflow timers,
  timeout decisions, compensation workflows, operator queues and durable
  wakeup delivery.
```

Runtime may request a workflow and later consume a workflow decision ref.
workflow-service may record durable decisions and wake Runtime. Neither service
may own the other's private state.

## Proposed Ownership

| Object / State | Owner | Cannot Be Owned By |
| --- | --- | --- |
| AgentRun metadata | Agent Runtime or approved Go runtime module | Python worker, workflow-service |
| AgentStep trace | Agent Runtime | workflow-service, action-executor |
| ContextPackage refs | Agent Runtime | workflow-service |
| Planner / verifier candidate refs | Agent Runtime | workflow-service, action-executor |
| ToolIntent before prepare | Agent Runtime | workflow-service |
| AgentCheckpoint | Agent Runtime | workflow-service, Python worker |
| RuntimeWakeup consumption | Agent Runtime | workflow-service durable queue |
| CancelToken / ResumeToken | Agent Runtime for run-local state; workflow-service for wait-local state | Python worker |
| ReplayIndex for cognitive lineage | Agent Runtime plus audit/replay reader policy | workflow-service as sole owner |
| BudgetLedger | Agent Runtime plus governance/policy owner | Python worker, model output |
| Approval wait / timeout | workflow-service | Agent Runtime |
| External callback wait | workflow-service | Agent Runtime |
| Repair / compensation workflow | workflow-service | Agent Runtime, action-executor |
| Approved side-effect execution | action-executor | Agent Runtime, workflow-service |
| Execution idempotency ledger | action-executor | Agent Runtime, Python worker |
| Audit archive | audit-service | Agent Runtime, Python worker |
| Candidate generation | Python AI Worker or model worker | final state owners |

## Non-Owner State

Agent Runtime must not own:

- human approval queues, workflow timers or external callback queues;
- workflow decision truth;
- repair or compensation durable state;
- execution attempts, execution receipts or idempotency ledger truth;
- ACTIVE memory state;
- audit archive of record;
- provider secrets;
- raw prompt, raw provider body, raw MCP payload or private service rows as
  normal checkpoint/replay material.

workflow-service must not own:

- model prompts;
- EvidencePack bodies or ContextPackage bodies;
- planner state;
- model output or verifier candidate text;
- tool selection;
- tool output safety decision;
- final action execution;
- business facts or memory facts.

Python AI Worker must not own:

- production AgentRun truth;
- checkpoint compatibility;
- workflow decision truth;
- final proposal state;
- ACTIVE memory;
- execution state;
- audit archive;
- operator override or release decision.

## Candidate L2 Flow

```text
Agent request
-> Agent Runtime creates AgentRun and budget refs
-> Runtime builds ContextPackage refs and planner/verifier candidates
-> Runtime reaches external-wait boundary
-> Runtime writes checkpoint refs and approval request refs
-> workflow-service owns durable wait, timer, callback and decision refs
-> workflow-service emits a wakeup ref
-> Runtime consumes wakeup idempotently after checkpoint/version/correlation checks
-> Runtime resumes cognitive run or finalizes failure
-> action-executor executes only approved and prepared actions
-> audit-service records low-sensitive lineage
-> ReplayIndex composes per-owner refs without re-executing providers
```

The L2 flow is a design path only. It does not create production messages,
tables, APIs, timers or workers.

## Scenario Rules

### Read-Only QA

- Runtime owns context building, model calls, citation verification and
  abstention.
- workflow-service and action-executor are not involved.
- Replay uses EvidencePack refs, ContextPackage refs, model metadata, candidate
  hashes and verifier refs.
- Insufficient evidence must produce abstain or clarification, not a workflow.

### Group Memory Admission

- Runtime or Python can produce a MemoryCandidate ref only.
- memory-service owns source validation, scope, conflict checks and ACTIVE /
  REJECTED / NEEDS_REVIEW state.
- workflow-service may own a human memory-review wait if needed, but cannot make
  memory active.
- Runtime replay records candidate and review refs only.

### Business Action Requiring Approval

- Runtime owns intent interpretation, proposal candidate and ToolIntent.
- mcp-gateway owns prepare / precheck / policy refs.
- workflow-service owns approval wait, timeout and decision.
- action-executor owns approved execution and idempotency.
- Runtime must not execute a tool or bypass a workflow decision.

### Long User Approval Wait

- Runtime checkpoints and stops at the external-wait boundary.
- workflow-service owns the long wait, timeout, escalation and durable callback.
- Resume requires a workflow decision ref, compatible checkpoint version,
  matching run/step refs, non-cancelled state and unconsumed wakeup dedupe key.
- If proposal content changes, the old workflow closes and a new proof path is
  required.

### External Tool Provider Timeout

- Timeout before prepare completion belongs to mcp-gateway / Runtime retry
  budget and can fail as provider timeout.
- Timeout after approved execution starts belongs to action-executor.
- action-executor may expose provider failure refs and request repair/redrive
  workflow, but cannot silently replay old raw provider input.
- Any business redrive requires fresh proposal, prepare, approval and executor
  lineage.

### Repair / Redrive

- Runtime may repair cognitive artifacts: invalid format, missing citations,
  unsafe candidate shape or verifier gaps.
- Runtime cannot directly repair execution truth or workflow truth.
- workflow-service owns repair approval waits.
- action-executor or the owning business service owns actual repair/redrive
  execution through public approved paths.

### Cancel / Resume / Replay

- Runtime cancel stops future runtime-owned steps and records cancel refs.
- workflow-service cancel stops workflow-owned wait state.
- Resume is accepted only after wakeup and checkpoint correlation passes.
- Replay reconstructs lineage from refs, hashes, versions and audit refs.
- Replay must not call model providers, tool providers or action-executor.

### Multi-Agent Handoff

- Runtime may create bounded delegation with scope, budget, deadline, evidence
  refs and taint refs.
- Specialist output remains candidate-only until verified by the primary run.
- workflow-service is not the default owner for multi-agent handoff.
- Future peer-agent contracts need separate identity, policy, budget, audit and
  replay review before production promotion.

## Checkpoint And Wakeup Rules

Every future checkpoint design must carry low-sensitive refs for:

- run id and step id;
- checkpoint version and compatibility window;
- ContextPackage and EvidencePack refs;
- proposal and ToolIntent refs when applicable;
- workflow request and decision refs when applicable;
- wakeup dedupe key and consumed state ref;
- cancel token state ref;
- BudgetLedger snapshot ref;
- ReplayIndex and audit lineage refs;
- replay-reader policy ref.

Checkpoint storage must reject:

- raw prompts;
- raw model/provider bodies;
- raw MCP payloads;
- secrets or tokens;
- private service table rows;
- business facts;
- ACTIVE memory facts;
- operator reason text beyond approved low-sensitive refs.

Runtime resume must fail closed when:

- checkpoint version is unsupported;
- wakeup decision does not match run, step and checkpoint refs;
- wakeup has already been consumed;
- cancel token blocks resume;
- BudgetLedger is over limit;
- replay-reader policy cannot explain the resumed path.

## Budget And Abuse Rules

BudgetLedger must exist for every future path that can perform long-running
model work, retrieval loops, tool prepare retries, workflow waits or multi-agent
handoff.

The ledger must track:

- model token and spend refs;
- retrieval and RAG call refs;
- tool prepare and execution budget refs;
- retry, repair and redrive counts;
- runtime duration and step count;
- workflow wait duration refs;
- tenant, actor, AgentDefinition and SkillPackage refs;
- failure-class and owner-review refs.

Over-budget runs fail closed or require explicit governance review. Runtime
must not convert an over-budget state into a fallback summary or partial action.

## Operator Surfaces

Before any implementation, owners must approve low-sensitive inspect-and-act
surfaces for:

- inspect AgentRun, checkpoint, workflow, executor, audit and ReplayIndex refs;
- cancel runtime-owned state;
- cancel workflow-owned wait state;
- resume from compatible checkpoint and workflow decision refs;
- reject stale, duplicate or mismatched wakeup refs;
- replay cognitive lineage without side effects;
- view budget state and failure class;
- see why cancel, resume, replay or redrive was rejected.

These surfaces belong to a Go control-plane / AgentOps owner. Python may create
test artifacts but cannot operate production controls.

## Service Promotion Choices

### Option A: Extend agent-service

Use only for small proposal-path or read-only baseline extensions.

Advantages:

- lowest immediate service count;
- easy to reuse existing proposal and verification boundaries;
- practical if AgentRun remains short and mostly synchronous.

Reject when:

- run/step/checkpoint/replay state becomes first-class;
- long-running model work or workflow waits are common;
- agent-service starts owning workflow-like queues;
- runtime workers need independent scaling, metrics or incident ownership.

### Option B: Add agent-runtime-service

Use only after queue ownership, wakeup durability, checkpoint pressure,
operator controls and capacity evidence prove a service boundary is needed.

Advantages:

- clearest cognitive-runtime owner;
- separates long-running run workers from public proposal APIs;
- gives AgentRun, AgentStep, checkpoint, replay and budget their own operational
  envelope.

Reject when:

- Runtime / Workflow owner review is incomplete;
- L3 real-service smoke is missing;
- checkpoint compatibility and wakeup idempotency are unproven;
- operator cancel/resume/replay UX is not approved;
- eval/replay gates cannot block unsafe promotion.

### Option C: Retain runtime module plus workflow-service

Recommended first controlled direction.

Runtime remains an internal Go module or harness boundary while workflow-service
continues to own durable waits. This avoids premature service creation while
preserving the future option to promote runtime into a service.

Reject when:

- the module needs independent database tables, queues, workers, metrics,
  capacity budgets or on-call ownership;
- multiple services need it as a public runtime boundary;
- module code starts hiding workflow, execution, audit or memory state.

Recommended order:

1. Keep Option C during L2 review and L3 smoke design.
2. Let owners review checkpoint, wakeup, cancel/resume/replay and BudgetLedger
   boundaries.
3. If capacity and ownership evidence justify it, write a separate ADR for
   Option B.
4. Use Option A only for narrow baseline extensions that do not introduce
   durable runtime ownership.

## L3 Real-Service Smoke Plan

Before implementation can start, owners must approve a smoke plan that proves
the following with low-sensitive records only:

| Smoke | Boundary | Required Proof |
| --- | --- | --- |
| Checkpoint write/read dry run | Agent Runtime -> future store owner -> audit | Raw prompts, provider bodies, secrets and private rows are rejected |
| Durable approval wait | Runtime -> workflow-service | Runtime stops at checkpoint; workflow owns timer and decision |
| Duplicate wakeup dedupe | workflow-service -> Runtime | Only one wakeup consumes the dedupe key; duplicates are rejected with replay refs |
| Stale checkpoint rejection | Runtime checkpoint reader | Unsupported or mismatched checkpoint versions fail closed |
| Resume correlation | workflow-service -> Runtime | Decision, checkpoint, run and step refs must match |
| Cancel propagation | Runtime and workflow-service | Runtime cancel and workflow wait cancel are distinct and auditable |
| Approved execution handoff | Runtime -> mcp-gateway -> workflow-service -> action-executor | Execution requires proposal, prepare, approval and idempotency refs |
| Replay reader dry run | Runtime -> audit -> per-owner refs | Replay does not call model, MCP provider or action-executor |
| Budget overrun | Runtime -> governance/operator | Over-budget path blocks continuation or requires explicit review |
| Operator inspect-and-act | AgentOps/control-plane | Authorized operator can inspect/cancel/resume/replay; passive-only view fails |

These smokes must not use real NexusIM IM data. Fixture evidence can prepare
the plan, but cannot substitute for L3 real-service smoke.

## Owner Review Checklist

| Owner | Must Approve |
| --- | --- |
| Main integration | Service boundary, allowed paths and no production shortcut |
| Agent Runtime owner | AgentRun, AgentStep, checkpoint, wakeup, replay and BudgetLedger ownership |
| workflow-service owner | Durable wait, timer, callback, repair and compensation ownership |
| action-executor owner | Approved execution, idempotency and provider failure handoff |
| mcp-gateway / policy owner | Prepare, precheck, lease, expiry and policy refs |
| audit/security owner | Low-sensitive refs, retention, redaction, deletion and replay-reader policy |
| governance/operator owner | Inspect-and-act, kill/cancel/resume/replay, budget override and failure-class workflow |
| SRE/incident owner | Runtime duration, queue pressure, checkpoint pressure, wait duration and incident handoff |

## Test And Gate Plan

Existing Agent Lab gates that must continue to pass:

```powershell
python -m pytest ai/python/tests -q
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts
.\tools\check-python-ai-worker-boundary.ps1
.\tools\check-runbook-consistency.ps1
git diff --check
git diff --cached --check
```

Focused fixture gates to rerun for this slice:

```powershell
python -m pytest ai/python/tests/test_agent_eval_runtime_workflow_ownership.py -q
python -m pytest ai/python/tests/test_agent_eval_operator_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_operational_readiness.py -q
python -m pytest ai/python/tests/test_agent_eval_controlled_implementation_readiness.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_negative_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_runtime_control_deeper_hardening_scenarios.json
```

## P0 / P1 / P2 Review

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside this L2 design draft | No production code, schema, service connection, real data, queue or runtime implementation is introduced |
| P1 | Owner review is missing | External blocker before implementation |
| P1 | L3 real-service smoke is missing | External blocker before implementation |
| P1 | Production operator inspect-and-act UX is not approved | External blocker before implementation |
| P1 | Capacity and checkpoint pressure evidence is missing for service promotion | Blocks agent-runtime-service ADR, not this design draft |
| P2 | Exact budget thresholds and SLOs are not approved | Keep for policy and SRE owner review |
| P2 | Workflow callback transport and dedupe storage are not selected | Keep out of L2 until workflow owner proposes production shape |

## Auto-Reject Rules

Reject any Runtime / Workflow implementation proposal that:

- creates proto, OpenAPI, Kafka schema, migration, service directory, queue,
  startup path or runtime worker from this L2 design alone;
- makes Runtime a durable approval, timer, callback, repair or compensation
  engine;
- lets workflow-service read raw prompt, EvidencePack body, ContextPackage,
  model output, planner state or tool output safety decisions;
- lets action-executor run an action without proposal, prepare, approval,
  idempotency and audit refs;
- lets replay re-execute model calls, MCP provider calls or business actions;
- stores raw prompt, provider body, MCP payload, secret or private service row as
  normal checkpoint/replay material;
- lets Python own production run state, checkpoint, approval, execution,
  workflow decision, ACTIVE memory, audit archive or operator override;
- treats fixture evidence as real-service smoke;
- hides timeout, over-budget, stale checkpoint or duplicate wakeup failures with
  fallback summaries or default success.

## Decision

This design closes the Agent Lab-side L2 design gap for the second candidate:
Runtime / Workflow. It does not authorize implementation.

Next safe action after main integration review is either:

- owner review of Eval / Replay and Runtime / Workflow L2 designs; or
- a third L2 scoped design for Context / EvidencePack, Memory, Tool / MCP or
  AgentOps if owners want the whole package prepared before any real-service
  smoke.
