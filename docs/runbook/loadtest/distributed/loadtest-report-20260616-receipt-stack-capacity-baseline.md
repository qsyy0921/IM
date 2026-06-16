# Receipt Stack Capacity Baseline

## Scope

This report records a local short capacity baseline for `receipt-service`. It proves that the receipt stack runner can exercise:

```text
conversation-service member join
-> message-service SendMessage
-> message outbox relay
-> delivery timeline consumer
-> delivery-service PullInbox / AckDelivery
-> delivery outbox relay
-> receipt delivery consumer
-> receipt-service MarkRead / receipt state / conversation list
-> receipt outbox relay
-> Kafka im.receipt.events readback
```

This is a local development / interview evidence run. It is not a production SLO, HA proof, or sizing claim.

## Environment

| Item | Value |
| --- | --- |
| Commit | `076fe304` |
| Raw result root | `H:\NexusIM\loadtest-results\capacity-baseline-receipt-stack-20260616` |
| Timeline topic | `conversation.timeline.receipt.20260616-125518` |
| Delivery topic | `im.delivery.events` |
| Receipt topic | `im.receipt.events` |
| Runner | `.\loadtest\receipt\run-local-smoke.ps1 -RunName capacity-baseline-receipt-stack-20260616` |

The smoke wrapper started temporary conversation, message, delivery and receipt service processes plus the required relay / consumer roles, then stopped them at the end of the run.

## Result

| Metric | Value |
| --- | ---: |
| Summary success | `true` |
| Message count | `2` |
| Pull item count | `3` |
| Ack count | `2` |
| Mark read count | `1` |
| Receipt state query count | `3` |
| Conversation list call count | `13` |
| State mutation count | `7` |
| Receipt Kafka event count | `3` |
| `receipt_outbox_published` | `3` |
| `receipt_outbox_pending` | `0` |
| `receipt_outbox_dlq` | `0` |
| `delivery_outbox_published` | `4` |
| `delivery_outbox_pending` | `0` |
| `delivery_outbox_dlq` | `0` |
| `operations_per_second` | `2.827239935103352` |
| `messages_per_second` | `0.18240257645828079` |
| `receipt_events_per_second` | `0.27360386468742115` |

Kafka readback returned:

```text
receipt.message.received.v1
receipt.message.read.v1
receipt.message.received.v1
```

The run also verified:

- `PullInbox` returned the sent message, then `AckDelivery` advanced the device cursor;
- receipt state showed received before read and received + read after `MarkRead`;
- unread conversation list dropped the item after read;
- archive, unarchive, pin, unpin, mute and unmute state mutations worked;
- a new message while archived stayed hidden from the default list but appeared when archived conversations were included;
- `MarkRead` beyond visible range returned the expected `FailedPrecondition` error.

## Limits

- This is a single short local run, not a long-duration capacity curve.
- It uses local temporary processes, not production deployment or HA topology.
- It does not validate Redis / Kafka / PostgreSQL failover.
- It does not cover future product fields such as drafts, tags or richer conversation summaries.
