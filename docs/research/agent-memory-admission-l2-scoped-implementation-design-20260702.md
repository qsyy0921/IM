# Agent Memory Admission L2 Scoped Implementation Design

Date: 2026-07-02

Status: L2 scoped implementation design draft for the Memory Admission
boundary. This is not an accepted production ADR, proto, OpenAPI, Kafka schema,
migration, service directory, startup path, memory event schema,
MemoryCandidate schema, MemoryClaim schema, retrieval contract or runtime
implementation.

## Verdict

Conditionally passed as the fourth L2 scoped design draft.

Rejected for implementation until main integration, memory-service,
retrieval-gateway, Agent Runtime, workflow-service, policy/security,
audit/legal, product/operator and SRE/incident owners review the design and
approve the required L3 real-service smoke plan.

Reason: L1 accepted the Memory Admission candidate for reviewability only. The
L2 question is narrower: how can a future controlled implementation prove that
personal, group, project, procedural and policy-like memories are source-backed,
scoped, revocable, replayable and operator-governed without turning model
summaries into durable truth or source-service fallback.

## Scope

The scoped slice is Memory Admission and retrieval eligibility.

It covers:

- MemoryCandidate, MemoryClaim, MemoryAdmissionDecision, MemoryReviewTask,
  MemoryScope, MemoryVersion, MemoryRevocationLedger, MemorySupersessionChain,
  GroupConsensus, KnowledgeState and RelationMemory ownership;
- personal, group, project, procedural and policy-like admission thresholds;
- candidate-only Python worker output;
- memory-service ownership of ACTIVE / REJECTED / NEEDS_REVIEW / SUPERSEDED /
  REVOKED / EXPIRED / QUARANTINED states;
- workflow-service ownership of durable human review wait only;
- revocation, deletion, expiry and dependency invalidation;
- memory retrieval eligibility and memory-use audit refs;
- operator review, correction, revocation and forget surfaces;
- L3 real-service smoke requirements.

It does not cover:

- final MemoryCandidate, MemoryClaim or memory event field shape;
- production memory-service API design;
- production retrieval-gateway API changes;
- production review workflow schema or queue;
- production database tables, indexes or migrations;
- real NexusIM IM data;
- production retention duration or legal deletion policy;
- large public memory benchmark thresholds;
- memory UI copy or final product review text.

## Boundary Thesis

Memory is governed state. It is not prompt cache, vector cache, model scratchpad
or fallback source truth.

```text
Python AI Worker may produce memory candidates:
  candidate text, class, confidence, source refs, conflict hints, dedupe hints
  and uncertainty refs.

memory-service owns durable memory truth:
  admission, rejection, review outcome, ACTIVE state, scope, version,
  supersession, revocation, expiry, retrieval eligibility and audit refs.

workflow-service owns durable review waits only:
  human review timer, timeout, escalation and reviewer callback refs.
```

Runtime may submit candidates and later consume retrieval-eligible memory refs.
Runtime, Python and workflow-service cannot make memory ACTIVE.

## Proposed Ownership

| Object / State | Owner | Cannot Be Owned By |
| --- | --- | --- |
| MemoryCandidate submission ref | memory-service after submission | Python worker as final owner |
| Candidate text/hash/classification | Python can propose; memory-service owns submitted record | Runtime as ACTIVE truth |
| MemoryClaim | memory-service | Python worker, workflow-service |
| MemoryAdmissionDecision | memory-service | Python worker, Runtime, workflow-service |
| ACTIVE memory | memory-service | Python worker, Runtime, workflow-service |
| MemoryReviewTask | memory-service plus workflow-service wait refs | workflow-service as ACTIVE owner |
| MemoryScope | memory-service plus policy/security owner | model output |
| MemoryVersion | memory-service | Runtime checkpoint |
| MemoryRevocationLedger | memory-service plus audit/legal | Python worker |
| MemorySupersessionChain | memory-service | retrieval-gateway |
| GroupConsensus | memory-service | one user's model summary |
| KnowledgeState | memory-service | Agent Runtime |
| RelationMemory | memory-service | Python worker |
| Retrieval eligibility | memory-service, consumed by retrieval-gateway | Runtime local cache |
| Memory-use audit refs | memory-service / audit-service | Python worker |
| Review wait / timeout | workflow-service | memory-service as durable timer |
| Source truth | owning business/retrieval source | memory-service |
| Retention / deletion policy refs | audit/legal/security owner | fixture code |
| Operator memory controls | product/operator plus memory-service/audit | Python worker |

## Non-Owner State

Python AI Worker must not own:

- ACTIVE memory;
- final admission, rejection or review state;
- retrieval eligibility;
- revocation or deletion decisions;
- operator override;
- audit archive;
- production source truth.

Agent Runtime must not own:

- ACTIVE memory;
- memory review outcome;
- revocation ledger;
- supersession truth;
- retrieval eligibility truth;
- memory as fallback for source-service failure.

workflow-service must not own:

- MemoryClaim truth;
- ACTIVE / REJECTED / REVOKED memory state;
- memory scope or version;
- retrieval eligibility;
- memory-use audit archive.

memory-service must not own:

- original business source truth;
- workflow timers;
- tool execution truth;
- policy authorization truth beyond decision refs;
- raw prompt/provider transcript archive as normal replay material.

## Candidate L2 Flow

```text
AgentRun or source event refs
-> Runtime requests candidate extraction
-> Python produces candidate-only text/class/confidence/source/conflict refs
-> Runtime submits candidate refs to memory-service
-> memory-service verifies source visibility, scope, category and sensitivity
-> memory-service runs dedupe, conflict, supersession and overgeneralization gates
-> memory-service auto-rejects, auto-admits conservatively or creates review task
-> workflow-service owns human review wait when review is required
-> memory-service records ACTIVE / REJECTED / NEEDS_REVIEW / SUPERSEDED / REVOKED / EXPIRED / QUARANTINED
-> retrieval-gateway consumes retrieval-eligible memory refs only
-> audit-service records low-sensitive admission and use lineage
-> ReplayReader explains admission, rejection, revocation and use from refs
```

The L2 flow is a design path only. It does not create production fields,
events, APIs, queues, tables or review workers.

## Memory Category Rules

### Personal Memory

- Requires user scope, source refs, sensitivity refs and either explicit
  confirmation or repeated-evidence threshold.
- Single casual statements should default to review or session memory.
- Personal memory cannot leak into group/project retrieval without explicit
  scope policy.

### Group Memory

- Requires group, speaker, subject, audience and membership-window refs.
- Requires explicit decision, multi-party confirmation or review.
- One user's preference in a group is not a group preference.
- Objections, ambiguity or missing audience refs force NEEDS_REVIEW.

### Project Memory

- Requires project source refs, temporal refs and project scope policy.
- Material changes require supersession or review, not silent overwrite.
- Current source truth overrides stale project memory.
- Superseded project memory may remain audit-visible but not normal retrieval
  eligible.

### Procedural Memory

- Requires SkillPackage or AgentDefinition version binding.
- Skill version migration requires migration/invalidation refs.
- Procedural memory cannot silently modify unrelated skills or policy rules.
- Unsafe or ambiguous procedure candidates require review.

### Policy-Like Memory

- Requires governed policy source refs and policy owner refs.
- Model summaries cannot create policy memory.
- Revoked policy source refs invalidate or quarantine dependent memory.
- Policy-like memories require stricter audit and operator inspection.

### Run And Session Memory

- Run scratch state is Runtime-owned and never automatically ACTIVE.
- Session memory may support short-term continuity under TTL, but is not durable
  fact unless it passes admission.
- Neither run nor session memory can hide retrieval/source failures.

## Admission Gates

Every future admission design must fail closed when any required gate is
missing:

- source refs and visibility refs;
- category and scope refs;
- speaker, audience and membership-window refs for group memory;
- SkillPackage or AgentDefinition version refs for procedural memory;
- governed policy source refs for policy-like memory;
- confidence and calibration refs;
- dedupe and near-duplicate refs;
- conflict and supersession refs;
- sensitivity, PII and redaction refs;
- review policy and timeout refs when review is needed;
- audit and replay-reader refs.

Memory missing source refs must reject. Memory with uncertain scope, conflict,
sensitive content or broad impact must review or reject, not auto-admit.

## Lifecycle And Retrieval Rules

Required conceptual lifecycle states:

- CANDIDATE;
- REJECTED;
- NEEDS_REVIEW;
- ACTIVE;
- SUPERSEDED;
- REVOKED;
- EXPIRED;
- QUARANTINED.

Only ACTIVE memory is normally retrieval-eligible.

SUPERSEDED, REVOKED, EXPIRED and QUARANTINED memory may remain available as
low-sensitive audit/replay refs under retention policy, but must not influence
normal ContextPackage construction.

Retrieval must check:

- memory state;
- scope and audience;
- source visibility/deletion;
- version and validity window;
- revocation ledger;
- taint labels and sensitivity;
- replay-reader compatibility.

If memory-service or source-service state is unavailable, retrieval must fail
closed or mark coverage gap. It must not fall back to stale local memory.

## Revocation And Dependency Invalidation

MemoryRevocationLedger must link:

- revocation request or source deletion ref;
- authority decision ref;
- affected MemoryClaim refs;
- dependent memory refs;
- supersession or invalidation refs;
- retrieval eligibility change refs;
- audit and replay-reader refs.

When a source is deleted, revoked or no longer visible, dependent ACTIVE memory
must be revoked, downgraded, expired, quarantined or sent to review before it can
be returned again.

Forget requests must distinguish:

- user-level personal memory forget;
- group/project correction;
- legal deletion;
- source revocation;
- policy source withdrawal;
- eval-only fixture cleanup.

## Version And Replay Rules

Every future controlled implementation design must carry low-sensitive refs for:

- MemoryCandidate version;
- MemoryClaim version;
- MemoryScope version;
- MemoryAdmissionDecision version;
- lifecycle state version;
- category threshold policy version;
- confidence calibration version;
- revocation ledger version;
- supersession chain version;
- retention/deletion policy version;
- replay-reader policy;
- compatibility window;
- audit lineage and operator action refs.

Replay must show why a memory was admitted, rejected, reviewed, superseded,
revoked, expired, quarantined or used. Normal replay must not require raw
prompts, full IM transcript archives, raw provider bodies, raw MCP payloads,
secrets or private service rows.

## Operator Surfaces

Before any implementation, owners must approve low-sensitive inspect-and-act
surfaces for:

- MemoryCandidate and MemoryClaim refs;
- source refs, speaker refs, audience refs and membership-window refs;
- scope, category and threshold policy refs;
- confidence, calibration and uncertainty refs;
- conflict, dedupe and supersession refs;
- lifecycle state and retrieval eligibility;
- revocation, deletion and dependency invalidation refs;
- review task, timeout, escalation and reviewer decision refs;
- memory-use audit refs;
- redaction, retention and ReplayReaderPolicy refs;
- correction, forget, revoke, restore-to-review and quarantine actions.

Operators and authorized users must be able to explain and correct memory
without seeing raw content they are not authorized to view. Python worker output
cannot override any operator or service-owned decision.

## L3 Real-Service Smoke Plan

Before implementation can start, owners must approve a smoke plan that proves
the following with low-sensitive records only:

| Smoke | Boundary | Required Proof |
| --- | --- | --- |
| Candidate submission dry run | Runtime/Python -> memory-service | Python output remains candidate-only; memory-service owns submitted state |
| Source-backed admission | memory-service -> retrieval/source owner | source visibility refs are required; source-less candidates reject |
| Category thresholds | memory-service -> policy/security | personal/group/project/procedural/policy-like thresholds differ |
| Group speaker/audience | memory-service | group memory without speaker/audience/membership refs cannot become ACTIVE |
| Review wait | memory-service -> workflow-service -> memory-service | workflow owns wait/timer only; memory-service owns final ACTIVE decision |
| Revocation retrieval block | memory-service -> retrieval-gateway -> Runtime | revoked/deleted/expired memory is not normal retrieval eligible |
| Supersession chain | memory-service -> retrieval/replay | current memory wins; old memory remains audit-visible only |
| Procedural migration | memory-service -> skill/AgentDefinition owner | skill-version change invalidates or reviews procedural memory |
| Memory-use audit | retrieval/Runtime -> memory-service/audit | use refs, scope refs, version refs and replay refs survive |
| Operator forget/correct | operator surface -> memory-service/audit | authorized correction/forget changes retrieval eligibility and audit refs |
| Replay reader dry run | memory-service -> audit/eval | admission/use replay works from refs without raw transcript archives |
| Public/synthetic eval separation | ai-eval -> governance | memory benchmarks never become product facts or fallback data |

These smokes must not use real NexusIM IM data until owner-approved test data
policy exists. Fixture evidence can prepare the plan, but cannot substitute for
L3 real-service smoke.

## Owner Review Checklist

| Owner | Must Approve |
| --- | --- |
| Main integration | Service boundaries, allowed paths and no production shortcut |
| memory-service owner | lifecycle, category thresholds, ACTIVE decisions, supersession, revocation, expiry and retrieval eligibility |
| retrieval-gateway owner | memory retrieval eligibility checks and memory-vs-source precedence |
| Agent Runtime owner | candidate submission, memory consumption and no ACTIVE ownership |
| workflow-service owner | durable review waits, timeout and escalation without ACTIVE ownership |
| policy/security owner | scope, audience, sensitivity, cross-group leakage and governed policy-source rules |
| audit/legal owner | admission explanation, memory-use audit, retention, deletion, forget and replay-reader policy |
| product/operator owner | review, correction, revoke, forget, quarantine and explanation UX |
| SRE/incident owner | review latency, admission backlog, retrieval miss/blocked metrics and incident escalation refs |

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
python -m pytest ai/python/tests/test_agent_eval_memory_admission_governance.py -q
python -m pytest ai/python/tests/test_agent_memory_calibration.py -q
python -m pytest ai/python/tests/test_agent_eval_cross_service_preservation.py -q
python -m pytest ai/python/tests/test_agent_eval_contract_version_compatibility.py -q
python -m pytest ai/python/tests/test_agent_eval_operator_governance.py -q
python -m pytest ai/python/tests/test_agent_eval_controlled_implementation_readiness.py -q
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_hardening_scenarios.json
python ai/python/scripts/run_agent_eval_fixture.py ai/python/fixtures/agent_eval/synthetic_memory_admission_deeper_hardening_scenarios.json
python ai/python/scripts/run_agent_memory_calibration.py ai/python/fixtures/agent_eval/memory_calibration_public_export.json
```

## P0 / P1 / P2 Review

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside this L2 design draft | No production memory writes, schema, service connection, real data or runtime implementation is introduced |
| P1 | Owner review is missing | External blocker before implementation |
| P1 | L3 real-service smoke is missing | External blocker before implementation |
| P1 | Production operator review/correction/forget UX is not approved | External blocker before implementation |
| P1 | Production retention, deletion and legal forget policy is not approved | External blocker before implementation |
| P2 | Confidence thresholds remain research baselines | Keep for larger public memory datasets and release review |
| P2 | Admission backlog and review latency SLOs are not approved | Keep for SRE owner review |

## Auto-Reject Rules

Reject any Memory Admission implementation proposal that:

- creates proto, OpenAPI, Kafka schema, migration, service directory, database
  table, startup path or production field shape from this L2 design alone;
- lets Python, Runtime or workflow-service make ACTIVE memory decisions;
- admits memory without source refs and source visibility refs;
- admits group memory without speaker, audience and membership-window refs;
- uses identical thresholds for personal, group, project, procedural and
  policy-like memory;
- lets policy-like memory come from model summaries instead of governed policy
  sources;
- returns SUPERSEDED, REVOKED, EXPIRED or QUARANTINED memory as normal
  retrieval-eligible memory;
- silently overwrites conflicting memory;
- uses memory as fallback for unavailable source services or missing retrieval
  lanes;
- requires raw prompt, full IM transcript archive, raw provider body, raw MCP
  payload, secret or private service row for normal replay;
- lets Python own revocation, deletion, operator override, audit archive,
  final proposal, approval, execution or production source truth;
- treats fixture evidence or public memory benchmark data as production smoke
  or product source truth.

## Decision

This design closes the Agent Lab-side L2 design gap for the fourth candidate:
Memory Admission. It does not authorize implementation.

Next safe action after main integration review is either:

- owner review of the first four L2 designs; or
- a fifth L2 scoped design for Tool / MCP or AgentOps if owners want the whole
  package prepared before any real-service smoke.
