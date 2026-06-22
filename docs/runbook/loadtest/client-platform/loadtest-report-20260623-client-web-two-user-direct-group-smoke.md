# NexusIM Client Web Two-User Direct + Group Smoke

Date: 2026-06-23

Scope:

- Local private backend started by `loadtest/clientweb/run-local-smoke.ps1`.
- Public client-facing path only for the verified phase:
  `api-gateway` HTTP BFF + `push-gateway` WebSocket.
- Not a capacity test, production SLO, TLS test, or clean committed baseline.

Source summary:

```text
H:\NexusIM\loadtest-results\client-web-bff-push-smoke-20260623-015417\client-web-summary.json
```

Clean baseline:

```text
H:\NexusIM\loadtest-results\client-web-bff-push-smoke-20260623-015417\client-web-summary.json
commit=6a08fb14
git_dirty=false
success=true
```

Key evidence:

- Setup registered both users through public identity APIs.
- Contact path completed:
  - `sender_active=true`
  - `receiver_active=true`
- Direct chat path completed:
  - `conversation_type=DIRECT`
  - direct `SendMessage` returned `conversation_seq=3`
  - receiver WebSocket received `delivery.notify` for the same message id
  - `PullInbox` returned one direct item at seq `3`
  - `AckDelivery` advanced receiver device cursor to seq `3`
- Group chat path completed:
  - group conversation created through BFF
  - receiver JOIN completed through conversation-service member change
  - group `SendMessage` returned `conversation_seq=3`
  - receiver WebSocket received `delivery.notify` for the same message id
  - `PullInbox` returned one group item at seq `3`
  - `AckDelivery` advanced receiver device cursor to seq `3`
- `ListConversations` for receiver returned both the group and direct
  conversations after the two messages.

Fixes validated by this smoke:

- Conversation creation member boundary events now include `change_id`, so the
  message-service outbox relay can build and publish the Kafka timeline event.
- `loadtest/clientweb` now verifies both friend-direct and group first paths in
  the same run.

Limits:

- Android WebView login and Windows installer packaging were not part of this
  run.
- This run does not validate production HA, public TLS, long soak, sizing or
  capacity.
