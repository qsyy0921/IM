# policy-service Decision Audit Relay Smoke 2026-06-13

## Scope

This smoke verifies the first-stage policy decision audit publish path:

```text
CheckMessageAction
-> policy_decision_audit_outbox
-> policy-service outbox-relay
-> im.policy.events
```

It reuses the contact projection scenario so the three direct `SEND` decisions cover allow, contact-block deny, and allow-after-unblock. This is not a capacity test, not a retention policy, not an external audit sink, and not a DLQ repair workflow.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-decision-audit-relay-smoke-20260613-clean
```

Raw summary:

```text
H:\NexusIM\loadtest-results\policy-decision-audit-relay-smoke-20260613-clean\policy-contact-summary.json
```

## Result

```text
commit=10bc901
git_dirty=false
success=true
topic=im.contact.events.policy-decision-audit-relay-smoke-20260613-clean
policy_audit_topic=im.policy.events.policy-decision-audit-relay-smoke-20260613-clean
checkpoint_offset_value=3
policy_decision_audit_outbox_count=3
policy_decision_audit_outbox_status.total=3
policy_decision_audit_outbox_status.published=3
policy_decision_audit_outbox_status.pending=0
policy_decision_audit_outbox_status.dlq=0
policy_audit_kafka_event_count=3
```

Decision evidence:

```text
after accepted: allowed=true, permission_version=1, classification=POLICY_STATIC_ALLOW
after blocked: allowed=false, permission_version=2, classification=CONTACT_BLOCKED
after unblocked: allowed=true, permission_version=1, classification=POLICY_STATIC_ALLOW
```

## Interpretation

The smoke proves that policy-service can publish low-sensitive decision audit events to Kafka through its own outbox relay. The relay uses `im.policy.events` protobuf messages and fails closed on malformed payloads or publish errors through retry / DLQ state transitions.

The audit payload is intentionally narrow: stable object keys, context-present flags, action, allow/deny, permission version, classification, reason code and trace/request correlation. It must not be treated as a message fact source or as a place for raw user/device/session/conversation/message IDs, message content, SQL errors, rule parameters or free-text provider bodies.

## Remaining Work

- DLQ repair and replay operator for policy audit events.
- Retention and external audit sink.
- Production metrics / alerts for policy audit relay lag and DLQ.
- Group / role / tenant / risk policy behavior.
