# Agent Operator Governance Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for operator inspect-and-act surfaces. This
is not an accepted ADR, production admin UI, service API, proto, OpenAPI, Kafka
schema, database migration, release pipeline or runtime implementation.

## Verdict

Conditionally passed for the operator governance surface rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that the current
ADR candidate package has an operator governance path for memory, evidence,
replay, approval, release, failure class, kill switch and rollback surfaces.
Each surface must provide inspect refs, action refs, owner refs, auth-policy
refs, audit refs, redaction refs, replay-reader refs, failure-class refs,
evidence refs and rejection-condition refs before promotion can be allowed.

This does not authorize production operator UI or production control-plane APIs.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/operator_governance.py`
- `ai/python/tests/test_agent_eval_operator_governance.py`

Fixture:

- `ai/python/fixtures/agent_eval/operator_governance_rehearsal.json`

The helper verifies:

- all required operator surfaces are covered exactly once;
- memory governance is owned by memory-service plus governance;
- evidence governance is owned by retrieval-gateway plus governance;
- replay governance is owned by Agent Runtime plus ai-eval-service;
- approval governance is owned by workflow-service;
- release, failure class, kill switch and rollback surfaces are owned by their
  governance/control-plane owners;
- every surface has an inspect path and an action path, not just a passive view;
- every surface has low-sensitive visible refs, action target refs, audit refs,
  redaction policy refs, replay-reader policy refs and rejection refs;
- body exposure, unauthorized actor access, Python override, missing audit,
  actionless views and release-with-gap behavior block promotion;
- fixture evidence cannot authorize a production contract.

## Review Closure

This closes the Agent Lab fixture-only portion of the operator governance P1
gap:

- operator acceptance is no longer only a prose checklist;
- the current ADR candidate package has executable evidence for the
  "authorized operators can inspect and act on high-risk states" requirement;
- a passing-looking release gate cannot promote if any required operator
  surface is missing, passive-only, unaudited, body-exposing or Python-owned.

It does not close:

- accepted ADR promotion;
- production admin UI design;
- production auth policy design;
- real service query/action APIs;
- production on-call workflows;
- main integration owner acceptance.

## Next Evidence Target

The remaining next step should be main integration review of the ADR candidate
package and fixture evidence, or a focused P0/P1 requested by that review. Do
not promote production contracts from this evidence alone.
