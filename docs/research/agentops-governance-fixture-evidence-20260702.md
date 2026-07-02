# AgentOps / Governance Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for the AgentOps / Governance ADR
candidate. This is not an accepted ADR, production release pipeline, admin
console, migration, schema or service directory.

## Verdict

Conditionally passed for the AgentOps governance rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that release gates
block P0/P1 eval failures, replay gaps and audit gaps; kill switches have
owners, scopes, propagation refs and block new runs; baseline refresh requires
explicit approval; P0/P1 failure classes need owners and regression disposition;
and canary / shadow results must be comparable to offline baselines before any
release promotion.

This does not authorize production AgentOps control-plane APIs or UI.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/agentops_governance.py`
- `ai/python/tests/test_agent_eval_agentops_governance.py`

Fixture:

- `ai/python/fixtures/agent_eval/agentops_governance_rehearsal.json`

The helper verifies:

- governance owns AgentDefinition metadata, AgentRelease, BaselineApproval,
  FailureClassOwner, KillSwitch, RollbackPlan and CanaryReport refs;
- Python worker and model output cannot own governance decisions;
- production-enabled releases require owner, pinned AgentDefinition /
  SkillPackage refs, EvalReport, ReplayBundle, BaselineApproval, rollback,
  disable switch and audit refs;
- P0/P1 eval failure, replay gap, audit gap or missing baseline blocks release;
- active kill switch blocks new runs, carries propagation acks and records
  running-run behavior;
- baseline refresh cannot happen silently, especially when failure classes,
  datasets, risk tier or required suites change;
- open P0/P1 failure classes block release and baseline refresh unless they have
  owner plus regression fixture or explicit no-fixture reason;
- canary / shadow reports must compare compatible metrics to offline baseline
  and cannot promote P0/P1 regressions;
- operator controls expose only low-sensitive refs and cannot be overridden by
  Python worker output.

## Review Closure

This closes the fixture evidence portion of the AgentOps ADR review condition:

- "Future fixture hardening should prove release-blocking behavior and baseline
  approval records."

It also adds fixture evidence for kill-switch propagation, failure-class owner
workflow, canary / shadow comparability, rollback / hold behavior and operator
controls.

It does not close:

- main integration review for governance / control-plane ownership;
- admin UX owner review for production release pinning and baseline approval;
- production on-call, SLO and incident escalation design;
- production canary telemetry and rollback automation.

## Next Evidence Target

All six focused ADR candidate areas now have fixture-only evidence. The next
step should be main integration review or memory calibration / dataset
reproducibility hardening, not production contract promotion.
