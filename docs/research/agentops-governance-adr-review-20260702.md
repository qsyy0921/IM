# AgentOps / Governance ADR Candidate Review

Date: 2026-07-02

Status: focused review of `adr-candidate-agentops-governance-boundary.md`. This
is not an accepted ADR and does not create a production release pipeline or
admin console.

## Verdict

Initial verdict: rejected for ADR acceptance.

Reason: the candidate correctly made AgentOps a release-control boundary, but
left four P1 concerns as future evidence: kill-switch ownership, release pinning
and baseline approval UX, failure-class owner workflow and canary/shadow
comparison policy.

After this pass: conditionally passed for main integration review as the sixth
ADR candidate.

The condition is that main integration must still accept governance/control-plane
ownership and no production release pipeline is authorized by this review.

## P0/P1/P2 Findings

| Severity | Finding | Impact | Closure |
| --- | --- | --- | --- |
| P0 | None inside isolated Agent Lab scope | No production release control path exists yet | Keep hard boundary unchanged |
| P1 | Kill-switch owner was future evidence | Incident response could be blocked by unclear authority | Candidate now requires owner, scope, propagation and audit refs |
| P1 | Release pinning and baseline approval UX were underspecified | Releases could drift or silently refresh baselines | Candidate now requires release pin and baseline decision records |
| P1 | Failure-class owner workflow was a note | Repeated failures could remain unowned and untested | Candidate now requires owner, fixture backfill and retirement rules |
| P1 | Canary/shadow comparison policy was not explicit | Production-like regressions could be incomparable to offline baseline | Candidate now requires comparable metrics and rollback behavior |
| P2 | On-call escalation and SLO budgets were conceptual | Does not block candidate review, but blocks production rollout without owner-approved telemetry and escalation policy | Fixture-only operational readiness rehearsal added; production SLO remains blocked |

## Re-Review Checklist

| Gate | Result | Evidence |
| --- | --- | --- |
| Release-control boundary | Pass | Candidate keeps AgentOps as release control, not business truth |
| Owner and disable switch | Pass | Candidate blocks production agent without owner or disable switch |
| Eval/replay gate | Pass | Candidate blocks P0/P1, replay gaps and audit gaps |
| Kill switch | Pass after this pass | Candidate now requires scope, propagation and audit refs |
| Release pinning and baseline | Pass after this pass | Candidate now requires pinned release and baseline approval records |
| Failure owner workflow | Pass after this pass | Candidate now requires owner and regression fixture feedback |
| Canary/shadow comparison | Pass after this pass | Candidate now requires comparable metrics and rollback behavior |
| Production boundary | Pass | Candidate still does not create a release pipeline or admin console |

## Remaining Conditions

- Main integration must accept governance/control-plane ownership.
- Fixture code now proves release blocking, baseline approval, kill-switch
  propagation, failure-class owner workflow, canary / shadow comparability and
  operator controls in `ai/python/nexusim_ai_eval/agentops_governance.py`,
  `ai/python/fixtures/agent_eval/agentops_governance_rehearsal.json` and
  `ai/python/tests/test_agent_eval_agentops_governance.py`.
- Fixture code also proves operational budget evidence for runtime steps,
  model spend, provider timeout, retrieval latency, eval retention, canary
  telemetry and incident escalation in
  `ai/python/nexusim_ai_eval/operational_readiness.py`.
- Production on-call, SLO, real telemetry and incident escalation remain
  integration-scope.

## Fixture Evidence Update

Fixture evidence is recorded in
`docs/research/agentops-governance-fixture-evidence-20260702.md`.
Operational readiness fixture evidence is recorded in
`docs/research/agent-operational-readiness-fixture-evidence-20260702.md`.

This update closes the fixture-only proof request for release-blocking behavior
and baseline approval records. The operational readiness update closes the
fixture-only budget proof request. These updates do not close production
governance owner acceptance, admin UX, on-call / SLO / incident escalation or
production canary telemetry.

## Next Review Target

All six ADR candidates now have first-pass focused review ledgers and
fixture-only evidence slices. Dataset reproducibility evidence is recorded in
`docs/research/agent-dataset-reproducibility-fixture-evidence-20260702.md`;
the next loop should wait for main integration review or focused
contract/version hardening.
