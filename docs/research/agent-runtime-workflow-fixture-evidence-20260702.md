# Agent Runtime / Workflow Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for the Runtime / Workflow ADR candidate.
This is not an accepted ADR, production schema, service directory or runtime
implementation.

## Verdict

Conditionally passed for the Runtime / Workflow ownership rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that Runtime can
consume workflow wakeups idempotently, reject stale checkpoints, block cancelled
resume, expose operator controls without payload bodies and fail closed on
over-budget runs.

This does not authorize `agent-runtime-service`, workflow-service changes or
production callback queues.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/runtime_workflow_ownership.py`
- `ai/python/tests/test_agent_eval_runtime_workflow_ownership.py`

Fixture:

- `ai/python/fixtures/agent_eval/runtime_workflow_ownership_rehearsal.json`

The helper verifies:

- Agent Runtime, workflow-service, action-executor, audit-service and Python AI
  Worker ownership assertions reject forbidden state;
- checkpoints carry refs, versions, audit and ReplayIndex refs, not raw payload
  or business facts;
- stale checkpoint refs are explicitly rejected;
- only one wakeup is consumed per dedupe key;
- consumed wakeups must correlate decision, checkpoint, run and step refs;
- cancelled or stale resume attempts fail closed;
- operator cancel / resume / replay / inspect controls expose low-sensitive refs
  and redaction policy refs;
- replay and resume do not re-execute side effects;
- BudgetLedger records fail closed when over budget.

## Review Closure

This closes the fixture evidence portion of the Runtime / Workflow ADR review
condition:

- "Future fixture hardening should prove duplicate wakeup and stale checkpoint
  rejection."

It also adds fixture evidence for resume correlation, cancel-state checks,
operator controls and budget-ledger fail-closed behavior.

It does not close:

- main integration owner review for checkpoint and workflow wakeup storage;
- control-plane / AgentOps owner review for production cancel / resume / replay
  UX;
- policy owner review for production BudgetLedger limits;
- capacity and queue evidence required before service promotion.

## Next Evidence Target

Next fixture-only evidence should focus on one of:

- Memory revocation and category-threshold proof;
- Tool prepare-expiry re-prepare proof;
- AgentOps release-blocking and kill-switch proof.
