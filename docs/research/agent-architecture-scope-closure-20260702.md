# Agent Architecture Scope Closure

Date: 2026-07-02

Status: scope-closure decision for Agent Lab. This is not an accepted ADR,
proto, OpenAPI, Kafka schema, migration, service directory, release pipeline or
runtime implementation.

## Verdict

Agent architecture is closed for broad expansion.

The current package is sufficient for the project goal: interview-ready
explanation plus moderate production-grade awareness. The Agent Lab should stop
adding new capability planes, object families, taxonomies, service candidates or
governance layers unless a reviewer returns a concrete P0/P1 gap.

Future work should move from architecture expansion to a small runnable
backend-isolated Agent demo and focused hardening.

## What Is Now Considered Enough

The architecture already covers the required Agent-layer surfaces:

| Surface | Closure Basis |
| --- | --- |
| Runtime / Harness | L1 and L2 review material define cognitive runtime, checkpoints, cancel/resume/replay and workflow boundary |
| Context / EvidencePack / RAG | L1 and L2 review material define AI read boundary, source visibility, denied lanes, citation verifier and taint |
| Memory Admission | L1 and L2 review material define candidate-only Python, ACTIVE memory owner, revocation and retrieval eligibility |
| Tool / MCP | L1 and L2 review material define untrusted provider, capability lease, provider attestation, prepare and executor handoff |
| Workflow / Action handoff | Runtime and Tool L2s define durable wait owner, approval boundary and executor-owned side effects |
| Eval / Replay | L1 and L2 review material define EvalReport, ReplayBundle, baseline approval and failure-class blocking |
| AgentOps / Governance | L1 and L2 review material define release gate, kill switch, rollback, canary/shadow and operator action refs |
| Multi-agent / A2A boundary | Fixture evidence defines bounded delegation and candidate-only peer outputs without freezing production A2A |
| Operator / Security / Versioning | Fixture evidence defines operator surfaces, compatibility refs, preservation refs and Python owner limits |
| Production roadmap | `docs/architecture/agent-plane-initial-design.md` records the 12 production integration gap categories |

This is enough to explain the architecture in an interview and enough to guide a
minimal isolated implementation. It is not enough to authorize production
integration.

## Closure Rules

Do not add new broad architecture documents unless one of these is true:

- main integration or a domain owner returns a P0/P1 finding;
- an existing document contradicts another current fact source;
- a focused implementation slice discovers a boundary bug that cannot be fixed
  locally;
- the user explicitly asks to reopen production integration design.

Do not add new Agent planes, service candidates, taxonomies, object catalogs or
benchmark families for general completeness.

Do not continue literature or open-source exploration unless it answers a
specific unresolved P0/P1 question.

Keep the 12 production integration gap categories as a roadmap, not as an
immediate implementation backlog.

## Allowed Next Work

The next useful work is an interview-ready backend-isolated Agent demo:

```text
synthetic IM-like request
-> EvidencePack / ContextPackage refs
-> MemoryCandidate refs
-> ToolIntent / proposal refs
-> approval/action decision refs as fixture refs
-> EvalReport
-> ReplayBundle
```

Allowed code paths:

- `ai/python/nexusim_ai_eval/`
- `ai/python/nexusim_ai_memory/`
- `ai/python/nexusim_ai_common/`
- `ai/python/fixtures/agent_eval/`
- `ai/python/scripts/run_agent_eval_*.py`
- `ai/python/tests/test_agent_eval_*.py`

Allowed documents:

- concise runbook updates;
- interview explanation document;
- focused demo README;
- targeted SDD note only when implementation changes an established boundary.

## Explicitly Disallowed For This Goal

Do not start:

- production proto, OpenAPI, Kafka schema or migration work;
- production service directory or startup path;
- real PostgreSQL, Kafka, Redis, OpenSearch, workflow-service, memory-service,
  action-executor, audit-service, MCP provider or model-provider integration;
- production release pipeline, admin console or control-plane API;
- new Agent taxonomy, skill taxonomy, tool taxonomy or memory event shape;
- another broad research phase;
- another production object catalog expansion;
- another benchmark survey unless tied to a failing gate.

## Interview Completion Target

The interview-ready target is:

- one command runs the isolated demo;
- the output shows low-sensitive refs for context, memory, tool intent,
  proposal, eval and replay;
- the README explains why the Agent does not directly read private tables or
  execute side effects;
- tests prove the boundary: Python remains candidate-only, MCP/tool output is
  untrusted, memory admission is governed, and eval/replay can block unsafe
  promotion;
- the production roadmap is stated as future work, not as hidden unfinished
  implementation.

## Production Readiness Target

Production readiness remains a future phase and still requires:

- accepted ADRs;
- owner-approved contracts;
- real-service smoke;
- production operator UX;
- security review;
- SLO, incident, rollback and deployment evidence.

Meeting the interview target must not be described as production completion.

## Stop Criteria

Stop architecture work and switch to implementation when:

- all six L2 designs have main integration review material acceptance;
- the 12 production gap categories are documented;
- this scope-closure document is present;
- no open P0/P1 exists inside Agent Lab fixture scope.

At that point, additional architecture writing is lower value than a runnable
demo and focused test hardening.
