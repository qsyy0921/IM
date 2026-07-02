# Agent Operational Readiness Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for operational readiness budget closure.
This is not an accepted ADR, production SLO, on-call policy, schema, service
directory or runtime implementation.

## Verdict

Conditionally passed for the isolated operational readiness rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that Agent-layer
promotion evidence can require explicit owner, limit, measurement, operator
view, audit, failure-class and release-gate refs for operational budgets before
future runtime or release promotion.

This does not authorize production SLOs, production capacity envelopes, real
provider limits, release automation or on-call escalation policy.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/operational_readiness.py`
- `ai/python/tests/test_agent_eval_operational_readiness.py`

Fixture:

- `ai/python/fixtures/agent_eval/operational_readiness_rehearsal.json`

The helper verifies budget evidence for:

- runtime step limits;
- model spend envelopes;
- MCP/tool provider timeout limits;
- retrieval / EvidencePack latency limits;
- eval report retention limits;
- canary / shadow telemetry coverage;
- P0/P1 incident escalation windows.

## Required Budget Shape

Each budget record must carry these low-sensitive refs:

- `budget_ref`
- `budget_kind`
- `owner_ref`
- `applies_to_ref`
- `risk_tier_ref`
- `limit_ref`
- `measurement_ref`
- `enforcement_ref`
- `operator_view_ref`
- `audit_ref`
- `evidence_ref`
- `release_gate_ref`
- `failure_class_ref`
- `rejection_condition_refs`

The rehearsal rejects unsupported budget kinds, missing coverage, duplicate
coverage, owner mismatch, missing measurements, missing operator views, raw body
retention, Python override, unreviewed capacity, release-with-gap and fixture
claims that production SLOs are already authorized.

Promotion gate records must fail closed when budget coverage or measurement
evidence is missing. A gate cannot allow release when failed budget evidence is
present.

## Review Closure

This closes the fixture-only proof request for the review-loop P2:

- "Capacity, cost, retention and latency budgets remain conceptual."

The closure is intentionally narrow. It proves a reusable budget-evidence gate
exists in the isolated harness. It does not close:

- main integration acceptance for production ownership;
- real provider capacity planning;
- production SLO targets;
- production telemetry wiring;
- production on-call and incident escalation workflow;
- release automation or rollback automation.

## Checks

Focused verification:

```powershell
python -m pytest ai/python/tests/test_agent_eval_operational_readiness.py -q
```

Expected result:

```text
8 passed
```

Full workspace checks are tracked in the handoff for this module.

## Next Evidence Target

The next safe step is main integration review or another focused
contract/version hardening item. Do not promote operational budgets to
production SLOs until owners approve real telemetry, retention, capacity and
incident-response contracts.
