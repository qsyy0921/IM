# delivery-service gRPC mTLS Smoke - 2026-06-13

## Conclusion

This smoke verifies the first-stage `delivery-service` gRPC mTLS path with gateway verified metadata auth:

```text
delivery-service gRPC server
-> TLS enabled
-> client certificate required
-> client DNS SAN allowlist = push-gateway.nexusim.local
-> PullInbox via verified metadata
-> AckDelivery via verified metadata
-> device_delivery_cursors / delivery_outbox
```

It is not a full service-mesh rollout, certificate rotation test, or capacity result.

## Raw Result

- Raw directory: `H:\NexusIM\loadtest-results\delivery-mtls-smoke-20260613-201046`
- Summary: `H:\NexusIM\loadtest-results\delivery-mtls-smoke-20260613-201046\delivery-summary.json`
- Tested code commit: `e42d7689b72ebd2d8d7538502aba8307f7b09152`
- `git_dirty=false`

## Setup

The smoke generated a short-lived local CA and certificates under the raw result directory:

- Server cert SAN: `delivery-service.nexusim.local`, `localhost`, `127.0.0.1`
- Client cert SAN: `push-gateway.nexusim.local`
- Client cert URI SAN: `spiffe://nexusim/push-gateway`

The delivery-service process was started with:

```text
NEXUSIM_DELIVERY_SERVICE_MODE=grpc
NEXUSIM_DELIVERY_GRPC_ADDR=127.0.0.1:12697
NEXUSIM_DELIVERY_AUTH_MODE=metadata
NEXUSIM_DELIVERY_GRPC_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=push-gateway.nexusim.local
NEXUSIM_DELIVERY_GRPC_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/push-gateway
```

The runner used `--verified-auth-metadata` plus client CA/server-name/client-cert/client-key flags.

## Evidence

Key summary fields:

```json
{
  "tls_enabled": true,
  "verified_auth_metadata": true,
  "poll_count": 1,
  "item_count": 1,
  "max_seq": 1,
  "ack_last_received_seq": 1,
  "success": true,
  "pull_p99_ms": 36.617,
  "ack_latency_ms": 8.41,
  "inbox_count": 1,
  "delivery_outbox_total": 1,
  "delivery_outbox_pending": 1,
  "delivery_outbox_dlq": 0,
  "cursor_last_received_seq": 1
}
```

The pulled item was:

```text
conversation_seq=1
event_id=evt-delivery-mtls-1
event_type=message.persisted.v1
message_id=msg-delivery-mtls-1
sender_id=sender-delivery-mtls
```

## Interpretation

- The gRPC server accepted only a client certificate whose identity matched the configured allowlist.
- `PullInbox` and `AckDelivery` used gateway verified metadata, so `tenant_id/user_id/device_id/session_id` came from trusted metadata rather than caller-controlled request body identity.
- `AckDelivery` advanced `device_delivery_cursors.last_received_seq` to `1`.
- `delivery_outbox_pending=1` is expected in this smoke because the delivery outbox relay was not started; this test only validates the gRPC Pull/Ack path under mTLS.

## Limits

- No certificate issuance, rotation, revocation, or distribution workflow was tested.
- No dynamic service identity, SPIFFE control plane, service mesh, or mTLS policy rollout was tested.
- No delivery outbox relay was started, so `delivery.ack.recorded.v1` remained `PENDING`.
- This is a single-process local smoke, not a HA or capacity test.
