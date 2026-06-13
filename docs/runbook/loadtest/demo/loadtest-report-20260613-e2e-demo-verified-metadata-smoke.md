# NexusIM E2E demo verified metadata smoke

Date: 2026-06-13

## Conclusion

This smoke verifies the public demo path with gateway verified identity metadata on user-facing gRPC calls:

```text
CreateMemberChange(metadata auth)
-> SendMessage(metadata auth)
-> message outbox relay
-> delivery projection
-> push-gateway delivery.notify
-> PullInbox(metadata auth)
-> delivery.ack over WebSocket
-> AckDelivery(metadata auth forwarding)
-> MarkRead(metadata auth)
-> ListConversations(metadata auth)
```

The result is the interview-friendly end-to-end user path: a receiver joins a conversation, receives an online notification, pulls the durable inbox item, acknowledges delivery, marks the message read, and sees unread count drop from 1 to 0.

This is still a local small smoke. It is not a capacity result, not an API gateway implementation, and not a full mTLS rollout.

## Runtime Setup

The smoke used temporary local processes on ports `11795-11799`:

```text
message-service grpc              127.0.0.1:11795
conversation-service grpc         127.0.0.1:11796
delivery-service grpc             127.0.0.1:11797
push-gateway ws/all               127.0.0.1:11798
receipt-service grpc              127.0.0.1:11799
```

Kafka topics and groups:

```text
timeline_topic=conversation.timeline.demo.20260613-190232
delivery_topic=im.delivery.events
receipt_topic=im.receipt.events
delivery_consumer_group=nexusim-demo-delivery-20260613-190232
receipt_consumer_group=nexusim-demo-receipt-20260613-190232
push_consumer_group=nexusim-demo-push-20260613-190232
```

`push-gateway` intentionally consumed the fixed `im.delivery.events` topic. The smoke reset this run's receipt and push consumer groups to latest before generating new events, so historical development events were not part of the evidence.

## Runner Command

The final runner invocation was equivalent to:

```powershell
.\loadtest\demo\run-local-demo.ps1 `
  -SkipBuild `
  -VerifiedAuthMetadata `
  -ConversationTarget 127.0.0.1:11796 `
  -MessageTarget 127.0.0.1:11795 `
  -DeliveryTarget 127.0.0.1:11797 `
  -ReceiptTarget 127.0.0.1:11799 `
  -PushUrl ws://127.0.0.1:11798 `
  -RunName e2e-demo-verified-metadata-smoke-20260613-190232 `
  -TenantId tenant-e2e-demo-verified-metadata-smoke-20260613-190232 `
  -ConversationId conv-e2e-demo-verified-metadata-smoke-20260613-190232 `
  -ResultRoot H:\NexusIM\loadtest-results
```

## Raw Result

```text
H:\NexusIM\loadtest-results\e2e-demo-verified-metadata-smoke-20260613-190232\e2e-demo-summary.json
```

## Baseline

```text
commit=36535bd
git_dirty=false
verified_auth_metadata=true
conversation_tls_enabled=false
message_tls_enabled=false
delivery_tls_enabled=false
receipt_tls_enabled=false
success=true
```

## Key Evidence

Conversation setup and message:

```text
member_join.boundary_seq=1
member_join.status=MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED
send_message.message_id=msg_2340b75e-ed61-426e-8be1-67e0dbae1c44
send_message.conversation_seq=2
```

Online notification and durable inbox:

```text
delivery_notify.op=delivery.notify
delivery_notify.source_event_type=message.persisted.v1
delivery_notify.conversation_seq=2
delivery_notify.message_id=msg_2340b75e-ed61-426e-8be1-67e0dbae1c44
pull_inbox.item_count=1
pull_inbox.max_seq=2
pull_inbox.items[0].event_type=message.persisted.v1
```

ACK and read state:

```text
websocket_ack.op=delivery.ack.ok
websocket_ack.last_received_seq=2
mark_read.last_read_seq=2
```

Conversation list unread transition:

```text
list_conversations_before_read.item_count=1
list_conversations_before_read.items[0].unread_count=1
list_conversations_before_read.items[0].last_read_seq=0
list_conversations_after_read.item_count=1
list_conversations_after_read.items[0].unread_count=0
list_conversations_after_read.items[0].last_read_seq=2
```

PostgreSQL evidence:

```text
user_inbox_count=1
device_delivery_cursor_seq=2
user_read_cursor_seq=2
user_conversation_summaries=1
```

## Boundary

- `-VerifiedAuthMetadata` validates the gateway verified metadata interface shape for this demo path.
- Request body identity fields remain compatibility fields for default body-auth mode.
- `push-gateway` still derives ACK metadata from WebSocket auth and forwards it to delivery-service.
- The demo keeps `push-gateway` as an online wakeup layer; display facts still come from `delivery-service PullInbox`.
- This smoke does not cover certificate issuance, rotation, dynamic service identity, production API gateway policy, Redis HA, Kafka HA, PostgreSQL failover, or capacity.
