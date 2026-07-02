# Agent Runtime / Workflow ADR Candidate Review

Date: 2026-07-02

Status: focused review of `adr-candidate-agent-runtime-workflow-boundary.md`.
This is not an accepted ADR and does not authorize `agent-runtime-service`,
workflow changes, schema or production runtime implementation.

## Verdict

Initial verdict: rejected for ADR acceptance.

Reason: the candidate correctly split cognitive run state from durable
human-wait state, but left four P1 concerns as future evidence: checkpoint
storage ownership, wakeup race semantics, operator cancel/resume/replay controls
and budget abuse handling.

After this pass: conditionally passed for main integration review as the second
ADR candidate.

The condition is that main integration must still accept the owner mapping, and
no service split is authorized until queue ownership, wakeup durability,
checkpoint pressure and operator control-plane needs are proven.

## P0/P1/P2 Findings

| Severity | Finding | Impact | Closure |
| --- | --- | --- | --- |
| P0 | None inside isolated Agent Lab scope | No production runtime exists and Python remains candidate-only | Keep hard boundary unchanged |
| P1 | Checkpoint storage owner was not constrained enough | Runtime could persist raw prompts/provider bodies or become a business fact store | Candidate now requires low-sensitive refs, version checks and raw-payload rejection |
| P1 | Wakeup race and duplicate callback handling were evidence-only | Runtime could double-resume or consume stale approvals | Candidate now requires idempotent wakeup consumption and correlation verification |
| P1 | Operator controls were not a promotion gate | Cancel/resume/replay could exist without inspectable controls | Candidate now requires low-sensitive operator controls before promotion |
| P1 | Budget ledger and abuse review were undefined | Long-running agents could consume unbounded model/tool/runtime budget | Candidate now requires budget ledger limits and fail-closed budget behavior |
| P2 | Capacity model for future queues is not measured | Does not block candidate review, but blocks service promotion | Keep as service-promotion evidence |

## Re-Review Checklist

| Gate | Result | Evidence |
| --- | --- | --- |
| Runtime is not workflow engine | Pass | Runtime cannot durably own human waits or external callback queues |
| workflow-service cannot inspect planner state | Pass | Candidate rejects raw prompt, EvidencePack body and planner state in workflow |
| Checkpoint safety | Pass after this pass | Candidate now requires low-sensitive refs, version validation and stale rejection |
| Wakeup idempotency | Pass after this pass | Candidate now requires dedupe key, decision ref and checkpoint correlation |
| Operator controls | Pass after this pass | Candidate now requires cancel/resume/replay controls with low-sensitive refs |
| Budget governance | Pass after this pass | Candidate now requires BudgetLedger and fail-closed over-budget behavior |
| Service promotion | Pass | Candidate still rejects creating `agent-runtime-service` before evidence |

## Remaining Conditions

- Main integration review must accept the ownership split.
- Fixture code now proves duplicate wakeup, stale checkpoint, resume correlation,
  cancel-state, operator control and over-budget fail-closed behavior in
  `ai/python/nexusim_ai_eval/runtime_workflow_ownership.py` and
  `ai/python/fixtures/agent_eval/runtime_workflow_ownership_rehearsal.json`.
- Production workflow integration remains blocked until checkpoint storage,
  wakeup durability, control-plane UX and policy owners accept the mapping.
- Capacity and queue evidence are required before any service promotion.

## Fixture Evidence Update

`docs/research/agent-runtime-workflow-fixture-evidence-20260702.md` records the
fixture-only ownership rehearsal. It proves Runtime consumes workflow wakeups
idempotently, rejects stale or cancelled resume, preserves low-sensitive
checkpoint / workflow / audit / ReplayIndex refs and does not re-execute side
effects during replay or resume.

## Next Review Target

Review Context / EvidencePack next, then Memory Admission and Tool / MCP. The
next focus should be whether source visibility, denied lanes and taint labels
are sufficient to prevent unauthorized context from becoming model input.
