# NexusIM Agent Plane Redesign Exploration

Date: 2026-07-01
Mode: Agent Exploration Mode
Status: Draft for main integration review, not ADR / SDD / proto contract.

## 1. Scope

This document takes over the prior Agent Plane discussion from thread
`019f1d05-9b10-7fc0-9bd6-1ce19ea06d0b` and reworks it against the current
NexusIM repository state.

The current IM / search / memory / retrieval / RAG / workflow / audit design is
used only as reference. This draft does not freeze a proto, OpenAPI contract,
Kafka schema, EvidencePack shape, memory event schema, workflow taxonomy, skill
taxonomy, MCP contract or final agent type list.

## 2. Problem Definition

NexusIM should not treat Agent as a chatbot attached to IM. The target problem
is a controlled enterprise collaboration plane where human users, IM facts,
retrieval evidence, long-term group memory, tools and approval workflows can
coordinate without letting model output become an ungoverned business fact.

The hard parts are:

- preserving IM hot-path reliability while Agent work is asynchronous;
- grounding answers and proposals in permission-filtered EvidencePack;
- separating memory candidates, reviewed group memory and personal profile
  aggregates;
- allowing tool use without direct business mutation by the model;
- keeping Python workers candidate-only;
- making every agent run replayable, auditable and evaluable;
- avoiding a single frozen "agent type" too early.

## 3. Design Inputs

### 3.1 Prior Thread Takeover

The prior thread concluded that the Agent layer should be a full Agent Plane:

- Agent Gateway converts IM events into agent runs.
- Agent Control Plane owns agent identity, release channels and policy profile.
- Runtime / Harness owns run state, checkpoint, retry, pause, resume and budget.
- Context Plane owns RAG, memory, compaction and citations.
- Tool Plane owns MCP / tool registry / sandbox / action ledger / approval.
- Skill Plane owns versioned domain capability packages.
- Policy, approval, eval and replay are first-class, not append-only logs.

That result is useful, but it was still too close to a target architecture. In
this exploration we keep the principles and reopen the concrete service split,
taxonomy and contract shapes.

### 3.2 Current Repository Constraints

Accepted boundaries already present in the repository:

- `retrieval-gateway` is the AI read boundary and returns EvidencePack.
- `rag-service`, `summary-service` and `agent-service` must consume EvidencePack.
- `memory-service` owns structured memory, graph edges and profile aggregates.
- `agent-service` first path is read-only / proposal-only.
- `mcp-gateway` currently prepares and audits tool intent; it does not execute.
- `action-executor` executes only low-risk allowlist or approved actions through
  public business APIs.
- Python AI Worker is candidate-only; Go validates, authorizes, audits and
  persists or rejects.
- `ai-eval-service` records low-sensitive eval run metadata; it is not an online
  decision service.

These are strong safety constraints, but they do not require a fixed agent
architecture. The redesign should keep the safety invariants while allowing
multiple implementation shapes.

### 3.3 External Signals

Recent agent literature and platform reports push the same direction:

- tau-bench and ToolSandbox show that realistic agents fail on domain policy,
  tool state and multi-turn user interaction, so the runtime must evaluate rule
  following and state transitions, not just answer quality.
- Agent-Diff frames enterprise API agents as code-executing, environment-bound
  systems where tool access, framework choice and sandbox realism materially
  change results.
- STATE-Bench emphasizes stateful task memory: memory should be evaluated by
  whether experience improves later enterprise tasks, not by retrieval recall
  alone.
- MCP and A2A point to two distinct integration modes: tools/data via MCP-like
  gateways, and peer agents via A2A-like communication. NexusIM should not
  collapse both into one "tool call" abstraction.

## 4. Core Invariants

These should survive any candidate architecture:

1. IM services remain the business fact source. Agent does not enter message
   delivery hot paths.
2. Agent reads organization context only through retrieval / EvidencePack or
   public service APIs with policy checks.
3. Agent writes are never model-direct writes. The write chain is proposal,
   approval when required, executor, public API, audit.
4. Memory is not a summary cache. Long-term memory must be source-backed,
   scoped, versioned, reviewed and reversible.
5. Python workers produce candidates only. They cannot own final state,
   approval, durable memory, final proposal status or execution status.
6. Tool and MCP integration must be schema-checked, policy-checked, idempotent
   and audited before execution.
7. Eval gates must classify failures by retrieval, reasoning, temporal version,
   attribution, permission, tool policy, memory pollution and execution safety.
8. Any fake / fixture / mock path must remain in explicit experiments or tests
   and must not become production fallback.

## 5. Candidate Architectures

### Candidate A: Conservative Evidence-First Agent

Shape:

```text
IM event / explicit request
-> agent-service run
-> retrieval-gateway EvidencePack
-> deterministic or model-backed planner candidate
-> proposal / answer
-> optional approval
-> action-executor
```

Strengths:

- Fits the existing service boundary with minimal new moving parts.
- Best for read-only assistants, grounded Q&A, meeting summaries and
  proposal-only business actions.
- Easy to gate with current RAG-Agent service-stack evals.

Weaknesses:

- Runtime semantics remain thin: hard to support long-running tasks,
  checkpoint, pause/resume, subagents or rich replay.
- Tool use is mostly a proposal pipeline, not a general agent run harness.
- Can become a large `agent-service` if more workflows are added directly.

Best use:

- Keep as the near-term baseline and regression reference.

### Candidate B: Agent Runtime / Harness Plane

Shape:

```text
Agent Gateway
-> Run Harness
   -> context builder
   -> planner / critic / verifier candidates
   -> retrieval / memory / tool prepare
   -> checkpoint / approval wait / resume
   -> proposal or answer
-> executor / audit / eval
```

New logical objects:

- `AgentRun`
- `AgentStep`
- `ContextPackage`
- `ToolIntent`
- `Checkpoint`
- `ApprovalWait`
- `RunBudget`
- `ReplayBundle`

Strengths:

- Natural place for long-running work, replay, checkpoints, budgets, failure
  recovery and subagent isolation.
- Avoids pushing orchestration complexity into RAG, memory or action-executor.
- Better match for enterprise agent benchmarks that evaluate stateful tasks.

Weaknesses:

- Requires a new service or a major `agent-service` promotion.
- Needs careful handoff to workflow-service so there are not two competing long
  transaction engines.
- More contracts to evaluate before implementation.

Best use:

- Recommended target direction once exploration graduates to ADR.

### Candidate C: Workflow-Centric Agent

Shape:

```text
agent-service creates intent
-> workflow-service owns long state machine
-> activities call retrieval / planner / approval / executor
-> audit-service records lineage
```

Strengths:

- Reuses the existing long workflow / approval direction.
- Good for human approval, callback wait, compensation and operator workflows.
- Reduces number of new state-machine services.

Weaknesses:

- Workflow-service can become too business-general and model-specific at once.
- Fine-grained model call replay, context packages and token budgets are awkward
  if modeled as generic workflow activities only.
- Requires strict separation between business workflow state and agent cognitive
  state.

Best use:

- High-risk actions, repair workflows, provider redrive and admin handoffs.
  Not ideal as the only agent runtime.

### Candidate D: Multi-Agent / A2A-Oriented Plane

Shape:

```text
Primary Agent
-> Specialist Agents
   -> retrieval specialist
   -> memory specialist
   -> tool specialist
   -> safety reviewer
-> primary final response / proposal
```

Strengths:

- Good for complex enterprise work where different agents own different tools or
  knowledge domains.
- Maps well to future A2A-like peer agent interop.
- Keeps primary context smaller by delegating isolated tasks.

Weaknesses:

- Higher cost and harder debugging.
- Responsibility can become unclear unless primary agent owns final answer.
- Premature before single-agent run replay and evidence discipline are strong.

Best use:

- Future specialist isolation after Candidate B exists.

## 6. Recommended Direction

Use a two-speed design:

1. Preserve Candidate A as the current safe first path.
2. Explore Candidate B as the next architecture, with Candidate C used for
   approval / compensation / provider replay, and Candidate D postponed until
   run replay, policy and eval are strong enough.

This gives NexusIM a practical route:

```text
Current first path:
  EvidencePack -> grounded answer / proposal -> approval -> executor

Exploration target:
  Agent Gateway -> Agent Runtime Harness -> Context / Tool / Memory / Eval
```

The key decision is to split "agent cognitive runtime" from "business workflow":

- Agent Runtime owns planning, context packages, model/provider calls,
  tool-intent preparation, checkpoints, replay and budgets.
- Workflow-service owns human approval waits, admin/repair workflows,
  compensation and long external callbacks.
- Action-executor owns final mutation attempts and result projections.

## 7. Proposed Plane Model

### 7.1 Agent Gateway

Responsibilities:

- Convert `@agent`, private agent chats, scheduled tasks, webhook events and card
  button events into `AgentRun` requests.
- Normalize actor identity: user, device, tenant, conversation and delegated
  agent identity.
- Decide whether the trigger is read-only, proposal-oriented or workflow-bound.
- Emit run accepted / rejected events without blocking IM delivery.

Open questions:

- Whether Agent Gateway is a module inside api-gateway, a new service, or a
  thin facade in agent-service.
- Whether group resident agents subscribe through normal message events or a
  separate low-volume agent trigger topic.

### 7.2 Agent Runtime / Harness

Responsibilities:

- Own `AgentRun` and `AgentStep` state.
- Build `ContextPackage` from EvidencePack, memory summaries and run-local
  scratch state.
- Enforce time, token, tool-call and cost budgets.
- Create checkpoints before model calls, tool prepare and approval waits.
- Support cancel, retry, pause, resume and replay.
- Call Python workers only for candidates: planner, critic, rerank, memory
  extraction, verifier or eval.

Suggested states:

```text
CREATED
ACCEPTED
CONTEXT_BUILDING
PLANNING
WAITING_APPROVAL
TOOL_PREPARING
EXECUTION_REQUESTED
OBSERVING_RESULT
WRITING_MEMORY_CANDIDATE
COMPLETED
FAILED
CANCELLED
EXPIRED
```

This is not a schema proposal; it is a state vocabulary for evaluation.

### 7.3 Context Plane

Responsibilities:

- Keep EvidencePack as the hard read boundary.
- Add `ContextPackage` as a run-local assembly artifact that can include:
  EvidencePack refs, selected snippets, source coverage, conflict markers,
  memory graph hints, tool results and prior run checkpoints.
- Keep compaction as a runtime operation, not a free-form model summary.
- Distinguish current facts from historical facts.

Important distinction:

- EvidencePack is an audited retrieval artifact.
- ContextPackage is a runtime prompt/input package derived from audited evidence.
- MemoryCandidate is a proposed durable memory change that still needs review.

### 7.4 Memory Plane

Recommended memory layers:

- conversation memory: source-backed decisions, tasks and status;
- project memory: cross-conversation facts with explicit source chain;
- personal preference: user-confirmed or multi-evidence profile aggregate;
- run memory: ephemeral task state and checkpoints;
- policy memory: not real memory, only policy-service facts or admin config.

Memory admission:

```text
candidate
-> source visibility check
-> classification
-> dedupe
-> conflict / supersedes check
-> profile overgeneralization check
-> review or auto-admit policy
-> ACTIVE / REJECTED / NEEDS_REVIEW
```

Do not let the planner write ACTIVE memory directly.

### 7.5 Tool / MCP Plane

Keep two separate integration modes:

- Tool calls: deterministic actions on registered business tools through
  skill-registry, mcp-gateway and action-executor.
- Peer agents: future A2A-like collaboration with another agent that has its own
  identity, policy and audit lineage.

Tool exposure should be progressive:

```text
L0: tool names and short descriptions
L1: search/select candidate tools
L2: load selected schemas
L3: prepare tool intent through mcp-gateway
L4: approved execute through action-executor
```

MCP servers should not be trusted as policy engines. They are external
integration endpoints behind NexusIM policy, schema validation, idempotency and
audit.

### 7.6 Skill Plane

Do not freeze agent taxonomy yet. Freeze only the idea that skills are versioned
capability packages.

Possible skill manifest dimensions:

- owner and release channel;
- intended tasks and denied tasks;
- required evidence sources;
- allowed tool intents;
- risk tier and approval policy;
- output contract;
- eval suite binding;
- rollout / shadow mode policy.

Candidate initial skills:

- conversation evidence assistant;
- group memory reviewer;
- project decision assistant;
- support triage assistant;
- note/proposal assistant;
- policy explanation assistant.

These names are exploratory.

### 7.7 Eval Plane

The eval plane should become the promotion gate for Agent architecture changes.

Required eval families:

- evidence grounding: answer / proposal only cites visible evidence;
- source coverage: missing retrieval lane is surfaced instead of ignored;
- temporal version: active vs superseded vs historical answer;
- memory admission: no single-message profile overgeneralization;
- group memory ambiguity: asker, speaker, audience and group scope preserved;
- tool policy: no prepare / execute bypass;
- approval: high-risk action cannot execute without approved proposal;
- provider failure: unsafe output, timeout and malformed candidate fail closed;
- replay: same run inputs reconstruct same evidence and decision lineage.

## 8. Service / Middleware Implications

No immediate service promotion is proposed in this draft. If Candidate B is
promoted, the likely options are:

1. Extend `agent-service` into runtime/harness with new internal modules.
2. Add `agent-runtime-service` and keep `agent-service` as public proposal API.
3. Treat `workflow-service` as long-wait orchestration while adding only a small
   `agent-runner` module for cognitive runtime.

Likely middleware needs if promoted:

- PostgreSQL for run / step / checkpoint metadata.
- Kafka for low-sensitive agent events and audit handoff.
- Object storage only if replay bundles or large low-sensitive artifacts outgrow
  PostgreSQL refs.
- No new vector store or graph database requirement from agent runtime itself.

## 9. Risks

- Over-splitting too early: many AI services already exist as first paths.
- Under-splitting too late: `agent-service` can become a monolith if it absorbs
  runtime, workflow, tool, memory and eval logic.
- MCP over-trust: external tool servers must not carry NexusIM authorization.
- Memory pollution: model-generated memory without source-backed admission will
  corrupt future answers.
- Eval theater: pass/fail without failure taxonomy will not tell whether the bug
  is retrieval, model reasoning, policy, memory or execution.
- Workflow collision: agent run state and business workflow state must have a
  clean ownership split.

## 10. Next Experiments

1. Agent run trace model prototype in `ai/experiments/agent-runtime-trace/`:
   fixture-only JSON traces for run, step, EvidencePack refs, tool intent,
   approval wait and replay bundle.
2. ContextPackage builder prototype:
   consume a small fake EvidencePack fixture and produce a prompt-safe,
   citation-preserving context package with conflict and source coverage fields.
3. Memory admission eval expansion:
   add cases for group ambiguity, superseded project decision and personal
   profile overgeneralization.
4. Tool prepare vs execute replay:
   model the lineage from skill contract to mcp prepare audit to proposal to
   executor result, without adding new production schemas. Initial fixture
   evidence is now recorded in
   `docs/research/agent-tool-mcp-fixture-evidence-20260702.md`.
5. Runtime vs workflow ownership matrix:
   decide which states belong to Agent Runtime and which belong to
   workflow-service before any ADR.

## 11. Handoff Summary for Main Integration

Recommendation:

- Keep the existing EvidencePack -> proposal / approval / executor first path as
  the safety baseline.
- Explore a dedicated Agent Runtime / Harness plane for long-running,
  checkpointed, replayable and evaluable agent work.
- Use workflow-service for human approval and repair/compensation waits, not as
  the only cognitive runtime.
- Keep Python workers candidate-only.
- Keep MCP/tool servers behind NexusIM policy, skill, prepare, approval,
  executor and audit boundaries.

Not requested in this draft:

- no proto;
- no OpenAPI;
- no Kafka schema;
- no migration;
- no production service directory;
- no fixed agent taxonomy.

Current missing routing file:

- `docs/runbook/codex-sessions.md` was referenced by delegation instructions but
  is not present in this workspace at the time of this draft.

## 12. Source Notes

Local repository inputs:

- `docs/architecture/target-architecture-ai.md`
- `docs/architecture/adr/ADR-035-ai-foundation-service-boundary.md`
- `docs/architecture/adr/ADR-036-python-ai-worker-boundary.md`
- `docs/sdd/agent-service.md`
- `docs/sdd/retrieval-gateway.md`
- `docs/sdd/memory-service.md`
- `docs/sdd/rag-service.md`
- `docs/sdd/skill-registry.md`
- `docs/sdd/mcp-gateway.md`
- `docs/sdd/action-executor.md`
- `docs/runbook/loadtest/ai-eval-service/loadtest-report-20260625-rag-agent-business-mutation-execute-gate.md`

External references checked on 2026-07-01:

- tau-bench: A Benchmark for Tool-Agent-User Interaction in Real-World Domains.
- ToolSandbox: A Stateful, Conversational, Interactive Evaluation Benchmark for
  LLM Tool Use.
- Agent-Diff: Benchmarking LLM Agents on Enterprise API Tasks via Code
  Execution.
- Microsoft STATE-Bench announcement.
- Anthropic Model Context Protocol announcement and specification.
- Google Agent2Agent announcement and A2A project notes.
