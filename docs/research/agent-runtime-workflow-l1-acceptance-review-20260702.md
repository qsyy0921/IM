# Agent Runtime / Workflow L1 Acceptance Review

Date: 2026-07-02

Status: Agent Lab L1 self-review for
`adr-candidate-agent-runtime-workflow-boundary.md`. This is not an accepted
ADR, production contract, schema, migration, service directory, workflow
change or runtime implementation.

## Verdict

Recommendation: accept the Runtime / Workflow candidate for L1 ADR acceptance
review after the accepted Eval / Replay L1 gate.

Do not approve production implementation from this review.

Reason: the candidate has no known P0/P1 gap inside Agent Lab scope after the
focused review and fixture-evidence hardening. It defines the cognitive runtime
vs durable workflow split, checkpoint safety, wakeup idempotency, operator
controls, BudgetLedger fail-closed behavior and service-promotion refusal in a
way that can be reviewed by main integration.

## Playbook Result

```text
Candidate: Agent Runtime / Workflow Boundary
Verdict: recommend accept for L1 ADR review; defer implementation
Severity: none inside Agent Lab scope
Required signoffs checked: Agent Lab complete; Eval / Replay L1 accepted; main integration pending; runtime owner and workflow owner required before implementation
Agent Lab evidence checked: Runtime SDD, ADR candidate, ownership matrix, focused review, runtime-workflow fixture evidence, operational readiness, operator governance and controlled implementation readiness
External blocker, if any: real workflow wakeup/cancellation smoke; checkpoint storage owner review; production control-plane UX; service-promotion capacity proof
Rejected production shortcuts: agent-runtime-service creation, workflow queue takeover, raw prompt/provider checkpointing, Python-owned runtime truth and workflow-service planner inspection
Allowed next step: main integration may decide whether to accept this ADR candidate
Disallowed next step: production runtime code, workflow-service changes, schema, queue, migration, service registry, real backend/model/MCP integration or service promotion
```

## Evidence Reviewed

| Evidence | Result |
| --- | --- |
| `docs/sdd/agent-runtime.md` | Pass; Runtime owns cognitive run state and explicitly cannot own approval queues, execution truth, ACTIVE memory or workflow timers |
| `docs/research/adr-candidates/adr-candidate-agent-runtime-workflow-boundary.md` | Pass; ownership matrix, checkpoint boundary, wakeup semantics, operator controls, BudgetLedger and rejection rules are named |
| `docs/research/agent-runtime-workflow-ownership-20260701.md` | Pass; scenario and state matrices keep workflow-service as durable human/operational wait owner |
| `docs/research/agent-runtime-workflow-adr-review-20260702.md` | Pass; earlier P1 findings were closed or moved to explicit external conditions |
| `docs/research/agent-runtime-workflow-fixture-evidence-20260702.md` | Pass; duplicate wakeup, stale checkpoint, resume correlation, cancel-state, operator controls and over-budget fail-closed behavior are proven with fixture refs |
| `docs/research/agent-operator-governance-fixture-evidence-20260702.md` | Pass; cancel/resume/replay/inspect surfaces are covered by inspect-and-act governance |
| `docs/research/agent-operational-readiness-fixture-evidence-20260702.md` | Pass for fixture scope; runtime step, model spend, tool timeout and incident escalation budget evidence exists |
| `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md` | Pass; implementation remains blocked without accepted ADR, owner review and preservation evidence |

## Requirement Matrix

| L1 Requirement | Review Result | Notes |
| --- | --- | --- |
| Runtime is not a second workflow engine | Pass | Runtime may pause on workflow refs but cannot own human waits, timers, operator queues or compensation state |
| workflow-service cannot inspect planner internals | Pass | workflow-service must not read raw prompt, EvidencePack body, ContextPackage, model output or planner state |
| Checkpoints are low-sensitive resume refs | Pass | AgentCheckpoint stores refs, hashes, versions, replay-reader and audit refs; raw prompt/provider bodies/secrets/business facts are rejected |
| Wakeup consumption is idempotent | Pass | Resume verifies wakeup id, decision ref, checkpoint version, run/step correlation, cancel state and dedupe key |
| Cancel/resume/replay are governable | Pass | Operator controls require inspect, cancel, resume and replay refs without body exposure |
| BudgetLedger blocks abuse | Pass | Model, retrieval, tool, retry, runtime and tenant/actor/skill refs are required; over-budget runs fail closed or require review |
| Action execution remains separate | Pass | Runtime can request execution only through approved proposal and prepared tool lineage; action-executor owns side effects |
| Service promotion is not authorized | Pass | Candidate stays as runtime module/harness until queue ownership, wakeup durability, checkpoint pressure and operator needs are proven |
| Replay does not re-execute side effects | Pass | ReplayIndex links run, step, checkpoint, workflow, execution and audit refs; external calls are not repeated |

## Auto-Reject Audit

No auto-reject condition was found inside the current Agent Lab scope.

| Auto-Reject Rule | Result |
| --- | --- |
| Runtime becomes a second workflow engine | Not triggered; durable waits and timers remain workflow-service-owned |
| workflow-service reads planner/model/raw prompt/EvidencePack body | Not triggered; candidate and ownership matrix reject this |
| Runtime owns execution truth, ACTIVE memory, approval or audit archive | Not triggered; action-executor, memory-service, workflow-service and audit-service remain owners |
| Checkpoint or replay requires raw prompt/provider/private rows | Not triggered; low-sensitive refs and hashes are required |
| Cancel/resume/wakeup lacks idempotency | Not triggered; dedupe and correlation are required and fixture-tested |
| Fixture evidence authorizes runtime service creation | Not triggered; service promotion is explicitly deferred |
| Python owns durable runtime state | Not triggered; Python remains candidate/eval-only |

## Findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside Agent Lab scope | No production runtime exists and hard boundaries still block service creation |
| P1 | None inside Agent Lab scope | Previous P1s for checkpoint safety, wakeup race, operator controls and BudgetLedger are closed to fixture/review level |
| P2 | Real workflow wakeup / cancellation smoke is missing | External owner evidence before implementation, not an L1 review blocker |
| P2 | Queue capacity and checkpoint pressure are not measured | Blocks `agent-runtime-service` promotion, not L1 ownership acceptance |
| P2 | Production control-plane UX is not implemented | Deferred to L2/L3 owner review |

## External Deferrals

These items must be deferred to L2/L3 review before implementation:

- runtime / workflow owner approval for checkpoint storage, wakeup durability,
  cancel propagation and resume correlation;
- control-plane / AgentOps owner approval for production inspect, cancel, resume
  and replay UX;
- policy owner approval for BudgetLedger limits and over-budget review path;
- real-service smoke proving workflow decision refs, checkpoint refs, audit refs
  and ReplayIndex refs survive the future boundary;
- queue capacity, checkpoint pressure and worker isolation proof before any
  `agent-runtime-service` promotion.

## Allowed After L1 Acceptance

If main integration accepts this ADR candidate, the only allowed next step is a
scoped implementation design for the runtime/workflow boundary.

That design must name:

- whether Runtime remains a module/harness or needs a future service ADR;
- checkpoint owner, retention class and replay-reader policy;
- workflow decision and wakeup refs;
- cancel/resume/replay operator surfaces;
- BudgetLedger owner and enforcement boundary;
- action-executor handoff invariants;
- fixture gates that must continue to pass.

## Still Disallowed

L1 acceptance must not create or freeze:

- `agent-runtime-service` or production runtime workers;
- workflow-service schema, timers, queues or callback APIs;
- proto, OpenAPI, Kafka schema, migration or database tables;
- checkpoint, wakeup or ReplayIndex wire shape;
- real PostgreSQL, Kafka, Redis, OpenSearch, model provider, MCP provider,
  workflow-service, memory-service or action-executor integration;
- Python ownership of production run state, approval, execution, audit archive
  or ACTIVE memory.

## Re-Review Result

After applying the ADR acceptance playbook, the Runtime / Workflow candidate is
reviewable and has no known Agent Lab P0/P1 blocker.

Agent Lab recommends that main integration review this candidate second. If
main integration accepts it, Agent Lab should then prepare the Context /
EvidencePack L1 review package. If main integration rejects or defers, Agent Lab
should handle that P0/P1 or owner-evidence request before moving on.
