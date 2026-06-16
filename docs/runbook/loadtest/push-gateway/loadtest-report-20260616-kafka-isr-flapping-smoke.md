# Kafka ISR Flapping Smoke Summary

- Run: `kafka-isr-flapping-smoke-20260616-clean-2`
- Commit under test: `813641efc4313788e946fdc2f888fb835b5399cf`
- Git dirty: `false`
- Raw result root: `H:\NexusIM\loadtest-results\kafka-isr-flapping-smoke-20260616-clean-2`
- Summary JSON: `H:\NexusIM\loadtest-results\kafka-isr-flapping-smoke-20260616-clean-2\kafka-isr-flapping-summary.json`
- Report summary JSON: `H:\NexusIM\loadtest-results\kafka-isr-flapping-smoke-20260616-clean-2\kafka-isr-flapping-report-summary.json`

This is a local Kafka KRaft repeated ISR flapping observation. It is not a production Kafka HA proof, rebalance storm test, capacity benchmark, disk-loss test, or exactly-once producer proof.

## Scenario

The smoke used the local three-broker Kafka KRaft topology:

```text
brokers = 127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094
probe topic replication factor = 3
probe topic min.insync.replicas = 2
flap cycles = 2
flapped broker id = 1
```

For each cycle, the wrapper stopped one non-controller broker, waited until the replicated probe topic reached `ISR=2` and no longer included the stopped broker, then produced with `acks=all`. It then restarted the broker, waited until the probe topic returned to `ISR=3`, and produced again with `acks=all`.

## Evidence

Both cycles passed:

| Cycle | Degraded ISR OK | Degraded produce OK | Restored ISR OK | Restored produce OK |
| ---: | --- | --- | --- | --- |
| 1 | true | true | true | true |
| 2 | true | true | true | true |

During each degraded phase:

```text
partition_count = 3
replica_count = 3
isr_count = 2
flapped broker id 1 absent from ISR
acks=all probe accepted
```

During each restored phase:

```text
partition_count = 3
replica_count = 3
isr_count = 3
flapped broker id 1 present in ISR
acks=all probe accepted
```

## Interpretation

This strengthens the previous one-shot ISR observation by proving a local repeated stop/start path:

```text
broker down -> ISR shrinks to 2 -> producer probe still succeeds
broker restored -> ISR returns to 3 -> producer probe still succeeds
repeat
```

The evidence is useful for local development and interview demonstration. It does not prove production Kafka HA, sustained rebalance storm handling, in-flight produce deduplication, disk-loss recovery, rack-aware placement, or long-duration capacity behavior.
