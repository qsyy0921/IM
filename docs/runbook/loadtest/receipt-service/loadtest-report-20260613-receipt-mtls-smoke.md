# receipt-service gRPC mTLS Smoke - 2026-06-13

## Conclusion

This smoke verifies `receipt-service` gRPC under first-stage static mTLS while the user-facing RPCs use gateway verified metadata auth:

```text
CreateMemberChange
-> SendMessage
-> delivery projection / PullInbox / AckDelivery
-> receipt-service gRPC mTLS
-> GetReceiptState
-> MarkRead
-> ListConversations / unread filter / archive / pin / mute
-> receipt_outbox
-> im.receipt.events
```

Only `receipt-service` gRPC was run with mTLS in this smoke. Conversation, message, and delivery gRPC remained plaintext but used metadata auth. This is not a full service-mesh rollout, certificate lifecycle test, or capacity result.

## Raw Result

- Raw directory: `H:\NexusIM\loadtest-results\receipt-mtls-smoke-20260613-202446`
- Summary: `H:\NexusIM\loadtest-results\receipt-mtls-smoke-20260613-202446\receipt-summary.json`
- Tested code commit: `c462e572c5bfc156cfb88495c3196582021b15f7`
- `git_dirty=false`

## TLS Setup

The smoke generated short-lived local test certificates in the raw result directory:

- Server cert SAN: `receipt-service.nexusim.local`, `localhost`, `127.0.0.1`
- Client cert DNS SAN: `api-gateway.nexusim.local`
- Client cert URI SAN: `spiffe://nexusim/api-gateway`

The receipt-service gRPC process inherited:

```text
NEXUSIM_RECEIPT_SERVICE_MODE=grpc
NEXUSIM_RECEIPT_GRPC_ADDR=127.0.0.1:11699
NEXUSIM_RECEIPT_AUTH_MODE=metadata
NEXUSIM_RECEIPT_GRPC_TLS_REQUIRE_CLIENT_CERT=true
NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=api-gateway.nexusim.local
NEXUSIM_RECEIPT_GRPC_TLS_CLIENT_ALLOWED_URIS=spiffe://nexusim/api-gateway
```

The runner used `-VerifiedAuthMetadata` and receipt client TLS flags:

```text
-ReceiptTlsCaFile
-ReceiptTlsServerName receipt-service.nexusim.local
-ReceiptTlsClientCertFile
-ReceiptTlsClientKeyFile
```

## Evidence

Key summary fields:

```json
{
  "conversation_tls_enabled": false,
  "message_tls_enabled": false,
  "delivery_tls_enabled": false,
  "receipt_tls_enabled": true,
  "verified_auth_metadata": true,
  "success": true,
  "send_message": {
    "conversation_seq": 2
  },
  "ack_delivery": {
    "last_received_seq": 2
  },
  "receipt_after_read_by_seq": {
    "received_user_count": 1,
    "read_user_count": 1
  },
  "conversation_list_unread_after_read": {
    "item_count": 0
  },
  "receipt_outbox": {
    "total": 3,
    "pending": 0,
    "published": 3,
    "dlq": 0
  },
  "delivery_outbox": {
    "total": 4,
    "pending": 0,
    "published": 4,
    "dlq": 0
  }
}
```

Additional verified behavior:

- `GetReceiptState` before read showed `received_user_count=1` and `read_user_count=0`.
- `MarkRead` advanced `last_read_seq=2`.
- `GetReceiptState` by both `conversation_seq` and `message_id` showed `read_user_count=1`.
- `ListConversations` changed unread from `1` to `0` after `MarkRead`.
- `ListConversations(unread_only=true)` returned `1` item before read and `0` after read.
- Archive, unarchive, pin, unpin, mute, and unmute RPCs completed under the receipt mTLS connection.
- Receipt outbox relay published two `receipt.message.received.v1` events and one `receipt.message.read.v1` event.

## Interpretation

- The receipt-service gRPC server accepted the client certificate whose identity matched the configured DNS / URI allowlist.
- Receipt user-facing RPC identity was sourced from gateway verified metadata rather than caller-controlled request body identity.
- Receipt-service still did not read delivery-service internal tables. It consumed `im.delivery.events` and maintained its own read model.
- The smoke keeps receipt mTLS evidence narrow: it proves the receipt gRPC server/client transport and metadata auth path, not all service-to-service traffic.

## Limits

- Conversation, message, and delivery gRPC were plaintext in this run.
- No certificate issuance, rotation, revocation, or distribution workflow was tested.
- No dynamic service identity, SPIFFE control plane, service mesh, or full mTLS rollout was tested.
- This is a small local smoke, not a HA or capacity test.
