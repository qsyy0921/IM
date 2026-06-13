# policy-service decision audit repair smoke - 2026-06-13

## Scope

This smoke verifies the first-stage DLQ repair operator for `policy_decision_audit_outbox`.

It covers:

- normal policy decision audit relay to `im.policy.events`;
- synthetic transition of one published audit row to `DLQ`;
- `NEXUSIM_POLICY_SERVICE_MODE=outbox-repair` redrive by explicit `event_id`;
- repair audit row creation;
- normal relay publishing the repaired row again.

It does not cover repair-all, payload rewriting, poison payload classification, retention, external audit sinks, Kafka HA, or production operator approval workflow.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-decision-audit-repair-smoke-20260613-clean -ExerciseAuditRepair
```

Raw summary:

```text
H:\NexusIM\loadtest-results\policy-decision-audit-repair-smoke-20260613-clean\policy-contact-summary.json
```

## Result

```text
commit=b5dec20
git_dirty=false
success=true
policy_audit_topic=im.policy.events.policy-decision-audit-repair-smoke-20260613-clean
policy_decision_audit_outbox_status.total=3
policy_decision_audit_outbox_status.published=3
policy_decision_audit_outbox_status.pending=0
policy_decision_audit_outbox_status.dlq=0
policy_audit_kafka_event_count=3
repair.event_id=fc7b0b59-f806-455e-9388-027c0ce422ac
repair.repaired=1
repair.skipped=0
repair.repair_audit_count=1
repair.kafka_end_offset=4
```

The first `policy_audit_kafka_event_count=3` is the normal relay readback from the three direct SEND decisions in the contact block scenario. The repair step then forces one audit row to `DLQ`, runs the explicit event-id repair operator, waits for relay publication, and verifies the Kafka topic end offset reaches `4`.

## Interpretation

The smoke proves the operator can redrive a known DLQ audit event without publishing Kafka directly. The event returns to the same ordered relay path, so relay ordering and retry semantics remain centralized in the outbox relay.

The operator is intentionally narrow. Operators must inspect the event before redrive; blindly repairing poison payloads can re-enter retry or DLQ. Retention policy, external audit export, bulk repair and approval UI remain future work.
