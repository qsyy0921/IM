# Agent Memory Admission SDD

Date: 2026-07-01
Status: Detailed SDD draft for Agent Exploration Mode. This is not an ADR,
proto, OpenAPI, Kafka schema, migration, production service directory or memory
event contract.

## 1. Goal

Define how NexusIM Agent memory candidates become durable memory, how they are
scoped, reviewed, superseded, revoked and evaluated.

Memory is a governed subsystem. It is not prompt cache, vector cache or a place
to store model guesses. Durable memory must improve later tasks while avoiding
pollution, leakage and stale fact reuse.

## 2. Non-Goals

- Do not define final memory event shape or database schema.
- Do not admit real NexusIM IM data in first-stage eval.
- Do not let Python AI Worker write ACTIVE memory.
- Do not turn every conversation summary into long-term memory.
- Do not use memory as a fallback when retrieval or source-of-truth services
  are unavailable.

## 3. Memory Categories

| Category | Purpose | Admission Bias |
| --- | --- | --- |
| Run memory | Temporary task scratch state | Runtime-only, never ACTIVE |
| Session memory | Current interaction continuity | Short TTL, not durable fact |
| Episodic memory | Source-backed event or decision summary | Candidate with source refs |
| Semantic memory | Stable fact, preference or project knowledge | Review on conflict or broad scope |
| Procedural memory | Repeated task pattern or skill operating rule | Bind to SkillPackage version |
| Personal memory | User preference or working style | Require user scope and confidence |
| Group memory | Shared group decision or norm | Require speaker and audience scope |
| Project memory | Durable project fact, goal or decision | Require supersedes/revoke chain |
| Policy memory | Governed rule or compliance fact | Must come from policy owner, not model |

## 4. Component Responsibilities

`memory-service` owns:

- MemoryCandidate lifecycle;
- source, subject, speaker and audience refs;
- scope, version, review outcome, supersedes, revocation and expiry;
- conflict detection and dedupe;
- ACTIVE / REJECTED / NEEDS_REVIEW state;
- retrieval eligibility for memory refs;
- memory use audit refs.

Agent Runtime owns:

- candidate extraction request;
- candidate hash and source refs;
- run-local reasoning about why a memory might help;
- submission of MemoryCandidate refs.

Python AI Worker can produce:

- candidate text;
- classification;
- confidence;
- source refs;
- conflict hints;
- dedupe hints;
- uncertainty/failure reason.

Python AI Worker cannot decide final admission.

workflow-service may own:

- human review wait;
- reviewer timeout;
- escalation or operator queue.

workflow-service cannot make memory ACTIVE by itself.

## 5. State Ownership

| State | Owner | Notes |
| --- | --- | --- |
| Candidate text/ref | memory-service after submission | Candidate can be derived from worker output |
| Candidate source refs | memory-service | Must be permission-checkable |
| Speaker / subject / audience | memory-service | Required for group memory |
| Review outcome | memory-service | Optional workflow wait only |
| ACTIVE memory | memory-service | Not Runtime or Python |
| Supersedes chain | memory-service | Required for updates |
| Revocation / expiry | memory-service | Must affect retrieval |
| Memory use audit | memory-service / audit-service | Low-sensitive refs |
| Run scratchpad | Agent Runtime | Never promoted automatically |

Memory-service cannot own:

- original business fact truth;
- workflow timers;
- tool execution result truth;
- raw prompt/provider transcript archive;
- policy final authorization decision beyond decision refs.

## 6. Admission Pipeline

```text
source refs or AgentRun trace
-> candidate extraction
-> source visibility check
-> classification
-> scope and audience selection
-> PII / sensitivity check
-> confidence and uncertainty gate
-> dedupe and near-duplicate check
-> conflict and supersedes check
-> overgeneralization check
-> review or auto-admit policy
-> ACTIVE / REJECTED / NEEDS_REVIEW
-> retrieval eligibility and audit
```

Each stage must be explainable in low-sensitive metadata.

## 7. Admission Rules

### 7.1 Source-Backed Only

Every durable memory needs source refs. A model statement without source refs is
not durable memory.

### 7.2 Scope Must Be Explicit

Candidate scope must be one of:

- personal;
- group;
- project;
- organization;
- tenant;
- eval-only fixture.

Broader scope requires stronger evidence and more review.

### 7.3 Speaker and Audience Required for Group Memory

Group memory must record:

- who said or decided it;
- who was present or authorized;
- which group/project it applies to;
- whether it is a decision, preference, fact or open question.

One user's preference in a group is not automatically group preference.

### 7.4 Conflict Cannot Be Silent

If a candidate conflicts with ACTIVE memory, it must become:

- `NEEDS_REVIEW`;
- `SUPERSEDES_PENDING`;
- `REJECTED_CONFLICT`;
- or a clarification request.

Silent overwrite is not allowed.

### 7.5 Supersedes Is Different From Duplicate

A duplicate should not create a new durable fact. A superseding fact must keep
old memory reachable for audit but ineligible for normal retrieval if replaced.

### 7.6 Revocation and Expiry Affect Retrieval

Revoked or expired memory cannot be returned as normal retrieval result. It can
remain as low-sensitive audit/ref history under retention policy.

### 7.7 Procedural Memory Is Skill-Bound

Procedural memory must include the SkillPackage or AgentDefinition version it
applies to. It cannot silently modify unrelated skills.

### 7.8 Policy Facts Are Not Model Memories

Policy, compliance and legal facts must come from governed policy sources. Agent
summaries can cite them but cannot create them.

## 8. Review Modes

| Mode | Use When | Owner |
| --- | --- | --- |
| Auto-reject | Missing source, forbidden scope, low confidence, unsafe content | memory-service |
| Auto-admit | Narrow scope, high confidence, no conflict, low risk | memory-service policy |
| Human review | Group/project/procedural memory, conflict, broad scope, sensitive content | memory-service + workflow-service wait |
| User confirmation | Personal preference or profile aggregate | memory-service via product UX |
| Admin review | Org-wide or policy-like memory | memory-service + admin/governance |

Auto-admit should be conservative in early phases.

## 9. Key Flows

### 9.1 Personal Preference

```text
source refs
-> candidate: "User prefers concise status updates"
-> personal scope
-> overgeneralization check
-> user confirmation or repeated-evidence threshold
-> ACTIVE personal memory
```

Single casual statement should not become durable profile memory.

### 9.2 Group Decision

```text
group source refs
-> candidate decision
-> speaker/audience and membership-window check
-> conflict/supersedes check
-> group review if needed
-> ACTIVE group memory
```

If a later decision reverses it, the newer memory supersedes the old one.

### 9.3 Project Decision Supersede

```text
project source refs
-> candidate says decision changed
-> find old ACTIVE decision
-> mark supersedes relation
-> review if material
-> new ACTIVE, old superseded
```

Retrieval should prefer the active current decision and optionally cite history
when relevant.

### 9.4 Revocation

```text
revocation request or source deletion
-> verify authority
-> mark memory revoked
-> remove from retrieval eligibility
-> write audit ref
-> eval check prevents reuse
```

## 10. Failure Semantics

| Failure | Behavior |
| --- | --- |
| Source missing | Reject candidate |
| Source no longer visible | Reject or quarantine |
| Speaker ambiguous | Needs review |
| Audience unclear | Needs review |
| Scope too broad | Narrow scope or reject |
| Conflict found | Needs review or supersedes path |
| PII or sensitive content | Review / redact / reject under policy |
| Low confidence | Reject or keep run-local only |
| Worker malformed output | Repair candidate if safe; else reject |
| Retrieval after revocation | Must not return revoked memory |

## 11. Security Boundary

Memory admission defends against:

- prompt injection stored as memory;
- malicious tool output stored as memory;
- group content leaking into personal memory;
- personal preferences leaking into group memory;
- stale or revoked facts reused;
- overgeneralized profile facts;
- model speculation becoming durable truth.

All memory candidates are tainted until admitted.

## 12. Eval / Replay

Required eval families:

- recall of valid memories;
- update and supersedes correctness;
- forget / revocation correctness;
- scope isolation;
- speaker attribution;
- audience control;
- overgeneralization prevention;
- conflict handling;
- memory pollution rejection;
- downstream task improvement with memory vs without memory.

Candidate public inputs:

- STATE-Bench for memory improving enterprise tasks;
- LoCoMo and LongMemEval for long-horizon recall/update;
- GroupMemBench for multi-party group memory;
- EverMemBench for durable memory stress;
- synthetic NexusIM-like group/project fixtures.

Replay must show why a memory was admitted, rejected, superseded or used,
without requiring raw transcript archive.

## 13. Observability / Audit

Metrics:

- candidate volume by source, scope and skill;
- admit/reject/needs-review rate;
- conflict rate;
- supersedes rate;
- revocation rate;
- memory use rate in AgentRun;
- pollution findings;
- permission leakage findings;
- stale memory use attempts;
- review latency.

Audit refs:

- source refs;
- candidate ref/hash;
- reviewer/decision ref;
- supersedes/revocation refs;
- read/use refs;
- eval failure refs.

## 14. Risks / Rejection Conditions

Reject memory promotion if:

- candidate can become ACTIVE without source refs;
- Python Worker can make final ACTIVE decision;
- group memory lacks speaker/audience;
- revoked memory can still be retrieved;
- conflict overwrite is silent;
- memory eval measures only recall quantity, not pollution and task impact;
- fixture tests use real IM data;
- memory becomes fallback for unavailable source services.

## 15. Promotion Conditions

Memory admission can move from design to ADR only after:

- open-dataset memory suite passes baseline thresholds;
- synthetic group/project fixture proves scope and revocation;
- overgeneralization and pollution checks are automated;
- retrieval-gateway can respect memory state/version/scope;
- audit can explain memory admission and use.

## 16. Current Isolated Fixture Coverage

Current first-stage code is fixture-only and lives under
`ai/python/nexusim_ai_eval/` and
`ai/python/fixtures/agent_eval/synthetic_memory_admission_scenarios.json`.
It does not freeze a production memory event or MemoryCandidate schema.

Implemented checks:

- group memory must carry source refs, speaker refs and audience refs;
- project memory supersedes lineage must be represented when expected;
- profile aggregate memory can require review before activation;
- revoked memory use is detected as pollution;
- stale memory refs must not be used as current facts;
- overgeneralized memory candidates can be rejected.

Remaining hardening:

- duplicate and near-duplicate memory handling;
- low-confidence candidate rejection;
- procedural memory bound to SkillPackage / AgentDefinition version;
- policy-like memory rejection unless sourced from governed policy;
- review timeout and retry metadata.

## 17. References

- `docs/sdd/memory-service.md`
- `docs/sdd/agent-runtime.md`
- `docs/sdd/agent-context-evidencepack.md`
- `docs/research/agent-open-dataset-eval-plan-20260701.md`
- STATE-Bench:
  <https://opensource.microsoft.com/blog/2026/05/19/introducing-state-bench-a-benchmark-for-ai-agent-memory/>
- GroupMemBench:
  <https://www.microsoft.com/en-us/research/publication/groupmembench-benchmarking-llm-agent-memory-in-multi-party-conversations/>
- EverMemBench: <https://arxiv.org/abs/2602.01313>
