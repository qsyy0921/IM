# NexusIM E2E Demo API Gateway Audience Smoke - 2026-06-13

## Conclusion

This smoke passed on clean commit `9335bd1`. It verifies that the real `api-gateway` now defaults to the `api-gateway` token audience, and that the secure E2E demo runner signs its HMAC gateway token with `aud=api-gateway`.

Covered chain:

```text
desktop demo runner
-> api-gateway over gRPC mTLS with HMAC gateway token aud=api-gateway
-> conversation-service / message-service / delivery-service / receipt-service over downstream mTLS
-> push-gateway delivery.notify over WSS/mTLS
-> PullInbox
-> delivery.ack
-> MarkRead
-> ListConversations unread 1 -> 0
```

This is still a local smoke. It is not production certificate lifecycle, service mesh identity, rate limiting, WAF, OpenTelemetry rollout, HA, or capacity evidence.

## Run

Command:

```powershell
.\loadtest\demo\run-local-secure-demo.ps1 -RunName e2e-demo-api-gateway-audience-smoke-20260613-clean
```

Raw result:

```text
H:\NexusIM\loadtest-results\e2e-demo-api-gateway-audience-smoke-20260613-clean
```

Commit under test:

```text
9335bd18b702532cf462fe30aa50f570f92baa9c
git_dirty=false
```

## Evidence

`e2e-demo-summary.json`:

```json
{
  "commit": "9335bd1",
  "git_dirty": false,
  "success": true,
  "gateway_auth_mode": "hmac",
  "gateway_auth_audience": "api-gateway",
  "verified_auth_metadata": false,
  "conversation_tls_enabled": true,
  "message_tls_enabled": true,
  "delivery_tls_enabled": true,
  "receipt_tls_enabled": true,
  "push_tls_enabled": true
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
topic=im.policy.events.demo.secure.20260613-233623
event_count=1
event_type=policy.message_action_decision.v1
producer=policy-service
allowed=true
permission_version=2
classification=POLICY_DEMO_ALLOWED
```

api-gateway debug metrics:

```text
total_requests=7
total_errors=1
CreateMemberChange OK=1
SendMessage OK=1
PullInbox OK=1
MarkRead OK=1, FailedPrecondition=1
ListConversations OK=2
auth_jwks.remote_url_configured=false
auth_jwks.cached_key_count=0
auth_jwks.refresh_failure_count=0
```

The single `MarkRead FailedPrecondition` is expected in the demo flow: the runner first probes the read path before the final successful read cursor advance. The final functional state is `last_read_seq=2` and `unread_count=0`.

Runner summary:

```text
user_inbox_count=1
device_delivery_cursor_seq=2
user_read_cursor_seq=2
user_conversation_summaries=1
```

## What Changed

The api-gateway default audience is now `api-gateway` instead of `push-gateway`. The secure demo runner also passes `--gateway-auth-audience api-gateway`, and the runner summary records the audience used for the generated gateway token.

The previous `push-gateway` audience remains available only as an explicit compatibility setting through `NEXUSIM_API_GATEWAY_AUTH_AUDIENCE=push-gateway` or the demo runner's `--gateway-auth-audience push-gateway`.

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

It proves the local secure demo can use a token audience dedicated to api-gateway without reusing the push-gateway audience by default.
