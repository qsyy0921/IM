# NexusIM E2E Demo Secure mTLS/WSS Smoke - 2026-06-13

## Conclusion

This smoke passed. It proves the local multi-process E2E demo can run with:

- conversation / message / delivery / receipt user-facing gRPC TLS + client certificate auth.
- message-service to conversation-service gRPC mTLS.
- push-gateway WebSocket over WSS with client certificate auth.
- push-gateway to delivery-service AckDelivery gRPC mTLS.
- gateway verified identity metadata on user-facing gRPC calls.

This is not a production certificate lifecycle, service mesh, dynamic workload identity, or capacity result. The script generates short-lived local CA/certs under the raw result directory and runs a single local process set.

## Run

Command:

```powershell
. .\tools\go-env.ps1
.\loadtest\demo\run-local-secure-demo.ps1 -RunName e2e-demo-secure-mtls-wss-20260613-final-r2
```

Raw result:

```text
H:\NexusIM\loadtest-results\e2e-demo-secure-mtls-wss-20260613-final-r2
```

Commit under test:

```text
5eb52d0e2bb92cd5a206ea8b6e7d01fdcc9a60bd
git_dirty=false
```

Topics and groups:

```text
timeline_topic=conversation.timeline.demo.secure.20260613-210429
delivery_topic=im.delivery.events
receipt_topic=im.receipt.events
identity_topic=im.identity.events
delivery_consumer_group=nexusim-delivery-demo-secure-20260613210429
receipt_consumer_group=nexusim-receipt-demo-secure-20260613210429
push_consumer_group=nexusim-push-demo-secure-20260613210429
push_identity_consumer_group=nexusim-push-identity-demo-secure-20260613210429
push_url=wss://127.0.0.1:11898
```

## Evidence

`e2e-demo-summary.json`:

```json
{
  "commit": "5eb52d0",
  "git_dirty": false,
  "conversation_tls_enabled": true,
  "message_tls_enabled": true,
  "delivery_tls_enabled": true,
  "receipt_tls_enabled": true,
  "push_tls_enabled": true,
  "verified_auth_metadata": true,
  "success": true
}
```

Functional chain:

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

PostgreSQL state for the smoke tenant:

```text
message_outbox  PUBLISHED=2
delivery_outbox PUBLISHED=2
receipt_outbox  PUBLISHED=2
```

Runner summary:

```text
user_inbox_count=1
device_delivery_cursor_seq=2
user_read_cursor_seq=2
user_conversation_summaries=1
```

## Script Guardrails

`loadtest/demo/run-local-secure-demo.ps1` adds these guardrails:

- Generates a local CA and service/client certificates in `H:\NexusIM\loadtest-results\<run>\certs`.
- Uses static SAN allowlists for:
  - `api-gateway.nexusim.local`
  - `message-service.nexusim.local`
  - `push-gateway.nexusim.local`
  - `desktop-client.nexusim.local`
- Preflights fixed local ports before starting services.
- Clears process-scope `NEXUSIM_*` variables before each service launch, then injects only the env needed by that service.
- Isolates the `push-gateway all` mode identity consumer with its own `im.identity.events` topic and unique consumer group.
- Waits for message / delivery / receipt outbox rows for the smoke tenant to settle to `PUBLISHED` before stopping processes.

## Limits

This smoke does not prove:

- production certificate issuance, rotation, revocation, or distribution;
- dynamic SPIFFE/SPIRE or service mesh identity;
- multi-host mTLS;
- API gateway deployment;
- Redis route or multi-instance push routing;
- Kafka/PostgreSQL HA;
- capacity under load.

It is a secure local E2E demonstration path for interviews and regression checks.
