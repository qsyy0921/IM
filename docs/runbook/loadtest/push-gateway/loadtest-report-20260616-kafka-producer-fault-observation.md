# Kafka Producer Fault Observation

- Run: `kafka-producer-fault-observation-20260616-clean-3`
- Commit under test: `eb073f3b648113d3dc11bd16afe649f55cebfddb`
- Git dirty: `false`
- Raw result root: `H:\NexusIM\loadtest-results\kafka-producer-fault-observation-20260616-clean-3`
- Summary JSON: `H:\NexusIM\loadtest-results\kafka-producer-fault-observation-20260616-clean-3\kafka-producer-fault-observation-summary.json`
- Report summary JSON: `H:\NexusIM\loadtest-results\kafka-producer-fault-observation-20260616-clean-3\kafka-producer-fault-report-summary.json`

This is a local `kafka-go` producer in-flight broker-fault observation. It is not an exactly-once producer proof and does not change the project boundary: NexusIM still relies on outbox rows and event IDs for business idempotency.

## Scenario

The smoke used the same first-stage writer settings as the service producers:

```text
acks=all
bounded retry / backoff
allow auto topic creation = false
no idempotent / transactional producer claim
```

The wrapper created a replicated probe topic with:

```text
replication factor = 3
min.insync.replicas = 2
message count = 120
stopped broker id = 1
```

During the producer run, the wrapper stopped one non-controller broker, waited briefly, restored it, then consumed the probe topic from the beginning and compared produced record IDs with consumed record IDs.

## Evidence

Clean run result:

```text
producer_attempted = 120
producer_acked = 120
producer_failed = 0
consumed_total = 120
consumed_unique = 120
duplicate_count = 0
missing_acked_count = 0
unacked_observed_count = 0
```

## Interpretation

This run proves a narrow local observation:

```text
all acknowledged records were present in the consumed topic
no duplicates were observed in this specific broker-fault window
no unacknowledged records were observed in this specific broker-fault window
```

It does not prove that duplicates cannot happen. The current `kafka-go` writer still does not expose Kafka idempotent or transactional producer semantics. If NexusIM later needs to claim exactly-once producer behavior, the project must replace or extend the producer client and run dedicated duplicate / ambiguous-write verification.

## Limits

This run does not prove:

- exactly-once producer semantics
- transactional writes across Kafka and PostgreSQL
- duplicate-free behavior under every broker / network / timeout fault
- long-duration producer behavior under ISR flapping
- rack-aware or multi-AZ producer behavior
- consumer-side idempotency beyond the existing event ID / outbox design
