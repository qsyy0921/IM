# NexusIM Client Web Smoke

This runner verifies the first browser-client path through the public client
surfaces:

```text
api-gateway HTTP BFF + push-gateway WebSocket
```

The runner setup phase may call public gRPC APIs to prepare test users and
conversation membership. The verified client phase uses only:

- `POST /api/auth/login`
- `POST /api/messages/send`
- `GET /api/conversations/{conversation_id}/messages`
- `GET /api/conversations`
- `POST /api/delivery/ack`
- `push-gateway` WebSocket `delivery.notify`

It does not read private service tables for product behavior. PostgreSQL is used
only by the loadtest for setup cleanup and final invariant checks.

Run locally:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1
```

Raw summaries and logs are written under `H:\NexusIM\loadtest-results` by
default. The repository should only store reports or short runbook summaries.

This local smoke uses private, non-TLS listeners to keep the client path easy to
repeat. It does not replace the existing secure mTLS gateway / push-gateway
smoke reports.
