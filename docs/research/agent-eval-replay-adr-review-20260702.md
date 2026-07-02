# Agent Eval / Replay ADR Candidate Review

Date: 2026-07-02

Status: focused review of `adr-candidate-agent-eval-replay-harness.md`. This is
not an accepted ADR and does not authorize production schema or service
implementation.

## Verdict

Initial verdict: rejected for ADR acceptance.

Reason: the candidate correctly made Eval / Replay the first promotion gate, but
left four P1 concerns as "next evidence" instead of decision constraints:
failure-class lifecycle, baseline approval, retention/redaction and
contract-version bump rehearsal.

After this pass: conditionally passed for main integration review as the first
ADR candidate.

The condition is that main integration must still accept or change the owner
mapping and no production contract may be created from this document alone.

## P0/P1/P2 Findings

| Severity | Finding | Impact | Closure |
| --- | --- | --- | --- |
| P0 | None inside isolated Agent Lab scope | No safety contradiction found because real data and production integration remain blocked | Keep boundary unchanged |
| P1 | Failure-class lifecycle owner was a future note | Unknown or ownerless failures could pass as non-blocking | Candidate now requires owner, severity and disposition before promotion |
| P1 | Baseline approval UX was a future note | Baselines could drift without explicit reviewer intent | Candidate now requires approval/refusal record and reviewer ownership |
| P1 | Retention/redaction policy was a future note | Replay artifacts could retain raw or over-sensitive payloads | Candidate now requires low-sensitive retention refs and redaction policy refs |
| P1 | Version-bump rehearsal was not defined | Future reader changes could silently break old replay | Candidate now defines a fixture-only version-bump rehearsal before production contracts |
| P2 | Cost and latency budgets are not yet encoded as gate metrics | Does not block ADR-candidate review, but will block production readiness | Keep for later eval hardening |

## Re-Review Checklist

| Gate | Result | Evidence |
| --- | --- | --- |
| Release-gate semantics | Pass | Candidate blocks missing suite, P0/P1, incomplete replay, unapproved baseline and unreproducible dataset |
| Replay minimums | Pass | Candidate requires context, memory, tool, workflow, execution, state diff, audit and failure refs |
| Raw payload rejection | Pass | Candidate rejects raw prompt/provider/body reliance for normal replay |
| Failure owner lifecycle | Pass after this pass | Candidate now requires FailureClassOwner and unknown-failure blocking |
| Baseline approval | Pass after this pass | Candidate now requires BaselineApproval decision record |
| Retention/redaction | Pass after this pass | Candidate now requires low-sensitive retention and redaction policy refs |
| Version-bump rehearsal | Pass after this pass | Candidate now defines fixture-only old/new reader rehearsal |
| Production boundary | Pass | Candidate still does not freeze schemas or create services |

## Remaining Conditions

- Main integration review must accept the owner mapping.
- Fixture code now implements the version-bump rehearsal in
  `ai/python/nexusim_ai_eval/replay_compatibility.py` and
  `ai/python/fixtures/agent_eval/replay_version_bump_rehearsal.json`.
- Production retention durations and legal policy owners remain outside Agent Lab
  and must be set during integration design.

## Fixture Evidence Update

`docs/research/agent-fixture-evidence-hardening-20260702.md` records the
fixture-only version-bump rehearsal. It proves old ReplayBundle refs remain
explainable under the current replay reader policy and that legacy raw payload
fields fail closed or expire with explicit deprecation refs.

## Next Review Target

Review `adr-candidate-agent-runtime-workflow-boundary.md` next. It must prove
that Runtime cannot become a second workflow engine and workflow-service cannot
read planner state.
