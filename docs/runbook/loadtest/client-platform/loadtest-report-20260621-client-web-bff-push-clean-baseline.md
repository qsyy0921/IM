# Client Web BFF + Push Clean Baseline - 2026-06-21

## Scope

Focused clean-baseline smoke for the browser MVP path:

```text
Web client contract -> api-gateway HTTP BFF -> push-gateway WebSocket
-> delivery-service PullInbox/AckDelivery facts
```

This run validates the same path as the first WIP smoke, but from a clean
committed worktree.

## Environment

- Result directory: `H:\NexusIM\loadtest-results\client-web-bff-push-smoke-20260621-213936`
- Summary file: `client-web-summary.json`
- Commit: `6069b45ab5eed134c272d895caab31e3ac3bcc71`
- `git_dirty`: `false`
- Backend processes: local private non-TLS stack started by
  `loadtest/clientweb/run-local-smoke.ps1`
- Policy note: this client-path smoke uses message-service static policy and a
  real conversation-service send context. Dedicated policy-service smokes remain
  responsible for policy projection semantics.

## Verified Facts

- Sender and receiver registration succeeded.
- Sender and receiver login both returned gateway token, push-gateway token and
  refresh token.
- Receiver WebSocket `server.hello` succeeded.
- Receiver JOIN created member boundary `seq=1`.
- Sender sent one text message through BFF `POST /api/messages/send`.
- Message result:
  - `message_id=msg_5dcc34a5-a322-4847-9023-3e80e3d90643`
  - `conversation_seq=2`
- Push gateway delivered `delivery.notify` for the same message:
  - `conversation_seq=2`
  - `source_event_type=message.persisted.v1`
  - `pull_required=true`
- BFF `PullInbox` returned one item with `max_seq=2`.
- BFF conversation list returned one conversation with `last_visible_seq=2`
  and `unread_count=1`.
- BFF `AckDelivery` advanced receiver device cursor to `last_received_seq=2`.
- PostgreSQL verification:
  - `user_inbox_count=1`
  - `device_delivery_cursor_seq=2`

## Result

Passed.

This is the first clean baseline for the browser MVP client path. It proves the
gateway/BFF, push wakeup and durable delivery read model can work together from
the browser-facing contract without directly calling internal services.

## Remaining Work

- Repeat the smoke against the Windows / Mac wired LAN address.
- Add BFF HTTP-layer metrics and rate-limit adapter.
- Add PC desktop runner and first local Windows installer.
- Add Android runtime shell and first unsigned local APK.
