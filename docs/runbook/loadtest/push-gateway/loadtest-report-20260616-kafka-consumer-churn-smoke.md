# Kafka Consumer Churn Smoke Summary

- Run: `kafka-consumer-churn-smoke-20260616-clean`
- Commit under test: `e18c31daa94c3475d994a2fff488521847fc9866`
- Git dirty: `false`
- Raw result root: `H:\NexusIM\loadtest-results\kafka-consumer-churn-smoke-20260616-clean`
- Summary JSON: `H:\NexusIM\loadtest-results\kafka-consumer-churn-smoke-20260616-clean\kafka-consumer-churn-summary.json`
- Report summary JSON: `H:\NexusIM\loadtest-results\kafka-consumer-churn-smoke-20260616-clean\kafka-consumer-churn-report-summary.json`

This is a local Kafka consumer group churn observation. It is not a production rebalance storm SLO, capacity benchmark, or long-duration partition churn proof.

## Scenario

The smoke started two `push-gateway delivery-consumer` processes in the same consumer group against `im.delivery.events`.

It then ran two churn cycles:

```text
stop consumer A  -> wait Stable with 1 member and 3 assigned partitions
start consumer A -> wait Stable with 2 members and 3 assigned partitions
stop consumer B  -> wait Stable with 1 member and 3 assigned partitions
start consumer B -> wait Stable with 2 members and 3 assigned partitions
repeat
```

## Evidence

Clean run result:

```text
initial_member_count = 2
initial_assigned_partition_count = 3
churn_cycles = 2
transition_count = 8
```

All transitions passed:

| Cycle | Action | Members | Assigned partitions |
| ---: | --- | ---: | ---: |
| 1 | stop_a | 1 | 3 |
| 1 | start_a | 2 | 3 |
| 1 | stop_b | 1 | 3 |
| 1 | start_b | 2 | 3 |
| 2 | stop_a | 1 | 3 |
| 2 | start_a | 2 | 3 |
| 2 | stop_b | 1 | 3 |
| 2 | start_b | 2 | 3 |

## Interpretation

This strengthens the earlier single-stop rebalance smoke by proving a local repeated leave / rejoin pattern:

```text
consumer leaves -> group returns Stable -> all partitions assigned
consumer rejoins -> group returns Stable -> all partitions assigned
repeat
```

It is useful for local development and interview demonstration. It does not prove production rebalance SLO, high-frequency rebalance storms, message processing continuity during churn, consumer lag behavior under load, or long-duration capacity behavior.
