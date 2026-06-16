# Push Gateway Stack Capacity Baseline - 2026-06-16

## Scope

This is a local short baseline for the push-gateway `full` scenario. It is not a WebSocket capacity limit, production SLO, or long-running resource curve.

Covered path:

```text
conversation-service CreateMemberChange
-> message-service SendMessage
-> message outbox relay
-> delivery timeline projection
-> delivery_outbox
-> delivery-service outbox relay
-> Kafka im.delivery.events
-> push-gateway delivery consumer
-> WebSocket delivery.notify
-> delivery-service PullInbox
-> WebSocket delivery.ack
-> delivery-service AckDelivery
-> delivery.ack.ok
```

## Raw Result

Raw output is stored outside the Git workspace:

```text
H:\NexusIM\loadtest-results\capacity-baseline-push-gateway-stack-clean-20260616
```

Summary file:

```text
H:\NexusIM\loadtest-results\capacity-baseline-push-gateway-stack-clean-20260616\pushgateway-summary.json
```

## Result

The smoke passed on clean commit `d29bfb1363d3cd5481726bef1b74aabb0d4edbf0`.

```text
git_dirty = false
scenario = full
route_backend = memory
push_auth_mode = mock
success = true
```

Functional evidence:

```text
server.hello succeeded
CreateMemberChange JOIN boundary_seq = 1
SendMessage conversation_seq = 2
delivery.notify received for seq = 2
PullInbox item_count = 1, max_seq = 2
delivery.ack.ok last_received_seq = 2
device_delivery_cursor seq = 2
delivery_outbox total = 2
delivery_outbox published = 2
delivery_outbox pending = 0
delivery_outbox dlq = 0
```

Capacity summary:

```text
duration_ms = 1087.886
device_count = 1
message_count = 1
notify_frame_count = 1
ack_frame_count = 1
pull_inbox_item_count = 1
delivery_outbox_published = 2
messages_per_second = 0.9192136237754237
notify_frames_per_second = 0.9192136237754237
ack_frames_per_second = 0.9192136237754237
pull_items_per_second = 0.9192136237754237
```

## Limitations

- This is a one-user, one-device, one-message short baseline.
- It uses local `memory` route backend and mock query identity transport.
- It does not measure Redis route throughput, cross-instance route throughput, slow-client behavior, Sentinel behavior, WSS/mTLS, JWT auth, or production HA.
- It does not replace long-running WebSocket connection capacity tests or resource saturation curves.
