# NexusIM E2E Demo api-gateway facade-only smoke

## Scope

This smoke verifies the user-facing api-gateway public facade with legacy service descriptors disabled:

```text
desktop client
-> api-gateway nexusim.gateway.v1.GatewayService only
-> downstream conversation/message/delivery/receipt services with trusted metadata
-> push-gateway WebSocket notify
-> delivery PullInbox / AckDelivery
-> receipt MarkRead / ListConversations
```

It is a small local process smoke, not a capacity result and not a production API gateway claim. It proves the secure demo path no longer depends on registering the legacy conversation / message / delivery / receipt descriptors at api-gateway.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\demo\run-local-secure-demo.ps1 `
  -RunName e2e-demo-api-gateway-facade-only-smoke-20260614-clean
```

The script starts local PostgreSQL/Kafka/Redis-backed NexusIM processes, generates short-lived local smoke certificates, starts api-gateway with HMAC token auth, mTLS to downstream services, and:

```text
NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false
```

## Raw Result

```text
H:\NexusIM\loadtest-results\e2e-demo-api-gateway-facade-only-smoke-20260614-clean
```

Important files:

- `e2e-demo-summary.json`
- `api-gateway-debug-metrics.json`

## Result Summary

From `e2e-demo-summary.json`:

```text
commit=c1328ca
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

- This does not add REST/GraphQL/BFF behavior.
- This does not remove legacy descriptors from production by default; default remains compatible unless `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false` is configured.
- This does not prove production capacity, HA, certificate lifecycle, unified tracing, tenant quota, WAF, or full API gateway governance.
- It proves the public facade can drive the current secure E2E demo path while api-gateway only registers `nexusim.gateway.v1.GatewayService`.
