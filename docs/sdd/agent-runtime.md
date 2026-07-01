# Agent Runtime / Harness SDD

Date: 2026-07-01
Status: Detailed SDD draft for Agent Exploration Mode. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory or
runtime implementation.

## 1. Goal

Define the NexusIM Agent Runtime / Harness as the owner of cognitive run state:
context building, planning, model calls, tool intent, memory candidates,
checkpoint refs, replay refs, budget and cancellation.

The Runtime must make Agent work reproducible and reviewable without becoming a
second workflow engine or a business execution service.

## 2. Non-Goals

- Do not create `agent-runtime-service` yet.
- Do not define production API, event or database schema.
- Do not own approval queues, workflow timers, execution results or ACTIVE
  memory.
- Do not use fake/mock/fixture results as production fallback.
- Do not run against real NexusIM IM data in the first evaluation stage.

## 3. Component Responsibilities

Agent Runtime / Harness owns:

- `AgentRun` lifecycle for one cognitive task.
- `AgentStep` trace and failure class.
- ContextPackage refs derived from EvidencePack.
- Planner, critic, verifier and candidate output refs.
- Model/provider call metadata through model-gateway.
- ToolIntent before MCP prepare.
- MemoryCandidate refs before memory-service review.
- Safe checkpoints before external wait or side-effect boundary.
- ReplayBundle refs for low-sensitive debug/eval.
- Run budgets: token, wall-clock, steps, tool prepares and cost.
- Cancel/resume intent while the run is in runtime-owned state.

It coordinates with:

- agent-service for public facade and proposal baseline;
- retrieval-gateway / RAG for EvidencePack and grounded answer verification;
- model-gateway for model calls;
- mcp-gateway for tool prepare;
- memory-service for candidate admission;
- workflow-service for long-wait approval and repair workflows;
- action-executor for approved side-effect execution;
- audit-service and ai-eval-service for trace, replay and quality records.

## 4. State Ownership

| Runtime State | Description | Retention Direction |
| --- | --- | --- |
| Run metadata | agent, actor, tenant, skill, request, risk, budget refs | Low-sensitive durable summary |
| Step refs | ordered cognitive steps and outcome refs | Low-sensitive durable summary |
| Planner state | model-facing transient reasoning artifacts | Ephemeral or hash/ref only |
| Context refs | EvidencePack and ContextPackage refs | Durable refs, not raw bodies |
| Candidate refs | answer/proposal/memory/tool candidate hashes and metadata | Durable refs where useful for replay |
| Checkpoint refs | safe resumption point before workflow/executor boundary | Durable until run/replay TTL |
| Failure class | normalized failure reason | Durable for eval and ops |
| ReplayBundle ref | low-sensitive reconstruction artifact | Durable under eval/audit retention |

Runtime cannot own:

- workflow approval decision or timer;
- operator queue state;
- action-executor execution attempt or result truth;
- business service final state;
- ACTIVE memory state;
- provider secret;
- raw prompt archive beyond retention policy;
- audit archive of record.

## 5. Conceptual Run Model

These names are design vocabulary, not schema:

```text
CREATED
ACCEPTED
POLICY_CHECKING
CONTEXT_BUILDING
PLANNING
VERIFYING
TOOL_PREPARING
WAITING_WORKFLOW
EXECUTION_REQUESTED
OBSERVING_RESULT
MEMORY_CANDIDATE_CREATED
COMPLETED
FAILED
CANCELLED
EXPIRED
```

Allowed transitions must respect owner boundaries:

- Runtime can move into `WAITING_WORKFLOW` only after creating a checkpoint and
  workflow request ref.
- Runtime can leave `WAITING_WORKFLOW` only after receiving a workflow decision
  ref.
- Runtime can request execution only through approved proposal and prepared
  tool lineage.
- Runtime replay cannot re-execute external side effects.

## 6. Step Types

| Step | Owner | Required Metadata |
| --- | --- | --- |
| intake | Gateway / agent-service | actor ref, tenant ref, agent definition ref, risk tier |
| policy_check | policy boundary | decision ref, deny reason class, budget snapshot |
| context_build | Runtime + retrieval/RAG | EvidencePack ref, ContextPackage ref, coverage, conflicts |
| memory_retrieve | Runtime + memory-service | memory refs, scope, version, source lineage |
| model_call | Runtime + model-gateway | model ref, provider ref, usage, latency, candidate hash |
| verify | Runtime / RAG verifier | citation result, abstain reason, unsafe output reason |
| tool_intent | Runtime | tool id candidate, args hash, skill scope |
| tool_prepare | mcp-gateway | prepared ref, policy ref, risk, dry-run/precheck outcome |
| proposal | agent-service / Runtime facade | proposal ref, evidence refs, approval requirement |
| workflow_wait | workflow-service | workflow ref, timeout policy, decision ref |
| execution_request | action-executor | execution ref, idempotency ref, prepared lineage |
| memory_candidate | Runtime + memory-service | candidate ref, source refs, admission reason |
| handoff | Runtime | delegate ref, scoped context, budget, deadline |
| finalize | Runtime / agent-service | status, failure class, audit ref, replay ref |

## 7. Key Flows

### 7.1 Read-Only QA

```text
intake
-> policy_check
-> context_build
-> optional memory_retrieve
-> model_call
-> verify citations and permission
-> finalize answer or abstain
```

No workflow or executor is involved. If evidence is insufficient, the final
state is abstain or clarification, not hallucinated answer.

### 7.2 Group Memory Admission

```text
context_build over group source refs
-> model or deterministic extraction candidate
-> memory_candidate
-> memory-service source/scope/conflict review
-> runtime records candidate ref only
```

Runtime does not make memory ACTIVE.

### 7.3 Approval-Required Business Action

```text
context_build
-> proposal draft
-> tool_intent
-> tool_prepare
-> workflow request
-> pause with checkpoint
-> workflow decision callback
-> action-executor request
-> observe execution ref
-> finalize
```

Runtime must not maintain a private long polling loop for approval.

### 7.4 Long Approval Wait

Runtime creates a checkpoint and stops. workflow-service owns wait, timeout,
escalation and decision. Resume uses the checkpoint and workflow decision ref.
If proposal content must change, the old workflow is closed and a new proof path
is created.

### 7.5 Tool Provider Timeout

If timeout occurs before tool prepare returns, Runtime can retry within budget
or fail with `PROVIDER_TIMEOUT`. If timeout occurs during approved execution,
action-executor owns provider failure projection and redrive.

### 7.6 Repair / Redrive

Runtime may repair cognitive artifacts such as invalid JSON, missing citations
or unsafe candidate format. It cannot repair execution results directly. Any
business redrive requires fresh proposal, prepare, approval and executor path.

### 7.7 Cancel / Resume / Replay

- Cancel stops future runtime-owned steps and propagates cancellation to
  in-flight model/prepare calls when supported.
- Resume loads a safe current checkpoint version and a decision/callback ref.
  Stale checkpoint versions must be detected as drift, not silently reused.
- Duplicate or racing workflow wakeups are deduped before resume. Runtime records
  wakeup refs and race refs, but workflow-service still owns timers and wait
  state.
- Replay reconstructs cognitive lineage from refs, hashes and versions. It does
  not call external providers or execute business actions.

### 7.8 Multi-Agent Handoff

Runtime can create bounded delegation:

```text
primary run
-> delegation request with scope, budget, deadline
-> specialist candidate artifact
-> verifier step
-> primary integrates or rejects
```

Specialist output is untrusted candidate material until verified.

## 8. Failure Semantics

| Failure | Runtime Behavior |
| --- | --- |
| Policy deny | Stop and return `POLICY_DENIED`; no retry |
| Evidence missing | Return `INSUFFICIENT_EVIDENCE` or clarification |
| Citation failed | Reject candidate and repair if budget allows |
| Model timeout | Bounded retry; else `PROVIDER_TIMEOUT` |
| Unsafe model output | Reject or repair; never pass to tool/approval |
| Tool prepare denied | Stop with tool/policy failure class |
| Workflow timeout | Finalize as approval timeout unless workflow asks for redrive |
| Execution failed | Record execution ref/failure; executor owns repair |
| Memory candidate rejected | Record rejection; do not retry as ACTIVE memory |

## 9. Security Boundary

Runtime treats these as untrusted inputs:

- user prompt;
- retrieved content;
- memory text;
- tool description;
- MCP resource;
- tool output;
- peer-agent response;
- model output.

Security requirements:

- instruction hierarchy is applied before model call;
- ContextPackage only contains permission-filtered material;
- model output is validated before tool prepare or user-visible response;
- high-risk actions require proposal, prepare, workflow approval and executor;
- provider secrets are not visible to Runtime;
- low-sensitive refs/hashes are preferred over raw payload retention.

## 10. Eval / Replay

Runtime promotion requires offline tests for:

- read-only QA citation and abstain;
- group memory candidate extraction without ACTIVE admission;
- approval pause/resume;
- tool prepare denial and timeout;
- unsafe tool output quarantine;
- cancellation before and during workflow wait;
- checkpoint version drift detection before resume;
- workflow wakeup race dedupe before runtime-owned resume;
- replay without side-effect execution;
- ReplayBundle lineage completeness across context, model candidate, checkpoint,
  workflow decision and audit refs;
- bounded multi-agent handoff.

ReplayBundle should include:

- run and step refs;
- EvidencePack / ContextPackage refs;
- model/provider version metadata;
- candidate hashes;
- prepared tool refs;
- workflow refs;
- execution refs;
- memory candidate refs;
- checkpoint version and workflow wakeup refs where resume is involved;
- lineage refs that connect context, model candidate, checkpoint, workflow and
  audit records;
- failure class.

It should not require raw prompt, raw provider body, secret or private service
row content for normal replay.

## 11. Observability / Audit

Runtime emits low-sensitive trace events:

- run accepted/rejected;
- policy decision;
- context build coverage;
- model call metadata;
- verifier result;
- tool prepare request/result;
- workflow wait start/resume;
- execution request/result ref;
- memory candidate submitted;
- failure class and replay availability.

Operators should be able to aggregate by agent, skill, tenant, model, tool,
failure class, latency, cost and replay completeness.

## 12. Risks / Rejection Conditions

Reject Runtime implementation if:

- it stores approval timers or operator queues;
- it writes ACTIVE memory;
- it executes tools directly;
- it requires raw prompt archive for replay;
- it cannot abstain on insufficient evidence;
- it cannot cancel or replay safely;
- it uses fake/mock results as production fallback;
- it makes Python AI Worker a state owner.

## 13. Promotion Conditions

Stay as offline harness or internal module until:

- public dataset and synthetic fixture suites pass;
- run/step/checkpoint/replay state pressure is proven;
- workflow-service ownership remains clean;
- observability and audit lineage can explain failures;
- security fixtures for tool/MCP and memory pollution pass.

Only then write an ADR for `agent-runtime-service` or a durable runtime module.

## 14. References

- `docs/research/agent-runtime-workflow-ownership-20260701.md`
- `docs/research/agent-current-design-review-20260701.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`
- `docs/sdd/agent-platform.md`
- `docs/sdd/agent-service.md`
- `docs/sdd/workflow-service.md`
- `docs/sdd/action-executor.md`
