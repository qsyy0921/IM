# NexusIM E2E Demo API Gateway Secure Smoke - 2026-06-13

## Conclusion

This smoke passed on clean commit `cff1668`. It extends the secure local E2E demo by routing all user-facing gRPC calls through the real `api-gateway` instead of calling conversation / message / delivery / receipt directly.

Covered chain:

```text
desktop demo runner
-> api-gateway over gRPC mTLS with HMAC gateway token
-> conversation-service / message-service / delivery-service / receipt-service over downstream mTLS
-> push-gateway delivery.notify over WSS/mTLS
-> PullInbox
-> delivery.ack
-> MarkRead
-> ListConversations unread 1 -> 0
```

The gateway validates the gateway token, rewrites request `AuthContext`, injects trusted metadata for downstream services, and uses an `api-gateway.nexusim.local` client certificate when calling the backend gRPC services.

This is still a local smoke. It is not production certificate lifecycle, service mesh identity, rate limiting, WAF, OpenTelemetry rollout, HA, or capacity evidence.

## Run

Command:

```powershell
. .\tools\go-env.ps1
.\loadtest\demo\run-local-secure-demo.ps1 -RunName e2e-demo-api-gateway-secure-smoke-20260613-clean
```

Raw result:

```text
H:\NexusIM\loadtest-results\e2e-demo-api-gateway-secure-smoke-20260613-clean
```

Commit under test:

```text
cff16688ecf942cac92df06c4414ba9da9cff3d9
git_dirty=false
```

## Evidence

`e2e-demo-summary.json`:

```json
{
  "commit": "cff1668",
  "git_dirty": false,
  "conversation_tls_enabled": true,
  "message_tls_enabled": true,
  "delivery_tls_enabled": true,
  "receipt_tls_enabled": true,
  "push_tls_enabled": true,
  "verified_auth_metadata": false,
  "gateway_auth_mode": "hmac",
  "success": true
}
```

Functional result:

```text
api-gateway gRPC listening on 127.0.0.1:11903
server.hello succeeded
CreateMemberChange(JOIN) boundary_seq=1
SendMessage conversation_seq=2
delivery.notify received over WSS/mTLS, source_event_type=message.persisted.v1
PullInbox item_count=1, max_seq=2
delivery.ack.ok last_received_seq=2
MarkRead last_read_seq=2
ListConversations unread_count 1 -> 0
```

Policy audit Kafka read-back:

```text
topic=im.policy.events.demo.secure.20260613-231556
event_count=1
event_type=policy.message_action_decision.v1
producer=policy-service
allowed=true
permission_version=2
classification=POLICY_DEMO_ALLOWED
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

## What Changed

`loadtest/demo/run-local-secure-demo.ps1` now starts `api-gateway` in gRPC mode with inbound mTLS and points the demo runner's conversation / message / delivery / receipt targets to `127.0.0.1:11903`.

The runner uses `--gateway-auth-mode hmac` and presents a desktop client certificate to the gateway. The gateway calls downstream services over mTLS and injects trusted identity metadata; the runner no longer sends trusted metadata directly to the backend services in this secure path.

## Limits

This smoke does not prove:

- production certificate issuance, rotation, revocation, or distribution;
- dynamic workload identity such as SPIFFE/SPIRE;
- public HTTP / REST / GraphQL gateway behavior;
- gateway rate limiting, quota, WAF, or abuse protection;
- centralized access logs, tracing, or alerting;
- multi-host mTLS;
- Kafka/PostgreSQL HA;
- capacity under load.

It proves the local secure demo can now pass through the real `api-gateway` authentication and trusted metadata boundary before reaching backend services.
