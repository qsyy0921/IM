# policy-service decision audit outbox smoke - 2026-06-13

## Scope

This smoke verifies the first-stage local policy decision audit outbox:

```text
policy-service CheckMessageAction
-> final allow / deny decision
-> policy_decision_audit_outbox PENDING row
```

It reuses the direct contact-block decision path so the smoke also covers contact projection input:

```text
im.contact.events
-> policy_contact_edges_projection
-> CheckMessageAction(SEND, direct_peer_user_id)
-> CONTACT_BLOCKED hard deny
-> policy_decision_audit_outbox
```

This is a functional smoke, not a capacity result. It proves that policy decisions can be durably staged for audit inside policy-service's own PostgreSQL boundary. It does not prove a Kafka audit relay, DLQ repair, SIEM sink or compliance UI.

## Run

Command:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-decision-audit-outbox-smoke-20260613
```

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-decision-audit-outbox-smoke-20260613
```

Summary:

```text
commit=53d3758
git_dirty=false
topic=im.contact.events.policy-decision-audit-outbox-smoke-20260613
consumer_group=nexusim-policy-contact-policy-decision-audit-outbox-smoke-20260613
success=true
checkpoint_offset_value=3
policy_decision_audit_outbox_count=3
```

## Evidence

The runner starts:

```text
NEXUSIM_POLICY_SERVICE_MODE=contact-consumer
NEXUSIM_POLICY_SERVICE_MODE=grpc
NEXUSIM_POLICY_RULES_ENABLED=true
```

Observed decision sequence:

```text
accepted contact:
  allowed=true
  classification=POLICY_STATIC_ALLOW
  permission_version=1

blocked edge:
  allowed=false
  classification=CONTACT_BLOCKED
  reason=contact blocked
  permission_version=2

unblocked edge:
  allowed=true
  classification=POLICY_STATIC_ALLOW
  permission_version=1
```

`policy_decision_audit_outbox_count=3` proves all three public policy decisions staged an audit event.

Manual SQL spot-check confirmed the audit row stores low-sensitive fields:

```text
actor_user_key=<stable key>
device_key=<stable key>
conversation_key=<stable key>
direct_peer_key=<stable key>
direct_peer_context_present=true
classification=POLICY_STATIC_ALLOW / CONTACT_BLOCKED
reason_code=CONTACT_BLOCKED for deny
```

The row does not store raw `session_id`, raw `device_id`, raw direct peer user id, raw conversation id, raw message content, rule parameters, SQL errors or free-text provider/body data. `reason` remains part of the public gRPC response, but the audit outbox stores a bounded `reason_code`.

## Boundaries

- Audit outbox is enabled in `policy-service grpc` when `NEXUSIM_POLICY_RULES_ENABLED=true` and PostgreSQL is configured.
- Audit write failure fails closed as `codes.Unavailable / policy unavailable`; the decision is not returned as successful without audit staging.
- Rows remain `PENDING`; no `policy.audit.events` Kafka schema, relay, retry/DLQ smoke or repair operator is implemented in this slice.
- Object identifiers are stored as stable keys, not as raw ids. This is low-sensitive operational audit, not anonymization or legal compliance proof.
- Full policy audit productization still needs relay, DLQ/repair, retention, access control, alerting and an external audit/SIEM sink.
