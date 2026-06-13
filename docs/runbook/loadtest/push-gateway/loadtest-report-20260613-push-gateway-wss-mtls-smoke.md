# push-gateway WSS / mTLS Smoke - 2026-06-13

## Conclusion

The `push-gateway` WebSocket entrance passed a real-process `full` smoke over `wss://` with client certificate verification enabled.

This run proves the first-stage static WSS / mTLS path for the online gateway entrance:

```text
delivery_outbox
-> im.delivery.events
-> push-gateway delivery consumer
-> WSS client receives delivery.notify
-> PullInbox
-> delivery.ack
-> push-gateway AckDelivery metadata forwarding
-> delivery.ack.ok
```

It does not prove full service-mesh mTLS, certificate lifecycle automation, browser certificate UX, Redis cross-instance routing, or capacity.

## Evidence

Raw result directory:

```text
H:\NexusIM\loadtest-results\push-gateway-wss-mtls-smoke-20260613-203146
```

Summary file:

```text
H:\NexusIM\loadtest-results\push-gateway-wss-mtls-smoke-20260613-203146\pushgateway-summary.json
```

Code state:

```text
commit=c53ae419b3ebed6082d4ba3447f94b86fa58be7e
git_dirty=false
scenario=full
route_backend=memory
push_tls_enabled=true
verified_auth_metadata=true
push_url=wss://127.0.0.1:11598
push_metrics_url=http://127.0.0.1:11602/debug/metrics
```

TLS setup:

```text
server cert SAN DNS=push-gateway.nexusim.local, localhost
server cert SAN IP=127.0.0.1
client cert SAN DNS=desktop-client.nexusim.local
client cert SAN URI=spiffe://nexusim/desktop-client
client allowlist DNS=desktop-client.nexusim.local
client allowlist URI=spiffe://nexusim/desktop-client
```

The runner connected with:

```text
-PushTlsCaFile
-PushTlsServerName push-gateway.nexusim.local
-PushTlsClientCertFile
-PushTlsClientKeyFile
```

The server was started with:

```text
NEXUSIM_PUSH_WS_TLS_CERT_FILE
NEXUSIM_PUSH_WS_TLS_KEY_FILE
NEXUSIM_PUSH_WS_TLS_CLIENT_CA_FILE
NEXUSIM_PUSH_WS_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_DNS_NAMES=desktop-client.nexusim.local
NEXUSIM_PUSH_WS_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/desktop-client
```

Functional evidence:

```text
server.hello ok
member_join.boundary_seq=1
send_message.conversation_seq=2
delivery.notify.source_event_type=message.persisted.v1
delivery.notify.conversation_seq=2
PullInbox.item_count=1
PullInbox.max_seq=2
delivery.ack.ok.last_received_seq=2
cursor_last_received_seq=2
delivery_outbox_total=2
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
```

The `message_id` matched across `SendMessage`, `delivery.notify`, and `PullInbox`:

```text
msg_c8aa63a0-975e-4383-9950-89a7752b7a6b
```

## Scope Limits

- Only the push-gateway WebSocket listener used WSS / mTLS in this run.
- `conversation-service`, `message-service`, `delivery-service`, and `identity-service` gRPC clients were plaintext in this run.
- `-VerifiedAuthMetadata` was enabled, so user-facing RPC calls used gateway verified metadata where supported.
- The route backend was `memory`, so this is not a Redis route or cross-instance smoke.
- Push auth was still local smoke `mock` auth over query identity; this is not production WebSocket authentication.
- The run used short-lived local test certificates; it does not cover certificate issuance, rotation, revocation, distribution, or dynamic service identity.
- The plaintext debug metrics endpoint is expected in WSS smoke and is not part of the WSS listener.
