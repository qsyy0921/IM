# policy-service contact projection smoke - 2026-06-13

## Scope

This smoke verifies the first policy-service contact projection slice:

```text
im.contact.events
-> policy-service contact-consumer
-> policy_contact_edges_projection
-> policy_kafka_checkpoints
```

It does not verify message decision enforcement from contacts. `CheckMessageAction` does not yet receive direct peer / target-user context, so this slice intentionally stops at the policy-owned read model.

## Run

Command:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-contact-projection-smoke-20260613
```

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-contact-projection-smoke-20260613
```

Summary:

```text
commit=b91f410
git_dirty=false
topic=im.contact.events.policy-contact-projection-smoke-20260613
consumer_group=nexusim-policy-contact-policy-contact-projection-smoke-20260613
success=true
checkpoint_offset_value=12
```

## Evidence

The runner started `policy-service` in `NEXUSIM_POLICY_SERVICE_MODE=contact-consumer`, wrote three protobuf `ContactEvent` records to Kafka, and polled PostgreSQL until the projection caught up.

Observed projection states:

```text
contact.request.accepted.v1
  alice -> bob ACTIVE edge_version=1
  bob -> alice ACTIVE edge_version=1

contact.edge.blocked.v1
  alice -> bob BLOCKED edge_version=2
  bob -> alice remains ACTIVE edge_version=1

contact.edge.unblocked.v1
  alice -> bob ACTIVE edge_version=3
```

The final summary recorded:

```json
{
  "after_accepted": {"status": "ACTIVE", "edge_version": 1},
  "after_blocked": {"status": "BLOCKED", "edge_version": 2},
  "after_unblocked": {"status": "ACTIVE", "edge_version": 3},
  "reverse_edge": {"status": "ACTIVE", "edge_version": 1}
}
```

## Boundaries

- This is a small functional smoke, not a capacity result.
- The contact projection is a policy-service owned read model; contacts-service remains the source of truth.
- Unsupported / malformed contact events currently fail closed by stopping the worker without committing the Kafka record; projection DLQ / repair is still future work.
- Contact block / unblock is not yet consumed by `CheckMessageAction`. The next decision slice needs safe direct peer / target-user context or a conversation projection before enforcing blocked-contact send denies.
