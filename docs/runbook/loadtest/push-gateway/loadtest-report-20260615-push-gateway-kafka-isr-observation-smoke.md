# Push Gateway Kafka ISR Observation Smoke

## Summary

- Date: 2026-06-15
- Commit under test: `b48ffa3 test: add kafka isr observation smoke`
- Result: observation captured
- Raw result root: `H:\NexusIM\loadtest-results\kafka-isr-observation-smoke-20260615-204756`
- Summary JSON: `H:\NexusIM\loadtest-results\kafka-isr-observation-smoke-20260615-204756\kafka-isr-observation-summary.json`
- Baseline smoke summary: `H:\NexusIM\loadtest-results\kafka-isr-observation-smoke-20260615-204756-before\pushgateway-summary.json`
- One-broker-down smoke summary: `H:\NexusIM\loadtest-results\kafka-isr-observation-smoke-20260615-204756-one-broker-down\pushgateway-summary.json`
- Restore smoke summary: `H:\NexusIM\loadtest-results\kafka-isr-observation-smoke-20260615-204756-after-restore\pushgateway-summary.json`

This is a local Kafka KRaft ISR observation. It is not a production Kafka HA, multi-AZ, or disaster recovery proof.

## Scenario

The wrapper started the local three-broker Kafka KRaft topology:

```text
127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094
topic replication factor = 3
min.insync.replicas = 2
```

It ran a baseline NexusIM distributed smoke, then stopped one non-controller broker. After the delivery topic and a probe topic reached ISR size 2, it ran the full `delivery.notify -> PullInbox -> delivery.ack.ok` chain again.

It then stopped a second broker, leaving only one broker, and attempted a producer probe against a replicated topic configured with `min.insync.replicas=2`. Finally, it restored the stopped brokers and reran the full smoke.

## Evidence

Clean run metadata:

```text
commit=b48ffa3d7e6bef5e91c9ecfa75f56844100751c6
git_dirty=false
before_controller_broker_id=3
first_stopped_broker_id=1
second_stopped_broker_id=3
remaining_broker_id_after_two_stops=2
```

After stopping one broker, every `im.delivery.events` partition had three replicas and two in-sync replicas:

```text
partition 0: leader=2 replica_count=3 isr_count=2
partition 1: leader=3 replica_count=3 isr_count=2
partition 2: leader=2 replica_count=3 isr_count=2
```

The one-broker-down full smoke passed:

```text
success=true
delivery.notify conversation_seq=2
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox PUBLISHED=2 / PENDING=0 / DLQ=0
```

After stopping a second broker:

```text
admin_ready_after_two_broker_stops=false
probe_produce_after_two_broker_stops.accepted=false
probe_produce_after_two_broker_stops.contains_not_enough_replicas=true
```

The producer output included:

```text
NOT_ENOUGH_REPLICAS
NotEnoughReplicasException: Messages are rejected since there are fewer in-sync replicas than required.
```

After restoring the stopped brokers, the full smoke passed again:

```text
success=true
delivery.notify conversation_seq=2
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox PUBLISHED=2 / PENDING=0 / DLQ=0
```

## Interpretation

This result proves the current local Kafka HA setup has the expected first-stage ISR behavior:

```text
one broker down -> ISR shrinks to 2 and the IM chain still runs
two brokers down -> writes requiring min.insync.replicas=2 are rejected / unavailable
restore brokers -> the IM chain runs again
```

That is useful for local development and interview demonstration. It does not prove production Kafka HA, because it does not cover multi-AZ placement, sustained broker churn, controller quorum under wider faults, producer retry budgets, consumer rebalance storms, rack awareness, disk loss, or in-flight produce / commit continuity.

## Limits

This run does not prove:

- multi-AZ Kafka HA
- multi-broker loss beyond the local KRaft quorum boundary
- ISR recovery under sustained flapping
- rack-aware replica placement
- producer retry / idempotence tuning under long outages
- consumer group rebalance SLO
- durable DR or cross-region replication

It is a local ISR observation smoke that prevents over-claiming the current three-broker KRaft setup.
