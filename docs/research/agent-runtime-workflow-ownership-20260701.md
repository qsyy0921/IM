# Agent Runtime vs Workflow Ownership Matrix

Date: 2026-07-01
Mode: Agent Exploration Mode
Status: Draft for main integration review, not ADR / SDD / proto contract.

## 1. Purpose

This document defines the exploratory responsibility boundary between the
Candidate B Agent Runtime / Harness Plane and the existing `workflow-service`.
It is meant to prevent two failure modes:

- two competing long-transaction engines, one in Agent Runtime and one in
  workflow-service;
- one giant `agent-service` that owns runtime, workflow, tool execution, memory,
  approval, audit and eval.

This document does not define proto, OpenAPI, Kafka schema, migration, service
directory, final agent taxonomy, skill taxonomy, EvidencePack shape or memory
event shape.

## 2. Boundary Thesis

Agent Runtime and workflow-service should split by type of state:

```text
Agent Runtime owns cognitive run state:
  context building, planning, model/provider calls, tool intent, checkpoints,
  replay, budgets, cancellation and run-local memory candidates.

workflow-service owns human/operational long-wait state:
  approval waiting, repair approval, provider replay handoff, external callback
  delivery, compensation, timeout decision and operator queues.
```

The integration rule is:

```text
Agent Runtime may request a workflow.
workflow-service may report workflow decisions.
workflow-service must not interpret prompts, EvidencePack snippets, model
outputs or planner state.
Agent Runtime must not implement approval queues, compensation state,
operator repair state or provider replay approval state.
```

## 3. Component Ownership Summary

| Component | Owns | Must Not Own |
| --- | --- | --- |
| Agent Runtime / Harness | `AgentRun`, `AgentStep`, run-local `ContextPackage`, planner / critic / verifier candidate refs, model/provider call metadata, checkpoint refs, run budget, cancellation intent, replay bundle, tool intent before prepare, memory candidate refs | approval decisions, workflow timers, compensation state, provider replay approval state, final business execution state, audit archive, final ACTIVE memory, raw provider body |
| workflow-service | workflow request / step / decision / timer / compensation / external callback delivery / workflow outbox, operator queues, approval wait and timeout state | agent plan, model prompt, EvidencePack body, ContextPackage, tool schema selection, proposal text, final tool execution, business facts, raw operator reason |
| agent-service | public proposal API, proposal metadata, approval preflight refs, `VerifyApprovedAgentProposal`, current safe baseline for read-only / proposal-only flows | general long-running runtime engine, workflow queue, tool execution, final memory admission, external provider replay state |
| mcp-gateway | tool prepare / precheck audit, skill contract validation, tool intent policy precheck, input hash, low-sensitive prepare result | external tool execution, approval wait, business mutation, final action result, raw prompt / evidence text |
| action-executor | approved / allowlisted execution attempt, idempotency ledger, tool result projection, provider failure projection, controlled redrive execution API, public business API adapter | agent planning, approval decision, workflow state, retrieval, memory review, raw provider output storage |
| audit-service | append-only low-sensitive audit records, audit stream, audit export/query boundary | authorization decision, workflow state transition, business fact source, replay raw prompt/body store |
| Python AI Worker | candidate outputs for planner, critic, rerank, memory extraction, profile aggregation, eval or provider calls; candidate hash/citation/confidence metadata | final run state, approval, durable business state, ACTIVE memory, proposal status, execution status, audit archive |

## 4. State Ownership Rules

### 4.1 Agent Runtime State

Agent Runtime may own these state families:

- run identity and lifecycle;
- step identity and step status;
- selected EvidencePack refs and ContextPackage refs;
- prompt / provider version refs and low-sensitive hashes;
- planner / critic / verifier candidate refs;
- tool intent before prepare;
- checkpoint and replay refs;
- budget and timeout counters for model/tool planning;
- cancel / resume intent;
- memory candidate refs before memory-service review.

Agent Runtime must not own:

- workflow approval decision;
- operator queue;
- compensation execution state;
- provider replay approval state;
- final action execution result;
- final memory ACTIVE / REJECTED status;
- append-only enterprise audit archive.

### 4.2 workflow-service State

workflow-service may own:

- approval workflow request;
- workflow steps and decisions;
- timers and timeout state;
- compensation request and result refs;
- provider replay / repair workflow handoff;
- external callback delivery state;
- workflow outbox events;
- operator queue views.

workflow-service must not own:

- model prompts;
- ContextPackage;
- EvidencePack body;
- planner candidate text;
- proposal generation;
- tool selection;
- tool output safety decision;
- final business mutation.

### 4.3 Shared References

Cross-component references should be low-sensitive refs / hashes:

```text
agent_run_id
agent_step_id
evidence_pack_id
context_package_ref
proposal_id
prepared_audit_id
workflow_id
approval_id / decision_id
execution_id
audit_id
replay_bundle_ref
```

No component handoff should require raw prompt, raw provider body, raw tool
input, EvidencePack body, operator reason text, secret, token or private service
row content.

## 5. Scenario Matrix

### 5.1 Read-only Q&A

| Step | Owner | Notes |
| --- | --- | --- |
| IM trigger / explicit API request | Agent Gateway or agent-service baseline | Does not block IM delivery. |
| Retrieve EvidencePack | retrieval-gateway | Policy / visibility / temporal filtering stays here. |
| Build ContextPackage | Agent Runtime | Derived from EvidencePack; not a fact source. |
| Model / deterministic answer candidate | Agent Runtime, optional Python AI Worker | Python returns candidate only. |
| Citation verification | Agent Runtime or RAG boundary | Must fail closed on ungrounded output. |
| Final response metadata | Agent Runtime / agent-service | No workflow needed. |
| Audit | audit-service or service-local low-sensitive audit handoff | No approval state. |

Boundary:

- workflow-service is not involved.
- action-executor is not involved.

### 5.2 Group Memory Admission

| Step | Owner | Notes |
| --- | --- | --- |
| Detect possible durable memory | Agent Runtime or memory extraction worker | Candidate only. |
| Generate memory candidate | Python AI Worker optional | Must include source refs / hash metadata. |
| Source visibility and candidate validation | memory-service | Go owns final validation. |
| Review / admission policy | memory-service, optionally workflow-service for human review | Human review wait can be workflow; final memory state belongs to memory-service. |
| ACTIVE / REJECTED / NEEDS_REVIEW | memory-service | Not Agent Runtime. |
| Audit | audit-service / memory-service audit handoff | Low-sensitive refs. |

Boundary:

- Agent Runtime can propose, but cannot approve durable memory.
- workflow-service can wait for review, but cannot make memory facts active by
  itself.

### 5.3 Business Action Requiring Approval

| Step | Owner | Notes |
| --- | --- | --- |
| Intent detection | Agent Runtime | Based on EvidencePack / ContextPackage. |
| Tool candidate selection | Agent Runtime | No execution. |
| Tool prepare / policy precheck | mcp-gateway | Validates skill/tool/action and calls policy-service. |
| Proposal creation | agent-service or Agent Runtime through proposal API | Stores low-sensitive proposal refs. |
| Approval workflow request | workflow-service | Owns waiting, timeout and decision state. |
| Approved proposal verification | agent-service | `VerifyApprovedAgentProposal` style boundary. |
| Execution | action-executor | Calls public business API only. |
| Execution audit/result | action-executor + audit-service | Low-sensitive output refs / hashes. |

Boundary:

- Agent Runtime does not wait in a private approval loop.
- workflow-service does not execute the action.
- action-executor does not decide approval.

### 5.4 Long User Approval Wait

| Step | Owner | Notes |
| --- | --- | --- |
| Run reaches approval boundary | Agent Runtime | Creates checkpoint and workflow request. |
| Approval queue / timeout / external callback | workflow-service | Owns long-wait state. |
| User approves / rejects / timeout | workflow-service | Records decision. |
| Resume signal to run | Agent Runtime | Uses workflow decision ref and checkpoint. |
| Continue or terminate run | Agent Runtime | Does not rewrite workflow state. |

Boundary:

- Agent Runtime may be suspended and resumed from checkpoint.
- workflow-service is the only owner of approval timer and decision history.

### 5.5 External Tool Provider Timeout

| Step | Owner | Notes |
| --- | --- | --- |
| Approved execution starts | action-executor | After proposal / approval / prepare verification. |
| Provider timeout / failure classification | action-executor | Writes provider failure projection. |
| Retry bookkeeping | action-executor | Bounded retry metadata. |
| Need operator replay/redrive | action-executor creates low-sensitive handoff; workflow-service handles approval workflow | Old raw input must not be restored. |
| Fresh proposal / approval / prepared audit for redrive | Agent Runtime / agent-service + mcp-gateway + workflow-service | Fresh proof required. |
| Controlled redrive execution | action-executor | Uses normal execution path. |

Boundary:

- Agent Runtime does not own provider failure DLQ state.
- workflow-service does not redrive provider directly.
- action-executor must not auto-replay old raw provider input/output.

### 5.6 Repair / Redrive

| Step | Owner | Notes |
| --- | --- | --- |
| Failure projection / DLQ fact | Owning service, often action-executor or workflow-service | Each service owns its own failure table. |
| Operator review artifact | Owning service tooling | Low-sensitive refs / hashes. |
| Repair / redrive approval wait | workflow-service | `REPAIR_APPROVAL` style owner. |
| Fresh Agent proof if business/action redrive | Agent Runtime / agent-service / mcp-gateway | Fresh proposal, approval and prepare refs. |
| Execute repair/redrive | Owning service public API | Not workflow-service private mutation. |
| Audit append | audit-service | External append through approved path. |

Boundary:

- workflow-service approves and records decisions; it does not become the repair
  executor for every service.
- Agent Runtime can help produce fresh proof, but cannot mutate DLQ state.

### 5.7 Cancel / Resume / Replay

| Capability | Owner | Notes |
| --- | --- | --- |
| Cancel an agent run before external wait | Agent Runtime | Marks run cancel intent and stops future steps. |
| Cancel workflow wait | workflow-service | Only for workflow-owned waiting state. |
| Resume after workflow decision | Agent Runtime | Loads checkpoint and decision ref. |
| Replay cognitive run | Agent Runtime | Rebuilds ContextPackage / step lineage from refs / hashes. |
| Replay workflow decision history | workflow-service | Lists workflow decisions and step transitions. |
| Replay execution result | action-executor + audit-service | Execution refs / audit lineage. |

Boundary:

- There is no single global replay owner. Replay is composed from per-owner
  lineage.
- Agent Runtime replay must not require raw provider body or private service
  tables.

### 5.8 Multi-Agent Handoff

| Step | Owner | Notes |
| --- | --- | --- |
| Primary agent decides delegation | Agent Runtime | Must keep primary responsibility. |
| Specialist context package | Agent Runtime | Scoped, minimal, evidence-bound. |
| Specialist candidate | Python AI Worker or future peer agent | Candidate only unless future A2A contract is approved. |
| Handoff audit lineage | Agent Runtime + audit-service | Records delegate identity / refs. |
| Tool execution from specialist | mcp-gateway + action-executor | Same prepare / approval / execute path. |

Boundary:

- Multi-agent handoff is not workflow-service by default.
- Future A2A peer agents need identity, policy, budget and audit lineage before
  they can be treated as more than candidate producers.

## 6. Ownership by State Type

| State Type | Agent Runtime | workflow-service | agent-service | mcp-gateway | action-executor | audit-service | Python AI Worker |
| --- | --- | --- | --- | --- | --- | --- | --- |
| User intent interpretation | Owns run-local interpretation | No | May expose baseline API | No | No | No | Candidate only |
| Evidence retrieval result | Ref only | No | Ref only | No | No | Audit ref only | No |
| ContextPackage | Owns | No | Maybe ref | No | No | Ref/hash only | Candidate input only |
| Planner candidate | Owns / validates | No | May store proposal derivative | No | No | Ref/hash only | Produces candidate |
| Tool intent before prepare | Owns | No | May pass through | Prepares | No | Ref/hash only | Candidate only |
| Tool prepare audit | Ref only | No | Ref only | Owns | Ref only | May archive | No |
| Proposal status | Ref / maybe creates through API | No | Owns in current baseline | No | Verifies ref | May archive | No |
| Approval wait | Suspends on ref | Owns | Ref only | No | Ref only | May archive | No |
| Workflow timer | No | Owns | No | No | No | Ref only | No |
| Execution attempt | No | No | No | No | Owns | May archive | No |
| Provider failure | No | Approval handoff only | No | No | Owns for action providers | May archive | No |
| Memory candidate | Creates ref | Review wait only if needed | No | No | No | Ref/hash only | Produces candidate |
| ACTIVE memory | No | No | No | No | No | May archive | No |
| Replay bundle | Owns cognitive replay | Owns workflow replay | Proposal refs | Prepare refs | Execution refs | Audit refs | No |

## 7. Service Promotion Choices

### Option A: Extend `agent-service`

Description:

- Add runtime/harness modules inside existing `agent-service`.
- Keep current proposal APIs and add run / step / checkpoint internals later.

Pros:

- Lowest immediate service count.
- Reuses current proposal, approval preflight and EvidencePack integration.
- Good for short-term iteration.

Risks:

- `agent-service` can become a giant monolith.
- Runtime state and proposal state may get tangled.
- Harder to scale long-running run workers separately from simple proposal APIs.

Use when:

- Runtime remains small and mostly read-only/proposal-only.
- No persistent long-running planner workers are needed.

Reject when:

- run/step/checkpoint/replay state grows beyond proposal boundary;
- subagent, budget, cancel/resume and long-running model calls become common;
- service code starts owning workflow-like queues.

### Option B: Add `agent-runtime-service`

Description:

- New service owns AgentRun / AgentStep / ContextPackage / Checkpoint /
  ReplayBundle.
- Existing `agent-service` can remain public proposal / verification API or be
  gradually folded behind runtime.

Pros:

- Clearest cognitive runtime boundary.
- Separates long-running run workers from proposal APIs.
- Easier to scale and observe agent execution separately.

Risks:

- Adds service, migrations, deployment and gate cost.
- Requires careful integration with retrieval, memory, mcp, workflow and audit.
- Premature if runtime model is not validated by fixtures.

Use when:

- run replay, cancellation, checkpointing and budgets are first-class;
- multiple agent skills share the same runtime;
- long-running agent tasks are common.

Reject when:

- the only supported path remains EvidencePack -> deterministic proposal;
- ownership matrix still has unresolved conflicts with workflow-service;
- eval/replay fixtures have not proven the model.

### Option C: Runtime Module + workflow-service

Description:

- Keep a lightweight runtime module in `agent-service` or a worker package.
- Use workflow-service only for approval / repair / compensation / callback
  long waits.
- Delay new service promotion until runtime state pressure is proven.

Pros:

- Practical transition path.
- Avoids new service before trace model is validated.
- Keeps workflow-service focused on long waits.

Risks:

- Module can silently grow into a hidden runtime service without explicit ADR.
- If ownership checks are weak, workflow and runtime state can still overlap.

Use when:

- next step is architecture exploration and fixture-only trace/replay;
- production service split is not yet justified.

Reject when:

- module needs its own database tables, workers, metrics and replay APIs;
- module starts handling operator queues or compensation state;
- multiple services need to call it as a real runtime boundary.

## 8. Recommended Promotion Order

Recommended order:

1. Stay in Option C for the next exploration step.
2. Build fixture-only run trace / replay and ContextPackage experiments.
3. If experiments show durable run/step/checkpoint ownership is needed, write
   an ADR for Option B.
4. Use Option A only for small baseline extensions that do not introduce a
   durable runtime.

Rationale:

- Option C is safest for the current Agent Exploration Mode because it avoids
  premature production service promotion.
- Option B is the likely long-term clean boundary if Candidate B is promoted.
- Option A is acceptable for minor proposal-path improvements but should not
  become the default for runtime/harness growth.

## 9. Anti-Patterns

Avoid these designs:

- Agent Runtime stores workflow approval decisions locally.
- workflow-service stores prompt, EvidencePack body, planner state or model
  output.
- action-executor accepts model-generated tool calls without proposal /
  approval / prepare verification.
- Python worker decides final proposal status or memory ACTIVE status.
- provider failure redrive restores old raw input or old provider output.
- multi-agent handoff skips primary-agent responsibility.
- `agent-service` owns every new state because it was the first Agent service.
- workflow-service becomes the universal executor for all long tasks.

## 10. Next Suggested Experiment

Do not start runtime prototype before main integration review. If accepted, the
next safe experiment is a fixture-only trace model:

```text
ai/experiments/agent-runtime-trace/
  fixtures/
    read_only_qa_run.json
    approval_wait_run.json
    provider_timeout_redrive_run.json
  README.md
```

The experiment should model refs and ownership only. It must not define
production proto, migrations, Kafka schema or service startup path.

## 11. Handoff Summary

Decision draft:

- Agent Runtime owns cognitive run state.
- workflow-service owns human/operational long-wait state.
- agent-service remains proposal / verification baseline until runtime
  promotion is justified.
- mcp-gateway owns prepare / policy precheck.
- action-executor owns approved execution and provider failure projection.
- audit-service owns append-only low-sensitive audit.
- Python AI Worker owns candidates only.

Recommended next step:

- Main integration should review this matrix before any runtime prototype.
- If accepted, proceed with fixture-only run trace design, still without
  production contracts.
