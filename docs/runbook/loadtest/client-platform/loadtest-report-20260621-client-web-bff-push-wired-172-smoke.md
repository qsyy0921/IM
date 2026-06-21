# Client Web BFF + Push Wired 172 WIP Smoke - 2026-06-21

## Scope

Focused WIP smoke for the browser MVP path while binding the local private
backend, BFF and push-gateway listeners to the Windows wired `172.x` interface:

```text
Web client contract -> api-gateway HTTP BFF on 172.31.50.1
-> push-gateway WebSocket on 172.31.50.1
-> delivery-service PullInbox/AckDelivery facts
```

This proves the runner and first client path can use a private wired LAN
address. It is not a cross-Mac smoke and not a production network test.

## Environment

- Result directory:
  `H:\NexusIM\loadtest-results\client-web-bff-push-wired-172-smoke-20260621-01`
- Summary file: `client-web-summary.json`
- Commit: `83cf70a5301570124cd13b4c2c2eaa3adf5e5787`
- `git_dirty`: `true`
- `BindHost`: `172.31.50.1`
- `ClientHost`: `172.31.50.1`
- BFF URL: `http://172.31.50.1:55470`
- Push URL: `ws://172.31.50.1:55468/ws`
- Backend processes: local private non-TLS stack started by
  `loadtest/clientweb/run-local-smoke.ps1`

Because the script change adding `BindHost` / `ClientHost` was still
uncommitted, this run is WIP evidence only. A clean committed rerun is still
required before this becomes the wired-address baseline.

## Verified Facts

- Sender and receiver registration succeeded.
- Sender and receiver login both returned gateway token, push-gateway token and
  refresh token.
- Receiver WebSocket `server.hello` succeeded through `172.31.50.1`.
- Receiver JOIN created member boundary `seq=1`.
- Sender sent one text message through BFF `POST /api/messages/send`.
- Message result:
  - `message_id=msg_8d5d8dd9-70cd-4bdd-bdfc-5ef05ead17f3`
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

Passed as WIP evidence.

## Remaining Work

- Commit the script and documentation updates.
- Rerun the same `BindHost=172.31.50.1` / `ClientHost=172.31.50.1` smoke from a
  clean committed worktree and archive it as the clean wired baseline.
- Then continue BFF HTTP-layer metrics / rate-limit adapter.
