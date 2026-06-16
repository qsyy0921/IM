# Kafka Producer Hardening Evaluation

- Created at: 2026-06-16T08:47:18.7844714Z
- Scope: Kafka producer hardening evaluation; combines static producer guardrails with a local ISR fault observation
- Result: passed
- Producer packages covered: 6
- Producer guardrails: `acks=all`, auto topic creation disabled, 5 attempts, 100ms-1s write backoff
- One-broker-down producer probe accepted: True
- Two-broker-down write rejected with NOT_ENOUGH_REPLICAS: True
- Current producer client: segmentio/kafka-go
- Idempotent producer flag supported: false
- Exactly-once / transactional producer claimed: false

## Producer Packages

| Service | Path | Attempts | Backoff | Boundary |
| --- | --- | ---: | --- | --- |
| contacts-service | services\contacts-service\internal\infrastructure\kafka\producer.go | 5 | 100ms-1s | outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer |
| delivery-service | services\delivery-service\internal\infrastructure\kafka\producer.go | 5 | 100ms-1s | outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer |
| identity-service | services\identity-service\internal\infrastructure\kafka\producer.go | 5 | 100ms-1s | outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer |
| message-service | services\message-service\internal\infrastructure\kafka\producer.go | 5 | 100ms-1s | outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer |
| policy-service | services\policy-service\internal\infrastructure\kafka\producer.go | 5 | 100ms-1s | outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer |
| receipt-service | services\receipt-service\internal\infrastructure\kafka\producer.go | 5 | 100ms-1s | outbox/event_id idempotency; no Kafka transactional producer in this first-stage kafka-go writer |

## Interpretation

This evaluation proves the current first-stage Kafka producer guardrails and local ISR fault boundary. It does not prove idempotent, exactly-once, or transactional Kafka producer semantics. The reliable business boundary remains outbox rows plus event-id idempotency and downstream idempotent consumers.

Before claiming production exactly-once Kafka producer behavior, NexusIM must evaluate or adopt a producer client with explicit idempotence and transaction support, then add real broker-fault and duplicate-produce verification for that client.
