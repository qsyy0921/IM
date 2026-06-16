# Push-Gateway Kafka Consumer Churn Probe Smoke

Date: 2026-06-16

## Scope

This is a local Kafka consumer group churn observation for `push-gateway`
`delivery-consumer`.

It verifies:

- two `push-gateway delivery-consumer` processes join the same Kafka consumer
  group for `im.delivery.events`;
- one consumer repeatedly leaves and rejoins;
- Kafka returns the group to `Stable` after each transition;
- all three topic partitions are assigned after each transition;
- after each transition, a probe writes valid protobuf
  `delivery.inbox_item.created.v1` events and the consumer group drains them
  back to `lag=0`.

It does not prove production rebalance storm SLO, long-duration capacity, or
exactly-once delivery semantics.

## Evidence

Raw result directory:

```text
H:\NexusIM\loadtest-results\kafka-consumer-churn-probe-smoke-20260616
```

Generated summary:

```text
H:\NexusIM\loadtest-results\kafka-consumer-churn-probe-smoke-20260616\kafka-consumer-churn-report-summary.json
```

Key facts:

| Field | Value |
| --- | --- |
| Commit | `9ff277ff6bc15aa83515a1903a3163583a4de6cb` |
| Git dirty | `false` |
| Topic | `im.delivery.events` |
| Consumer group | `nexusim-push-churn-smoke-20260616182107` |
| Churn cycles | `2` |
| Transition count | `8` |
| Probe messages per transition | `3` |
| Probe attempted | `24` |
| Probe acked | `24` |

Transition summary:

| Cycle | Action | Members | Assigned partitions | Probe acked | Post-probe lag |
| ---: | --- | ---: | ---: | ---: | ---: |
| 1 | `stop_a` | 1 | 3 | 3 | 0 |
| 1 | `start_a` | 2 | 3 | 3 | 0 |
| 1 | `stop_b` | 1 | 3 | 3 | 0 |
| 1 | `start_b` | 2 | 3 | 3 | 0 |
| 2 | `stop_a` | 1 | 3 | 3 | 0 |
| 2 | `start_a` | 2 | 3 | 3 | 0 |
| 2 | `stop_b` | 1 | 3 | 3 | 0 |
| 2 | `start_b` | 2 | 3 | 3 | 0 |

## Interpretation

This improves on the earlier assignment-only churn smoke: the new probe confirms
that after each local leave / rejoin transition, the consumer group can still
consume and commit valid delivery events to zero lag.

The reliable user-visible recovery boundary remains unchanged: `push-gateway`
is an online wakeup layer, and durable recovery still relies on
`delivery-service` `PullInbox`.

## Remaining Limits

- This is still a short local run, not a high-frequency rebalance storm.
- It does not exercise a real online WebSocket session during churn.
- It does not prove long-duration capacity, production Kafka HA, or
  exactly-once producer semantics.
