# Push Gateway Kafka Controller Switch Smoke

## Summary

- Date: 2026-06-15
- Commit under test: `ffc529d test: add kafka controller switch smoke`
- Result: passed
- Raw result root: `H:\NexusIM\loadtest-results\kafka-controller-switch-smoke-20260615-201415`
- Summary JSON: `H:\NexusIM\loadtest-results\kafka-controller-switch-smoke-20260615-201415\kafka-controller-switch-summary.json`
- Before smoke summary: `H:\NexusIM\loadtest-results\kafka-controller-switch-smoke-20260615-201415-before\pushgateway-summary.json`
- After smoke summary: `H:\NexusIM\loadtest-results\kafka-controller-switch-smoke-20260615-201415-after\pushgateway-summary.json`

This is a local Docker KRaft controller-switch smoke. It is not a production Kafka HA, multi-fault, ISR instability, or capacity result.

## Scenario

The smoke used the local three-broker KRaft topology:

```text
127.0.0.1:19092
127.0.0.1:19093
127.0.0.1:19094
```

The wrapper first ran a baseline NexusIM distributed smoke, then queried the active KRaft controller through:

```text
kafka-metadata-quorum --bootstrap-server kafka-ha-0:29092 describe --status
```

It stopped the current controller broker, waited for a different controller to remain stable, then ran the distributed smoke again.

## Evidence

Controller switch:

```text
before_controller_broker_id=1
stopped_container=nexusim-kafka-ha-0
after_controller_broker_id=3
```

Before switch smoke:

```text
commit=ffc529d
git_dirty=false
success=true
delivery.notify conversation_seq=2
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox PUBLISHED=2 / PENDING=0 / DLQ=0
```

After controller switch smoke:

```text
commit=ffc529d
git_dirty=false
success=true
delivery.notify conversation_seq=2
PullInbox item_count=1
PullInbox max_seq=2
delivery.ack.ok last_received_seq=2
delivery_outbox PUBLISHED=2 / PENDING=0 / DLQ=0
```

## Interpretation

This proves a limited local property:

```text
the three-broker KRaft setup survived stopping the active controller broker,
elected a different controller,
and the NexusIM delivery notify / PullInbox / AckDelivery chain completed after that switch.
```

It does not prove:

- production Kafka HA
- two-broker loss or quorum loss behavior
- ISR churn under sustained load
- in-flight produce / commit continuity at the exact controller-stop window
- cross-machine Kafka HA
- tuned producer durability policy for every service
