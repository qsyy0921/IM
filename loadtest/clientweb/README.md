# NexusIM Client Web Smoke

This runner verifies the first browser-client path through the public client
surfaces:

```text
api-gateway HTTP BFF + push-gateway WebSocket
```

The runner setup phase may call public gRPC APIs to prepare test users and group
membership. The verified client phase uses the same public client-facing BFF
surfaces as the Web / PC shell:

- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/contact-requests/send`
- `POST /api/contact-requests/respond`
- `GET /api/contacts/state`
- `POST /api/conversations/direct`
- `POST /api/conversations/create`
- `GET /api/conversations/{conversation_id}/members`
- `POST /api/conversations/{conversation_id}/members/role`
- `POST /api/conversations/{conversation_id}/owner/transfer`
- `POST /api/conversations/{conversation_id}/members/remove`
- `POST /api/messages/send`
- `GET /api/conversations/{conversation_id}/messages`
- `GET /api/conversations`
- `POST /api/delivery/ack`
- `push-gateway` WebSocket `delivery.notify`

It verifies two user-visible conversations in one run:

- friend request accepted -> direct conversation opened -> direct message visible
  through push + PullInbox + ACK;
- group conversation created -> receiver joined -> group message visible through
  push + PullInbox + ACK.
- group membership actions through BFF: active member list -> role change ->
  owner transfer -> remove previous owner -> final active member list.

It does not read private service tables for product behavior. PostgreSQL is used
only by the loadtest for setup cleanup and final invariant checks.

Run locally:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1
```

Run as a fixed local backend for browser / Windows desktop client debugging:

```powershell
.\loadtest\clientweb\run-local-dev.ps1
```

This wrapper keeps the seeded local services alive on:

- API BFF: `http://127.0.0.1:8080`
- push WebSocket: `ws://127.0.0.1:8088/ws`

The local backend includes `contacts-service` because the browser and desktop
clients use the contact read model for friend lists and direct-chat permission
checks before opening a one-to-one conversation.

Use the first-stage local account:

```text
tenant_id: tenant-client-local
user_id: user-a
password: ClientWebReceiverPassw0rd!
conversation_id: conv-client-local
```

Stop the printed process ids when the local client debug session is finished.

Run against a private LAN address, for example the Windows wired `172.x`
interface:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 `
  -BindHost 172.31.50.1 `
  -ClientHost 172.31.50.1
```

`BindHost` controls where local service listeners bind. `ClientHost` controls
the address used by the smoke client and service-to-service targets. Keep both
on a private address unless the corresponding gateway public-listener guards are
explicitly configured for a separate test.

Raw summaries and logs are written under `H:\NexusIM\loadtest-results` by
default. The repository should only store reports or short runbook summaries.

This local smoke uses private, non-TLS listeners to keep the client path easy to
repeat. It does not replace the existing secure mTLS gateway / push-gateway
smoke reports.
