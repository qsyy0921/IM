# policy-service decision audit repair validation smoke - 2026-06-13

## Scope

This smoke verifies that the explicit DLQ repair path still works after adding the repair preflight gate.

The preflight gate validates each DLQ row through the same policy-event builder used by the outbox relay. Valid rows can be reset to `PENDING`; invalid envelope or payload rows remain `DLQ`, write a `SKIPPED / validation_failed` repair audit entry, and make the operator return a non-zero error.

This smoke covers the valid redrive path. The invalid payload branch is covered by the PostgreSQL integration test `TestOutboxStoreRepairDLQSkipsInvalidPolicyAuditIntegration`.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-decision-audit-repair-validated-smoke-20260613-clean -ExerciseAuditRepair
```

Raw summary:

```text
H:\NexusIM\loadtest-results\policy-decision-audit-repair-validated-smoke-20260613-clean\policy-contact-summary.json
```

## Result

```text
commit=f175d68
git_dirty=false
success=true
policy_audit_topic=im.policy.events.policy-decision-audit-repair-validated-smoke-20260613-clean
policy_decision_audit_outbox_status.total=3
policy_decision_audit_outbox_status.published=3
policy_decision_audit_outbox_status.pending=0
policy_decision_audit_outbox_status.dlq=0
policy_audit_kafka_event_count=3
repair.event_id=d16ac345-89df-4ff8-8221-07c5467b5c49
repair.repaired=1
repair.skipped=0
repair.repair_audit_count=1
repair.kafka_end_offset=4
```

## Interpretation

The valid DLQ event passed preflight validation, was reset to `PENDING`, and was published by the normal relay. Kafka end offset `4` proves the repaired event re-entered the relay path after the initial three decision audit events.

This is still not a broad repair workflow or poison-payload classifier. Operators must inspect DLQ rows before redrive. Invalid rows are fail-closed and remain blocked until a separate repair decision is made.
