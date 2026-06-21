# Client Web BFF + Push Smoke Report - 2026-06-21

## Scope

Focused client-platform MVP smoke:

```text
Web client contract -> api-gateway HTTP BFF -> push-gateway WebSocket
-> delivery-service PullInbox/AckDelivery facts
```

This run verifies the browser-facing protocol path. It is not a capacity test,
not a public TLS gateway test, and not a PC/Android packaging test.

## Environment

- Result directory: `H:\NexusIM\loadtest-results\client-web-bff-push-smoke-20260621-213332`
- Summary file: `client-web-summary.json`
- Commit in summary: `589a6e4d21b3e3abadfe2c00dd5de2e083ff1152`
- `git_dirty`: `true`
- Backend processes: local private non-TLS stack started by
  `loadtest/clientweb/run-local-smoke.ps1`
- Policy note: this client-path smoke uses message-service static policy and a
  real conversation-service send context. Dedicated policy-service smokes remain
  responsible for policy projection semantics.

## Verified Facts

- Sender and receiver were registered through public identity/gateway paths.
- Sender and receiver login returned gateway token, push-gateway token and
  refresh token.
- Receiver WebSocket `server.hello` succeeded.
- Receiver JOIN created member boundary `seq=1`.
- Sender sent one message through BFF `POST /api/messages/send`.
- Message result:
  - `message_id=msg_289a9d18-aa46-4f60-b5c4-cf4422a0c899`
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

This proves the first browser MVP client path is wired end-to-end through the
gateway/BFF and push wakeup layer while still using delivery-service durable
inbox as the message fact source.

## Remaining Work

- Re-run once after the current client-platform WIP is committed to produce a
  clean baseline report.
- Add BFF HTTP-layer metrics and rate-limit adapter.
- Repeat the smoke over the wired LAN address.
- Add PC desktop runner and Android runtime shell / unsigned APK.
