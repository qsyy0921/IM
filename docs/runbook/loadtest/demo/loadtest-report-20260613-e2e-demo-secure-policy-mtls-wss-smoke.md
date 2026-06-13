# NexusIM E2E Demo Secure Policy mTLS/WSS Smoke - 2026-06-13

## Conclusion

This smoke passed. It extends the secure local E2E demo by replacing message-service's local policy mock with a real `policy-service` gRPC process over mTLS, then verifies the policy audit event is published to Kafka and can be decoded as a typed `PolicyEvent`.

Covered chain:

```text
CreateMemberChange(JOIN)
-> SendMessage
   -> message-service -> conversation-service over mTLS
   -> message-service -> policy-service over mTLS
   -> policy_decision_audit_outbox -> per-run im.policy.events demo topic
   -> typed Kafka read-back of policy.message_action_decision.v1
-> delivery.notify over WSS/mTLS
-> PullInbox
-> delivery.ack
-> MarkRead
-> ListConversations unread 1 -> 0
```

This is still a local smoke. It is not production certificate lifecycle, dynamic workload identity, service mesh rollout, Kafka/PostgreSQL HA, or capacity evidence.

## Run

Command:

```powershell
. .\tools\go-env.ps1
.\loadtest\demo\run-local-secure-demo.ps1 -SkipBuild -RunName e2e-demo-secure-policy-readback-20260613-final
```

Raw result:

```text
H:\NexusIM\loadtest-results\e2e-demo-secure-policy-readback-20260613-final
```

Commit under test:

```text
6b525e91152233b8e795b723fafa8f35b1ccdfd0
git_dirty=false
```

Policy Kafka read-back:

```text
policy_topic=im.policy.events.demo.secure.20260613-212756
policy_audit_kafka.event_count=1
policy_audit_kafka.event_type=policy.message_action_decision.v1
policy_audit_kafka.producer=policy-service
policy_audit_kafka.allowed=true
policy_audit_kafka.permission_version=2
policy_audit_kafka.classification=POLICY_DEMO_ALLOWED
push_url=wss://127.0.0.1:11898
```

## Evidence

`e2e-demo-summary.json`:

```json
{
  "commit": "6b525e9",
  "git_dirty": false,
  "conversation_tls_enabled": true,
  "message_tls_enabled": true,
  "delivery_tls_enabled": true,
  "receipt_tls_enabled": true,
  "push_tls_enabled": true,
  "verified_auth_metadata": true,
  "policy_audit_kafka": {
    "topic": "im.policy.events.demo.secure.20260613-212756",
    "event_count": 1,
    "event_id": "8a34a06c-da68-4bd1-9061-673dcd67c7db",
    "event_type": "policy.message_action_decision.v1",
    "producer": "policy-service",
    "allowed": true,
    "permission_version": 2,
    "classification": "POLICY_DEMO_ALLOWED"
  },
  "success": true
}
```

Functional result:

```text
server.hello succeeded
CreateMemberChange(JOIN) boundary_seq=1
SendMessage conversation_seq=2
delivery.notify received over WSS, source_event_type=message.persisted.v1
PullInbox item_count=1, max_seq=2
delivery.ack.ok last_received_seq=2
MarkRead last_read_seq=2
ListConversations unread_count 1 -> 0
```

Policy-service evidence for the smoke tenant:

```text
policy_decision_audit_outbox PUBLISHED=1
Kafka read-back event_count=1
event_type=policy.message_action_decision.v1
producer=policy-service
allowed=true
permission_version=2
classification=POLICY_DEMO_ALLOWED
status=PUBLISHED
```

PostgreSQL outbox state for the smoke tenant:

```text
message_outbox                PUBLISHED=2
policy_decision_audit_outbox  PUBLISHED=1
delivery_outbox               PUBLISHED=2
receipt_outbox                PUBLISHED=2
```

Runner summary:

```text
user_inbox_count=1
device_delivery_cursor_seq=2
user_read_cursor_seq=2
user_conversation_summaries=1
```

## Script Changes

`loadtest/demo/run-local-secure-demo.ps1` now:

- builds and starts `policy-service`;
- applies all PostgreSQL policy migrations;
- creates a per-run `im.policy.events.demo.secure.*` topic;
- starts `policy-service` gRPC with server TLS, required client cert, and message-service client SAN allowlist;
- starts `policy-service` audit outbox relay;
- configures message-service to call policy-service over mTLS;
- asserts the smoke tenant has at least one `policy_decision_audit_outbox` row published;
- reads back at least one Kafka policy audit event and decodes it as `policy.message_action_decision.v1`.

## Limits

This smoke does not prove:

- full API gateway deployment;
- production certificate issuance, rotation, revocation, or distribution;
- dynamic SPIFFE/SPIRE or service mesh identity;
- product-grade policy authoring or ReBAC;
- policy audit retention or external audit sink;
- broad policy audit replay or repair workflows;
- multi-host mTLS;
- Kafka/PostgreSQL HA;
- capacity under load.

It is a stronger local secure E2E demonstration path: the send path now includes a real policy-service dependency instead of a local policy mock, and the policy audit decision is verified through both PostgreSQL outbox state and typed Kafka read-back.
