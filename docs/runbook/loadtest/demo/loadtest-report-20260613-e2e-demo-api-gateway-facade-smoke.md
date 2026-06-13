# NexusIM E2E Demo api-gateway facade smoke

## Scope

This smoke verifies the first-stage public api-gateway facade path:

```text
desktop client
-> api-gateway nexusim.gateway.v1.GatewayService
-> downstream conversation/message/delivery/receipt services with trusted metadata
-> push-gateway WebSocket notify
-> delivery PullInbox / AckDelivery
-> receipt MarkRead / ListConversations
```

It is a small local process smoke, not a capacity result and not a production API gateway claim. Legacy conversation / message / delivery / receipt descriptors are still registered for compatibility; this run proves the demo can use the new `GatewayService` facade instead.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\demo\run-local-secure-demo.ps1 `
  -RunName e2e-demo-api-gateway-facade-smoke-20260613-clean
```

The script starts local PostgreSQL/Kafka-backed NexusIM processes, generates short-lived local smoke certificates, starts api-gateway with HMAC token auth and mTLS to downstream services, and runs `loadtest/demo` with `--gateway-facade`.

## Raw Result

```text
H:\NexusIM\loadtest-results\e2e-demo-api-gateway-facade-smoke-20260613-clean
```

Important files:

- `e2e-demo-summary.json`
- `api-gateway-debug-metrics.json`

## Result Summary

From `e2e-demo-summary.json`:

```text
commit=bb13300
git_dirty=false
success=true
gateway_facade=true
gateway_auth_mode=hmac
gateway_auth_audience=api-gateway
conversation_tls_enabled=true
message_tls_enabled=true
delivery_tls_enabled=true
receipt_tls_enabled=true
push_tls_enabled=true
```

Functional evidence:

```text
CreateMemberChange(JOIN) boundary_seq=1
SendMessage conversation_seq=2
WebSocket delivery.notify conversation_seq=2
PullInbox item_count=1 max_seq=2 event_type=message.persisted.v1
delivery.ack.ok last_received_seq=2
MarkRead last_read_seq=2
ListConversations unread_count 1 -> 0
user_inbox_count=1
device_delivery_cursor_seq=2
user_read_cursor_seq=2
policy_audit_kafka event_count=1 allowed=true permission_version=2
```

From `api-gateway-debug-metrics.json`, every user-facing gRPC call in this run used the facade service name:

```text
/nexusim.gateway.v1.GatewayService/CreateMemberChange OK=1
/nexusim.gateway.v1.GatewayService/SendMessage OK=1
/nexusim.gateway.v1.GatewayService/PullInbox OK=1
/nexusim.gateway.v1.GatewayService/ListConversations OK=2
/nexusim.gateway.v1.GatewayService/MarkRead FailedPrecondition=1 OK=1
```

The one `MarkRead FailedPrecondition` is expected in this demo runner: it retries until the receipt projection catches up, then succeeds.

## Boundaries

- This does not disable legacy service descriptors yet.
- This does not add REST/GraphQL/BFF behavior.
- This does not prove production capacity, HA, certificate lifecycle, unified tracing, tenant quota, WAF, or full API gateway governance.
- It proves the public facade can drive the current secure E2E demo path without exposing the service-internal `GetSendContext` through the facade.
