# policy-service contact block decision smoke - 2026-06-13

## Scope

This smoke verifies that contact block projection can affect a message policy decision for direct conversations:

```text
conversation-service GetSendContext(direct_peer_user_id)
-> message-service PolicyCheckPort
-> policy-service CheckMessageAction(direct_peer_user_id)
-> policy_contact_edges_projection
-> allow / CONTACT_BLOCKED deny
```

It is a functional smoke, not a capacity result. The test uses `policy-service` PostgreSQL evaluator by setting `NEXUSIM_POLICY_RULES_ENABLED=true`; that evaluator now checks hard contact-block denies before exact message action rules and static default.

## Run

Command:

```powershell
.\loadtest\policycontacts\run-local-smoke.ps1 -RunName policy-contact-block-decision-smoke-20260613
```

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-contact-block-decision-smoke-20260613
```

Summary:

```text
commit=f044069
git_dirty=false
topic=im.contact.events.policy-contact-block-decision-smoke-20260613
consumer_group=nexusim-policy-contact-policy-contact-block-decision-smoke-20260613
success=true
checkpoint_offset_value=3
```

## Evidence

The runner starts two `policy-service` processes:

```text
NEXUSIM_POLICY_SERVICE_MODE=contact-consumer
NEXUSIM_POLICY_SERVICE_MODE=grpc
```

The contact consumer projects contact events into `policy_contact_edges_projection`. The gRPC service checks a `SEND` policy request with `direct_peer_user_id=bob`.

Observed state and decision sequence:

```text
contact.request.accepted.v1
  alice -> bob ACTIVE edge_version=1
  bob -> alice ACTIVE edge_version=1
  CheckMessageAction(SEND, direct_peer_user_id=bob)
  allowed=true classification=POLICY_STATIC_ALLOW permission_version=1

contact.edge.blocked.v1
  alice -> bob BLOCKED edge_version=2
  CheckMessageAction(SEND, direct_peer_user_id=bob)
  allowed=false classification=CONTACT_BLOCKED reason="contact blocked" permission_version=2

contact.edge.unblocked.v1
  alice -> bob ACTIVE edge_version=3
  CheckMessageAction(SEND, direct_peer_user_id=bob)
  allowed=true classification=POLICY_STATIC_ALLOW permission_version=1
```

The blocked decision is a hard deny. It is evaluated before exact message action rules or static default, and it checks either directed edge between the sender and direct peer for `BLOCKED`.

## Boundaries

- The rule applies only when `direct_peer_user_id` is present. Group conversation role / member policy is still future work.
- `conversation-service` derives `direct_peer_user_id` only for `DIRECT` conversations with exactly one other active member. Missing or ambiguous direct peer context fails closed.
- Contact projection is asynchronous. There is still a small lag window between contacts-service publishing a block event and policy-service consuming it.
- Projection DLQ / repair and policy audit outbox remain future hardening.
