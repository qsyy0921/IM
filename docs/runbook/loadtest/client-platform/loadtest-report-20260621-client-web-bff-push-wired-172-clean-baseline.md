# Client Web BFF + Push Wired 172 Clean Baseline - 2026-06-21

## Scope

Focused clean-baseline smoke for the browser MVP path while binding the local
private backend, BFF and push-gateway listeners to the Windows wired `172.x`
interface:

```text
Web client contract -> api-gateway HTTP BFF on 172.31.50.1
-> push-gateway WebSocket on 172.31.50.1
-> delivery-service PullInbox/AckDelivery facts
```

This validates the same client-facing path as the loopback clean baseline, but
uses the wired private address that can be reused for LAN-oriented client work.
It is not a cross-Mac smoke and not a production network test.

## Environment

- Result directory:
  `H:\NexusIM\loadtest-results\client-web-bff-push-wired-172-clean-baseline-20260621-01`
- Summary file: `client-web-summary.json`
- Commit: `4148e3c9ffa3167b3e4d27e196a5525d2072d524`
- `git_dirty`: `false`
- `BindHost`: `172.31.50.1`
- `ClientHost`: `172.31.50.1`
- BFF URL: `http://172.31.50.1:50873`
- Push URL: `ws://172.31.50.1:50871/ws`
- Backend processes: local private non-TLS stack started by
  `loadtest/clientweb/run-local-smoke.ps1`

## Verified Facts

- Sender and receiver registration succeeded.
- Sender and receiver login both returned gateway token, push-gateway token and
  refresh token.
- Receiver WebSocket `server.hello` succeeded through `172.31.50.1`.
- Receiver JOIN created member boundary `seq=1`.
- Sender sent one text message through BFF `POST /api/messages/send`.
- Message result:
  - `message_id=msg_db86101c-52d5-49b1-9004-84c361d47a4c`
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

This is the first clean baseline for the browser MVP client path on the Windows
wired `172.31.50.1` address. It proves the current BFF + push wakeup path can
run on the private LAN-facing listener while preserving the same durable
PullInbox / AckDelivery facts.

## Remaining Work

- Add BFF HTTP-layer metrics and rate-limit adapter.
- Add PC desktop runner and first local Windows installer.
- Add Android runtime shell and first unsigned local APK.
